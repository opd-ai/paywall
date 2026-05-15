# Architecture Documentation

This document describes the internal architecture, design decisions, and data flows of the paywall system.

## Table of Contents

- [System Overview](#system-overview)
- [Architecture Diagrams](#architecture-diagrams)
- [Component Details](#component-details)
- [Data Flow](#data-flow)
- [Package Structure](#package-structure)
- [Design Decisions](#design-decisions)
- [Security Architecture](#security-architecture)
- [Performance Considerations](#performance-considerations)

---

## System Overview

The paywall is a cryptocurrency payment verification middleware for Go web applications. It follows a layered architecture with clear separation of concerns:

**Layers**:
1. **HTTP Middleware Layer**: Request interception and payment verification
2. **Business Logic Layer**: Payment creation, tracking, and state management
3. **Wallet Layer**: HD wallet management and address derivation
4. **Storage Layer**: Payment persistence with pluggable backends
5. **Blockchain Layer**: External blockchain verification via RPC

**Key Design Principles**:
- **Modularity**: Components can be swapped (e.g., different storage backends)
- **Thread-Safety**: All shared state protected with mutexes
- **Idempotency**: Payment creation is idempotent
- **Fail-Safe**: Graceful degradation when optional services (XMR) unavailable
- **Zero-Trust**: Cryptographic verification of all signatures

---

## Architecture Diagrams

### System Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                         HTTP Layer                                 │
│  ┌──────────────┐         ┌──────────────┐                       │
│  │   Browser    │ ◄─────► │   Nginx      │                       │
│  │  (QR Code)   │  HTTPS  │  (Reverse    │                       │
│  └──────────────┘         │   Proxy)     │                       │
│                            └───────┬──────┘                       │
└────────────────────────────────────┼────────────────────────────────┘
                                     │ HTTP
                                     ▼
┌───────────────────────────────────────────────────────────────────┐
│                    Paywall Middleware                              │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Middleware(handler)                                       │  │
│  │  ├─ Check Cookie                                           │  │
│  │  ├─ Verify Payment Status                                  │  │
│  │  ├─ Serve Payment Page (if unpaid)                        │  │
│  │  └─ Forward to Protected Handler (if paid)                │  │
│  └────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────┬───────────────────────────────┘
                                     │
            ┌────────────────────────┼────────────────────────┐
            │                        │                        │
            ▼                        ▼                        ▼
┌──────────────────────┐  ┌──────────────────┐  ┌────────────────────┐
│   Paywall Core       │  │   Wallet Layer   │  │  Storage Layer     │
│  ┌────────────────┐  │  │  ┌────────────┐  │  │  ┌──────────────┐  │
│  │ CreatePayment  │  │  │  │ BTC Wallet │  │  │  │  FileStore   │  │
│  │ VerifyPayment  │  │  │  │ XMR Wallet │  │  │  │  MemoryStore │  │
│  │ CheckPending   │◄─┼──┼─►│ Multisig   │◄─┼──┼─►│  Encrypted   │  │
│  └────────────────┘  │  │  └────────────┘  │  │  └──────────────┘  │
└──────────┬───────────┘  └──────────┬───────┘  └────────────────────┘
           │                         │
           │                         │
           ▼                         ▼
┌──────────────────────┐  ┌──────────────────┐
│  Escrow Manager      │  │  Audit Logger    │
│  ┌────────────────┐  │  │  ┌────────────┐  │
│  │ CreateEscrow   │  │  │  │  LogAction │  │
│  │ FundEscrow     │◄─┼──┼─►│  GetTrail  │  │
│  │ ResolveDispute │  │  │  └────────────┘  │
│  └────────────────┘  │  └──────────────────┘
└──────────┬───────────┘
           │
           ▼
┌──────────────────────────────────────────────────────────────┐
│                   Blockchain Layer                            │
│  ┌────────────────┐         ┌────────────────┐              │
│  │  Bitcoin RPC   │         │  Monero RPC    │              │
│  │  (blockstream) │         │  (wallet-rpc)  │              │
│  └────────────────┘         └────────────────┘              │
└──────────────────────────────────────────────────────────────┘
```

### Payment Lifecycle

```
┌──────────────────────────────────────────────────────────────────┐
│                      Payment Lifecycle                            │
└──────────────────────────────────────────────────────────────────┘

1. CREATION
   User → HTTP Request → Middleware
                           │
                           ▼
                    CreatePayment()
                           │
                           ├─► Derive BTC Address (HD Wallet)
                           ├─► Derive XMR Subaddress (if enabled)
                           ├─► Generate Payment ID
                           ├─► Set Expiration Timestamp
                           └─► Store in PaymentStore
                                 │
                                 ▼
                          Return Payment Page
                          (with QR code, addresses)

2. USER PAYMENT
   User → Blockchain Transaction → BTC/XMR Network
                                       │
                                       ▼
                              Transaction broadcasts,
                              gets mined into blocks

3. VERIFICATION (Background Goroutine)
   checkPendingPayments() [runs every 10-60s]
       │
       ├─► ListPendingPayments()
       │
       └─► For each pending payment:
             │
             ├─► CheckBTCPayments()
             │     └─► Query blockchain for address balance
             │           └─► Get confirmations
             │
             ├─► CheckXMRPayments()
             │     └─► Query wallet RPC for incoming transfers
             │           └─► Get confirmations
             │
             └─► If confirmations >= MinConfirmations:
                   │
                   ├─► payment.Status = StatusConfirmed
                   └─► UpdatePayment()

4. ACCESS GRANT
   User → HTTP Request (with payment cookie)
            │
            ▼
       Middleware checks cookie
            │
            ├─► GetPaymentByID()
            │
            └─► If status == StatusConfirmed:
                  │
                  └─► Forward to protected handler
                        (User sees protected content)
```

### Escrow Workflow (Multisig)

```
┌────────────────────────────────────────────────────────────────┐
│                    Escrow Workflow                              │
└────────────────────────────────────────────────────────────────┘

Participants: Buyer, Seller, Arbiter

1. ESCROW CREATION (2-of-3 Multisig)
   ┌────────────────────────────────────────┐
   │  Seller calls CreateEscrow()           │
   │  ├─ Generate 2-of-3 multisig address   │
   │  │   (buyer + seller + arbiter keys)   │
   │  ├─ Set escrow timeout                 │
   │  └─ State: EscrowPending               │
   └───────────────┬────────────────────────┘
                   │
                   ▼
2. BUYER FUNDS ESCROW
   ┌────────────────────────────────────────┐
   │  Buyer sends BTC/XMR to multisig addr  │
   └───────────────┬────────────────────────┘
                   │
                   ▼
   ┌────────────────────────────────────────┐
   │  Blockchain confirms transaction       │
   │  FundEscrow() called                   │
   │  State: EscrowFunded                   │
   └───────────────┬────────────────────────┘
                   │
         ┌─────────┴─────────┐
         │                   │
         ▼                   ▼
    HAPPY PATH          DISPUTE PATH
         │                   │
         │                   │
3a. RELEASE TO SELLER   3b. DISPUTE RAISED
   ┌──────────────────┐   ┌────────────────────────┐
   │ Buyer satisfied  │   │ Buyer/Seller unhappy   │
   │ Both sign release│   │ RequestDispute()       │
   │                  │   │ State: EscrowDisputed  │
   │ ReleaseToSeller()│   │                        │
   │ (buyer + seller) │   │ Evidence submitted     │
   │                  │   │ Arbiters vote          │
   │ State: Completed │   │                        │
   └──────────────────┘   └───────────┬────────────┘
                                       │
                         ┌─────────────┴─────────────┐
                         │                           │
                         ▼                           ▼
                  Favor Buyer                 Favor Seller
                         │                           │
                  ┌──────┴──────┐            ┌──────┴──────┐
                  │ RefundBuyer │            │Release      │
                  │ (arbiter+   │            │ToSeller     │
                  │  buyer)     │            │(arbiter+    │
                  │             │            │ seller)     │
                  │ State:      │            │State:       │
                  │ Refunded    │            │ Completed   │
                  └─────────────┘            └─────────────┘

4. TIMEOUT SCENARIO (if no resolution)
   ┌────────────────────────────────────────┐
   │  TimeoutMonitor detects expired escrow │
   │  (EscrowTimeout passed)                │
   │                                        │
   │  If AutoRefund enabled:                │
   │  ├─ Arbiter signs automatic refund     │
   │  └─ RefundBuyer(arbiter + buyer)       │
   │                                        │
   │  State: EscrowRefunded                 │
   └────────────────────────────────────────┘
```

### Multi-Arbiter Consensus

```
┌──────────────────────────────────────────────────────────┐
│           Multi-Arbiter Consensus (3-of-5)               │
└──────────────────────────────────────────────────────────┘

1. DISPUTE INITIATED
   RequestDispute(paymentID, reason)
        │
        ▼
   InitiateConsensus(paymentID)
        │
        ├─ Create ArbiterConsensus struct
        ├─ RequiredVotes = 3
        ├─ TotalArbiters = 5
        ├─ VotingDeadline = now + 7 days
        └─ Status = ConsensusOpen

2. ARBITERS VOTE
   For each arbiter (1-5):
        │
        ▼
   CastVote(paymentID, vote)
        │
        ├─ Validate arbiter authorized
        ├─ Check not duplicate vote
        ├─ Record vote with signature
        │
        └─► Track reputation:
              RecordDecision(arbiterID, withConsensus, responseTime)

3. CONSENSUS CHECK (after each vote)
   ┌────────────────────────────────────┐
   │  Count votes for each decision:   │
   │  ├─ Favor Buyer: 2 votes           │
   │  └─ Favor Seller: 1 vote           │
   │                                    │
   │  If any decision >= RequiredVotes: │
   │  ├─ ConsensusReached = true        │
   │  ├─ FinalDecision = majority       │
   │  └─ Status = ConsensusReached      │
   └────────────┬───────────────────────┘
                │
                ▼
   ResolveDispute(paymentID, arbiterSig, winnerSig)
                │
                └─► Execute resolution
                      (Release or Refund)

4. FALLBACK (if primary arbiters fail)
   ┌────────────────────────────────────┐
   │  VotingDeadline passed             │
   │  Votes < RequiredVotes             │
   │                                    │
   │  ActivateFallbackArbiters()        │
   │  ├─ Extend voting deadline         │
   │  ├─ Add fallback arbiters          │
   │  └─ Status = ConsensusFallback     │
   └────────────────────────────────────┘
```

---

## Component Details

### Paywall Core (`paywall.go`)

**Responsibility**: Coordinates all system components and provides the main API.

**Key Structures**:
```go
type Paywall struct {
    Config          Config                        // User configuration
    HDWallets       map[WalletType]HDWallet      // Bitcoin + Monero wallets
    Store           PaymentStore                  // Storage backend
    checkInterval   time.Duration                 // Verification frequency
    done            chan struct{}                 // Shutdown signal
    wg              sync.WaitGroup                // Goroutine coordination
    mu              sync.RWMutex                  // Protects internal state
}
```

**Key Methods**:
- `NewPaywall(config)`: Initialize system with configuration
- `Middleware(handler)`: HTTP middleware for payment verification
- `CreatePayment()`: Generate new payment request
- `checkPendingPayments()`: Background verification loop
- `Close()`: Graceful shutdown

**Thread Safety**: All public methods acquire appropriate locks before accessing shared state.

### Wallet Layer (`wallet/`)

**Responsibility**: HD wallet management, address derivation, and signature creation.

**Bitcoin HD Wallet** (`btc_hd_wallet.go`):
- Implements BIP32/BIP44 hierarchical deterministic wallet
- Derives unique addresses from master seed
- Supports both single-sig and multisig addresses
- Thread-safe address generation with mutex protection

```go
type BTCHDWallet struct {
    masterKey  *hdkeychain.ExtendedKey  // BIP32 master key
    nextIndex  uint32                    // Address derivation counter
    testnet    bool                      // Network selection
    mu         sync.Mutex                // Thread safety
}
```

**Key Methods**:
- `DeriveNextAddress()`: Generate next HD address (m/44'/0'/0'/0/n)
- `CreateMultisigAddress()`: Generate P2WSH multisig address
- `GetAddressBalance()`: Query blockchain for address UTXOs
- `SaveToFile()`: Persist wallet with AES-256 encryption

**Monero Wallet** (`xmr_hd_wallet.go`):
- RPC client wrapper for monero-wallet-rpc
- Subaddress generation for payment separation
- Transfer checking and confirmation tracking

**Multisig Support**:
- 2-of-3 and m-of-n multisig addresses
- P2WSH (Pay-to-Witness-Script-Hash) for Bitcoin
- Signature collection and verification

### Storage Layer

**Interface** (`types.go`):
```go
type PaymentStore interface {
    CreatePayment(payment *Payment) error
    GetPayment(id string) (*Payment, error)
    GetPaymentByAddress(address string) (*Payment, error)
    UpdatePayment(payment *Payment) error
    ListPendingPayments() ([]*Payment, error)
    GetEscrowsExpiringBefore(deadline time.Time) ([]*Payment, error)
    Close() error
}
```

**Implementations**:

1. **MemoryStore** (`memstore.go`):
   - In-memory map with RWMutex protection
   - Fast access, no persistence
   - Suitable for testing and temporary deployments

2. **FileStore** (`filestore.go`):
   - JSON files per payment (`<paymentID>.json`)
   - Simple, human-readable format
   - Single-process deployments

3. **EncryptedFileStore** (`encryptedfilestore.go`):
   - AES-256-GCM encrypted JSON files
   - Protects sensitive payment data at rest
   - Recommended for production

**Data Model**:
```go
type Payment struct {
    ID               string
    Addresses        map[WalletType]string
    Amounts          map[WalletType]float64
    Status           PaymentStatus
    Confirmations    int
    Version          int  // Optimistic locking
    
    // Multisig fields (optional)
    MultisigEnabled  bool
    Signatures       map[WalletType][]SignatureData
    
    // Escrow fields (optional)
    EscrowState      EscrowState
    EscrowTimeout    time.Time
    DisputeReason    string
}
```

### Escrow Manager (`escrow.go`)

**Responsibility**: Manages escrow payment lifecycle and dispute resolution.

**Key Components**:
- State machine enforcement (Pending → Funded → Completed/Refunded)
- 2-of-3 multisig signature validation
- Audit trail logging for all state transitions
- Timeout monitoring and automatic refunds

**State Transitions**:
```
EscrowNone → EscrowPending (CreateEscrow)
EscrowPending → EscrowFunded (FundEscrow)
EscrowFunded → EscrowCompleted (ReleaseToSeller)
EscrowFunded → EscrowRefunded (RefundBuyer)
EscrowFunded → EscrowDisputed (RequestDispute)
EscrowDisputed → EscrowCompleted | EscrowRefunded (ResolveDispute)
```

**Validation**:
- Signature verification against transaction data
- Role-based authorization (buyer/seller/arbiter)
- Replay protection with nonces
- Version conflict detection for concurrent updates

### Arbiter Consensus System

**Components**:

1. **ArbiterConsensusManager** (`arbiter_consensus.go`):
   - Manages multi-arbiter voting
   - Tracks vote tallies and consensus state
   - Activates fallback arbiters if needed

2. **ArbiterReputationTracker** (`arbiter_reputation.go`):
   - Records arbiter decision quality
   - Tracks response time and participation rate
   - Computes reputation scores (0-100)

3. **DisputeEnhancements** (`dispute_enhancements.go`):
   - Fee calculation (prevents spam)
   - Rate limiting (3 disputes per 24h)
   - Evidence validation (size limits, DoS prevention)

### Middleware Layer (`middleware.go`)

**Responsibility**: HTTP request interception and payment enforcement.

**Flow**:
1. Extract payment ID from cookie
2. Look up payment in store
3. Check payment status:
   - If confirmed: Forward to protected handler
   - If pending/expired: Serve payment page
4. Set secure cookie with payment ID

**Cookie Security**:
- `__Host-` prefix (requires HTTPS)
- `Secure: true` (HTTPS only)
- `HttpOnly: true` (no JavaScript access)
- `SameSite: Strict` (CSRF protection)

---

## Data Flow

### Payment Creation Flow

```
HTTP Request → Middleware
    │
    └─► No valid payment cookie
          │
          ▼
    CreatePayment()
          │
          ├─► Generate Payment ID (crypto/rand)
          │
          ├─► Derive BTC Address
          │     └─► HDWallet.DeriveNextAddress()
          │           └─► BIP44 m/44'/0'/0'/0/n
          │
          ├─► Derive XMR Subaddress (if enabled)
          │     └─► XMRWallet.CreateSubaddress()
          │           └─► RPC call to wallet
          │
          ├─► Calculate expiration
          │     └─► now + PaymentTimeout
          │
          ├─► Create Payment struct
          │
          └─► Store.CreatePayment()
                │
                └─► Persist to storage
                      │
                      └─► Return payment page HTML
                            ├─ QR code (address)
                            ├─ Amount
                            └─ Expiration timer
```

### Payment Verification Flow

```
Background Goroutine (runs every 10-60s)
    │
    └─► checkPendingPayments()
          │
          ├─► Store.ListPendingPayments()
          │     └─► Returns all payments with status=Pending
          │
          └─► For each payment:
                │
                ├─► CheckBTCPayments(payment)
                │     │
                │     ├─► HDWallet.GetAddressBalance(address)
                │     │     └─► Query blockchain API
                │     │           └─► Parse transactions
                │     │
                │     └─► If balance >= amount:
                │           │
                │           ├─► Get confirmations
                │           │
                │           └─► If confirmations >= MinConfirmations:
                │                 │
                │                 └─► payment.Status = Confirmed
                │
                ├─► CheckXMRPayments(payment)
                │     │
                │     └─► XMRWallet.CheckIncomingTransfers(subaddress)
                │           └─► RPC: get_transfers
                │                 └─► Filter by subaddress
                │
                └─► Store.UpdatePayment(payment)
                      └─► Persist updated status
```

### Escrow Resolution Flow

```
ResolveDispute(paymentID, arbiterSig, winnerSig)
    │
    ├─► GetPayment(paymentID)
    │
    ├─► Validate state = EscrowDisputed
    │
    ├─► Verify arbiter signature
    │     │
    │     ├─► Extract public key from signature
    │     ├─► Check pubkey in AuthorizedArbiters
    │     └─► Verify signature over transaction data
    │
    ├─► Verify winner signature (buyer or seller)
    │     │
    │     ├─► Derive role from public key
    │     ├─► Check role matches participant
    │     └─► Verify signature over transaction data
    │
    ├─► Check consensus (if multi-arbiter)
    │     │
    │     └─► ArbiterConsensusManager.GetConsensus()
    │           └─► Ensure required votes reached
    │
    ├─► Update payment state
    │     │
    │     └─► EscrowCompleted or EscrowRefunded
    │
    ├─► Log audit entry
    │     │
    │     └─► AuditLogger.LogAction(entry)
    │
    └─► Store.UpdatePayment(payment)
```

---

## Package Structure

```
paywall/
├── main package (github.com/opd-ai/paywall)
│   ├── paywall.go              - Core coordinator
│   ├── types.go                - Data structures and interfaces
│   ├── middleware.go           - HTTP middleware
│   ├── handlers.go             - HTTP handlers for payment pages
│   ├── construct.go            - Convenience constructors
│   ├── verification.go         - Blockchain verification logic
│   │
│   ├── Multisig support
│   ├── multisig_api.go         - Multisig payment API
│   ├── multisig_handlers.go    - HTTP handlers for multisig
│   │
│   ├── Escrow system
│   ├── escrow.go               - Escrow manager
│   ├── dispute.go              - Dispute tracking
│   ├── dispute_enhancements.go - Anti-spam, fees, rate limits
│   ├── state_validator.go      - State machine validation
│   ├── timeout_automation.go   - Automatic timeout resolution
│   │
│   ├── Arbiter consensus
│   ├── arbiter_consensus.go    - Multi-arbiter voting
│   ├── arbiter_reputation.go   - Arbiter performance tracking
│   │
│   ├── Storage implementations
│   ├── memstore.go             - In-memory storage
│   ├── filestore.go            - File-based storage
│   ├── encryptedfilestore.go   - Encrypted file storage
│   │
│   ├── Audit and logging
│   ├── audit.go                - Audit trail logging
│   ├── logger.go               - Structured logging
│   ├── metrics.go              - Performance metrics
│   │
│   └── Blockchain integration
│       ├── broadcast.go        - Bitcoin transaction broadcasting
│       └── xmr_broadcast.go    - Monero transaction broadcasting
│
├── wallet/ (github.com/opd-ai/paywall/wallet)
│   ├── types.go                - Wallet interfaces
│   ├── btc_hd_wallet.go        - Bitcoin HD wallet (BIP32/44)
│   ├── xmr_hd_wallet.go        - Monero RPC wallet
│   ├── multisig_storage.go     - Multisig metadata persistence
│   └── storage.go              - Wallet file encryption
│
├── example/
│   ├── basic-server/           - Simple paywall example
│   ├── bitcoin-only/           - Bitcoin-only configuration
│   ├── reverse-proxy/          - Reverse proxy example
│   ├── subscription-service/   - Subscription use case
│   ├── digital-downloads/      - File download gating
│   └── api-monetization/       - REST API payment gate
│
├── migration/
│   └── migrate.go              - Wallet encryption migration
│
├── integration_test/
│   └── integration_test.go     - End-to-end tests
│
├── docs/
│   ├── API.md                  - API reference
│   ├── CONFIGURATION.md        - Config guide
│   ├── TROUBLESHOOTING.md      - Common issues
│   ├── DEPLOYMENT.md           - Production deployment
│   ├── DOCKER.md               - Container deployment
│   └── ARCHITECTURE.md         - This file
│
└── templates/
    └── payment.html            - Payment page template
```

### Package Responsibilities

| Package | Responsibility | Dependencies |
|---------|----------------|--------------|
| `paywall` | Core business logic, HTTP middleware | `wallet`, standard library |
| `wallet` | HD wallet management, address derivation | `btcsuite`, `go-monero-rpc-client` |
| `example` | Reference implementations | `paywall` |
| `migration` | Data migration utilities | `paywall/wallet` |
| `integration_test` | End-to-end testing | `paywall`, `wallet` |

---

## Design Decisions

### 1. HD Wallet Instead of Key Pool

**Decision**: Use BIP32/44 hierarchical deterministic wallets instead of pre-generating address pools.

**Rationale**:
- **Infinite Addresses**: Never run out of addresses
- **Backup Simplicity**: Single mnemonic phrase backs up all addresses
- **Deterministic**: Same seed always produces same addresses (reproducibility)
- **Space Efficient**: No need to store thousands of pre-generated keys

**Trade-offs**:
- Requires tracking `nextIndex` to avoid address reuse
- Slightly slower address generation (negligible in practice)

### 2. Pluggable Storage Backend

**Decision**: Define `PaymentStore` interface rather than hard-coding storage.

**Rationale**:
- **Flexibility**: Users can choose MemoryStore (dev) vs FileStore (prod) vs custom (database)
- **Testability**: Easy to mock storage in unit tests
- **Future-Proof**: Can add PostgreSQL, Redis, etc. without changing core

**Trade-offs**:
- Interface adds slight complexity
- Storage layer must handle all edge cases (race conditions, partial writes)

### 3. Background Verification Goroutine

**Decision**: Single background goroutine checks all pending payments periodically.

**Rationale**:
- **Simplicity**: One goroutine easier to manage than per-payment goroutines
- **Resource Efficient**: Avoids goroutine explosion with thousands of payments
- **Blockchain Rate Limits**: Batched queries more respectful of API limits

**Trade-offs**:
- Verification latency = checkInterval (10-60s typical)
- All payments checked even if only one recently created (could optimize)

### 4. Cookie-Based Payment Tracking

**Decision**: Use HTTP cookies to track payment IDs rather than query parameters or headers.

**Rationale**:
- **User Experience**: User doesn't need to bookmark payment ID URL
- **Security**: `__Host-` cookies require HTTPS and prevent subdomains from setting
- **Standard Practice**: Well-understood by all HTTP clients and browsers

**Trade-offs**:
- Requires HTTPS (which is mandatory anyway for production)
- User must have cookies enabled
- Cookie per payment (could accumulate if user creates many)

### 5. Optimistic Locking for Concurrent Updates

**Decision**: Add `Version` field to `Payment` and reject stale updates.

**Rationale**:
- **Race Condition Prevention**: Prevents simultaneous Release and Refund
- **Simple Implementation**: Just increment version on update
- **Database-Agnostic**: Works with any storage backend

**Trade-offs**:
- Requires retry logic in calling code
- Doesn't prevent all race conditions (read-modify-write still needs transaction)

### 6. Multi-Arbiter Consensus

**Decision**: Support n-of-m arbiter voting rather than single arbiter.

**Rationale**:
- **Decentralization**: No single point of failure or corruption
- **Trust**: Majority vote more trusted than single arbiter decision
- **Availability**: System continues if some arbiters offline

**Trade-offs**:
- Increased complexity (voting, consensus tracking)
- Slower resolution (must wait for multiple arbiter responses)
- Higher arbitration cost (pay multiple arbiters)

### 7. Embedded Templates and Static Assets

**Decision**: Use Go 1.16+ `embed` package for templates and JavaScript.

**Rationale**:
- **Single Binary**: No need to deploy template files separately
- **Reliability**: Templates always present, can't be accidentally deleted
- **Version Sync**: Template version always matches code version

**Trade-offs**:
- Template changes require recompile
- Binary size increases (but negligible for a few KB)

---

## Security Architecture

### Threat Model

**Assets**:
- Private wallet keys (BIP32 master key)
- Payment data (addresses, amounts, user sessions)
- Escrow funds (multisig UTXOs)

**Threats**:
1. **Private Key Theft**: Attacker steals wallet keys from disk/memory
2. **Payment Bypass**: Attacker accesses content without paying
3. **Fund Theft**: Attacker steals escrowed funds via fake signatures
4. **DoS**: Attacker overwhelms system with requests or disputes
5. **Replay Attack**: Attacker reuses signatures across different payments

### Security Measures

#### 1. Key Management

**Wallet Encryption**:
- AES-256-GCM for wallet files
- Key derived from user-provided passphrase or random key
- Keys never stored in plaintext

**Key Separation**:
- Master key only in memory during operation
- Derived keys (per-address) not stored
- Testnet and mainnet wallets use different key spaces

#### 2. Payment Verification

**Cookie Security**:
- `__Host-` prefix (HTTPS required, domain-locked)
- `Secure` flag (HTTPS only)
- `HttpOnly` flag (no JavaScript access)
- `SameSite: Strict` (CSRF protection)

**Payment ID Generation**:
- `crypto/rand` for cryptographically secure IDs
- 128-bit entropy (2^128 = 3.4×10^38 possibilities)

#### 3. Escrow Security

**Signature Verification**:
- All signatures verified against public keys
- Signature must match transaction data exactly
- Nonces prevent replay attacks

**Role Authorization**:
- Role derived from public key, not user input
- Only authorized participants can sign

**Optimistic Locking**:
- Version field prevents race conditions
- Concurrent Release and Refund detected and rejected

**Audit Trail**:
- All state transitions logged immutably
- Includes actor, signatures, timestamp

#### 4. DoS Protection

**Rate Limiting**:
- Dispute rate limit: 3 per 24 hours per participant
- Evidence size limit: 10 MB per submission
- Max evidence count: 20 per dispute

**Resource Limits**:
- Payment store pagination
- Background goroutine sleep intervals
- HTTP request timeouts

#### 5. Input Validation

**Address Validation**:
- Bitcoin: Base58Check or Bech32 validation
- Monero: Address checksum verification

**Amount Validation**:
- Dust limit enforcement (0.00001 BTC minimum)
- Maximum amount checks (prevent overflow)

**Timeout Validation**:
- Minimum timeout: 1 hour
- Maximum timeout: 30 days
- Extension limit: 7 days

---

## Performance Considerations

### Scalability Limits

**Single-Instance Limits**:
- **Payments**: ~10,000 pending payments (with FileStore)
- **Throughput**: ~100 payment creations/sec (CPU bound)
- **Verification**: ~1,000 pending payments per minute (blockchain API limited)

**Bottlenecks**:
1. **Blockchain APIs**: Rate limited (5-100 req/min typical)
2. **File I/O**: FileStore locks during write (use database for high concurrency)
3. **Signature Verification**: CPU-intensive (ECDSA operations)

### Optimization Strategies

**1. Caching**:
```go
// Cache blockchain queries
type CachedBalance struct {
    Balance   float64
    CachedAt  time.Time
    TTL       time.Duration
}

// Cache payment lookups
var paymentCache = sync.Map{} // payment ID → *Payment
```

**2. Batching**:
```go
// Batch blockchain queries
addresses := make([]string, 0, 100)
for _, p := range pendingPayments {
    addresses = append(addresses, p.Addresses[Bitcoin])
}
balances, err := wallet.GetBalancesBatch(addresses)
```

**3. Database Backend**:
```go
// Replace FileStore with PostgreSQL for multi-instance
store := NewPostgreSQLStore("postgres://...")
```

**4. Horizontal Scaling**:
- Use shared database (PostgreSQL, Redis)
- Load balance across multiple paywall instances
- Separate verification workers from web servers

**5. Indexing**:
```go
// Add indexes for common queries
type IndexedStore struct {
    payments    map[string]*Payment
    byAddress   map[string]*Payment  // Address → Payment index
    byStatus    map[Status][]*Payment // Status → Payments index
    expiringAt  *btree.BTree          // Sorted by expiration
}
```

### Monitoring Metrics

**Key Metrics**:
- Payment creation rate (payments/sec)
- Payment confirmation rate (confirmations/min)
- Average payment lifetime (creation → confirmation)
- Verification goroutine lag (pending payments queue depth)
- Storage operation latency (ms per CRUD op)
- Blockchain API latency and error rate

**Alerting Thresholds**:
- Pending payments > 1,000 (verification falling behind)
- Average confirmation time > 1 hour (blockchain issue)
- Storage latency > 100ms (I/O bottleneck)
- Blockchain API error rate > 10% (connectivity issue)

---

## Future Enhancements

### Planned Features (from ROADMAP.md)

1. **Lightning Network Support**: Instant payments with low fees
2. **Hardware Wallet Integration**: Trezor/Ledger for signing
3. **Taproot Multisig**: BIP341/342 for improved privacy
4. **Webhook Notifications**: Event-driven external integrations
5. **Ethereum/ERC-20**: Broader cryptocurrency support

### Architectural Changes for Scale

**For 100,000+ concurrent payments**:
- Replace FileStore with PostgreSQL/Redis
- Separate verification workers (message queue)
- Implement payment sharding (by address prefix)
- Add L2 caching layer (Redis)
- Use read replicas for payment lookups

**For Multi-Region**:
- Deploy paywall instances in multiple regions
- Use distributed database (CockroachDB, YugabyteDB)
- Geo-route users to nearest instance
- Replicate blockchain cache across regions

---

## Contributing

When contributing to the paywall codebase, please ensure:

1. **Preserve Architecture**: Maintain separation of concerns between layers
2. **Extend Interfaces**: Add new storage backends or wallet types via interfaces
3. **Document Decisions**: Update this file when making architectural changes
4. **Test Thoroughly**: Add unit tests for business logic, integration tests for flows
5. **Consider Security**: Review changes for security implications (see Threat Model)

For detailed contribution guidelines, see [CONTRIBUTING.md](../CONTRIBUTING.md).

---

## References

- [BIP32](https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki) - Hierarchical Deterministic Wallets
- [BIP44](https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki) - Multi-Account Hierarchy for Deterministic Wallets
- [BIP39](https://github.com/bitcoin/bips/blob/master/bip-0039.mediawiki) - Mnemonic Code for Generating Deterministic Keys
- [Monero RPC Documentation](https://www.getmonero.org/resources/developer-guides/wallet-rpc.html)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
