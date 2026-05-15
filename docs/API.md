# API Reference

Complete reference for the opd-ai/paywall public API.

## Table of Contents

- [Main Package](#main-package)
- [Multisig Support](#multisig-support)
- [Wallet Package](#wallet-package)
- [Storage Interface](#storage-interface)

---

## Main Package

### Types

#### Config

Configuration for creating a new Paywall instance.

```go
type Config struct {
    // Payment prices (at least one must be > 0)
    PriceInBTC     float64
    PriceInXMR     float64

    // Blockchain configuration
    TestNet        bool          // Use Bitcoin/Monero testnet
    MinConfirmations int          // Confirmations required for payment validation
    PaymentTimeout time.Duration // How long to wait for payment before expiring

    // Storage backend
    Store          Store         // Payment store (Memory, File, or EncryptedFile)

    // Monero RPC configuration (optional if not using Monero)
    XMRUser        string        // Monero wallet RPC username
    XMRPassword    string        // Monero wallet RPC password
    XMRRPC         string        // Monero wallet RPC endpoint
}
```

**Requirements**:
- At least one of `PriceInBTC` or `PriceInXMR` must be greater than 0
- `PriceInBTC` must be >= 0.00001 (dust limit)
- `PriceInXMR` must be >= 0.0001 (dust limit) if set
- `PaymentTimeout` must be positive
- `Store` cannot be nil

**Example**:
```go
config := paywall.Config{
    PriceInBTC:      0.001,
    TestNet:         true,
    Store:           paywall.NewMemoryStore(),
    PaymentTimeout:  time.Hour * 24,
    MinConfirmations: 1,
}
```

#### Payment

Represents a single payment request.

```go
type Payment struct {
    ID          string                    // Unique payment identifier
    Status      string                    // StatusPending, StatusConfirmed, etc.
    Amounts     map[WalletType]float64    // Amount in BTC, XMR per address
    Addresses   map[WalletType]string     // Payment address per currency
    Confirmations uint64                  // Current blockchain confirmations
    Expiration  time.Time                 // When payment request expires
    CreatedAt   time.Time                 // Payment creation timestamp
}
```

**Status Values**:
- `StatusPending` — Awaiting payment
- `StatusConfirmed` — Payment received and confirmed
- `StatusExpired` — Payment deadline passed

#### FileStoreConfig

Configuration for persistent encrypted payment storage.

```go
type FileStoreConfig struct {
    DataDir       string // Directory for payment files
    EncryptionKey []byte // 32-byte AES-256 encryption key
}
```

### Functions

#### NewPaywall

```go
func NewPaywall(config Config) (*Paywall, error)
```

Creates a new Paywall instance with the given configuration.

**Returns**: 
- `*Paywall` on success
- Error if configuration is invalid (e.g., invalid prices, missing store)

**Behavior**:
- Validates all configuration parameters
- Generates new BTC HD wallet
- Initializes Monero wallet (if configured)
- Starts background payment verification goroutine
- Returns error if crypto/rand fails (fatal security requirement)

**Example**:
```go
pw, err := paywall.NewPaywall(paywall.Config{
    PriceInBTC: 0.001,
    TestNet:    true,
    Store:      paywall.NewMemoryStore(),
})
if err != nil {
    log.Fatal(err)
}
defer pw.Close()
```

#### NewMemoryStore

```go
func NewMemoryStore() Store
```

Creates an in-memory payment store suitable for testing and development.

**Properties**:
- Thread-safe (uses sync.RWMutex)
- All data lost on shutdown
- Fast, no I/O latency

#### NewFileStore

```go
func NewFileStore(dataDir string) (Store, error)
```

Creates a file-based payment store with optional encryption.

**Parameters**:
- `dataDir`: Directory for storing payment files

**Returns**: 
- `Store` interface implementation
- Error if directory cannot be created

#### NewFileStoreWithConfig

```go
func NewFileStoreWithConfig(config FileStoreConfig) (Store, error)
```

Creates a file-based payment store with AES-256 encryption.

**Parameters**:
- `config.DataDir`: Directory for encrypted payment files
- `config.EncryptionKey`: 32-byte encryption key from `wallet.GenerateEncryptionKey()`

**Returns**:
- `Store` with encryption enabled
- Error if configuration invalid or directory inaccessible

**Security**: Encryption key is never logged or transmitted. Store securely in environment variables or secrets manager.

### Methods

#### (*Paywall) Middleware

```go
func (p *Paywall) Middleware(next http.Handler) http.Handler
```

HTTP middleware that protects the next handler behind a paywall.

**Behavior**:
1. Generates or retrieves payment request from encrypted cookie
2. Checks if payment is confirmed
3. If confirmed within timeout: calls `next` handler
4. If not confirmed: renders payment page with QR codes
5. Sets secure HttpOnly cookie with payment tracking

**Security**:
- Uses `__Host-` prefixed cookies (HTTPS-only, HttpOnly, SameSite=Strict)
- Cookies expire after `PaymentTimeout`
- Cannot be manipulated by JavaScript

**Example**:
```go
protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Access granted"))
})

http.Handle("/protected", pw.Middleware(protected))
```

#### (*Paywall) CreatePayment

```go
func (p *Paywall) CreatePayment() (*Payment, error)
```

Creates a new payment request.

**Returns**:
- `*Payment` with generated addresses for each enabled currency
- Error if payment storage fails

**Example**:
```go
payment, err := pw.CreatePayment()
if err != nil {
    log.Fatal(err)
}
log.Printf("Send payment to %s", payment.Addresses[wallet.Bitcoin])
```

#### (*Paywall) Stop / (*Paywall) Close

```go
func (p *Paywall) Close() error
```

Stops the background payment verification goroutine and releases resources.

**Must be called**: On application shutdown to prevent goroutine leaks.

**Example**:
```go
pw, _ := paywall.NewPaywall(config)
defer pw.Close()  // Cleanup on exit
```

---

## Multisig Support

The paywall package provides optional multisig (multi-signature) support for escrow and dispute resolution scenarios. Multisig features are opt-in and fully backward compatible with single-signature mode.

### Multisig Configuration

Extend the `Config` struct with multisig parameters:

```go
type Config struct {
    // ... existing fields ...
    
    // Multisig configuration (optional - all zero values mean single-sig mode)
    MultisigEnabled     bool                              // Enable multisig address generation
    MultisigRequired    int                               // m in m-of-n multisig
    MultisigTotal       int                               // n in m-of-n multisig
    ParticipantPubKeys  map[wallet.WalletType][][]byte    // Public keys per wallet type
    MultisigRole        MultisigRole                      // This instance's role
}
```

**Requirements**:
- When `MultisigEnabled = true`, all other multisig fields must be provided
- `MultisigRequired` must be >= 1 and <= `MultisigTotal`
- `MultisigTotal` must match the length of `ParticipantPubKeys` for each wallet type
- Maximum 15 public keys (Bitcoin consensus limit)
- Public keys must be in compressed format (33 bytes)

**Example**:
```go
pubKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

config := paywall.Config{
    PriceInBTC:         0.001,
    TestNet:            true,
    Store:              paywall.NewMemoryStore(),
    PaymentTimeout:     time.Hour * 24,
    MultisigEnabled:    true,
    MultisigRequired:   2,  // 2-of-3 multisig
    MultisigTotal:      3,
    ParticipantPubKeys: map[wallet.WalletType][][]byte{
        wallet.Bitcoin: pubKeys,
    },
    MultisigRole:       paywall.RoleBuyer,
}
```

### Multisig Types

#### MultisigRole

```go
type MultisigRole string

const (
    RoleBuyer   MultisigRole = "buyer"
    RoleSeller  MultisigRole = "seller"
    RoleArbiter MultisigRole = "arbiter"
)
```

Identifies the role of a participant in a multisig transaction.

#### SignatureData

```go
type SignatureData struct {
    SignerID  string       // Unique identifier for the signer
    Role      MultisigRole // Signer's role (buyer, seller, arbiter)
    Signature []byte       // Cryptographic signature bytes
    PublicKey []byte       // Signer's public key for verification
    SignedAt  time.Time    // Timestamp when signature was created
}
```

Contains a partial signature for multisig transactions.

#### EscrowState

```go
type EscrowState int

const (
    EscrowNone      EscrowState = iota  // Not an escrow payment
    EscrowPending                       // Escrow created, awaiting funding
    EscrowFunded                        // Buyer funded the escrow
    EscrowCompleted                     // Released to seller
    EscrowDisputed                      // Dispute raised
    EscrowRefunded                      // Refunded to buyer
)
```

Tracks the state of an escrow payment.

### Multisig Payment Fields

When `MultisigEnabled = true`, the `Payment` struct includes additional fields:

```go
type Payment struct {
    // ... existing fields ...
    
    // Multisig fields
    MultisigEnabled    bool                                      // True if multisig
    MultisigMetadata   map[wallet.WalletType]*MultisigMetadata  // Metadata per wallet
    RequiredSignatures map[wallet.WalletType]int                // Required sigs per wallet
    Signatures         map[wallet.WalletType][]SignatureData    // Collected signatures
    
    // Escrow fields (optional)
    EscrowState        EscrowState  // Current escrow state
    EscrowTimeout      time.Time    // When escrow auto-refunds
    DisputeReason      string       // Reason if disputed
}
```

### Multisig Coordinator

#### NewMultisigCoordinator

```go
func NewMultisigCoordinator(
    pw *Paywall,
    auth MultisigAuthenticator,
    notifier MultisigWebhookNotifier,
) *MultisigCoordinator
```

Creates a coordinator for managing multisig signature collection.

**Parameters**:
- `pw`: Configured paywall with multisig enabled
- `auth`: Optional authenticator for restricting API access
- `notifier`: Optional webhook notifier for signature events

**Returns**: `*MultisigCoordinator` for attaching HTTP handlers

**Example**:
```go
coordinator := paywall.NewMultisigCoordinator(pw, nil, nil)

http.HandleFunc("/multisig/initiate", coordinator.HandleInitiate)
http.HandleFunc("/multisig/sign", coordinator.HandleSign)
http.HandleFunc("/multisig/status/", coordinator.HandleStatus)
http.HandleFunc("/multisig/broadcast", coordinator.HandleBroadcast)
```

### Multisig HTTP Handlers

#### POST /multisig/initiate

Initiates a new multisig payment.

**Request**:
```json
{
    "wallet_type": "bitcoin",
    "required_sigs": 2,
    "public_keys": ["<pubkey1>", "<pubkey2>", "<pubkey3>"],
    "role": "buyer",
    "price_multiplier": 1.0
}
```

**Response**:
```json
{
    "payment_id": "abc123",
    "address": "3MultiSigAddress",
    "amount": 0.001,
    "redeem_script": "<base64>",
    "expires_at": "2026-05-14T18:00:00Z"
}
```

#### POST /multisig/sign

Submits a partial signature.

**Request**:
```json
{
    "payment_id": "abc123",
    "wallet_type": "bitcoin",
    "signer_id": "buyer1",
    "role": "buyer",
    "signature": "<signature_bytes>",
    "public_key": "<pubkey_bytes>"
}
```

**Response**:
```json
{
    "success": true,
    "signature_count": 2,
    "required_signatures": 2,
    "message": "Signature accepted (2 of 2)"
}
```

#### GET /multisig/status/:paymentID

Retrieves current signing status.

**Response**:
```json
{
    "payment_id": "abc123",
    "status": "pending",
    "confirmations": 0,
    "signatures": {
        "bitcoin": [
            {
                "signer_id": "buyer1",
                "role": "buyer",
                "signed_at": "2026-05-13T18:00:00Z"
            }
        ]
    },
    "required_signatures": {
        "bitcoin": 2
    },
    "ready_to_broadcast": false,
    "escrow_state": "funded"
}
```

#### POST /multisig/broadcast

Broadcasts a fully-signed transaction.

**Request**:
```json
{
    "payment_id": "abc123",
    "wallet_type": "bitcoin",
    "transaction": "<signed_tx_bytes>"
}
```

**Response**:
```json
{
    "success": true,
    "transaction_id": "abc123def456",
    "message": "Transaction broadcast successful"
}
```

### Multisig API Client

#### NewMultisigClient

```go
func NewMultisigClient(baseURL string, authToken string) *MultisigClient
```

Creates an API client for interacting with multisig endpoints.

**Parameters**:
- `baseURL`: Paywall server URL (e.g., "https://api.example.com")
- `authToken`: Optional bearer token for authentication

**Returns**: `*MultisigClient` for making API calls

**Example**:
```go
client := paywall.NewMultisigClient("https://api.example.com", "token123")

// Initiate multisig payment
resp, err := client.InitiateMultisig(
    wallet.Bitcoin,
    2,  // 2-of-3
    [][]byte{pubKey1, pubKey2, pubKey3},
    paywall.RoleBuyer,
    1.0,
)
```

#### (*MultisigClient) InitiateMultisig

```go
func (mc *MultisigClient) InitiateMultisig(
    walletType wallet.WalletType,
    requiredSigs int,
    publicKeys [][]byte,
    role MultisigRole,
    priceMultiplier float64,
) (*MultisigInitiateResponse, error)
```

Starts a new multisig payment setup.

**Parameters**:
- `walletType`: Bitcoin or Monero
- `requiredSigs`: Number of signatures required (m in m-of-n)
- `publicKeys`: All participant public keys
- `role`: Role of the initiator
- `priceMultiplier`: Multiplier for the base price (default 1.0)

**Returns**:
- `*MultisigInitiateResponse` with payment ID and address
- Error if initiation fails

#### (*MultisigClient) SubmitSignature

```go
func (mc *MultisigClient) SubmitSignature(
    paymentID string,
    walletType wallet.WalletType,
    signerID string,
    role MultisigRole,
    signature []byte,
    publicKey []byte,
) (*MultisigSignResponse, error)
```

Submits a partial signature for a multisig payment.

**Parameters**:
- `paymentID`: Payment to sign
- `walletType`: Wallet type for this signature
- `signerID`: Unique identifier for the signer
- `role`: Signer's role
- `signature`: Cryptographic signature bytes
- `publicKey`: Signer's public key

**Returns**:
- `*MultisigSignResponse` with signature count
- Error if submission fails

#### (*MultisigClient) GetStatus

```go
func (mc *MultisigClient) GetStatus(paymentID string) (*MultisigStatusResponse, error)
```

Retrieves current signing status for a payment.

**Returns**:
- `*MultisigStatusResponse` with signatures and readiness
- Error if payment not found

#### (*MultisigClient) BroadcastTransaction

```go
func (mc *MultisigClient) BroadcastTransaction(
    paymentID string,
    walletType wallet.WalletType,
    transaction []byte,
) (*MultisigBroadcastResponse, error)
```

Broadcasts a fully-signed multisig transaction.

**Parameters**:
- `paymentID`: Payment to broadcast
- `walletType`: Wallet type
- `transaction`: Fully-signed transaction bytes

**Returns**:
- `*MultisigBroadcastResponse` with transaction ID
- Error if broadcast fails or insufficient signatures

#### (*MultisigClient) WaitForSignatures

```go
func (mc *MultisigClient) WaitForSignatures(
    paymentID string,
    timeout time.Duration,
    pollInterval time.Duration,
) (*MultisigStatusResponse, error)
```

Polls for signatures until required count is reached or timeout expires.

**Parameters**:
- `paymentID`: Payment to monitor
- `timeout`: Maximum time to wait
- `pollInterval`: How often to check status

**Returns**:
- `*MultisigStatusResponse` when ready
- Error if timeout or polling fails

**Example**:
```go
status, err := client.WaitForSignatures(
    "payment123",
    5 * time.Minute,
    10 * time.Second,
)
if err != nil {
    log.Fatal(err)
}
if status.ReadyToBroadcast {
    // Proceed with broadcast
}
```

### Escrow Manager

#### NewEscrowManager

```go
func NewEscrowManager(pw *Paywall) (*EscrowManager, error)
```

Creates an escrow manager for 2-of-3 multisig escrow workflows.

**Requires**: Paywall must have multisig enabled

**Returns**:
- `*EscrowManager` for managing escrow lifecycles
- Error if multisig not enabled

#### (*EscrowManager) CreateEscrow

```go
func (em *EscrowManager) CreateEscrow(
    priceMultiplier float64,
    escrowTimeout time.Duration,
) (string, error)
```

Creates a new escrow payment.

**Parameters**:
- `priceMultiplier`: Price adjustment factor
- `escrowTimeout`: When to auto-refund if unresolved

**Returns**:
- Payment ID
- Error if creation fails

#### (*EscrowManager) FundEscrow

```go
func (em *EscrowManager) FundEscrow(paymentID string) error
```

Marks an escrow as funded after buyer payment confirmed.

#### (*EscrowManager) ReleaseToSeller

```go
func (em *EscrowManager) ReleaseToSeller(paymentID string) error
```

Releases escrow funds to seller (requires buyer + seller or arbiter signatures).

#### (*EscrowManager) RefundBuyer

```go
func (em *EscrowManager) RefundBuyer(paymentID string) error
```

Refunds escrow to buyer (requires buyer + seller or arbiter signatures).

### Dispute Resolution

#### (*EscrowManager) RequestDispute

```go
func (em *EscrowManager) RequestDispute(paymentID string, reason string) error
```

Raises a dispute for an escrow payment.

**Parameters**:
- `paymentID`: Escrow to dispute
- `reason`: Explanation for the dispute

**Returns**: Error if dispute cannot be raised (e.g., wrong state)

#### (*EscrowManager) ResolveDispute

```go
func (em *EscrowManager) ResolveDispute(
    paymentID string,
    favorBuyer bool,
    resolution string,
) error
```

Resolves a dispute in favor of buyer or seller.

**Parameters**:
- `paymentID`: Disputed payment
- `favorBuyer`: True to refund buyer, false to pay seller
- `resolution`: Explanation of the resolution

**Returns**: Error if resolution fails

#### (*EscrowManager) CheckEscrowTimeouts

```go
func (em *EscrowManager) CheckEscrowTimeouts() ([]string, error)
```

Checks for expired escrows and returns their payment IDs.

**Returns**:
- List of payment IDs that have timed out
- Error if check fails

**Use case**: Periodic cleanup of expired escrows

#### (*EscrowManager) ExtendTimeout

```go
func (em *EscrowManager) ExtendTimeout(
    paymentID string,
    extension time.Duration,
    sig1, sig2 *SignatureData,
) error
```

Extends the timeout deadline for an escrow payment.

**Parameters**:
- `paymentID`: Escrow to extend
- `extension`: Additional time to add (max 7 days)
- `sig1`, `sig2`: Signatures from 2-of-3 participants authorizing extension

**Requirements**:
- Extension must be <= 7 days
- Requires 2-of-3 multisig authorization
- Only valid for funded escrows

**Returns**: Error if extension denied or invalid

#### (*EscrowManager) CastArbiterVote

```go
func (em *EscrowManager) CastArbiterVote(
    paymentID string,
    vote *ArbiterVote,
) error
```

Records an arbiter's vote on a disputed payment (multi-arbiter consensus).

**Parameters**:
- `paymentID`: Payment under dispute
- `vote`: Arbiter's decision and signature

**Returns**: Error if vote invalid or duplicate

**See also**: ArbiterConsensusManager for multi-arbiter voting

#### (*EscrowManager) GetConsensusStatus

```go
func (em *EscrowManager) GetConsensusStatus(
    paymentID string,
) (*ArbiterConsensus, error)
```

Retrieves the current multi-arbiter consensus state.

**Returns**:
- `*ArbiterConsensus` with voting status and tallies
- Error if consensus not found

#### (*EscrowManager) ActivateFallbackArbiters

```go
func (em *EscrowManager) ActivateFallbackArbiters(
    paymentID string,
) error
```

Activates fallback arbiters if primary arbiters fail to reach consensus.

**Use case**: When primary arbiters are unresponsive or fail to vote

**Returns**: Error if activation fails

#### (*EscrowManager) SubmitDisputeEvidence

```go
func (em *EscrowManager) SubmitDisputeEvidence(
    paymentID string,
    evidence *Evidence,
) error
```

Submits evidence for a disputed payment.

**Parameters**:
- `paymentID`: Disputed payment
- `evidence`: Evidence struct with data, description, and submitter info

**Validation**:
- Evidence size must be <= 10 MB
- Maximum 20 pieces of evidence per dispute
- Evidence must be signed by submitter

**Returns**: Error if evidence invalid or limit exceeded

#### (*EscrowManager) StartTimeoutMonitor

```go
func (em *EscrowManager) StartTimeoutMonitor(
    config TimeoutMonitorConfig,
) *TimeoutMonitor
```

Starts background goroutine to automatically monitor and resolve escrow timeouts.

**Parameters**:
- `config.CheckInterval`: How often to check for timeouts (default: 5 minutes)
- `config.UseBlockchainTime`: Use blockchain timestamps instead of system time
- `config.AutoRefund`: Automatically process refunds (requires arbiter signer)

**Returns**: `*TimeoutMonitor` instance for controlling the monitor

**Important**: Call `monitor.Stop()` on shutdown to cleanup goroutine

**Example**:
```go
monitor := em.StartTimeoutMonitor(paywall.TimeoutMonitorConfig{
    CheckInterval:     5 * time.Minute,
    UseBlockchainTime: true,
    AutoRefund:        true,
})
defer monitor.Stop()
```

---

## Arbiter Consensus System

Multi-arbiter voting for decentralized dispute resolution.

### Types

#### ArbiterVote

```go
type ArbiterVote struct {
    ArbiterPubKey []byte         // Arbiter's public key
    ArbiterID     string          // Unique arbiter identifier
    Decision      MultisigRole    // RoleBuyer or RoleSeller
    Reason        string          // Explanation for decision
    Signature     *SignatureData  // Arbiter's cryptographic signature
    VotedAt       time.Time       // Vote timestamp
}
```

Represents a single arbiter's vote on a disputed payment.

#### ArbiterConsensus

```go
type ArbiterConsensus struct {
    PaymentID        string          // Payment under dispute
    RequiredVotes    int             // Votes needed (e.g., 3 in 3-of-5)
    TotalArbiters    int             // Total arbiters (e.g., 5 in 3-of-5)
    Votes            []*ArbiterVote  // All votes cast
    VotingDeadline   time.Time       // When voting closes
    ConsensusReached bool            // True if consensus achieved
    FinalDecision    MultisigRole    // Winner (if consensus reached)
    Status           ConsensusStatus // Current voting state
}
```

Tracks multi-arbiter voting progress and results.

**Status values**:
- `ConsensusOpen` — Voting in progress
- `ConsensusReached` — Required votes achieved
- `ConsensusExpired` — Deadline passed without consensus
- `ConsensusFallback` — Fallback arbiters activated

#### ArbiterConfig

```go
type ArbiterConfig struct {
    RequiredArbiterVotes int            // Votes needed (e.g., 3)
    TotalArbiters        int            // Total arbiters (e.g., 5)
    PrimaryArbiters      [][]byte       // Primary arbiter public keys
    FallbackArbiters     [][]byte       // Backup arbiter public keys
    VotingTimeout        time.Duration  // Time limit for voting
}
```

Configuration for multi-arbiter consensus system.

### ArbiterConsensusManager

Coordinates multi-arbiter voting on disputes.

#### NewArbiterConsensusManager

```go
func NewArbiterConsensusManager(
    config *ArbiterConfig,
    reputationTracker *ArbiterReputationTracker,
) (*ArbiterConsensusManager, error)
```

Creates a new multi-arbiter consensus manager.

**Parameters**:
- `config`: Multi-arbiter configuration (required votes, total arbiters, etc.)
- `reputationTracker`: Optional tracker for arbiter performance metrics

**Validation**:
- `RequiredArbiterVotes` must be >= 1 and <= `TotalArbiters`
- `TotalArbiters` must match length of `PrimaryArbiters`
- At least one arbiter required

**Returns**:
- `*ArbiterConsensusManager` for managing consensus
- Error if configuration invalid

#### (*ArbiterConsensusManager) InitiateConsensus

```go
func (acm *ArbiterConsensusManager) InitiateConsensus(
    paymentID string,
) (*ArbiterConsensus, error)
```

Starts a new consensus process for a disputed payment.

**Parameters**:
- `paymentID`: Payment to initiate consensus for

**Returns**:
- `*ArbiterConsensus` with initial state
- Error if consensus already exists

#### (*ArbiterConsensusManager) CastVote

```go
func (acm *ArbiterConsensusManager) CastVote(
    paymentID string,
    vote *ArbiterVote,
) error
```

Records an arbiter's vote and checks if consensus is reached.

**Parameters**:
- `paymentID`: Payment being voted on
- `vote`: Arbiter's decision with signature

**Validation**:
- Arbiter must be authorized (in PrimaryArbiters or FallbackArbiters)
- Cannot vote twice on same dispute
- Voting must still be open (before deadline)

**Side effects**:
- Updates consensus status if required votes reached
- Records reputation metrics via tracker

**Returns**: Error if vote invalid or duplicate

#### (*ArbiterConsensusManager) GetConsensus

```go
func (acm *ArbiterConsensusManager) GetConsensus(
    paymentID string,
) (*ArbiterConsensus, error)
```

Retrieves current consensus state for a payment.

**Returns**:
- `*ArbiterConsensus` with current votes and status
- Error if consensus not found

#### (*ArbiterConsensusManager) CheckExpiredVoting

```go
func (acm *ArbiterConsensusManager) CheckExpiredVoting()
```

Checks for expired voting deadlines and updates consensus status.

**Use case**: Call periodically (e.g., every 5 minutes) to detect expired votes

**Side effects**: Updates consensus status to `ConsensusExpired` for expired deadlines

#### (*ArbiterConsensusManager) ActivateFallbackArbiters

```go
func (acm *ArbiterConsensusManager) ActivateFallbackArbiters(
    paymentID string,
) error
```

Activates fallback arbiters when primary arbiters fail to reach consensus.

**Use case**: When voting expires without consensus or primary arbiters are unresponsive

**Returns**: Error if no fallback arbiters configured

---

## Arbiter Reputation System

Tracks arbiter performance and reliability metrics.

### Types

#### ArbiterReputation

```go
type ArbiterReputation struct {
    ArbiterID            string        // Unique arbiter identifier
    PublicKey            []byte        // Arbiter's public key
    TotalDecisions       int           // Total disputes voted on
    ConsensusDecisions   int           // Votes matching consensus
    DissentingDecisions  int           // Votes against consensus
    NonParticipations    int           // Failed to vote count
    AverageResponseTime  time.Duration // Average time to vote
    ReputationScore      float64       // Computed score (0-100)
    FirstDecisionAt      time.Time     // First vote timestamp
    LastDecisionAt       time.Time     // Most recent vote timestamp
    LastUpdated          time.Time     // Last reputation update
}
```

Performance metrics for a single arbiter.

**Reputation score calculation**:
- Consensus rate: 50% weight
- Response time: 25% weight
- Participation rate: 25% weight

### ArbiterReputationTracker

Manages reputation for all arbiters.

#### NewArbiterReputationTracker

```go
func NewArbiterReputationTracker() *ArbiterReputationTracker
```

Creates a new reputation tracker with empty reputation map.

#### (*ArbiterReputationTracker) RecordDecision

```go
func (art *ArbiterReputationTracker) RecordDecision(
    arbiterID string,
    withConsensus bool,
    responseTime time.Duration,
)
```

Records an arbiter's decision and updates their reputation.

**Parameters**:
- `arbiterID`: Arbiter who voted
- `withConsensus`: True if arbiter voted with majority
- `responseTime`: Time taken to cast vote

**Side effects**:
- Increments `TotalDecisions` and `ConsensusDecisions` or `DissentingDecisions`
- Updates `AverageResponseTime` with new data point
- Recalculates `ReputationScore`

#### (*ArbiterReputationTracker) RecordNonParticipation

```go
func (art *ArbiterReputationTracker) RecordNonParticipation(
    arbiterID string,
)
```

Records when an arbiter failed to vote by the deadline.

**Side effects**: Increments `NonParticipations` and reduces `ReputationScore`

#### (*ArbiterReputationTracker) GetReputation

```go
func (art *ArbiterReputationTracker) GetReputation(
    arbiterID string,
) (*ArbiterReputation, error)
```

Retrieves reputation metrics for a specific arbiter.

**Returns**:
- `*ArbiterReputation` with current metrics
- Error if arbiter not found

#### (*ArbiterReputationTracker) ListReputations

```go
func (art *ArbiterReputationTracker) ListReputations() []*ArbiterReputation
```

Returns reputation data for all tracked arbiters.

#### (*ArbiterReputationTracker) GetTopArbiters

```go
func (art *ArbiterReputationTracker) GetTopArbiters(
    n int,
) []*ArbiterReputation
```

Returns the top N arbiters sorted by reputation score.

**Use case**: Selecting best arbiters for new disputes

#### (*ArbiterReputationTracker) RegisterArbiter

```go
func (art *ArbiterReputationTracker) RegisterArbiter(
    arbiterID string,
    publicKey []byte,
) error
```

Registers a new arbiter in the reputation system.

**Parameters**:
- `arbiterID`: Unique identifier
- `publicKey`: Arbiter's public key for signature verification

**Returns**: Error if arbiter already registered

#### (*ArbiterReputationTracker) RemoveArbiter

```go
func (art *ArbiterReputationTracker) RemoveArbiter(
    arbiterID string,
) error
```

Removes an arbiter from the reputation system.

**Use case**: Disabling compromised or retired arbiters

**Returns**: Error if arbiter not found

#### (*ArbiterReputationTracker) GetStatistics

```go
func (art *ArbiterReputationTracker) GetStatistics() *ArbiterStatistics
```

Computes aggregate statistics across all arbiters.

**Returns**:
- `*ArbiterStatistics` with totals and averages

---

## Dispute Enhancements

Anti-spam and anti-abuse features for dispute system.

### Types

#### DisputeEnhancements

```go
type DisputeEnhancements struct {
    DisputeFeePercentage  float64       // Fee as % of payment (e.g., 0.01 = 1%)
    MinDisputeFee         float64       // Minimum fee in base units
    MaxDisputeFee         float64       // Maximum fee in base units
    MaxEvidenceSize       int64         // Max evidence size in bytes
    MaxEvidenceCount      int           // Max evidence pieces per dispute
    DisputeRateLimit      int           // Max disputes per time window
    DisputeRateWindow     time.Duration // Time window for rate limiting
    AllowTimeoutExtension bool          // Enable timeout extensions
    MaxTimeoutExtension   time.Duration // Maximum extension time
}
```

Configuration for dispute fees, rate limiting, and evidence validation.

**Default values**:
- Fee: 1% of payment (min 0.00001 BTC, max 0.01 BTC)
- Evidence: 10 MB max, 20 items max
- Rate limit: 3 disputes per 24 hours
- Extension: 7 days max

#### Evidence

```go
type Evidence struct {
    ID          string    // Unique evidence identifier
    SubmitterID string    // Who submitted (buyer, seller, arbiter)
    Description string    // Evidence description
    DataHash    []byte    // SHA256 hash of evidence data
    Signature   []byte    // Submitter's signature
    SubmittedAt time.Time // Submission timestamp
    SizeBytes   int64     // Evidence size for DoS prevention
}
```

Evidence submitted during dispute resolution.

### DisputeFeeCalculator

Calculates dispute fees to prevent frivolous disputes.

#### NewDisputeFeeCalculator

```go
func NewDisputeFeeCalculator(
    enhancements *DisputeEnhancements,
) *DisputeFeeCalculator
```

Creates a new fee calculator with specified configuration.

**Parameters**:
- `enhancements`: Fee configuration (or nil for defaults)

#### (*DisputeFeeCalculator) CalculateFee

```go
func (dfc *DisputeFeeCalculator) CalculateFee(
    paymentAmount float64,
) float64
```

Calculates the dispute fee for a payment.

**Parameters**:
- `paymentAmount`: Payment amount in base currency units

**Returns**: Fee amount (percentage-based with min/max bounds)

**Example**:
```go
calculator := paywall.NewDisputeFeeCalculator(nil) // Use defaults
fee := calculator.CalculateFee(0.1) // 0.1 BTC payment
// Returns 0.001 BTC (1% of 0.1)
```

#### (*DisputeFeeCalculator) ValidateFeePayment

```go
func (dfc *DisputeFeeCalculator) ValidateFeePayment(
    requiredFee, paidFee float64,
) error
```

Validates that sufficient fee has been paid to file a dispute.

**Returns**: Error if insufficient fee paid

### DisputeRateLimiter

Prevents dispute spam by rate-limiting per participant.

#### NewDisputeRateLimiter

```go
func NewDisputeRateLimiter(
    enhancements *DisputeEnhancements,
) *DisputeRateLimiter
```

Creates a new rate limiter with specified configuration.

#### (*DisputeRateLimiter) CheckRateLimit

```go
func (drl *DisputeRateLimiter) CheckRateLimit(
    participantID string,
) error
```

Checks if a participant can file a new dispute.

**Parameters**:
- `participantID`: Participant attempting to file dispute

**Returns**: Error if rate limit exceeded (includes retry time)

**Example error**: `"dispute rate limit exceeded: 3 disputes in last 24h0m0s, try again in 18h30m15s"`

#### (*DisputeRateLimiter) RecordDispute

```go
func (drl *DisputeRateLimiter) RecordDispute(
    participantID string,
)
```

Records that a participant filed a dispute.

**Side effects**: Updates participant's dispute history

#### (*DisputeRateLimiter) GetDisputeCount

```go
func (drl *DisputeRateLimiter) GetDisputeCount(
    participantID string,
) int
```

Returns the number of disputes filed by a participant in the current time window.

### EvidenceValidator

Validates evidence submissions to prevent DoS attacks.

#### NewEvidenceValidator

```go
func NewEvidenceValidator(
    enhancements *DisputeEnhancements,
) *EvidenceValidator
```

Creates a new evidence validator with size and count limits.

#### (*EvidenceValidator) ValidateEvidence

```go
func (ev *EvidenceValidator) ValidateEvidence(
    evidence *Evidence,
    currentEvidenceCount int,
) error
```

Validates evidence before accepting submission.

**Checks**:
- Evidence size <= MaxEvidenceSize
- Total evidence count < MaxEvidenceCount
- Evidence has valid signature
- Evidence data hash is present

**Returns**: Error if evidence invalid or limits exceeded

### TimeoutExtensionManager

Manages timeout extension requests and approvals.

#### NewTimeoutExtensionManager

```go
func NewTimeoutExtensionManager(
    store PaymentStore,
    enhancements *DisputeEnhancements,
) *TimeoutExtensionManager
```

Creates a new timeout extension manager.

**Parameters**:
- `store`: Payment store for persisting extensions
- `enhancements`: Configuration with max extension limit

#### (*TimeoutExtensionManager) RequestExtension

```go
func (tem *TimeoutExtensionManager) RequestExtension(
    paymentID string,
    requestedBy MultisigRole,
    reason string,
    extension time.Duration,
) error
```

Requests a timeout extension for an escrow.

**Parameters**:
- `paymentID`: Escrow to extend
- `requestedBy`: Participant requesting (buyer or seller)
- `reason`: Justification for extension
- `extension`: Additional time requested (max 7 days)

**Validation**:
- Extension must be > 0 and <= MaxTimeoutExtension
- Only buyer or seller can request
- Escrow must be in funded state

**Returns**: Error if request invalid

#### (*TimeoutExtensionManager) ApproveExtension

```go
func (tem *TimeoutExtensionManager) ApproveExtension(
    paymentID string,
    approver MultisigRole,
) error
```

Approves a pending timeout extension request.

**Parameters**:
- `paymentID`: Escrow with pending extension
- `approver`: Participant approving (must be different from requester)

**Requirements**:
- 2-of-3 approval: Requires requester + one other participant

**Returns**: Error if approval invalid

#### (*TimeoutExtensionManager) IsExtensionApproved

```go
func (tem *TimeoutExtensionManager) IsExtensionApproved(
    paymentID string,
) (bool, time.Duration, error)
```

Checks if an extension request has been approved.

**Returns**:
- `bool`: True if approved by 2-of-3
- `time.Duration`: Extension duration
- `error`: Error if check fails

#### (*TimeoutExtensionManager) CompleteExtension

```go
func (tem *TimeoutExtensionManager) CompleteExtension(
    paymentID string,
)
```

Marks an extension as completed and clears the pending request.

#### (*TimeoutExtensionManager) GetPendingExtension

```go
func (tem *TimeoutExtensionManager) GetPendingExtension(
    paymentID string,
) (*TimeoutExtensionRequest, error)
```

Retrieves the pending extension request for a payment.

**Returns**:
- `*TimeoutExtensionRequest` with request details
- Error if no pending extension

---

## Wallet Package

### Types

#### WalletType

```go
type WalletType string

const (
    Bitcoin WalletType = "bitcoin"
    Monero  WalletType = "monero"
)
```

Identifies cryptocurrency wallet type.

#### HDWallet

```go
type HDWallet interface {
    // Address returns a new derived address
    GetAddress() (string, error)

    // GetAddressBalance returns the balance for a specific address
    GetAddressBalance(address string) (float64, error)

    // GetLastAddress returns the last derived address
    GetLastAddress() (string, error)
}
```

Interface for hierarchical deterministic wallet implementations.

#### BTCHDWallet

Bitcoin wallet implementing BIP32/BIP44 standards.

**Features**:
- Deterministic address generation from seed
- Automatic change address tracking
- HD path: `m/44'/0'/0'/0/index` (BIP44 standard)

#### MoneroHDWallet

Monero wallet connecting via RPC interface.

**Features**:
- Subaddress generation for payment isolation
- Balance verification per address
- Integration with Monero wallet service

### Functions

#### GenerateEncryptionKey

```go
func GenerateEncryptionKey() ([]byte, error)
```

Generates a 32-byte AES-256 encryption key using `crypto/rand`.

**Returns**:
- 32-byte encryption key
- Error if entropy unavailable (fatal security failure)

**Usage**:
```go
key, err := wallet.GenerateEncryptionKey()
if err != nil {
    log.Fatal(err)  // Entropy exhaustion
}
// Store key securely
```

#### GenerateMnemonic

```go
func GenerateMnemonic(strength MnemonicStrength) (string, error)
```

Generates a new BIP39 mnemonic phrase for wallet backup.

**Parameters**:
- `strength`: `Mnemonic12Words` (128 bits) or `Mnemonic24Words` (256 bits, recommended)

**Returns**:
- Space-separated mnemonic phrase (12 or 24 English words)
- Error if entropy generation fails

**Security**:
- Uses `crypto/rand` for secure entropy
- 24-word phrases provide maximum security (256-bit entropy)
- 12-word phrases acceptable for lower-value wallets (128-bit entropy)

**Usage**:
```go
// Generate 24-word mnemonic (recommended)
mnemonic, err := wallet.GenerateMnemonic(wallet.Mnemonic24Words)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Write this down:", mnemonic)
// Example output: "abandon ability able about above absent absorb..."

// Generate 12-word mnemonic
mnemonic12, err := wallet.GenerateMnemonic(wallet.Mnemonic12Words)
```

#### ImportFromMnemonic

```go
func ImportFromMnemonic(mnemonic string, passphrase string) ([]byte, error)
```

Converts a BIP39 mnemonic phrase to a wallet seed.

**Parameters**:
- `mnemonic`: Space-separated BIP39 phrase (12 or 24 words)
- `passphrase`: Optional BIP39 passphrase (25th word) for additional security, use "" for none

**Returns**:
- 64-byte seed suitable for `NewBTCHDWallet`
- Error if mnemonic is invalid (wrong word, checksum failure)

**Security**:
- Validates mnemonic checksum before generating seed
- Normalizes whitespace automatically
- Supports optional passphrase protection

**Usage**:
```go
// Import without passphrase
seed, err := wallet.ImportFromMnemonic(
    "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
    "",
)
if err != nil {
    log.Fatal("Invalid mnemonic:", err)
}
wallet, err := wallet.NewBTCHDWallet(seed[:32], true, 1)

// Import with passphrase for extra security
seed, err := wallet.ImportFromMnemonic(mnemonic, "my-secret-passphrase")
```

#### ValidateMnemonic

```go
func ValidateMnemonic(mnemonic string) bool
```

Checks if a mnemonic phrase is valid according to BIP39 specification.

**Parameters**:
- `mnemonic`: Space-separated phrase to validate

**Returns**:
- `true` if valid (correct checksum, recognized words)
- `false` if invalid

**Validation**:
- Checks word count (12, 15, 18, 21, or 24 words)
- Verifies all words in BIP39 English wordlist
- Validates checksum integrity
- Handles extra whitespace gracefully

**Usage**:
```go
userInput := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
if !wallet.ValidateMnemonic(userInput) {
    fmt.Println("Invalid mnemonic. Please check for typos.")
    return
}
// Proceed with import
```

#### NewBTCHDWalletFromMnemonic

```go
func NewBTCHDWalletFromMnemonic(
    mnemonic string,
    passphrase string,
    testnet bool,
    minConf int,
) (*BTCHDWallet, error)
```

Creates a Bitcoin HD wallet directly from a BIP39 mnemonic phrase.

**Parameters**:
- `mnemonic`: Space-separated BIP39 phrase (12 or 24 words)
- `passphrase`: Optional BIP39 passphrase (use "" for none)
- `testnet`: True for testnet, false for mainnet
- `minConf`: Minimum confirmations for payment verification

**Returns**:
- `*BTCHDWallet` with deterministic address generation
- Error if mnemonic invalid or wallet creation fails

**Determinism**:
- Same mnemonic + passphrase always produces same addresses
- Enables wallet recovery from backed-up phrase
- Address order is deterministic and reproducible

**Usage**:
```go
// Create wallet from mnemonic
wallet, err := wallet.NewBTCHDWalletFromMnemonic(
    "abandon ability able about above absent...",
    "",    // No passphrase
    true,  // Testnet
    1,     // 1 confirmation
)
if err != nil {
    log.Fatal(err)
}

// Derive first address
addr, err := wallet.DeriveNextAddress()
```

#### MnemonicToSeed

```go
func MnemonicToSeed(mnemonic string) ([]byte, error)
```

Convenience function to convert mnemonic to seed without passphrase.

Equivalent to `ImportFromMnemonic(mnemonic, "")`.

**Usage**:
```go
seed, err := wallet.MnemonicToSeed(mnemonic)
wallet, err := wallet.NewBTCHDWallet(seed[:32], false, 6)
```

#### NewBTCHDWallet

```go
func NewBTCHDWallet(seed []byte, testnet bool, minConfirmations int) (*BTCHDWallet, error)
```

Creates a new Bitcoin HD wallet from a seed.

**Parameters**:
- `seed`: 32-byte random seed (use `crypto/rand`, not `math/rand`)
- `testnet`: True for testnet, false for mainnet
- `minConfirmations`: Minimum confirmations required for payment verification

**Returns**:
- `*BTCHDWallet`
- Error if seed validation fails

#### LoadFromFile

```go
func LoadFromFile(config StorageConfig) (*BTCHDWallet, error)
```

Loads an existing encrypted wallet from disk.

**Parameters**:
- `config.DataDir`: Directory containing wallet file
- `config.EncryptionKey`: Matching encryption key used to save

**Returns**:
- `*BTCHDWallet` with restored state
- Error if decryption or parsing fails

### Methods (BTCHDWallet)

#### (*BTCHDWallet) DeriveNextAddress

```go
func (w *BTCHDWallet) DeriveNextAddress() (string, error)
```

Derives the next payment address in sequence.

**Returns**:
- Bitcoin address (testnet or mainnet format based on creation config)

**Guarantees**:
- Addresses never reused (counter increments)
- Thread-safe (uses internal mutex)
- Deterministic (same seed always produces same sequence)

#### (*BTCHDWallet) SaveToFile

```go
func (w *BTCHDWallet) SaveToFile(config StorageConfig) error
```

Persists wallet state to encrypted file.

**Parameters**:
- `config.DataDir`: Directory for wallet file
- `config.EncryptionKey`: Encryption key for storage

**Security**:
- Uses AES-256-GCM encryption
- Creates encrypted backup of wallet seed
- Can be restored later with `LoadFromFile()`

**Example**:
```go
key, _ := wallet.GenerateEncryptionKey()
btcWallet := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet)
err := btcWallet.SaveToFile(wallet.StorageConfig{
    DataDir:       "./paywallet",
    EncryptionKey: key,
})
```

---

## Storage Interface

### Store

Interface for persisting payment records.

```go
type Store interface {
    // CreatePayment saves a new payment
    CreatePayment(payment *Payment) error

    // GetPaymentByID retrieves a payment by ID
    GetPaymentByID(id string) (*Payment, error)

    // GetPaymentByAddress retrieves a payment by Bitcoin address
    GetPaymentByAddress(addr string) (*Payment, error)

    // UpdatePayment updates an existing payment
    UpdatePayment(payment *Payment) error

    // ListPendingPayments returns all payments with < 1 confirmation
    ListPendingPayments() ([]*Payment, error)

    // ListExpiredPayments returns all expired payments
    ListExpiredPayments(before time.Time) ([]*Payment, error)

    // Close cleanup (if needed)
    Close() error
}
```

**Implementations**:
- `MemoryStore` — In-memory (fast, no persistence)
- `FileStore` — Plain file JSON (persistent, readable)
- `EncryptedFileStore` — Encrypted JSON files (secure, persistent)

### Custom Store Implementation

To implement a custom storage backend (e.g., PostgreSQL, DynamoDB):

```go
type MyDatabaseStore struct {
    db *sql.DB
}

func (s *MyDatabaseStore) CreatePayment(p *Payment) error {
    // INSERT into database
}

func (s *MyDatabaseStore) GetPaymentByID(id string) (*Payment, error) {
    // SELECT from database
}

// ... implement remaining methods
```

Then use in config:

```go
store := &MyDatabaseStore{db: dbConnection}
pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC: 0.001,
    Store:      store,
})
```

---

## Error Handling

### Error Types

Functions return standard Go errors. Key error messages:

| Condition | Error Message |
|-----------|--------------|
| Invalid config | `"PriceInBTC X is below dust limit"` |
| Invalid config | `"PriceInXMR required if XMR configured"` |
| Entropy failure | `"crypto/rand.Int failed: cannot initialize wallet securely"` (FATAL) |
| XMR missing env | `"XMR wallet password not provided"` |
| Storage failure | `"payment store: <error>"` |
| Wallet failure | `"create wallet: <error>"` |

### Error Handling Pattern

```go
pw, err := paywall.NewPaywall(config)
if err != nil {
    // Configuration or initialization error
    log.Fatal(err)
}

payment, err := pw.CreatePayment()
if err != nil {
    // Payment creation failed (storage or wallet error)
    http.Error(w, "Payment creation failed", http.StatusInternalServerError)
    return
}
```

---

## Constants

### Payment Status

```go
const (
    StatusPending   = "pending"
    StatusConfirmed = "confirmed"
    StatusExpired   = "expired"
)
```

---

## Examples

### Bitcoin-Only Paywall

```go
package main

import (
    "github.com/opd-ai/paywall"
    "net/http"
    "time"
)

func main() {
    pw, _ := paywall.NewPaywall(paywall.Config{
        PriceInBTC:     0.001,
        TestNet:        true,
        Store:          paywall.NewMemoryStore(),
        PaymentTimeout: time.Hour * 24,
    })
    defer pw.Close()

    http.Handle("/protected", pw.Middleware(
        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Write([]byte("Protected content"))
        }),
    ))
    http.ListenAndServe(":8000", nil)
}
```

### With Persistent Storage

```go
key, _ := wallet.GenerateEncryptionKey()
store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "./paywallet",
    EncryptionKey: key,
})

pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC: 0.001,
    TestNet:    false,
    Store:      store,
})
```

---

## See Also

- [CONFIGURATION.md](CONFIGURATION.md) — Detailed configuration guide
- [SECURITY.md](SECURITY.md) — Security considerations
- [EXAMPLES.md](EXAMPLES.md) — More usage examples
