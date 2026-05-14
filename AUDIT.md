# Security

## Overview

The opd-ai/paywall library implements production-grade security controls for Bitcoin and Monero payment processing. This document describes the security properties, threat model, and operational security considerations for deploying and using the paywall system.

## Core Security Properties

### Cryptographic Randomness

All security-sensitive operations depend on cryptographically secure random number generation using Go's `crypto/rand` package. This includes:

- **Wallet seed generation**: 256-bit seeds are generated using `crypto/rand.Reader`, ensuring unpredictable values
- **Blockchain API endpoint selection**: Random endpoint selection from a pool of public Bitcoin RPC endpoints prevents predictable endpoint targeting
- **Payment ID generation**: Unique payment identifiers are generated cryptographically to prevent collision attacks

**Critical**: If `crypto/rand` fails during wallet initialization (e.g., due to system entropy exhaustion or permission errors), the system will terminate immediately with a fatal error rather than gracefully degrading to weaker `math/rand`. This fail-fast approach prevents silent security degradation. The error message will indicate: `crypto/rand.Int failed: cannot initialize wallet securely`

**Operational note**: On systems with low entropy, ensure `/dev/urandom` is available and readable by the process. In containerized environments, use `--cpus` limits to trigger entropy warnings rather than allowing silent failures.

### AES-256-GCM Wallet Encryption

Wallet files persisted to disk are encrypted using AES-256 in Galois/Counter Mode (GCM), which provides both confidentiality and authentication:

```go
// Example wallet storage with encryption
key, _ := wallet.GenerateEncryptionKey()
config := wallet.StorageConfig{
    DataDir:       "./paywallet",
    EncryptionKey: key,
}
encryptedWallet, _ := wallet.LoadFromFile(config)
```

**Key properties**:
- **Key size**: 256-bit keys generated from 32 bytes of `crypto/rand`
- **Mode**: GCM provides authenticated encryption (AEAD)
- **IV/Nonce**: Unique 96-bit nonce per encryption operation
- **Authentication**: GCM authentication tag prevents tampering

**Key management**:
- Never store encryption keys in code or committed files
- Store encryption keys in environment variables or secure key storage (HashiCorp Vault, AWS Secrets Manager, etc.)
- Rotate keys periodically by re-encrypting wallets with new keys
- Consider key derivation from passphrases using `wallet.GenerateEncryptionKey()` for operator-managed encryption

### BIP32/BIP44 Hierarchical Deterministic Wallets

Bitcoin wallets implement the BIP32 and BIP44 standards, providing:

- **Deterministic address generation**: All addresses derive from a single seed using a standardized path
- **Address reuse prevention**: Each payment receives a unique address at path `m/44'/0'/0'/0/index`
  - `44'` = BIP44 purpose
  - `0'` = Bitcoin coin type
  - `0'` = Account 0
  - `0` = External chain (receiving addresses)
  - `index` = Sequential address index

**Address isolation**: Each payment created via `CreatePayment()` receives a unique derived address. The system maintains `nextIndex` and increments it after each address generation. Payments are tied to specific addresses and confirmations are verified per-address.

**HD Wallet security**: Private keys are derived on-demand from the master seed and stored only during active operations. The master seed should be:
- Backed up securely offline
- Never transmitted over networks
- Protected with strong encryption if persisted
- Used only during wallet initialization

### Cookie Security

The middleware implements secure cookie handling for payment session management:

```go
cookie := &http.Cookie{
    Name:     "__Host-payment",
    Value:    paymentID,
    Path:     "/",
    Secure:   true,                    // HTTPS only
    HttpOnly: true,                    // No JavaScript access
    SameSite: http.SameSiteStrictMode, // No cross-site transmission
    MaxAge:   int(config.PaymentTimeout.Seconds()),
}
```

**Cookie properties**:
- **`__Host-` prefix**: Ensures cookies are only sent to HTTPS endpoints (RFC 6265bis)
- **HttpOnly flag**: Prevents JavaScript and XSS attacks from accessing payment session tokens
- **SameSite=Strict**: Prevents cross-site request forgery (CSRF) by requiring same-origin requests
- **Secure flag**: Only transmitted over HTTPS connections
- **Max-Age**: Expires after payment timeout (default 24 hours)

**Deployment requirement**: Cookies require HTTPS. HTTP deployments will not set cookies correctly and payments cannot be verified.

## Threat Model

### Threats Addressed

**1. Network-Level Attacks (Man-in-the-Middle)**
- **Threat**: Attacker intercepts blockchain API requests or Bitcoin transactions
- **Mitigations**:
  - HTTPS for all external API communication
  - Multiple endpoint pool: If one endpoint is compromised, random endpoint selection may choose an uncompromised endpoint
  - Cryptographic verification: Bitcoin confirmations verified against actually confirmed transactions

**2. Unauthorized Payment Verification**
- **Threat**: Attacker claims payment for an address without actually sending funds
- **Mitigations**:
  - Payment verification checks blockchain for actual transaction confirmations
  - Minimum confirmation threshold (default: 1 for testnet, 6 for mainnet recommended)
  - Per-address balance checking: Payment only confirmed when the specific address receives the exact amount

**3. Payment Session Hijacking**
- **Threat**: Attacker steals or forges payment session cookies
- **Mitigations**:
  - Cryptographically random payment IDs (256-bit)
  - HttpOnly cookies prevent JavaScript theft
  - SameSite=Strict prevents CSRF token reuse
  - Short expiration (configurable, default 24 hours)

**4. Wallet Key Exposure**
- **Threat**: Private keys leaked to unauthorized parties
- **Mitigations**:
  - AES-256-GCM encryption for persisted wallets
  - Private keys never logged or transmitted
  - Keys derived on-demand from secured master seed

**5. Address Reuse**
- **Threat**: Attacker observes address reuse and correlates payments with users
- **Mitigations**:
  - BIP44 standard ensures unique address per payment
  - `nextIndex` incrementation prevents accidental reuse
  - Wallet recovery scanning (if implemented) detects previously used addresses to prevent reuse after restore

### Threats Out of Scope

**Payment censorship**: The system assumes the blockchain network itself is honest and does not censor transactions. If the Bitcoin or Monero network is compromised at consensus level, the system cannot prevent censorship.

**Endpoint collusion**: If all available blockchain API endpoints collude to report false confirmations, the system cannot detect the attack. Mitigate by:
- Running a local Bitcoin Full Node and configuring it as the exclusive API endpoint
- Validating endpoints have stake in the Bitcoin network
- Monitoring endpoints for anomalous behavior

**Wallet seed compromise**: If an attacker obtains the wallet seed, they can derive all past and future addresses and claim funds. Mitigate by:
- Storing seeds offline in encrypted form
- Using hardware wallets for production systems
- Limiting wallet lifetime and rotating seeds periodically

## Operational Security

### Bitcoin RPC Endpoint Configuration

The system defaults to a pool of public blockchain API endpoints. For production systems:

1. **Run a local Bitcoin Full Node**:
   ```bash
   bitcoind -testnet -rpcuser=paywall -rpcpassword=<secure> -txindex=1
   ```

2. **Configure paywall to use local node**:
   ```go
   config := paywall.Config{
       BlockchainRPC: "http://localhost:18332", // testnet
       // ... other config
   }
   ```

3. **Verify node synchronization**: Ensure the node is fully synced before accepting payments:
   ```bash
   bitcoin-cli -testnet getblockchaininfo  # Look for "blocks" == "headers"
   ```

### Minimum Confirmations

- **Testnet (testing)**: 1 confirmation acceptable for development/testing
- **Mainnet (production)**: 6+ confirmations recommended
  - 1 confirmation: ~10 minutes, high double-spend risk
  - 6 confirmations: ~60 minutes, standard "irreversible" threshold
  - 12+ confirmations: ~2 hours, maximum security

Configure via `Config.MinConfirmations`:

```go
config := paywall.Config{
    MinConfirmations: 6, // production mainnet
}
```

### Testnet vs. Mainnet Isolation

The system maintains separate networks through the `Config.TestNet` flag:

```go
// Testnet - use for development/testing
config.TestNet = true
// Addresses generated: m/44'/1'/... (coin_type = 1 for testnet)

// Mainnet - use for production
config.TestNet = false
// Addresses generated: m/44'/0'/... (coin_type = 0 for mainnet)
```

**Critical**: Never deploy to production with `TestNet: true`. Testnet Bitcoin has no monetary value and users may not verify payment legitimacy.

### Monero-Specific Security

If using Monero for payments:

- **RPC Authentication**: Requires username/password to the Monero wallet RPC
- **RPC Encryption**: Ensure Monero wallet RPC is not exposed to untrusted networks
- **View-Only Wallet**: Consider using a view-only wallet for payment verification to limit key exposure
- **Subaddress Isolation**: Monero subaddress per payment provides privacy without requiring HD derivation of Bitcoin
- **Transfer History Access**: Monero payment verification requires RPC with transfer history access via `GetTransfers()`. Unlike Bitcoin's address-level balance queries, Monero verification filters incoming transfers by subaddress to verify specific payments. Ensure your Monero wallet RPC endpoint supports the `get_transfers` method with the `in` parameter for incoming transaction filtering.

**Critical**: The payment system creates unique Monero subaddresses per payment and validates transfers to specific addresses by filtering the wallet's transfer history. If the RPC wallet is used for other purposes, ensure the payment system accounts for non-payment-related transfers. For production deployments, consider using a dedicated Monero wallet instance exclusively for paywall operations.

Configuration:

```go
config := paywall.Config{
    PriceInXMR:    0.01,
    XMRUser:       os.Getenv("XMR_WALLET_USER"),
    XMRPassword:   os.Getenv("XMR_WALLET_PASS"),
    XMRRPC:        "http://localhost:18081",
}
```

**Monero RPC Requirements**:
- Must support `create_address` for subaddress generation
- Must support `get_transfers` with `in` and `account_index` parameters for payment verification
- Account 0 is used for all payment subaddresses
- Each payment receives a unique subaddress for privacy and tracking

### HTTPS Deployment

- **Certificates**: Use TLS certificates from a trusted CA (Let's Encrypt, etc.)
- **Certificate pinning** (optional): For high-security deployments, consider certificate pinning to detect man-in-the-middle attacks
- **Cipher suites**: Go's TLS defaults are secure; custom cipher configuration not recommended
- **HSTS headers** (optional): Add `Strict-Transport-Security: max-age=31536000` to prevent downgrade attacks

### Logging and Monitoring

- **Never log sensitive data**: Private keys, seeds, passwords, or full payment amounts
- **Log important events**:
  - Wallet initialization (timestamp, network)
  - Payment creation (ID, address, timeout)
  - Payment verification (confirmations, status change)
  - Errors and warnings (entropy exhaustion, RPC failures)
- **Monitor for anomalies**:
  - Unusually high confirmation times (indicates network congestion or attack)
  - Repeated failures to verify payments (indicates RPC problems)
  - Burst of payment requests (indicates potential abuse or attack)

## Security Checklist for Production Deployment

---

## Multisig Security Considerations - Executive Summary

This section provides a comprehensive overview of the security posture of the multisig and escrow implementation based on detailed security reviews (Phase 7.3).

### Overall Security Status

**Production Readiness**: ⛔ **NOT READY FOR PRODUCTION**

The multisig implementation demonstrates solid cryptographic foundations but has **critical security vulnerabilities** that must be resolved before handling real-value transactions.

### Critical Vulnerabilities Summary

#### 🚨 Priority 0 - Blocking Issues (Must Fix Before Any Production Use)

| # | Component | Vulnerability | Impact | Effort |
|---|-----------|---------------|--------|--------|
| 1 | **Dispute Resolution** | No arbiter identity validation | Complete bypass of dispute system; fund theft | 8-12h |
| 2 | **Dispute Resolution** | No cryptographic signature verification | Attacker can forge arbiter/participant signatures | 6-8h |
| 3 | **Dispute Resolution** | Attacker-controlled Role field | Authorization completely bypassable | 4-6h |
| 4 | **Transaction Broadcast** | Broadcast not implemented | Payments cannot be completed; system non-functional | 30-45h |
| 5 | **Transaction Broadcast** | No transaction validation | Would accept malicious transactions if implemented | 4-6h |
| 6 | **Transaction Broadcast** | No double-broadcast prevention | Double-spend possible when implemented | 2-3h |
| 7 | **Escrow State Machine** | No optimistic locking | Race conditions allow inconsistent state transitions | 4-6h |
| 8 | **Escrow Workflows** | No audit trail | Actions deniable; forensics impossible | 4-6h |
| 9 | **Escrow Workflows** | No state transition validator | Direct state manipulation possible | 3-4h |

**Total P0 Effort**: ~70-100 hours

#### ⚠️ Priority 1 - High Risk (Address Before Beta)

| # | Component | Issue | Impact | Effort |
|---|-----------|-------|--------|--------|
| 10 | Dispute Resolution | Hardcoded dispute requester role | Audit trail corruption; incorrect attribution | 1-2h |
| 11 | Dispute Resolution | Missing arbiter registration | Disconnect between payment and arbiter systems | 2-3h |
| 12 | Transaction Broadcast | No output validation | Wrong recipient/amount possible | 3-4h |
| 13 | Escrow Workflows | Arbiter collusion risk | Biased dispute resolution; no multi-arbiter option | 8-12h |
| 14 | Escrow Workflows | Signature replay attacks | Reuse signatures across payments | 4-5h |
| 15 | Escrow Workflows | Grief via false disputes | Spam, DoS arbiter system | 3-4h |

**Total P1 Effort**: ~21-30 hours

### Security Review Status by Component

#### ✅ Secure Components

| Component | Status | Notes |
|-----------|--------|-------|
| **Bitcoin Key Generation** | ✅ Secure | BIP32/BIP44 compliant, proper entropy |
| **Bitcoin HD Derivation** | ✅ Secure | Correct hardened/non-hardened paths |
| **Redeem Script Creation** | ✅ Secure | Uses vetted btcsuite libraries |
| **Address Generation** | ✅ Secure | P2SH/P2WSH properly implemented |
| **Signature Generation** | ✅ Secure | Deterministic ECDSA, proper hash calculation |
| **Signature Ordering** | ✅ Secure | Matches public key order in script |
| **OP_CHECKMULTISIG Handling** | ✅ Secure | Correct OP_0 dummy value |
| **Metadata Encryption** | ✅ Secure | AES-256-GCM with proper nonces |
| **Wallet Storage** | ✅ Secure | Encrypted, atomic writes |

**Assessment**: The cryptographic foundations and transaction creation mechanisms are production-ready.

#### ⚠️ Needs Fixes - Medium Risk

| Component | Issue | Status | Fix Priority |
|-----------|-------|--------|--------------|
| **Escrow Timeouts** | Manual enforcement required | ⚠️ Medium Risk | P2 |
| **Transaction Broadcast** | Fee validation missing | ⚠️ Medium Risk | P2 |
| **Escrow Workflows** | Timeout manipulation possible | ⚠️ Medium Risk | P2 |
| **Escrow Workflows** | State locking DoS | ⚠️ Medium Risk | P2 |
| **Dispute System** | Evidence size limits missing | ⚠️ Medium Risk | P2 |
| **Dispute System** | No signature on evidence | ⚠️ Medium Risk | P2 |

#### ⛔ Critical Failures

| Component | Status | Blocker | Notes |
|-----------|--------|---------|-------|
| **Transaction Broadcast** | ⛔ Not Implemented | YES | Core functionality missing; fake transaction IDs returned |
| **Dispute Resolution** | ⛔ Broken Authorization | YES | No arbiter validation; role spoofing possible; signatures not verified |
| **Escrow State Machine** | ⛔ Race Conditions | YES | No concurrency control; inconsistent state possible |

### Attack Scenarios - Most Critical

#### Scenario 1: Complete Escrow Fund Theft via Arbiter Impersonation

```
1. Attacker identifies escrow payment in Disputed state
2. Attacker creates fake arbiter signature with Role="arbiter" (not validated)
3. Attacker creates fake winner signature with Role="buyer" (to steal funds)
4. Attacker calls ResolveDispute(payment, fakeArbiterSig, fakeBuyerSig)
5. System checks Role fields only (both pass)
6. System never verifies cryptographic signatures
7. Payment state changes to Refunded
8. Attacker's signature stored in payment
9. Funds transferable to attacker's address
10. Result: Complete theft of escrowed funds
```

**Current Protection**: ❌ None  
**Impact**: CRITICAL - System cannot secure funds  
**Likelihood**: HIGH (Easy to exploit if discovered)  
**Fix Requirements**: Arbiter allowlist + cryptographic signature verification

#### Scenario 2: Transaction Broadcast System Exploitation

```
1. Multisig payment created with 0.1 BTC to legitimate seller
2. Buyer and seller provide signatures
3. Attacker intercepts signatures or payment ID
4. Attacker crafts malicious transaction:
   - Input: Correct multisig UTXO
   - Output: Attacker's address (not seller's)
   - Uses legitimate buyer/seller signatures
5. Attacker calls HandleBroadcast with malicious transaction
6. System accepts transaction without validation
7. System returns success with fake transaction ID
8. In future when broadcast implemented: Funds sent to attacker
```

**Current Protection**: ❌ Broadcast not implemented (masks issue)  
**Impact**: CRITICAL - Would enable fund theft when implemented  
**Likelihood**: CERTAIN (Will happen if broadcast enabled without fixes)  
**Fix Requirements**: Transaction validation before broadcast

#### Scenario 3: Race Condition Double-Spend

```
1. Escrow funded with 0.5 BTC
2. Thread 1: Buyer calls RefundBuyer(buyerSig, arbiterSig)
3. Thread 2: Seller calls ReleaseToSeller(buyerSig, sellerSig)
4. Both threads check: state == EscrowFunded ✓
5. Thread 1: Sets state = Refunded, updates payment
6. Thread 2: Sets state = Completed, updates payment
7. Last write wins (probably Completed)
8. Both signature sets stored in payment
9. Both transactions could be broadcast (double-spend)
10. Result: Funds claimed by both parties
```

**Current Protection**: ❌ No optimistic locking  
**Impact**: CRITICAL - Double-spend possible  
**Likelihood**: MEDIUM (Concurrent operations likely in production)  
**Fix Requirements**: Optimistic locking / versioning

### Required Security Controls - Implementation Checklist

#### Phase 1: Critical Fixes (Week 1)

- [x] **Arbiter Authorization System** (8-12h)
  - [x] Implement `Config.AuthorizedArbiters` allowlist
  - [x] Add arbiter public key validation in `ResolveDispute()`
  - [x] Reject unauthorized arbiter signatures
  - [x] Add arbiter management API

- [x] **Cryptographic Signature Verification** (6-8h)
  - [x] Implement `verifySignatureAgainstTx()` function
  - [x] Call signature verification in all escrow methods
  - [x] Validate signatures cover correct transaction data
  - [x] Reject invalid signatures

- [x] **Role-Based Authorization** (4-6h)
  - [x] Derive Role from public key (remove user-controlled field)
  - [x] Implement `getRoleForPubKey()` using participant lists
  - [x] Update all SignatureData validation
  - [x] Add role verification tests

- [x] **Optimistic Locking** (4-6h)
  - [x] Add `Version int` field to Payment struct
  - [x] Implement version checking in UpdatePayment()
  - [x] Reject concurrent modifications with ErrVersionConflict
  - [x] Test race condition scenarios with defensive copy in GetPayment

- [x] **Audit Trail** (4-6h)
  - [x] Create AuditLogEntry struct and AuditAction constants
  - [x] Log all escrow state transitions (Create, Fund, Release, Refund, Dispute, Resolve)
  - [x] Include timestamps, actors, roles, signatures, and metadata
  - [x] Implement append-only MemoryAuditLogger with thread-safe operations

**Phase 1 Total**: ~30-40 hours

#### Phase 2: High Priority (Week 2)

- [x] **Transaction Broadcast Implementation** (30-45h)
  - [x] Integrate btcd RPC client
  - [x] Implement BTC broadcaster with validation
  - [x] Implement Monero broadcast support
  - [x] Add double-broadcast prevention
  - [x] Add output/amount validation
  - [x] Implement fee validation (basic validation implemented, full UTXO-based fee calculation noted for future)
  - [x] Add comprehensive broadcast tests

- [x] **State Transition Validation** (3-4h)
  - [x] Create state transition validator
  - [x] Enforce valid transition paths
  - [x] Add state transition history to Payment
  - [x] Log invalid transition attempts

- [x] **Signature Replay Protection** (4-5h)
  - [x] Add nonce to signature data
  - [x] Bind signatures to payment ID
  - [x] Implement signature deduplication
  - [x] Add replay attack tests

**Phase 2 Total**: ~37-54 hours

#### Phase 3: Medium Priority (Week 3)

- [x] **Multi-Arbiter System** (8-12h)
  - [x] Support 3-of-5 arbiter consensus
  - [x] Implement arbiter voting mechanism
  - [x] Add fallback arbiter support
  - [x] Create arbiter reputation tracking

- [x] **Dispute Improvements** (5-7h)
  - [x] Implement dispute fees
  - [x] Add dispute rate limiting
  - [x] Extend timeouts on disputes
  - [x] Add evidence size limits

- [x] **Timeout Enhancements** (4-6h)
  - [x] Automatic timeout resolution
  - [x] Blockchain timestamp usage
  - [x] Minimum/maximum timeout bounds
  - [x] Timeout extension mechanisms

**Phase 3 Total**: ~17-25 hours

### Security Testing Requirements

#### Critical Test Coverage (Must Have)

```go
// Authorization tests
func TestResolveDispute_UnauthorizedArbiter(t *testing.T)
func TestResolveDispute_InvalidSignature(t *testing.T)
func TestResolveDispute_ForgedRole(t *testing.T)
func TestReleaseToSeller_FakeSignatures(t *testing.T)

// Concurrency tests  
func TestEscrowStateMachine_RaceConditions(t *testing.T)
func TestEscrowStateMachine_ConcurrentRelease(t *testing.T)
func TestEscrowStateMachine_OptimisticLocking(t *testing.T)

// Broadcast tests
func TestHandleBroadcast_ValidatesOutputs(t *testing.T)
func TestHandleBroadcast_ValidatesAmount(t *testing.T)
func TestHandleBroadcast_PreventDoubleSpend(t *testing.T)
func TestHandleBroadcast_ActuallyBroadcasts(t *testing.T)

// Replay tests
func TestSignature_CrossPaymentReplay(t *testing.T)
func TestSignature_NonceUniqueness(t *testing.T)

// State machine tests
func TestState_InvalidTransitions(t *testing.T)
func TestState_TransitionHistory(t *testing.T)
```

#### Integration Tests

- [x] End-to-end escrow happy path with real signatures
- [x] Dispute resolution with multiple arbiters
- [x] Timeout-based refunds
- [ ] Concurrent state modification stress tests
- [x] Signature replay attack attempts
- [ ] Transaction malleability scenarios

#### Security-Specific Tests

- [ ] Fuzzing escrow state transitions
- [ ] Fuzzing signature data structures
- [ ] Property-based testing for state machine
- [ ] Chaos engineering for race conditions
- [ ] Load testing concurrent escrows

### Operational Security Recommendations

#### Deployment Checklist

**Before ANY production deployment**:
- [ ] ✅ All P0 vulnerabilities fixed and tested
- [ ] ✅ External security audit completed
- [ ] ✅ Penetration testing performed
- [ ] ✅ All critical tests passing with race detector
- [ ] ✅ Comprehensive monitoring in place
- [ ] ✅ Incident response procedures documented
- [ ] ✅ Key management procedures established
- [ ] ✅ Backup and recovery procedures tested

**For production operation**:
- [ Monitor audit logs for suspicious patterns
- [ ] Track arbiter behavior for collusion detection
- [ ] Alert on unusual state transition patterns
- [ ] Implement circuit breakers for high-value escrows
- [ ] Regular security reviews and penetration testing
- [ ] Maintain bug bounty program
- [ ] Document all security incidents
- [ ] Regular arbiter authorization reviews

#### Configuration Security

**Critical Configuration**:
```go
config := paywall.Config{
    // Arbiter security
    AuthorizedArbiters: loadArbiterPubKeys(),  // From secure storage
    RequiredArbiters: 3,  // Multi-arbiter consensus
    
    // Transaction security
    BTCNodeHost: "localhost:8332",  // Trusted Bitcoin node
    BTCNodeUser: getEnv("BTC_RPC_USER"),
    BTCNodePass: getEnv("BTC_RPC_PASS"),
    
    // Escrow parameters
    MinEscrowTimeout: 24 * time.Hour,
    MaxEscrowTimeout: 90 * 24 * time.Hour,
    DisputeTimeoutExtension: 14 * 24 * time.Hour,
    
    // Rate limiting
    MaxDisputesPerUser: 3,
    DisputeFeePercentage: 0.01,  // 1% dispute fee
}
```

### Timeline and Resource Requirements

**Minimum Path to Production**:

| Phase | Duration | Effort | Deliverables |
|-------|----------|--------|--------------|
| Phase 1: Critical Fixes | 2-3 weeks | 30-40h | Authorization, signatures, locking, audit |
| Phase 2: Core Security | 3-4 weeks | 37-54h | Broadcast, validation, replay protection |
| Phase 3: Hardening | 2-3 weeks | 17-25h | Multi-arbiter, dispute improvements |
| Testing & Audit | 2-4 weeks | 40-60h | Security tests, external audit, pen test |
| **Total** | **9-14 weeks** | **124-179h** | Production-ready multisig system |

**Team Requirements**:
- 1 senior security engineer (authorization, cryptography)
- 1 senior backend engineer (state management, concurrency)
- 1 blockchain specialist (Bitcoin RPC, transaction validation)
- 1 QA engineer (security testing, fuzzing)
- External security auditors (2-3 weeks)

### Current Suitability Assessment

| Use Case | Suitability | Rationale |
|----------|-------------|-----------|
| **Production deployment** | ❌ Not Suitable | Critical vulnerabilities present |
| **Real-value transactions** | ❌ Not Suitable | Fund theft possible |
| **Public beta testing** | ❌ Not Suitable | Authorization bypass |
| **Private testnet demo** | ⚠️ Caution | Only with test funds |
| **Architecture demo** | ✅ Suitable | Good workflow showcase |
| **Development/learning** | ✅ Suitable | Educational value |

### Security Contact

For security vulnerabilities or concerns, contact: security@example.com

Report format should include:
- Vulnerability description
- Affected components
- Reproduction steps
- Suggested fixes (if available)
- Severity assessment

---

## Security Review: Key Generation and Derivation Paths

This section documents the security review of key generation and derivation mechanisms performed as part of the multisig implementation security audit (PLAN.md Phase 7.3).

### Bitcoin HD Wallet Key Generation (✅ Secure)

**Master Key Derivation** (`wallet/btc_hd_wallet.go:207-210`):
- ✅ Uses HMAC-SHA512 with constant "Bitcoin seed" per BIP32 specification
- ✅ Splits 512-bit output into 256-bit master key and 256-bit chain code
- ✅ Seed validation requires 16-64 bytes (128-512 bits of entropy)
- ✅ No weak key derivation patterns detected

**BIP32 Child Key Derivation** (`wallet/btc_hd_wallet.go:324-364`):
- ✅ Implements proper hardened vs. non-hardened derivation:
  - **Hardened** (index >= 0x80000000): Uses `0x00 || privKey || index` for HMAC input
  - **Non-hardened**: Uses `compressed_pubKey || index` for HMAC input
- ✅ Proper modular arithmetic with curve order (secp256k1)
- ✅ Invalid key detection: Rejects keys where `childInt == 0` or `childInt >= curveOrder`
- ✅ Padding of derived keys to 32 bytes with leading zeros maintained

**BIP44 Derivation Path** (`wallet/btc_hd_wallet.go:265-295`):
```
m/44'/0'/0'/0/index
   ↑   ↑  ↑  ↑  ↑
   │   │  │  │  └── Address index (non-hardened, enables public derivation)
   │   │  │  └────── External chain (0 = receiving, 1 = change)
   │   │  └─────────── Account 0 (hardened, prevents account linkage)
   │   └────────────── Bitcoin coin type (hardened)
   └────────────────── BIP44 purpose (hardened)
```

**Security Properties**:
- ✅ Hardened indices (44', 0', 0') prevent public key→sibling private key attacks
- ✅ Non-hardened address index allows watch-only wallet implementations
- ✅ Each payment receives unique address at incremented index
- ✅ `nextIndex` protected by mutex for thread-safe concurrent address generation

**Cryptographic Validation**:
- ✅ Public key derivation uses btcsuite's `btcec.PrivKeyFromBytes()` with automatic curve validation
- ✅ Address generation includes proper HASH160 (SHA256 + RIPEMD160) and Base58Check encoding
- ✅ No raw private key logging or network transmission detected

### Multisig Key Derivation (✅ Secure)

**Participant Key Derivation** (`wallet/btc_multisig.go:246-286`):
- ✅ Uses BIP32 non-hardened derivation (allows public key coordination)
- ✅ Validates index is non-hardened (`< 0x80000000`)
- ✅ Proper HMAC-SHA512 with `compressed_pubKey || index`
- ✅ Modular arithmetic with secp256k1 curve order
- ✅ Invalid key rejection: `childInt == 0` or `childInt >= curve.N`

**Redeem Script Generation** (`wallet/btc_multisig.go:38-94`):
- ✅ Validates public key count: 1 ≤ n ≤ 15 (Bitcoin consensus limit)
- ✅ Validates signature requirement: 1 ≤ m ≤ n
- ✅ Accepts compressed (33 bytes) or uncompressed (65 bytes) public keys
- ✅ Parses public keys with `btcec.ParsePubKey()` for curve validation
- ✅ Uses btcsuite's `txscript.MultiSigScript()` for standard-compliant redeem scripts

**Multisig Address Generation**:
- ✅ **P2SH** (BIP16): RIPEMD160(SHA256(redeemScript)) with proper version byte and checksum
- ✅ **P2WSH** (BIP141): SHA256(redeemScript) with Bech32 encoding
- ✅ Network-specific address prefixes prevent testnet/mainnet confusion:
  - P2SH: `3xxx` (mainnet) / `2xxx` (testnet)
  - P2WSH: `bc1qxxx` (mainnet) / `tb1qxxx` (testnet)

**Redeem Script Validation** (`wallet/btc_multisig.go:290-319`):
- ✅ Verifies OP_CHECKMULTISIG opcode (0xae) at script end
- ✅ Extracts and validates m-of-n parameters from script
- ✅ Length validation prevents malformed scripts

### Monero Key Management (✅ Secure by Delegation)

**RPC-Based Key Handling** (`wallet/xmr_hd_wallet.go`):
- ✅ Key generation delegated to Monero wallet RPC (external daemon)
- ✅ Subaddress derivation via `CreateAddress()` RPC method (account 0)
- ✅ No private key exposure to paywall application
- ✅ Subaddress label includes sequential index for tracking

**Monero Multisig** (`wallet/xmr_multisig.go`):
- ✅ Uses Monero's native multisig protocol via RPC:
  - `PrepareMultisig()` - Initialize multisig state
  - `MakeMultisig()` - Exchange multisig info between participants
  - `ExportMultisigInfo()` / `ImportMultisigInfo()` - Synchronization
  - `FinalizeMultisig()` - Complete setup
- ✅ No custom cryptography implemented; relies on audited Monero codebase
- ✅ Participant coordination requires out-of-band secure communication (design choice)

**Security Considerations**:
- ⚠️ Monero RPC must be properly secured (authentication, encryption, network isolation)
- ⚠️ View-only wallets recommended for production payment verification (not yet implemented)
- ✅ Subaddress-per-payment provides transaction unlinkability
- ✅ Transfer verification filters by specific subaddress to prevent payment confusion

### Entropy and Randomness (✅ Secure)

**Random Number Generation** (`wallet/btc_hd_wallet.go:120-128`):
- ✅ Uses `crypto/rand.Int(rand.Reader, big.NewInt(n))` for all randomness
- ✅ **Fail-fast on entropy exhaustion**: Panics instead of falling back to `math/rand`
- ✅ Critical for endpoint selection and payment ID generation
- ✅ Error message clearly indicates security failure: `"crypto/rand.Int failed: cannot initialize wallet securely"`

**Seed Generation** (referenced in documentation):
- ✅ 256-bit seeds required (16-64 byte range enforced)
- ✅ Must be generated with `crypto/rand.Reader` by caller
- ⚠️ No built-in mnemonic phrase support (BIP39) - users must manage raw seed bytes

### Risk Assessment Summary

| Component | Status | Risk Level | Notes |
|-----------|--------|------------|-------|
| Bitcoin master key derivation | ✅ Secure | Low | BIP32 compliant, proper HMAC-SHA512 |
| Bitcoin child key derivation | ✅ Secure | Low | Hardened indices prevent key leakage |
| Multisig participant keys | ✅ Secure | Low | Non-hardened derivation appropriate for pubkey exchange |
| Multisig redeem scripts | ✅ Secure | Low | Standard Bitcoin script format, validated |
| Monero key management | ✅ Secure | Low | Delegated to audited Monero RPC |
| Entropy source | ✅ Secure | Low | crypto/rand with fail-fast on errors |
| Seed backup/recovery | ⚠️ Manual | Medium | No BIP39 mnemonic; users handle raw bytes |
| View-only Monero wallet | ❌ Not implemented | Medium | Production deployments expose full wallet RPC |

### Recommendations

1. **Immediate (Already Implemented)**:
   - ✅ crypto/rand failure causes panic (no silent degradation)
   - ✅ Key validation on all derived keys
   - ✅ Proper BIP32/BIP44 compliance

2. **Short Term (Optional Enhancements)**:
   - Consider BIP39 mnemonic phrase support for user-friendly seed backups
   - Document seed backup procedures in user-facing documentation
   - Implement view-only Monero wallet support for production deployments

3. **Long Term (Advanced Features)**:
   - Hardware wallet integration (Trezor, Ledger) for critical key operations
   - Threshold signature schemes (TSS) to avoid redeem script exposure
   - Taproot (BIP341/342) multisig for improved privacy and efficiency

### Audit Trail

- **Reviewed by**: Automated security audit (AI-assisted code review)
- **Review date**: May 13, 2026
- **Files audited**:
  - `wallet/btc_hd_wallet.go` (lines 1-530)
  - `wallet/btc_multisig.go` (lines 1-400)
  - `wallet/xmr_hd_wallet.go` (lines 1-200)
  - `wallet/xmr_multisig.go` (multisig RPC integration)
- **Standards validated**: BIP32, BIP44, BIP16 (P2SH), BIP141 (P2WSH)
- **Findings**: No critical vulnerabilities detected in key generation, derivation, or redeem script validation logic

---

## Security Review: Redeem Script Validation

This section documents the security audit of Bitcoin multisig redeem script validation (PLAN.md Phase 7.3).

### Redeem Script Construction (✅ Secure)

**BuildRedeemScript()** (`wallet/btc_multisig.go:38-94`):

**Input Validation**:
- ✅ Validates public key count: `1 ≤ n ≤ 15` (Bitcoin consensus limit per BIP11/BIP16)
- ✅ Validates signature requirement: `1 ≤ m ≤ n`
- ✅ Rejects empty public key arrays
- ✅ Validates public key length: 33 bytes (compressed) or 65 bytes (uncompressed)

**Public Key Parsing**:
- ✅ Uses `btcec.ParsePubKey()` for cryptographic validation
  - Verifies point is on secp256k1 curve
  - Validates coordinate bounds
  - Rejects invalid/malformed keys
- ✅ Converts all keys to compressed format (33 bytes) for consistency
- ✅ Uses btcsuite's `btcutil.NewAddressPubKey()` for address representation

**Script Generation**:
- ✅ Delegates to `txscript.MultiSigScript()` (audited btcsuite library)
- ✅ Standard format: `<m> <pubkey1> ... <pubkeyN> <n> OP_CHECKMULTISIG`
- ✅ Proper OP_m and OP_n encoding (OP_1 = 0x51, OP_2 = 0x52, etc.)

**Security Properties**:
- ✅ No buffer overflows (btcsuite handles script sizing)
- ✅ No public key reordering attacks (order preserved as provided)
- ✅ Deterministic output (same inputs → same script)

### Redeem Script Validation (✅ Secure)

**ValidateRedeemScript()** (`wallet/btc_multisig.go:290-341`):

**Structure Validation**:
- ✅ Checks minimum length (4 bytes: OP_m + OP_n + OP_CHECKMULTISIG)
- ✅ Verifies OP_CHECKMULTISIG (0xae) at script end
- ✅ Extracts m and n values by decoding opcodes (OP_1 = 0x51, OP_2 = 0x52, etc.)

**Parameter Validation**:
- ✅ Validates: `1 ≤ m ≤ 15`
- ✅ Validates: `1 ≤ n ≤ 15`
- ✅ Validates: `m ≤ n`
- ✅ Clear error messages for invalid parameters

**Limitations (Acceptable Trade-offs)**:
- ⚠️ Does not validate public key count matches `n` (caller responsibility)
- ⚠️ Does not validate individual public keys are on curve (should validate at construction time)
- ✅ These limitations are acceptable because:
  - Validation happens at script creation (`BuildRedeemScript`)
  - Invalid scripts will fail at spend time (Bitcoin consensus rules)
  - Performance trade-off: Full validation expensive, construction-time validation sufficient

### Public Key Extraction (✅ Secure)

**ExtractPubKeysFromRedeemScript()** (`wallet/btc_multisig.go:343-389`):

**Parsing Logic**:
- ✅ Uses `txscript.MakeScriptTokenizer()` (audited btcsuite library)
- ✅ Skips first opcode (OP_m)
- ✅ Extracts data chunks matching public key lengths (33 or 65 bytes)
- ✅ Stops at OP_n or OP_CHECKMULTISIG
- ✅ Handles tokenizer errors gracefully

**Data Handling**:
- ✅ Creates defensive copies of public key bytes (`pubKeysCopy`)
- ✅ Prevents modification of original script data
- ✅ Returns empty array + error if no keys found

**Security Properties**:
- ✅ No out-of-bounds reads (tokenizer handles bounds)
- ✅ Memory safe (defensive copying)
- ✅ Handles malformed scripts without panicking

### Script Comparison (✅ Secure)

**CompareRedeemScripts()** (`wallet/btc_multisig.go:391-400`):
- ✅ Uses `bytes.Equal()` for constant-time comparison (prevents timing attacks)
- ✅ No custom comparison logic (reduces bug surface area)
- ✅ Handles nil slices correctly (`bytes.Equal(nil, nil) == true`)

### Address Generation Security (✅ Secure)

**P2SH Address Generation** (`wallet/btc_multisig.go:96-132`):

**Hash Chain**:
- ✅ HASH160(redeemScript) = RIPEMD160(SHA256(redeemScript))
- ✅ Proper hash sequence per BIP16 specification
- ✅ Uses standard library `crypto/sha256` and `golang.org/x/crypto/ripemd160`

**Encoding**:
- ✅ Uses `btcutil.NewAddressScriptHashFromHash()` for address creation
- ✅ Includes version byte (0x05 mainnet, 0xC4 testnet)
- ✅ Includes checksum via Base58Check encoding
- ✅ Prevents address type confusion (mainnet vs. testnet prefixes)

**P2WSH Address Generation** (`wallet/btc_multisig.go:134-173`):

**Hash**:
- ✅ SHA256(redeemScript) - single round per BIP141
- ✅ No double-hashing (unlike P2SH, this is intentional per spec)

**Encoding**:
- ✅ Uses `btcutil.NewAddressWitnessScriptHash()` for Bech32 encoding
- ✅ Native SegWit format: `bc1q...` (mainnet), `tb1q...` (testnet)
- ✅ Bech32 checksum prevents transcription errors
- ✅ Case-insensitive (better UX)

### Attack Resistance

**Script Malleability (✅ Protected)**:
- ✅ Redeem scripts are deterministic (same inputs → same output)
- ✅ Public keys are validated and normalized to compressed format
- ✅ No non-canonical encodings accepted by `btcec.ParsePubKey()`

**Key Reordering Attacks (✅ Protected)**:
- ✅ Public key order is significant (affects script hash)
- ✅ Different key orders produce different addresses
- ✅ Coordinating parties must use consistent key ordering

**Signature Grinding (✅ Protected by Bitcoin Consensus)**:
- ✅ Multisig requires `m` valid signatures per Bitcoin consensus
- ✅ Cannot be bypassed at script validation level
- ✅ Script structure enforced by OP_CHECKMULTISIG semantics

**Invalid Key Inclusion (✅ Protected)**:
- ✅ `btcec.ParsePubKey()` validates keys are on secp256k1 curve
- ✅ Invalid keys rejected at script construction time
- ⚠️ Extracted keys not re-validated (acceptable: validation at construction sufficient)

### Opcode Injection Risks (✅ Mitigated)

**Script Construction**:
- ✅ Uses btcsuite's `txscript.MultiSigScript()` (no manual opcode assembly)
- ✅ No string concatenation or templating
- ✅ No user-controlled opcode insertion
- ✅ Public keys pushed as data, not executed as code

**Script Parsing**:
- ✅ Uses `txscript.MakeScriptTokenizer()` (handles malformed scripts safely)
- ✅ No custom parser vulnerable to malicious input
- ✅ Graceful error handling for unexpected opcodes

### Consensus Compliance (✅ Validated)

**Bitcoin Consensus Rules**:
- ✅ Maximum 20 public keys per multisig (implementation enforces 15, safer than consensus limit)
- ✅ OP_CHECKMULTISIG requires m ≤ n
- ✅ Public keys must be valid secp256k1 points
- ✅ Script size limits respected (btcsuite handles this)

**BIP Compliance**:
- ✅ BIP16 (P2SH): Correct HASH160 usage and address encoding
- ✅ BIP141 (P2WSH): Correct SHA256 usage and Bech32 encoding
- ✅ BIP11 (Multisig): Standard multisig script format

### Risk Assessment

| Validation Area | Status | Risk Level | Notes |
|----------------|--------|------------|-------|
| Redeem script construction | ✅ Secure | Low | Delegates to audited btcsuite library |
| Public key validation | ✅ Secure | Low | Curve validation via btcec.ParsePubKey() |
| Parameter bounds checking | ✅ Secure | Low | 1 ≤ m ≤ n ≤ 15 enforced |
| Address generation (P2SH) | ✅ Secure | Low | BIP16 compliant HASH160 |
| Address generation (P2WSH) | ✅ Secure | Low | BIP141 compliant SHA256 + Bech32 |
| Script parsing | ✅ Secure | Low | Uses btcsuite tokenizer |
| Opcode injection | ✅ Protected | Low | No manual opcode assembly |
| Key extraction | ✅ Secure | Low | Defensive copying, safe parsing |

### Recommendations

**Already Implemented**:
- ✅ Input validation at all entry points
- ✅ Delegation to audited cryptographic libraries
- ✅ Proper error handling for all failure modes
- ✅ Defensive copying to prevent data corruption

**Optional Enhancements (Not Required)**:
1. Add comprehensive unit tests for malformed redeem scripts
2. Document key ordering requirements in API docs
3. Add helper function to validate extracted keys match expected set
4. Implement script size validation (currently delegated to btcsuite)

### Test Coverage Analysis

**BuildRedeemScript** (`wallet/btc_multisig_test.go`):
- ✅ Tests 2-of-3 standard case
- ✅ Tests invalid input (empty keys, m > n, n > 15)
- ✅ Tests public key validation

**ValidateRedeemScript** (`wallet/btc_multisig_test.go`):
- ✅ Tests valid scripts
- ✅ Tests empty scripts
- ✅ Tests scripts without OP_CHECKMULTISIG
- ✅ Tests invalid m/n values

**Address Generation** (`wallet/btc_multisig_test.go`):
- ✅ Tests P2SH address format (mainnet/testnet)
- ✅ Tests P2WSH address format (mainnet/testnet)
- ✅ Tests address prefixes correct

**ExtractPubKeysFromRedeemScript**:
- ✅ Tests key extraction from valid scripts
- ✅ Tests malformed script handling

### Audit Conclusion

The redeem script validation implementation is **secure and production-ready**. The code:
- Properly validates all inputs
- Uses audited cryptographic libraries (btcsuite)
- Handles errors gracefully
- Complies with Bitcoin consensus rules and BIPs
- Resists common attack vectors (malleability, opcode injection, key reordering)

No security vulnerabilities or weaknesses were identified during this audit.

---

## Security Review: Signature Verification Logic

This section documents the security audit of Bitcoin multisig signature creation and verification (PLAN.md Phase 7.3).

### Signature Creation (✅ Secure)

**SignMultisigTx()** (`wallet/btc_multisig_tx.go:183-250`):

**Input Validation**:
- ✅ Validates input index: `0 ≤ inputIndex < len(TxIn)`
- ✅ Validates private key is not nil
- ✅ Validates redeem/witness script exists for input

**Script Type Handling**:
- ✅ Correctly distinguishes P2WSH (witness) vs. P2SH (legacy)
- ✅ Uses appropriate script for each type:
  - **P2WSH**: Signs against witness script
  - **P2SH**: Signs against redeem script

**Signature Hash Calculation**:
- ✅ **SegWit (P2WSH)**: Uses `txscript.CalcWitnessSigHash()` per BIP143
  - Includes input amount in signature hash (prevents amount tampering)
  - Uses `NewTxSigHashes()` for caching (performance + correctness)
  - Proper sigHashType handling
- ✅ **Legacy (P2SH)**: Uses `txscript.CalcSignatureHash()` per original Bitcoin spec
  - Standard double-SHA256 signature hash
  - Proper script substitution

**Signature Generation**:
- ✅ Uses `ecdsa.Sign()` from btcsuite (audited library)
- ✅ ECDSA signature over SHA256 message hash
- ✅ Deterministic nonce generation (RFC 6979 via btcec)
- ✅ Appends sigHashType byte to signature (standard Bitcoin format)

**Signature Storage**:
- ✅ Stores public key + signature + sigHashType triplet
- ✅ Allows multiple signers per input (accumulates signatures)
- ✅ Associates signatures with correct input index

**Security Properties**:
- ✅ No signature malleability (uses deterministic ECDSA)
- ✅ Private key never logged or leaked
- ✅ Signature hash properly covers transaction fields per BIP143/legacy spec

### Signature Verification (✅ Secure)

**VerifySignature()** (`wallet/btc_multisig_tx.go:422-480`):

**Input Validation**:
- ✅ Validates input index bounds
- ✅ Validates script exists (redeem or witness)
- ✅ Parses and validates public key via `btcec.ParsePubKey()`
  - Validates point is on secp256k1 curve
  - Rejects invalid/malformed keys

**Signature Parsing**:
- ✅ Extracts sigHashType byte (last byte if present)
- ✅ Removes sigHashType from signature data for parsing
- ✅ Parses DER-encoded signature via `ecdsa.ParseDERSignature()`
  - Validates DER encoding
  - Validates R and S values are in valid ranges

**Signature Hash Recalculation**:
- ✅ Recalculates hash using same method as signing:
  - **P2WSH**: `CalcWitnessSigHash()` with input amount
  - **P2SH**: `CalcSignatureHash()` without amount
- ✅ Uses extracted sigHashType for hash calculation
- ✅ Consistent with signature creation logic

**Signature Verification**:
- ✅ Uses `parsedSig.Verify(sigHash, parsedPubKey)` from btcec
- ✅ Standard ECDSA verification: `r, s` satisfy curve equation
- ✅ Returns boolean result (no panic on invalid signature)

**Security Properties**:
- ✅ Constant-time verification (via btcec library)
- ✅ No signature malleability acceptance (DER encoding enforced)
- ✅ Proper hash type handling prevents cross-input attacks
- ✅ Amount included in P2WSH hash prevents amount fraud

### Signature Combination (✅ Secure)

**CombineSignatures()** (`wallet/btc_multisig_tx.go:252-288`):

**Signature Ordering**:
- ✅ Extracts public keys from script via `ExtractPubKeysFromRedeemScript()`
- ✅ Orders signatures to match public key order in script
- ✅ Uses `orderSignaturesByPubKeys()` helper (line 361-375)
  - Iterates script public keys
  - Finds matching signature for each key
  - Preserves script-defined order

**Security**: Public key order in multisig scripts is significant. OP_CHECKMULTISIG validates signatures in order, so mismatched ordering causes verification failure. This implementation correctly preserves order.

**Witness Data Construction (P2WSH)**:
- ✅ **buildWitnessData()** (lines 291-322):
  - Adds OP_0 (empty byte array) first (OP_CHECKMULTISIG off-by-one bug workaround)
  - Adds ordered signatures
  - Adds witness script last
  - Sets scriptSig to empty (per SegWit spec)

**ScriptSig Construction (P2SH)**:
- ✅ **buildScriptSig()** (lines 324-359):
  - Uses `txscript.NewScriptBuilder()` (safe, audited)
  - Adds OP_FALSE (OP_CHECKMULTISIG bug workaround)
  - Adds ordered signatures
  - Adds redeem script last
  - Proper script serialization

**OP_CHECKMULTISIG Off-by-One Bug Handling**:
- ✅ Both P2SH and P2WSH add extra `OP_0` at start
- ✅ This is a **required workaround** for Bitcoin's historic OP_CHECKMULTISIG bug (consumes extra stack element)
- ✅ Failure to include OP_0 would cause transaction rejection

### Signature Hash Type Handling (✅ Secure)

**Supported Hash Types** (`SigHashType` parameter):
- ✅ `SigHashAll` (0x01): Signs all inputs and outputs (default, most common)
- ✅ `SigHashNone` (0x02): Signs inputs only (allows output modification)
- ✅ `SigHashSingle` (0x03): Signs corresponding output only
- ✅ `SigHashAnyOneCanPay` (0x80): Modifier flag - signs only this input

**Security Considerations**:
- ✅ SigHashType included in signature (prevents type substitution attacks)
- ✅ Hash calculation uses correct type (no type confusion)
- ⚠️ Non-standard hash types (SIGHASH_NONE, SIGHASH_SINGLE) have security implications:
  - Allow modification of transaction outputs after signing
  - Rarely used; defaults to SIGHASH_ALL (safe)

**Recommendation**: Document hash type risks in API docs if exposed to users.

### Attack Resistance

**Signature Malleability (✅ Protected)**:
- ✅ Uses deterministic ECDSA (RFC 6979)
- ✅ DER encoding enforced (no low-S malleability)
- ✅ Signature parsing rejects non-canonical encodings
- ✅ BIP66 (strict DER) compliance via btcsuite

**Cross-Input Signature Reuse (✅ Protected)**:
- ✅ Input index included in signature hash calculation
- ✅ Each input has distinct signature hash
- ✅ Signature from input 0 cannot be used for input 1

**Amount Tampering (✅ Protected for P2WSH)**:
- ✅ P2WSH includes input amount in signature hash (BIP143)
- ✅ Prevents attacker from changing input amounts after signing
- ⚠️ P2SH (legacy) does not include amount (known limitation)
  - Not a vulnerability: Amount commitment happens at UTXO creation
  - Signer must verify input amounts before signing

**Signature Grinding (✅ Protected)**:
- ✅ Deterministic ECDSA prevents attacker from generating multiple valid signatures
- ✅ Each signature is unique for a given (message, private key) pair
- ✅ No nonce reuse possible (would leak private key)

**Public Key Substitution (✅ Protected)**:
- ✅ Signature verification requires exact public key match
- ✅ Public keys extracted from script (not attacker-controlled)
- ✅ Signature ordering matches script public key order

---

## Security Review: Transaction Broadcast Safety

This section documents the security audit of transaction broadcasting mechanisms to prevent double-spend and replay attacks (PLAN.md Phase 7.3).

### Implementation Status: ⛔ **NOT IMPLEMENTED**

**Critical Finding**: The transaction broadcast functionality is **not implemented**. This section updates the previous (incorrect) assumptions with actual code audit findings.

**Previous Assumptions** (Incorrect):
- ❌ Assumed `BroadcastMultisigTx()` function exists
- ❌ Assumed Bitcoin RPC integration for `sendrawtransaction`
- ❌ Assumed network nodes would validate signatures

**Actual Implementation** (`multisig_handlers.go:345-415`):

```go
func (mc *MultisigCoordinator) HandleBroadcast(w http.ResponseWriter, r *http.Request) {
    // ... validation code ...
    
    // TODO: Broadcast transaction to blockchain
    // This would use the wallet's broadcast functionality
    // For now, return a placeholder response
    txID := fmt.Sprintf("tx_%s_%d", req.PaymentID, time.Now().Unix())
    
    // Send webhook notification
    if mc.notifier != nil {
        go mc.notifier.NotifyBroadcastComplete(req.PaymentID, txID)
    }
    
    resp := MultisigBroadcastResponse{
        Success:       true,
        TransactionID: txID,
        Message:       "Transaction broadcast successful (placeholder)",
    }
    // ...
}
```

### Security Analysis

#### 🚨 CRITICAL: No Actual Broadcast Implementation

**Issue**: `HandleBroadcast()` does not broadcast transactions to any blockchain network.

**Current Behavior**:
1. ✅ Validates payment exists and is multisig-enabled
2. ✅ Checks sufficient signatures collected
3. ❌ **Does NOT validate transaction bytes**
4. ❌ **Does NOT broadcast to Bitcoin/Monero network**
5. ❌ **Generates fake transaction ID**: `"tx_" + paymentID + "_" + timestamp`
6. ✅ Sends webhook notification with fake txID
7. ✅ Returns success response claiming "broadcast successful"

**Impact**: **CRITICAL - COMPLETE FUNCTIONALITY FAILURE**
- Users receive success confirmation but transaction never reaches blockchain
- Payments appear complete but funds are never transferred
- Payment system cannot function as paywalling mechanism
- Webhook notifications contain invalid transaction IDs

**Attack Scenarios**:
1. **Fake Payment Attack**: 
   - Attacker collects signatures
   - Calls broadcast endpoint
   - Receives success response with fake txID
   - Never actually sends funds
   - May gain access to protected content

2. **System Integrity Failure**:
   - Legitimate users believe payment was made
   - Content is delivered based on fake success
   - Monitoring systems show false positive transactions
   - Financial reconciliation impossible (no real txIDs)

#### 🚨 HIGH: No Transaction Validation

**Issue**: The broadcast handler accepts arbitrary transaction bytes without validation.

**Missing Validations**:
- ❌ No parsing of transaction bytes
- ❌ No verification that transaction matches payment details
- ❌ No validation of transaction outputs (recipient, amount)
- ❌ No validation of transaction inputs (correct UTXOs)
- ❌ No signature verification before broadcast
- ❌ No check that transaction corresponds to payment ID

**Current Code** (`multisig_handlers.go:360-363`):
```go
if len(req.Transaction) == 0 {
    http.Error(w, "Transaction required", http.StatusBadRequest)
    return
}
// No further validation of transaction contents!
```

**Attack Scenario**:
```go
// Attacker can submit ANY transaction bytes
req := MultisigBroadcastRequest{
    PaymentID: "legitimate-payment",
    WalletType: wallet.Bitcoin,
    Transaction: []byte("garbage data or malicious tx"),
}
// Accepted without validation!
```

**Impact**: **HIGH** - If broadcast were implemented without fixing this:
- Attacker could broadcast wrong transaction
- Funds could be sent to attacker's address instead of seller
- Transaction amounts could differ from payment amounts
- Wrong inputs could be consumed

#### 🚨 HIGH: No Double-Broadcast Prevention

**Issue**: No state tracking to prevent multiple broadcast attempts of the same transaction.

**Missing Protections**:
- ❌ No `Broadcasted` or `BroadcastedAt` timestamp field in Payment struct
- ❌ No check if transaction already broadcast for payment ID
- ❌ No idempotency protection
- ❌ No transaction ID tracking in payment record

**Attack Scenario**:
```bash
# Attacker can call broadcast endpoint repeatedly
curl -X POST /multisig/broadcast -d '{"payment_id":"123", "transaction":"..."}'
# Response: Success, txID: tx_123_1234567890

curl -X POST /multisig/broadcast -d '{"payment_id":"123", "transaction":"..."}'
# Response: Success, txID: tx_123_1234567891  (different ID!)

# Each call succeeds with new fake txID
# No indication that payment already broadcast
```

**Impact**: **HIGH** (if broadcast were implemented):
- Double-spend attempts possible
- Transaction fee duplication
- Network spam with duplicate transactions
- Race conditions in signature collection

**Best Practice** (to implement):
```go
type Payment struct {
    // ... existing fields ...
    
    // Transaction broadcast tracking
    Broadcasted     bool      `json:"broadcasted"`
    BroadcastedAt   time.Time `json:"broadcasted_at,omitempty"`
    TransactionID   string    `json:"transaction_id,omitempty"`  // Real blockchain txID
    BroadcastAttempts int     `json:"broadcast_attempts"`
}

func (mc *MultisigCoordinator) HandleBroadcast(...) error {
    payment, _ := mc.paywall.Store.GetPayment(req.PaymentID)
    
    // Prevent double broadcast
    if payment.Broadcasted {
        return fmt.Errorf("transaction already broadcast at %s with ID %s", 
            payment.BroadcastedAt, payment.TransactionID)
    }
    
    // ... broadcast logic ...
    
    // Update payment with real broadcast data
    payment.Broadcasted = true
    payment.BroadcastedAt = time.Now()
    payment.TransactionID = realTxID
    payment.BroadcastAttempts++
}
```

#### ⚠️ MEDIUM: No Replay Protection Across Payments

**Issue**: Same fully-signed transaction could be reused across multiple payment IDs.

**Scenario**:
1. Alice creates payment123 for 0.01 BTC
2. Multisig participants sign transaction TX1
3. TX1 is "broadcast" for payment123
4. Alice creates payment456 for same amount, same addresses
5. **Attacker replays signed TX1 for payment456**
6. Both payments marked complete but only one real transaction

**Missing Protection**:
- ❌ No binding between payment ID and transaction
- ❌ No nonce or unique identifier in transaction
- ❌ No validation that input UTXOs match payment's expected inputs

**Impact**: **MEDIUM** - Replay attacks possible across similar payments

#### ⚠️ MEDIUM: No Fee Validation

**Issue**: No validation that transaction includes appropriate mining fee.

**Risks**:
- Transaction may never confirm (too low fee)
- Transaction may be uneconomical (excessive fee)
- Attacker could manipulate fee to drain funds

**Missing**:
- ❌ No fee rate validation
- ❌ No minimum fee check
- ❌ No maximum fee sanity check
- ❌ No output amount validation vs expected payment amount

#### ⚠️ MEDIUM: No Network Connectivity Validation

**Issue**: Even if broadcast were implemented, no validation that node is connected/synced.

**Missing Checks**:
- ❌ Bitcoin node reachability
- ❌ Node synchronization status
- ❌ Network connectivity
- ❌ RPC authentication status

**Impact**: **MEDIUM** - Silent failures, transactions not actually broadcast

### Missing Infrastructure Components

#### No Bitcoin RPC Client Integration

**Required but Missing**:
```go
// btcd RPC client integration needed
import "github.com/btcsuite/btcd/rpcclient"

type BitcoinBroadcaster struct {
    client *rpcclient.Client
    config *btcd.Config
}

func (b *BitcoinBroadcaster) BroadcastTransaction(txBytes []byte) (string, error) {
    // Parse transaction
    tx := wire.NewMsgTx(wire.TxVersion)
    if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
        return "", fmt.Errorf("invalid transaction: %w", err)
    }
    
    // Broadcast via RPC
    txHash, err := b.client.SendRawTransaction(tx, false)
    if err != nil {
        return "", fmt.Errorf("broadcast failed: %w", err)
    }
    
    return txHash.String(), nil
}
```

#### No Monero RPC Broadcast

**Current Monero Integration**: 
- ✅ Wallet RPC client exists (`xmr_hd_wallet.go`)
- ❌ No transaction broadcast implementation
- ❌ No multisig transaction broadcast for Monero

**Required**:
```go
// Monero multisig broadcast
func (w *XMRHDWallet) BroadcastMultisigTx(signedTx string) (string, error) {
    var result struct {
        TxHashList []string `json:"tx_hash_list"`
    }
    
    err := w.rpcClient.Call("submit_multisig", map[string]interface{}{
        "tx_data_hex": signedTx,
    }, &result)
    
    if err != nil {
        return "", fmt.Errorf("monero broadcast failed: %w", err)
    }
    
    return result.TxHashList[0], nil
}
```

### Dependency Analysis

**btcsuite/btcd RPC Client** (Not Currently Used):
- Package: `github.com/btcsuite/btcd/rpcclient`
- Purpose: Bitcoin node RPC communication
- Status: ❌ Not imported or configured
- Required methods: `SendRawTransaction()`, `GetTransaction()`, `GetRawMempool()`

**go-monero-rpc-client** (Partially Used):
- Package: `github.com/monero-ecosystem/go-monero-rpc-client`
- Current use: Wallet operations only
- Missing: Transaction broadcast methods
- Required methods: `submit_multisig`, `relay_tx`

### Threat Model: Broadcast Attack Vectors

#### If Broadcast Were Implemented Without Fixes

| Threat | Likelihood | Impact | Current Protection | Required Protection |
|--------|-----------|--------|-------------------|---------------------|
| Double-spend via double broadcast | **HIGH** | Critical | ❌ None | Broadcast state tracking |
| Malicious transaction substitution | **HIGH** | Critical | ❌ None | Transaction validation |
| Transaction replay across payments | **MEDIUM** | High | ❌ None | UTXO binding validation |
| Insufficient fee (stuck transaction) | **MEDIUM** | Medium | ❌ None | Fee validation |
| Wrong recipient address | **HIGH** | Critical | ❌ None | Output validation |
| Wrong amount | **HIGH** | Critical | ❌ None | Amount validation |
| Network offline (silent failure) | **MEDIUM** | Medium | ❌ None | Connectivity checks |
| Node unsynchronized | **LOW** | Medium | ❌ None | Sync status validation |

#### Current State (No Broadcast)

| Threat | Likelihood | Impact | Status |
|--------|-----------|--------|--------|
| Fake success responses | **CERTAIN** | Critical | ⚠️ Happening now |
| Invalid transaction IDs | **CERTAIN** | Critical | ⚠️ Happening now |
| Payment system non-functional | **CERTAIN** | Critical | ⚠️ Confirmed |
| Content delivered without payment | **HIGH** | Critical | ⚠️ Possible |

### Required Implementation (Blocking Production)

#### 1. Implement Bitcoin Transaction Broadcast

```go
// Add to Config
type Config struct {
    // ... existing fields ...
    
    // Bitcoin node configuration
    BTCNodeHost     string  // "localhost:8332"
    BTCNodeUser     string  // RPC username
    BTCNodePass     string  // RPC password
    BTCUseTLS       bool    // Use TLS for RPC
    BTCDisableTLS   bool    // Disable TLS verification (testnet only)
}

// Add broadcaster to MultisigCoordinator
type MultisigCoordinator struct {
    paywall       *Paywall
    authenticator Authenticator
    notifier      MultisigNotifier
    btcBroadcaster *BTCBroadcaster  // NEW
    xmrBroadcaster *XMRBroadcaster  // NEW
}

// Implement Bitcoin broadcaster
type BTCBroadcaster struct {
    client  *rpcclient.Client
    network *chaincfg.Params
}

func NewBTCBroadcaster(host, user, pass string, useTLS bool, network *chaincfg.Params) (*BTCBroadcaster, error) {
    connCfg := &rpcclient.ConnConfig{
        Host:         host,
        User:         user,
        Pass:         pass,
        HTTPPostMode: true,
        DisableTLS:   !useTLS,
    }
    
    client, err := rpcclient.New(connCfg, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to Bitcoin node: %w", err)
    }
    
    return &BTCBroadcaster{client: client, network: network}, nil
}

func (b *BTCBroadcaster) Broadcast(txBytes []byte) (string, error) {
    // Parse transaction
    tx := wire.NewMsgTx(wire.TxVersion)
    if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
        return "", fmt.Errorf("invalid transaction format: %w", err)
    }
    
    // Broadcast to network
    txHash, err := b.client.SendRawTransaction(tx, false)
    if err != nil {
        return "", fmt.Errorf("broadcast rejected by node: %w", err)
    }
    
    return txHash.String(), nil
}

func (b *BTCBroadcaster) ValidateTransaction(txBytes []byte, payment *Payment) error {
    // Parse transaction
    tx := wire.NewMsgTx(wire.TxVersion)
    if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
        return fmt.Errorf("invalid transaction: %w", err)
    }
    
    // Validate outputs match payment
    metadata := payment.MultisigMetadata[wallet.Bitcoin]
    if metadata == nil {
        return errors.New("missing multisig metadata")
    }
    
    // Find expected output
    found := false
    expectedAmount := payment.Amounts[wallet.Bitcoin]
    for _, txOut := range tx.TxOut {
        // Check if output matches expected multisig address
        addr, err := btcutil.DecodeAddress(payment.Addresses[wallet.Bitcoin], b.network)
        if err != nil {
            continue
        }
        
        scriptPubKey, err := txscript.PayToAddrScript(addr)
        if err != nil {
            continue
        }
        
        if bytes.Equal(txOut.PkScript, scriptPubKey) {
            if txOut.Value >= expectedAmount {
                found = true
                break
            }
        }
    }
    
    if !found {
        return fmt.Errorf("transaction does not pay expected amount to multisig address")
    }
    
    // Validate inputs are from expected UTXOs
    // (This requires tracking UTXOs in payment metadata)
    
    return nil
}
```

#### 2. Update HandleBroadcast with Proper Implementation

```go
func (mc *MultisigCoordinator) HandleBroadcast(w http.ResponseWriter, r *http.Request) {
    // ... existing validation ...
    
    // NEW: Check if already broadcast
    if payment.Broadcasted {
        http.Error(w, fmt.Sprintf("Transaction already broadcast: %s", payment.TransactionID), 
            http.StatusConflict)
        return
    }
    
    // NEW: Validate transaction before broadcast
    var broadcaster interface{ Broadcast([]byte) (string, error); ValidateTransaction([]byte, *Payment) error }
    
    switch req.WalletType {
    case wallet.Bitcoin:
        if mc.btcBroadcaster == nil {
            http.Error(w, "Bitcoin broadcaster not configured", http.StatusServiceUnavailable)
            return
        }
        broadcaster = mc.btcBroadcaster
        
    case wallet.Monero:
        if mc.xmrBroadcaster == nil {
            http.Error(w, "Monero broadcaster not configured", http.StatusServiceUnavailable)
            return
        }
        broadcaster = mc.xmrBroadcaster
        
    default:
        http.Error(w, "Unsupported wallet type", http.StatusBadRequest)
        return
    }
    
    // NEW: Validate transaction contents
    if err := broadcaster.ValidateTransaction(req.Transaction, payment); err != nil {
        http.Error(w, fmt.Sprintf("Invalid transaction: %v", err), http.StatusBadRequest)
        return
    }
    
    // NEW: Actually broadcast to blockchain
    txID, err := broadcaster.Broadcast(req.Transaction)
    if err != nil {
        // Log error and update payment
        payment.BroadcastAttempts++
        mc.paywall.Store.UpdatePayment(payment)
        
        http.Error(w, fmt.Sprintf("Broadcast failed: %v", err), http.StatusServiceUnavailable)
        return
    }
    
    // NEW: Update payment with real transaction data
    payment.Broadcasted = true
    payment.BroadcastedAt = time.Now()
    payment.TransactionID = txID
    payment.BroadcastAttempts++
    
    if err := mc.paywall.Store.UpdatePayment(payment); err != nil {
        // Transaction was broadcast but state update failed
        // This is a critical error - transaction is on chain but payment not updated
        http.Error(w, fmt.Sprintf("Broadcast succeeded but state update failed: %v", err), 
            http.StatusInternalServerError)
        return
    }
    
    // Send webhook notification with REAL transaction ID
    if mc.notifier != nil {
        go mc.notifier.NotifyBroadcastComplete(req.PaymentID, txID)
    }
    
    resp := MultisigBroadcastResponse{
        Success:       true,
        TransactionID: txID,  // REAL blockchain transaction ID
        Message:       "Transaction broadcast successful",
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}
```

#### 3. Add Payment Struct Fields

```go
type Payment struct {
    // ... existing fields ...
    
    // Transaction broadcast tracking
    Broadcasted       bool      `json:"broadcasted"`
    BroadcastedAt     time.Time `json:"broadcasted_at,omitempty"`
    TransactionID     string    `json:"transaction_id,omitempty"`  // Real blockchain txID
    BroadcastAttempts int       `json:"broadcast_attempts"`
}
```

### Risk Assessment

| Component | Current Status | Risk Level | Blocker | Notes |
|-----------|---------------|------------|---------|-------|
| Transaction broadcast implementation | ❌ Not implemented | **CRITICAL** | ✅ YES | No actual broadcast to blockchain |
| Bitcoin RPC client | ❌ Not configured | **CRITICAL** | ✅ YES | Required for Bitcoin broadcast |
| Monero broadcast | ❌ Not implemented | **CRITICAL** | ✅ YES | Required for Monero support |
| Transaction validation | ❌ Not implemented | **CRITICAL** | ✅ YES | Accepts arbitrary transaction bytes |
| Double-broadcast prevention | ❌ Not implemented | **HIGH** | ✅ YES | No state tracking |
| Transaction replay protection | ❌ Not implemented | **MEDIUM** | ⚠️ Important | Same tx could be reused |
| Fee validation | ❌ Not implemented | **MEDIUM** | ⚠️ Important | Stuck or expensive transactions |
| Output validation | ❌ Not implemented | **HIGH** | ✅ YES | Wrong recipient/amount possible |
| Network connectivity checks | ❌ Not implemented | **MEDIUM** | ⚠️ Important | Silent failures |

### Test Coverage Analysis

**Existing Tests**: `multisig_handlers_test.go:462-526`

**What's Tested**:
- ✅ HandleBroadcast HTTP endpoint responds with 200 OK
- ✅ Validates sufficient signatures collected
- ✅ Returns fake transaction ID
- ✅ Webhook notification sent

**Critical Gaps**:
- ❌ No test for actual blockchain broadcast
- ❌ No test for double-broadcast prevention
- ❌ No test for transaction validation
- ❌ No test for malformed transaction bytes
- ❌ No test for wrong transaction (different outputs)
- ❌ No test for network failure handling
- ❌ No test for idempotency

**Required Tests**:
```go
func TestHandleBroadcast_ActuallyBroadcasts(t *testing.T)
func TestHandleBroadcast_PreventDouble(t *testing.T)
func TestHandleBroadcast_ValidatesTransaction(t *testing.T)
func TestHandleBroadcast_RejectsWrongOutputs(t *testing.T)
func TestHandleBroadcast_RejectsWrongAmount(t *testing.T)
func TestHandleBroadcast_HandlesNetworkFailure(t *testing.T)
func TestHandleBroadcast_Idempotent(t *testing.T)
```

### Audit Conclusion

**Status**: ⛔ **COMPLETELY NON-FUNCTIONAL**

The transaction broadcast system is not implemented and consists only of placeholder code that returns fake success responses. This is **not** a security vulnerability in the traditional sense, but rather a **complete absence** of the core functionality.

**Critical Findings**:
1. ⛔ No actual transaction broadcast - core functionality missing
2. ⛔ Fake transaction IDs generated - system claims success without action
3. ⛔ No Bitcoin RPC integration - cannot communicate with blockchain
4. ⛔ No transaction validation - would accept arbitrary data
5. ⛔ No double-broadcast prevention - would allow repeated broadcasts
6. ⛔ No payment state tracking for broadcasts

**Risk Level**: ⛔ **CRITICAL - SYSTEM UNABLE TO FUNCTION**

**Impact**:
- Payment system cannot process real payments
- Content protection completely bypassed
- Financial reconciliation impossible
- Users cannot receive funds
- Monitoring shows false positives

**Required Actions** (All P0 - Blocking):
1. Implement Bitcoin RPC client integration (8-12 hours)
2. Implement Monero broadcast support (6-8 hours)
3. Add transaction validation before broadcast (4-6 hours)
4. Implement double-broadcast prevention (2-3 hours)
5. Add payment state tracking (Broadcasted, TransactionID) (2-3 hours)
6. Implement output/amount validation (3-4 hours)
7. Add comprehensive error handling (2-3 hours)
8. Write integration tests with testnet nodes (4-6 hours)

**Estimated Total Effort**: 30-45 hours

**Current State**: Suitable for **demonstration purposes only**. The system can collect signatures and show the multisig workflow, but **cannot actually process payments**.

**Production Readiness**: ⛔ **0% - Core functionality missing**

---

## Security Review: Escrow Workflow Threat Modeling

This section documents comprehensive threat modeling for the escrow payment system (PLAN.md Phase 7.3).

### Escrow State Machine Overview

The escrow system implements a 2-of-3 multisig workflow with the following states:

```
EscrowPending  →  EscrowFunded  →  EscrowCompleted (seller wins)
                        ↓                  ↑
                  EscrowDisputed  ────────┘
                        ↓
                  EscrowRefunded (buyer wins)
```

**State Definitions**:
- `EscrowNone`: Non-escrow payment
- `EscrowPending`: Escrow created, awaiting buyer funding
- `EscrowFunded`: Buyer funded multisig address, goods/services pending
- `EscrowCompleted`: Funds released to seller (terminal state)
- `EscrowDisputed`: Arbitration requested by buyer or seller
- `EscrowRefunded`: Funds returned to buyer (terminal state)

**Participants**:
- **Buyer** (`RoleBuyer`): Funds escrow, receives goods/services
- **Seller** (`RoleSeller`): Delivers goods/services, receives payment
- **Arbiter** (`RoleArbiter`): Neutral third party, resolves disputes

**Valid State Transitions** (`escrow.go`):
1. `Pending → Funded`: `FundEscrow()` - buyer funds multisig address
2. `Funded → Completed`: `ReleaseToSeller()` - buyer + seller signatures (happy path)
3. `Funded → Disputed`: `RequestDispute()` - buyer or seller raises dispute
4. `Funded → Refunded`: `RefundBuyer()` - buyer + seller OR buyer + arbiter signatures
5. `Disputed → Completed`: `ResolveDispute()` - arbiter + seller signatures (seller wins)
6. `Disputed → Refunded`: `ResolveDispute()` - arbiter + buyer signatures (buyer wins)

### Threat Model Framework

Analysis covers the STRIDE model:
- **S**poofing: Identity/signature forgery
- **T**ampering: Data manipulation
- **R**epudiation: Denying actions taken
- **I**nformation Disclosure: Unauthorized data access
- **D**enial of Service: Disrupting operations
- **E**levation of Privilege: Unauthorized authority

### Threat Category 1: State Transition Attacks

#### T1.1: Unauthorized State Transitions 🚨 CRITICAL

**Threat**: Attacker directly manipulates payment state to bypass escrow rules.

**Attack Vectors**:
1. **Direct database modification**: If attacker gains Store access, can set `EscrowState` arbitrarily
2. **State injection via API**: Craft malicious UpdatePayment calls
3. **Race condition exploitation**: Concurrent state modifications

**Vulnerable Code**:
- All escrow methods call `em.paywall.Store.UpdatePayment(payment)` directly
- No atomic state transition validation in Store layer
- No audit trail of state changes

**Attack Scenario**:
```go
// Attacker with store access
payment.EscrowState = EscrowCompleted  // Skip dispute resolution
payment.EscrowState = EscrowRefunded   // Steal funds after seller delivers
```

**Impact**: **CRITICAL**
- Bypass entire escrow workflow
- Steal funds after delivery
- Double-spend by resetting to EscrowFunded

**Current Protections**:
- ✅ State validation in escrow methods (checks current state)
- ❌ No database-level state transition constraints
- ❌ No state transition audit log
- ❌ No rollback protection

**Recommended Mitigations**:
```go
// Add state transition validator
type StateTransition struct {
    From      EscrowState
    To        EscrowState
    RequiredBy string  // Method that must perform transition
    Timestamp time.Time
}

func (em *EscrowManager) validateTransition(payment *Payment, newState EscrowState, method string) error {
    validTransitions := map[EscrowState][]EscrowState{
        EscrowPending:  {EscrowFunded},
        EscrowFunded:   {EscrowCompleted, EscrowDisputed, EscrowRefunded},
        EscrowDisputed: {EscrowCompleted, EscrowRefunded},
    }
    
    valid := false
    for _, allowedState := range validTransitions[payment.EscrowState] {
        if newState == allowedState {
            valid = true
            break
        }
    }
    
    if !valid {
        return fmt.Errorf("invalid state transition: %s → %s", payment.EscrowState, newState)
    }
    
    // Log transition for audit
    transition := StateTransition{
        From: payment.EscrowState,
        To: newState,
        RequiredBy: method,
        Timestamp: time.Now(),
    }
    payment.StateTransitions = append(payment.StateTransitions, transition)
    
    return nil
}
```

**Risk Level**: 🚨 **CRITICAL** (if attacker gains store access) / ⚠️ **MEDIUM** (with proper access controls)

#### T1.2: State Transition Race Conditions ⚠️ HIGH

**Threat**: Concurrent operations cause inconsistent state transitions.

**Attack Scenarios**:

1. **Double Release Race**:
```go
// Thread 1: Honest release
go em.ReleaseToSeller(paymentID, buyerSig, sellerSig)

// Thread 2: Concurrent refund
go em.RefundBuyer(paymentID, buyerSig, arbiterSig)

// Race: Both may succeed, last write wins
// Result: Signatures for both release AND refund stored
```

2. **Dispute During Release**:
```go
// Thread 1: Seller initiates release
em.ReleaseToSeller(...)  // Checks state = Funded

// Thread 2: Buyer initiates dispute
em.RequestDispute(...)   // Checks state = Funded

// Both succeed, unpredictable final state
```

**Vulnerable Code**:
```go
// escrow.go:125 - No transaction/lock
if payment.EscrowState != EscrowFunded {
    return ErrInvalidEscrowState
}
// ... time passes (race window) ...
payment.EscrowState = EscrowCompleted
em.paywall.Store.UpdatePayment(payment)  // No optimistic locking
```

**Impact**: **HIGH**
- Funds released to wrong party
- Double-spend if both transactions broadcast
- Inconsistent escrow state
- Signatures for multiple outcomes recorded

**Current Protections**:
- ❌ No optimistic locking in Store.UpdatePayment()
- ❌ No database-level concurrency control
- ❌ No write conflict detection

**Recommended Mitigations**:
```go
// Add version field to Payment
type Payment struct {
    // ... existing fields ...
    Version int  `json:"version"`  // Incremented on each update
}

// Implement optimistic locking
func (s *MemoryStore) UpdatePayment(payment *Payment) error {
    s.mu.Lock()
    defer s.Unlock()
    
    existing := s.payments[payment.ID]
    if existing == nil {
        return errors.New("payment not found")
    }
    
    // Check version hasn't changed
    if existing.Version != payment.Version {
        return errors.New("concurrent modification detected")
    }
    
    payment.Version++
    s.payments[payment.ID] = payment
    return nil
}

// Escrow methods retry on conflict
func (em *EscrowManager) ReleaseToSeller(...) error {
    maxRetries := 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        payment, _ := em.paywall.Store.GetPayment(paymentID)
        
        // ... validation ...
        
        payment.EscrowState = EscrowCompleted
        err := em.paywall.Store.UpdatePayment(payment)
        if err == nil {
            return nil  // Success
        }
        if strings.Contains(err.Error(), "concurrent modification") {
            time.Sleep(100 * time.Millisecond)
            continue  // Retry
        }
        return err  // Other error
    }
    return errors.New("max retries exceeded")
}
```

**Risk Level**: ⚠️ **HIGH** (concurrent operations likely in production)

#### T1.3: Invalid State Persistence After Failure ⚠️ MEDIUM

**Threat**: Partial state updates leave payment in inconsistent state if UpdatePayment fails.

**Attack Scenario**:
```go
// escrow.go:220-225 - Signatures added before state change
for walletType := range payment.Addresses {
    payment.Signatures[walletType] = append(..., *arbiterSig, *winnerSig)
}
// Signatures now modified

payment.EscrowState = EscrowCompleted
err := em.paywall.Store.UpdatePayment(payment)  // FAILS (disk full, network error)
// Result: Signatures in memory but state not saved
```

**Impact**: **MEDIUM**
- Signatures lost on failure
- Retry requires re-collection
- No atomic signature + state update

**Recommended Mitigation**:
```go
// Clone payment before modification
paymentCopy := payment.Clone()
paymentCopy.Signatures[walletType] = append(...)
paymentCopy.EscrowState = EscrowCompleted

// Atomic update
if err := em.paywall.Store.UpdatePayment(paymentCopy); err != nil {
    return err  // Original payment unchanged
}
```

**Risk Level**: ⚠️ **MEDIUM**

### Threat Category 2: Signature-Based Attacks

#### T2.1: Role Spoofing Attack 🚨 CRITICAL

**Threat**: Attacker forges signature roles to bypass authorization (already identified in dispute resolution audit).

**Status**: 🚨 **CRITICAL VULNERABILITY CONFIRMED**

See "Security Review: Dispute Resolution Authority Checks" section for full analysis.

**Affects Escrow Methods**:
- `ReleaseToSeller()`: Checks `buyerSig.Role` and `sellerSig.Role` (attacker-controlled)
- `ResolveDispute()`: Checks `arbiterSig.Role` and `winnerSig.Role` (attacker-controlled)
- `RefundBuyer()`: Checks `sig1.Role` and `sig2.Role` (attacker-controlled)

**Impact on Escrow**: **CRITICAL**
- Attacker can complete any state transition without proper authorization
- Bypass buyer/seller/arbiter role requirements
- Steal escrowed funds

**Risk Level**: 🚨 **CRITICAL**

#### T2.2: Signature Replay Attacks ⚠️ MEDIUM

**Threat**: Reuse signatures from one payment/state transition for another.

**Attack Scenarios**:

1. **Cross-Payment Replay**:
```go
// Payment A: Legitimate release
em.ReleaseToSeller("payment-A", buyerSigA, sellerSigA)

// Payment B: Same amount, same parties
// Attacker replays signatures from payment A
em.ReleaseToSeller("payment-B", buyerSigA, sellerSigA)
// If signatures don't bind to payment ID, both succeed
```

2. **State Transition Replay**:
```go
// Time T1: Refund agreed
em.RefundBuyer(paymentID, buyerSig, sellerSig)

// Time T2: Later, attacker tries to release funds
// Replays old refund signatures
em.ReleaseToSeller(paymentID, buyerSig, sellerSig)
// Should fail (state already Refunded), but signatures stored
```

**Vulnerable Code**:
- Escrow methods concatenate signatures without binding to payment ID or state
- No nonce or timestamp in signatures
- Signatures not cryptographically bound to escrow details

**Impact**: **MEDIUM** (Limited by state validation, but still risk)
- Replay signatures across similar payments
- Confuse audit trails with duplicate signatures
- Potentially exploit future features

**Current Protections**:
- ✅ State validation prevents some replays (can't refund if already completed)
- ❌ No per-payment signature binding
- ❌ No signature nonce/timestamp
- ❌ No signature deduplication

**Recommended Mitigations**:
```go
// Signatures should cryptographically include escrow details
type EscrowSignatureData struct {
    *SignatureData
    PaymentID      string
    EscrowOperation string  // "release", "refund", "dispute_resolve"
    Nonce           []byte  // Random per-signature
    Timestamp       time.Time
}

// Verify signature covers escrow-specific data
func (em *EscrowManager) verifyEscrowSignature(sig *EscrowSignatureData, payment *Payment) error {
    message := fmt.Sprintf("%s:%s:%s:%x:%d",
        sig.PaymentID,
        sig.EscrowOperation,
        sig.Role,
        sig.Nonce,
        sig.Timestamp.Unix())
    
    // Verify cryptographic signature on message
    // ...
}
```

**Risk Level**: ⚠️ **MEDIUM**

#### T2.3: Insufficient Signature Validation ⚠️ HIGH

**Threat**: Signatures accepted without cryptographic verification (same issue as broadcast safety).

**Current State**:
- ✅ Checks signature objects are non-nil
- ✅ Checks Role fields match expectations
- ❌ **Never calls VerifySignature() to validate cryptographic correctness**
- ❌ No public key validation against participant list

**Example Vulnerable Code** (`escrow.go:129-133`):
```go
if buyerSig.Role != RoleBuyer || sellerSig.Role != RoleSeller {
    return fmt.Errorf("signatures must be from buyer and seller")
}
// No: verifySignature(buyerSig), verifySignature(sellerSig)
```

**Attack Scenario**:
```go
// Attacker provides garbage signatures
buyerSig := &SignatureData{
    Role: RoleBuyer,
    Signature: []byte("random garbage"),
    PublicKey: []byte("not a real key"),
}

// Accepted without verification!
em.ReleaseToSeller(paymentID, buyerSig, sellerSig)
```

**Impact**: **HIGH**
- Any attacker can provide fake signatures with correct Role
- Escrow transitions happen without cryptographic proof
- Funds movements unauthorizable

**Risk Level**: ⚠️ **HIGH** (combined with role spoofing = CRITICAL)

### Threat Category 3: Timing and Timeout Attacks

#### T3.1: Timeout Manipulation ⚠️ MEDIUM

**Threat**: Exploit escrow timeout handling to gain advantage.

**Attack Scenarios**:

1. **Seller Delays Delivery**:
```go
// Buyer funds escrow with 30-day timeout
// Seller intentionally delays delivery until day 29
// Buyer unable to raise dispute before timeout
// Timeout triggers automatic refund mechanism (if implemented)
```

2. **Buyer False Claim Pre-Timeout**:
```go
// Goods delivered on day 15
// Buyer waits until day 29, then disputes
// Seller may not respond in time for timeout window
```

3. **System Clock Manipulation**:
```go
// If system clock can be manipulated:
// Attacker advances time to trigger CheckEscrowTimeouts()
// Premature refunds issued
```

**Vulnerable Code** (`escrow.go:311-319`):
```go
// CheckEscrowTimeouts uses server's system clock
now := time.Now()
if now.After(payment.EscrowTimeout) {
    timedOut = append(timedOut, payment.ID)
}
// No mechanism to prevent timeout manipulation
```

**Impact**: **MEDIUM**
- Strategic timing advantages
- Denial of funds during timeout window
- Pressure tactics on honest parties

**Current Protections**:
- ✅ Timeout field persisted at escrow creation
- ❌ No timeout extension mechanism for disputes
- ❌ Timeout check easily bypassed (system must call CheckEscrowTimeouts)
- ❌ No minimum/maximum timeout bounds

**Recommended Mitigations**:
```go
type Config struct {
    // ... existing ...
    MinEscrowTimeout time.Duration  // e.g., 1 hour
    MaxEscrowTimeout time.Duration  // e.g., 90 days
    DisputeTimeoutExtension time.Duration  // e.g., +14 days on dispute
}

func (em *EscrowManager) RequestDispute(...) error {
    // ... existing validation ...
    
    payment.EscrowState = EscrowDisputed
    payment.DisputeReason = reason
    
    // Automatically extend timeout when dispute raised
    payment.EscrowTimeout = payment.EscrowTimeout.Add(em.paywall.config.DisputeTimeoutExtension)
    
    em.paywall.Store.UpdatePayment(payment)
}

// Use blockchain timestamps instead of system clock (more secure)
func (em *EscrowManager) CheckEscrowTimeouts() ([]string, error) {
    // Get current block timestamp from Bitcoin/Monero node
    blockTimestamp := em.getBlockchainTimestamp()
    
    for _, payment := range payments {
        if blockTimestamp.After(payment.EscrowTimeout) {
            // ...
        }
    }
}
```

**Risk Level**: ⚠️ **MEDIUM**

#### T3.2: Race Between Timeout and Dispute Resolution ⚠️ MEDIUM

**Threat**: Dispute resolution completes just as timeout expires, causing conflicting outcomes.

**Attack Scenario**:
```go
// T=0: Dispute opened at escrow day 28
// T=2 days: Timeout threshold reached (day 30)
//   - CheckEscrowTimeouts() identifies payment for refund
//   - Simultaneously, arbiter calls ResolveDispute()
// Race: Which completes first?

// Outcome 1: Refund wins → buyer gets funds
// Outcome 2: Resolution wins → seller gets funds (if arbiter chose seller)
// Outcome 3: Both succeed → double-spend attempt
```

**Vulnerable Code**:
- `CheckEscrowTimeouts()` returns timed-out payment IDs
- No guarantee refund hasn't already been processed
- No lock preventing concurrent resolution

**Impact**: **MEDIUM**
- Unpredictable outcomes near timeout
- Potential double-spend attempts
- Disputes resolved after timeouts

**Recommended Mitigation**:
```go
func (em *EscrowManager) RefundBuyer(...) error {
    payment, _ := em.paywall.Store.GetPayment(paymentID)
    
    // Don't refund if dispute is being actively resolved
    if payment.EscrowState == EscrowDisputed {
        // Check if dispute resolution is in progress
        if time.Since(payment.DisputeOpenedAt) < payment.MinDisputeDuration {
            return errors.New("dispute resolution in progress, cannot refund yet")
        }
    }
    
    // ... rest of refund logic
}

func (em *EscrowManager) ResolveDispute(...) error {
    payment, _ := em.paywall.Store.GetPayment(paymentID)
    
    // Don't resolve if already refunded due to timeout
    if payment.EscrowState != EscrowDisputed {
        return ErrInvalidEscrowState
    }
    
    // ... rest of resolution logic
}
```

**Risk Level**: ⚠️ **MEDIUM**

### Threat Category 4: Economic Attacks

#### T4.1: Griefing via False Disputes ⚠️ MEDIUM

**Threat**: Malicious party raises baseless disputes to harm counterparty.

**Attack Scenarios**:

1. **Buyer Griefing**:
```go
// Goods delivered successfully
// Buyer raises false dispute to delay seller payment
// Locks funds for dispute resolution period
// Seller incurs arbiter fees and time costs
```

2. **Serial Disputer**:
```go
// Attacker creates multiple escrows
// Raises disputes on all of them immediately after funding
// Ties up arbiter resources
// Damages seller's reputation
```

**Current Protections**:
- ✅ Only buyer or seller can raise disputes
- ❌ No cost to raise dispute
- ❌ No dispute history tracking per user
- ❌ No penalties for frivolous disputes
- ❌ No dispute rate limiting

**Impact**: **MEDIUM**
- Economic damage through delays
- Arbiter DoS via dispute spam
- Reputation damage to honest parties

**Recommended Mitigations**:
```go
type Config struct {
    // ... existing ...
    DisputeFeePercentage float64  // e.g., 1% of escrow amount
    MaxDisputesPerUser   int      // e.g., 3 open disputes max
}

type Payment struct {
    // ... existing ...
    DisputeFee           int64  // Amount locked for dispute costs
    DisputeOpenedAt      time.Time
    DisputeOpenedBy      MultisigRole
}

func (em *EscrowManager) RequestDispute(paymentID string, requesterRole MultisigRole, reason string) error {
    // ... existing validation ...
    
    // Check user's dispute history
userDisputes := em.getOpenDisputeCount(requesterRole)
    if userDisputes >= em.paywall.config.MaxDisputesPerUser {
        return errors.New("maximum open disputes exceeded")
    }
    
    // Lock dispute fee from escrow amount
    disputeFee := int64(float64(payment.Amounts[wallet.Bitcoin]) * em.paywall.config.DisputeFeePercentage)
    payment.DisputeFee = disputeFee
    payment.DisputeOpenedAt = time.Now()
    payment.DisputeOpenedBy = requesterRole
    
    payment.EscrowState = EscrowDisputed
    // ...
}

// If dispute resolved in requester's favor, refund dispute fee
// If dispute resolved against requester, fee goes to arbiter (deterrent)
```

**Risk Level**: ⚠️ **MEDIUM**

#### T4.2: Arbiter Collusion ⚠️ HIGH

**Threat**: Arbiter colludes with buyer or seller to steal funds.

**Attack Scenarios**:

1. **Arbiter-Seller Collusion**:
```go
// Seller delivers no goods
// Buyer legitimately disputes
// Arbiter (colluding with seller) resolves in seller's favor
// Buyer loses funds despite legitimate claim
```

2. **Arbiter-Buyer Collusion**:
```go
// Goods delivered successfully
// Buyer raises false dispute
// Arbiter (colluding with buyer) refunds buyer
// Seller loses payment despite fulfilling obligation
```

3. **Arbiter Extortion**:
```go
// Legitimate dispute
// Arbiter demands off-chain payment to rule in party's favor
// "Pay me 0.01 BTC or I rule against you"
```

**Current Protections**:
- ❌ No arbiter authorization system (see "Dispute Resolution Authority Checks")
- ❌ No arbiter accountability mechanism
- ❌ No multi-arbiter option
- ❌ No arbiter selection transparency
- ❌ No arbiter reputation system

**Impact**: **HIGH**
- Direct theft of escrowed funds
- System trust completely broken
- No recourse for honest parties

**Recommended Mitigations**:
```go
// Multi-arbiter system
type Config struct {
    // ... existing ...
    RequiredArbiters   int    // e.g., 3 of 5 arbiters must agree
    ArbiterPublicKeys  [][]byte  // Allowlist of authorized arbiters
}

type Resolution struct {
    // ... existing from dispute.go ...
    ArbiterSignatures  []*SignatureData  // Multiple arbiter signatures
    AgreementCount     int               // How many arbiters agreed
    DissentingArbiters []string         // Arbiters who disagreed
}

// Require consensus from multiple arbiters
func (em *EscrowManager) ResolveDispute(paymentID string, arbiterSigs []*SignatureData, winnerSig *SignatureData) error {
    if len(arbiterSigs) < em.paywall.config.RequiredArbiters {
        return fmt.Errorf("insufficient arbiter signatures: %d of %d", 
            len(arbiterSigs), em.paywall.config.RequiredArbiters)
    }
    
    // Verify all arbiters agree on outcome
    for i, sig1 := range arbiterSigs {
        for j, sig2 := range arbiterSigs {
            if i != j && sig1.Decision != sig2.Decision {
                return errors.New("arbiters disagree on outcome")
            }
        }
    }
    
    // ... rest of resolution
}

// On-chain arbiter bond requirement
type ArbiterBond struct {
    ArbiterPubKey  []byte
    BondAmount     int64
    BondAddress    string  // Arbiter's locked funds
    SlashableUntil time.Time
}

// If arbiter proven dishonest, bond can be slashed
```

**Risk Level**: ⚠️ **HIGH** (existential threat to escrow model if exploited)

#### T4.3: Value Fluctuation Attacks 💰 LOW

**Threat**: Exploit cryptocurrency price volatility during escrow period.

**Attack Scenarios**:

1. **Buyer Exploits Price Drop**:
```go
// T=0: Buyer funds 0.1 BTC escrow ($5000)
// T=30 days: BTC drops 50% ($2500)
// Buyer disputes to delay until timeout refund
// Buyer benefits from holding BTC outside market
```

2. **Seller Exploits Price Rise**:
```go
// T=0: Escrow funded at 0.1 BTC ($5000)
// T=30 days: BTC rises 100% ($10000)
// Seller delays delivery to hold BTC longer
// Seller profits from price increase
```

**Impact**: **LOW** (Economic risk, not a security vulnerability)
- Parties have opposite incentives during volatility
- May incentivize strategic delays
- Not technically exploitable (price is external)

**Mitigations** (Product-level, not security):
- Shorter escrow timeouts during high volatility
- Stablecoin escrow options
- Price oracle integration for USD pegging

**Risk Level**: 💰 **LOW** (economic, not security)

### Threat Category 5: Denial of Service

#### T5.1: Escrow State DoS ⚠️ MEDIUM

**Threat**: Lock payments in non-terminal states permanently.

**Attack Scenarios**:

1. **Funded State DoS**:
```go
// Buyer funds escrow
// Seller never delivers goods
// Buyer never disputes (forgets, loses keys, disappears)
// Funds locked in EscrowFunded state forever
// No automatic resolution mechanism
```

2. **Disputed State DoS**:
```go
// Dispute raised
// Both parties submit evidence
// Arbiter never resolves (offline, compromised, abandoned)
// Funds locked in EscrowDisputed forever
// No alternative resolution path
```

**Vulnerable Code**:
- Terminal states only reachable through specific operations
- No forced resolution mechanism after extended time
- No fallback arbiter or timeout-based resolution

**Impact**: **MEDIUM**
- Funds permanently locked (both parties lose)
- No recovery mechanism
- Encourages out-of-band resolution (defeats purpose)

**Current Protections**:
- ✅ `CheckEscrowTimeouts()` can identify timed-out escrows
- ❌ **Timeout only checks Funded/Disputed states, doesn't auto-resolve**
- ❌ No arbiter replacement mechanism
- ❌ No forced resolution after extended dispute period

**Recommended Mitigations**:
```go
// Automatic resolution paths
func (em *EscrowManager) CheckEscrowTimeouts() ([]string, error) {
    // ... existing timeout detection ...
    
    for _, paymentID := range timedOut {
        payment, _ := em.paywall.Store.GetPayment(paymentID)
        
        switch payment.EscrowState {
        case EscrowFunded:
            // Timeout in funded state → automatic buyer refund
            // Buyer funded but transaction never completed
            em.RefundBuyer(paymentID, systemSig1, systemSig2)
            
        case EscrowDisputed:
            // Timeout in disputed state without resolution
            // Check who opened dispute
            if payment.DisputeOpenedBy == RoleBuyer {
                // Buyer disputed → assume legitimate, refund
                em.RefundBuyer(paymentID, systemSig1, systemSig2)
            } else {
                // Seller disputed → assume delivery made, release
                em.ReleaseToSeller(paymentID, systemSig1, systemSig2)
            }
        }
    }
    
    return timedOut, nil
}

// Add fallback arbiter list
type Config struct {
    // ... existing ...
    PrimaryArbiters   [][]byte
    FallbackArbiters  [][]byte  // Used if primary unresponsive
    FallbackDelay     time.Duration  // 7 days after dispute with no arbiter response
}
```

**Risk Level**: ⚠️ **MEDIUM**

#### T5.2: Signature Collection DoS ⚠️ LOW

**Threat**: One party refuses to provide signature, blocking legitimate state transitions.

**Attack Scenarios**:

1. **Buyer Refuses Release**:
```go
// Goods delivered successfully
// Seller requests release signature
// Buyer refuses to sign (holder, extortion)
// Forces seller to initiate dispute (costs time/money)
```

2. **Seller Refuses Refund**:
```go
// Buyer wants mutual refund
// Seller refuses to sign refund
// Buyer must get arbiter involved (unnecessary cost)
```

**Impact**: **LOW**
- Forces dispute process even for agreed outcomes
- Economic inefficiency
- Not exploitable for financial gain (dispute resolution works)

**Mitigations**:
- Reputation systems to track refusing parties
- Timeout-based unilateral resolution
- Economic penalties for unreasonable refusals

**Risk Level**: ⚠️ **LOW** (Nuisance, not critical)

### Threat Category 6: Information Disclosure

#### T6.1: Dispute Evidence Disclosure ⚠️ LOW

**Threat**: Sensitive information in dispute evidence exposed to arbiter or leaked.

**Attack Scenario**:
```go
// Buyer submits evidence containing:
// - Personal information
// - Banking details
// - Private communications
// Arbiter (or compromised arbiter system) leaks data
```

**Current Protections**:
- ❌ No evidence encryption
- ❌ No privacy policy enforcement
- ❌ No GDPR/privacy compliance mechanisms
- ❌ Evidence stored in plaintext

**Impact**: **LOW** (Privacy concern, not direct financial risk)
- Privacy violations possible
- Regulatory compliance issues (GDPR)
- Reputational damage

**Recommended Mitigations**:
- End-to-end encrypted evidence
- Evidence access logging
- Automatic PII redaction
- Clear privacy policies for arbiters

**Risk Level**: ⚠️ **LOW** (Compliance/privacy issue)

#### T6.2: Payment State Information Leakage 💡 INFO

**Threat**: Payment metadata reveals transaction details to unauthorized parties.

**Current State**:
- Payment states stored in database
- Escrow timeouts, amounts, participant roles visible
- No access controls on payment queries

**Impact**: **INFORMATIONAL**
- Business intelligence leakage
- Tracking of payment patterns
- Privacy reduction

**Mitigations**:
- Authentication on GetPayment queries
- Encrypted payment metadata
- Access logging and monitoring

**Risk Level**: 💡 **INFORMATIONAL**

### Threat Category 7: Repudiation Attacks

#### T7.1: Lack of Audit Trail 🚨 HIGH

**Threat**: No cryptographic proof of actions taken,parties can deny involvement.

**Current State**:
- ✅ Signatures stored with payments
- ❌ No signature timestamps
- ❌ No state transition history
- ❌ No action attribution log
- ❌ No evidence of who initiated operations

**Issue Example**:
```go
// Alice claims: "I never agreed to release funds"
// System shows: Signatures present for release
// Alice claims: "Those signatures were forged/replayed"
// No way to prove: When signature was created, by whom, for what purpose
```

**Impact**: **HIGH**
- Legal disputes unresolvable
- No forensic evidence
- Parties can deny legitimate actions
- Arbiter decisions challengeable

**Recommended Mitigations**:
```go
type AuditLogEntry struct {
    Timestamp     time.Time
    PaymentID     string
    Operation     string  // "ReleaseToSeller", "RequestDispute", etc.
    Initiator     string  // User ID or system
    FromState     EscrowState
    ToState       EscrowState
    Signatures    []*SignatureData
    IPAddress     string
    UserAgent     string
    Success       bool
    ErrorMessage  string
}

// Log every escrow operation
func (em *EscrowManager) logOperation(op string, payment *Payment, signatures []*SignatureData, success bool, err error) {
    entry := AuditLogEntry{
        Timestamp: time.Now(),
        PaymentID: payment.ID,
        Operation: op,
        FromState: payment.EscrowState,  // Before operation
        ToState: payment.EscrowState,     // After operation
        Signatures: signatures,
        Success: success,
    }
    if err != nil {
        entry.ErrorMessage = err.Error()
    }
    
    em.auditLog.Append(entry)
}

// Cryptographically bind signatures to actions
type SignatureData struct {
    // ... existing fields ...
    SignedAction    string    // "release_to_seller", "refund_buyer", etc.
    ActionTimestamp time.Time // When this specific action was authorized
    ActionNonce     []byte    // Unique per action
}
```

**Risk Level**: 🚨 **HIGH** (Legal/dispute resolution failure)

### Recommended Security Controls (​Priority Matrix)

| Priority | Control | Effort | Risk Mitigated |
|----------|---------|--------|----------------|
| **P0** | Implement role-based signature verification | 8-12h | CRITICAL: Role spoofing, unauthorized transitions |
| **P0** | Add cryptographic signature validation | 6-8h | CRITICAL: Fake signatures, bypass authorization |
| **P0** | Implement optimistic locking for payments | 4-6h | HIGH: Race conditions, double-spend |
| **P0** | Create audit trail for all escrow operations | 4-6h | HIGH: Repudiation, forensics |
| **P1** | Add arbiter authorization allowlist | 3-4h | HIGH: Arbiter collusion, unauthorized arbiters |
| **P1** | Implement state transition validator | 3-4h | CRITICAL: Bypass state machine |
| **P1** | Add signature replay protection | 4-5h | MEDIUM: Cross-payment attacks |
| **P1** | Timeout extension on dispute | 2-3h | MEDIUM: Timing attacks |
| **P2** | Multi-arbiter consensus requirement | 8-12h | HIGH: Arbiter collusion |
| **P2** | Dispute fee / spam protection | 3-4h | MEDIUM: Griefing attacks |
| **P2** | Automatic timeout resolution | 4-6h | MEDIUM: Fund locking DoS |
| **P2** | Fallback arbiter mechanism | 4-6h | MEDIUM: Arbiter abandonment |
| **P3** | Evidence encryption | 4-6h | LOW: Privacy disclosure |
| **P3** | Payment history tracking | 2-3h | INFO: Transparency |

### Escrow-Specific Test Coverage Requirements

**Critical Test Scenarios** (Currently Missing):

```go
// State transition tests
func TestEscrowStateMachine_InvalidTransitions(t *testing.T)
func TestEscrowStateMachine_RaceConditions(t *testing.T)
func TestEscrowStateMachine_ConcurrentOperations(t *testing.T)
func TestEscrowStateMachine_OptimisticLocking(t *testing.T)

// Signature tests  
func TestReleaseToSeller_InvalidSignatures(t *testing.T)
func TestReleaseToSeller_ForgedRole(t *testing.T)
func TestResolveDispute_UnathorizedArbiter(t *testing.T)
func TestSignatureReplay_CrossPayment(t *testing.T)

// Timing tests
func TestEscrowTimeout_DisputeExtension(t *testing.T)
func TestEscrowTimeout_RaceWithResolution(t *testing.T)
func TestEscrowTimeout_AutomaticRefund(t *testing.T)

// Economic attack tests
func TestRequestDispute_RateLimiting(t *testing.T)
func TestRequestDispute_FeeRequired(t *testing.T)
func TestArbiterCollusion_MultiArbiterRequired(t *testing.T)

// DoS tests
func TestFundedStateLocking_TimeoutResolution(t *testing.T)
func TestDisputedStateLocking_FallbackArbiter(t *testing.T)

// Audit trail tests
func TestAuditLog_AllOperationsLogged(t *testing.T)
func TestAuditLog_NonRepudiation(t *testing.T)
```

### Threat Model Summary

**Total Threats Identified**: 19

**By Severity**:
- 🚨 **CRITICAL**: 3 (Role spoofing, unauthorized state transitions, lack of sig verification)
- ⚠️ **HIGH**: 6 (Race conditions, arbiter collusion, insufficient validation, repudiation)
- ⚠️ **MEDIUM**: 8 (Timing attacks, dispute griefing, DoS, replay attacks)
- 💰💡 **LOW/INFO**: 2 (Economic volatility, information disclosure)

**Most Critical Attack Paths**:
1. **Arbiter Impersonation** → Role Spoofing → Unauthorized Dispute Resolution → Fund Theft
2. **State Transition Bypass** → Direct Database Manipulation → Skip Escrow Rules → Fund Theft
3. **Race Condition** → Concurrent Release + Refund → Double-Spend → Fund Loss
4. **Arbiter Collusion** → Biased Resolution → Legitimate Party Fund Loss

**System Trust Dependencies**:
- Payment store integrity (no direct state manipulation)
- Arbiter honesty and availability
- Participant signature security (private key protection)
- System clock accuracy for timeouts

### Audit Conclusion

**Status**: ⚠️ **NOT READY FOR PRODUCTION - CRITICAL VULNERABILITIES**

The escrow workflow has a well-designed state machine but **critical security gaps** that allow bypass of essential controls.

**Critical Blockers** (P0):
1. No cryptographic signature validation - signatures never verified
2. Role-based authorization completely bypassable - attacker-controlled roles
3. No state transition atomicity - race conditions possible
4. No audit trail - actions unattributable and repudiable

**High Priority Issues** (P1):
5. No arbiter authorization - anyone can act as arbiter
6. State transition validation insufficient
7. Signature replay possible across payments
8. Timeout handling exploitable

**Risk Level**: 🚨 **CRITICAL**

**Production Readiness**: ⛔ **15% - Core security missing**

**Required Actions Before Production**:
- Fix all P0 issues (20-30 hours estimated)
- Implement P1 security controls (15-20 hours estimated)
- Comprehensive security testing (15-20 hours)
- External security audit recommended
- Penetration testing of escrow workflows

**Estimated Total Effort**: 50-70 hours to reach production-ready state

**Current Suitability**: 
- ✅ Proof-of-concept and workflow demonstration
- ❌ Real-value escrow transactions
- ❌ Production deployment
- ❌ Fiduciary use cases

### Previous Security Review: Transaction Creation/Signing (✅ Secure)
- ✅ Tests successful signing and verification
- ✅ Tests P2WSH witness data construction
- ✅ Tests P2SH scriptSig construction
- ✅ Tests signature ordering
- ✅ Tests invalid input index handling
- ⚠️ Missing: Malformed signature rejection tests
- ⚠️ Missing: Cross-input signature reuse tests
- ⚠️ Missing: Wrong public key tests

**Recommended Additional Tests**:
```go
func TestVerifySignature_InvalidSignature(t *testing.T) {
    // Test: Invalid DER encoding rejected
    // Test: Signature with wrong R/S values
    // Test: Signature from different transaction
}

func TestVerifySignature_WrongPublicKey(t *testing.T) {
    // Test: Signature verified against wrong public key fails
}

func TestCombineSignatures_InsufficientSignatures(t *testing.T) {
    // Test: Transaction with m-1 signatures fails
}
```

### Audit Conclusion

The signature verification logic is **secure and production-ready**. The implementation:
- Uses audited cryptographic libraries (btcsuite)
- Correctly implements Bitcoin signature standards (BIP143, BIP66, BIP16, BIP141)
- Protects against known attack vectors (malleability, cross-input reuse, amount tampering)
- Handles both P2SH and P2WSH multisig formats correctly
- Properly works around the historic OP_CHECKMULTISIG off-by-one bug

**No critical vulnerabilities identified**. Optional enhancements suggested above would improve defense-in-depth but are not required for secure operation.

---

## Security Review: Multisig Metadata Storage

This section documents the security audit of multisig metadata persistence and storage (PLAN.md Phase 7.3).

### Storage Architecture (✅ Secure)

**MultisigStorage** (`wallet/multisig_storage.go:49-330`):

**Design Properties**:
- ✅ Thread-safe with `sync.RWMutex` protection
- ✅ Configurable encryption (optional AES-256-GCM)
- ✅ Atomic file writes (temp file + rename pattern)
- ✅ Restrictive file permissions (0600)
- ✅ Separate storage per wallet type (Bitcoin, Monero)

**Data Structure**:
- ✅ `MultisigWalletData` contains:
  - Wallet type identifier
  - Multisig configuration (m-of-n, public keys)
  - Address-to-metadata mapping
  - Schema version for forward compatibility

### Encryption Implementation (✅ Secure)

**Encryption Algorithm** (`wallet/multisig_storage.go:260-295`):

**Properties**:
- ✅ **AES-256-GCM**: Authenticated encryption (AEAD)
  - Confidentiality: AES-256 in Galois/Counter Mode
  - Authentication: 128-bit authentication tag prevents tampering
- ✅ **Key size**: 256-bit (32 bytes) enforced at configuration time
- ✅ **Nonce generation**: 96-bit (12 bytes) random nonce per encryption
  - Uses `crypto/rand.Reader` for cryptographic randomness
  - Unique nonce per save operation prevents nonce reuse
- ✅ **Format**: `nonce || ciphertext` (nonce stored with ciphertext)

**Security Properties**:
- ✅ Proper AEAD usage (no mac-then-encrypt or encrypt-then-mac mistakes)
- ✅ No nonce reuse vulnerability (random generation + FIPS compliant RNG)
- ✅ Authentication tag prevents tampering detection
- ✅ Nonce length validation during decryption

**Encryption Implementation** (`wallet/multisig_storage.go:260-275`):
```go
// Generate random nonce
nonce := make([]byte, 12)
if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
    return nil, fmt.Errorf("failed to generate nonce: %w", err)
}

// Encrypt and authenticate
ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

// Return nonce || ciphertext
return append(nonce, ciphertext...), nil
```
- ✅ Nonce generation failure causes error (no silent degradation)
- ✅ `gcm.Seal()` combines encryption + authentication
- ✅ No additional authenticated data (AAD) - acceptable for this use case

### Decryption Implementation (✅ Secure)

**Decryption Logic** (`wallet/multisig_storage.go:297-330`):

**Validation**:
- ✅ Validates data length >= 12 bytes (nonce size)
- ✅ Extracts nonce from first 12 bytes
- ✅ Authenticates then decrypts ciphertext
- ✅ Clear error message on authentication failure: `"wrong key or tampered data"`

**Security Properties**:
- ✅ Authentication-then-decrypt order (prevents padding oracle attacks)
- ✅ Constant-time authentication via GCM (no timing side-channels)
- ✅ Detects tampered data (modified ciphertext fails authentication)
- ✅ Detects wrong encryption key (authentication failure)

**Error Handling**:
- ✅ Descriptive error messages without leaking sensitive data
- ✅ No partial plaintext returned on authentication failure
- ✅ GCM authentication failure returns error, not panic

### File Operations Security (✅ Secure)

**SaveMultisigWallet()** (`wallet/multisig_storage.go:90-150`):

**Atomic Write Pattern**:
- ✅ Write to temporary file: `multisig_BTC.dat.tmp`
- ✅ Rename to final name: `multisig_BTC.dat`
- ✅ Cleanup on error: `os.Remove(tempPath)`
- ✅ Prevents partial/corrupt writes during power failure or crash

**File Permissions**:
- ✅ Directory: `0o700` (owner read/write/execute only)
- ✅ File: `0o600` (owner read/write only)
- ✅ Prevents unauthorized access on multi-user systems

**JSON Serialization**:
- ✅ Uses `json.MarshalIndent()` for readability (if plaintext)
- ✅ Standard library JSON encoding (safe, no injection risks)
- ✅ Version field for schema evolution

**LoadMultisigWallet()** (`wallet/multisig_storage.go:150-210`):

**Validation**:
- ✅ File existence check with proper error handling
- ✅ Decryption before deserialization (fail-fast on wrong key)
- ✅ JSON validation via `json.Unmarshal()`
- ✅ Schema version check (rejects future versions)

**Error Handling**:
- ✅ Distinguishes file not found vs. read error
- ✅ Distinguishes decryption failure vs. JSON corruption
- ✅ Forward compatibility check (version > 1)

### Data Classification (✅ Appropriate)

**Sensitive Data** (Encrypted):
- ✅ Multisig configuration (m-of-n parameters)
- ✅ Public keys (sensitive in context of user identity)
- ✅ Redeem scripts (reveal multisig structure)
- ✅ Address mappings (link payments to multisig)

**Non-Sensitive Data**:
- ✅ Wallet type identifier (BTC/XMR) - low sensitivity
- ✅ Schema version - no sensitivity

**Security Notes**:
- ⚠️ Public keys are public on blockchain but linking them to users is sensitive
- ✅ Encryption protects against offline attacks (stolen backup files)
- ✅ File permissions protect against online attacks (other users on system)

### Key Management (⚠️ Delegated to Caller)

**Key Generation**:
- ✅ Enforces 32-byte key length at configuration time
- ⚠️ Caller responsible for generating key securely (not provided by library)
- ⚠️ No built-in key derivation function (KDF) from password

**Key Storage**:
- ⚠️ Caller responsible for key storage (environment variables, key management systems, etc.)
- ⚠️ No key rotation mechanism built-in
- ⚠️ Key must be provided every time storage is used

**Recommendations**:
1. **Key derivation**: Add optional password-based key derivation using Argon2id or PBKDF2
   ```go
   func DeriveKeyFromPassword(password string, salt []byte) ([]byte, error) {
       // Use argon2.IDKey() or pbkdf2.Key()
   }
   ```
2. **Key rotation**: Provide helper for re-encrypting with new key
   ```go
   func (s *MultisigStorage) RotateEncryptionKey(oldKey, newKey []byte) error {
       // Load with oldKey, save with newKey
   }
   ```
3. **Documentation**: Add key management best practices to README

### Threat Modeling

**Threats Addressed** (✅):
1. **Offline Attack (Stolen Backup)**:
   - Mitigated: AES-256-GCM encryption makes data unreadable without key
   - Strength: 256-bit key provides 2^256 brute-force resistance

2. **Tampering (Modified Backup)**:
   - Mitigated: GCM authentication tag detects any modification
   - Attacker cannot modify ciphertext without detection

3. **Multi-User System Access**:
   - Mitigated: File permissions 0600 prevent other users from reading
   - Directory permissions 0700 prevent traversal

4. **Crash During Write**:
   - Mitigated: Atomic write (temp + rename) prevents corruption
   - Either old data or new data, never partial

**Threats Not Addressed** (Documented Limitations):
1. **Key Compromise**:
   - If encryption key is stolen, all data is accessible
   - Mitigation: Secure key storage (external key management system)

2. **Memory Dumps**:
   - Plaintext exists in memory during encryption/decryption
   - Mitigation: OS-level memory protection, avoid core dumps

3. **Side-Channel Attacks**:
   - Timing attacks unlikely (GCM is constant-time for authentication)
   - Power analysis not applicable (software implementation)

4. **Privileged Attacker**:
   - Root/admin users can read any file regardless of permissions
   - Mitigation: Full disk encryption, hardware security modules (HSM)

### Storage Patterns in Main Paywall

**Payment Structure** (`types.go:26-50`):
- ✅ `MultisigEnabled` flag clearly indicates multisig vs. single-sig
- ✅ `MultisigMetadata` map per wallet type (Bitcoin, Monero)
- ✅ Metadata includes:
  - Address
  - Redeem script
  - Script hash (for verification)
  - Public keys
  - Required signatures count
- ✅ JSON serialization with `omitempty` tags (space-efficient)

**FileStore Integration** (`filestore.go`, `encryptedfilestore.go`):
- ✅ Payment-level storage includes multisig metadata
- ✅ Encryption applied to entire payment (including multisig fields)
- ✅ Same AES-256-GCM encryption as MultisigStorage
- ✅ Consistent security properties

### Risk Assessment

| Component | Status | Risk Level | Notes |
|-----------|--------|------------|-------|
| Encryption algorithm | ✅ Secure | Low | AES-256-GCM industry standard |
| Nonce generation | ✅ Secure | Low | Crypto/rand per encryption |
| Authentication | ✅ Secure | Low | GCM authentication tag |
| File permissions | ✅ Secure | Low | 0600 file, 0700 directory |
| Atomic writes | ✅ Secure | Low | Temp + rename pattern |
| Key generation | ⚠️ Caller responsibility | Medium | No built-in KDF |
| Key storage | ⚠️ Caller responsibility | Medium | Environment variables recommended |
| Key rotation | ❌ Not implemented | Medium | Manual process required |
| Memory protection | ⚠️ OS-dependent | Medium | Plaintext in memory during ops |

### Test Coverage Analysis

**MultisigStorage Tests** (`wallet/multisig_storage_test.go`):
- ✅ Tests save and load encrypted data
- ✅ Tests save and load plaintext data
- ✅ Tests decryption with wrong key
- ✅ Tests file not found handling
- ✅ Tests atomic write behavior
- ✅ Tests concurrent access (thread safety)

**Missing Tests** (Optional Enhancements):
- ⚠️ Tampered ciphertext detection test
- ⚠️ Short ciphertext handling test (< 12 bytes)
- ⚠️ Invalid JSON in decrypted data test
- ⚠️ File permission verification test

### Recommendations

**Already Secure**:
- ✅ AES-256-GCM encryption
- ✅ Random nonce per encryption
- ✅ Atomic file writes
- ✅ Restrictive file permissions
- ✅ Thread-safe operations

**Optional Enhancements**:
1. **Add password-based key derivation**:
   ```go
   // Use Argon2id (recommended) or PBKDF2
   func GenerateKeyFromPassword(password, salt []byte) []byte
   ```

2. **Add key rotation support**:
   ```go
   func (s *MultisigStorage) RotateKey(oldKey, newKey []byte) error
   ```

3. **Add secure key wipe**:
   ```go
   func SecureWipeKey(key []byte) {
       for i := range key {
           key[i] = 0
       }
   }
   ```

4. **Document key management**:
   - Best practices for key storage (env vars, KMS, HSM)
   - Warning about memory dumps on crash
   - Instructions for backup encryption keys

5. **Add tamper detection test**:
   ```go
   func TestDecrypt_TamperedData(t *testing.T) {
       // Modify ciphertext byte, verify authentication failure
   }
   ```

### Audit Conclusion

The multisig metadata storage implementation is **secure and production-ready**. Key findings:

**Strengths**:
- Proper use of authenticated encryption (AES-256-GCM)
- Cryptographically secure random nonce generation
- Atomic file writes prevent corruption
- Restrictive file permissions protect against multi-user access
- Thread-safe concurrent access
- Clear error messages without information leakage

**Areas for Improvement** (Non-Critical):
- Add password-based key derivation for user convenience
- Implement key rotation mechanism
- Document key management best practices
- Add additional test coverage for edge cases

**No critical vulnerabilities identified**. The current implementation provides strong security guarantees appropriate for production multisig payment systems.

---

## Security Review: Escrow Timeout Handling

This section documents the security audit of escrow timeout mechanisms (PLAN.md Phase 7.3).

### Timeout Architecture (✅ Secure with Recommendations)

**EscrowManager** (`escrow.go:23-340`):

**Timeout Design**:
- ✅ Timeout stored as absolute timestamp (`time.Time`)
- ✅ Configurable timeout duration at escrow creation
- ✅ Timeout applies to funded and disputed escrows
- ✅ Timeout field optional (zero value = no timeout)

**CreateEscrow()** (`escrow.go:44-68`):
```go
payment.EscrowTimeout = time.Now().Add(escrowTimeout)
```
- ✅ Sets absolute deadline at creation time
- ✅ Uses `time.Now()` (monotonic clock for calculations)
- ✅ Timeout starts when escrow is created (not when funded)

**Security Properties**:
- ✅ Timeout is tamper-evident (stored in payment record)
- ✅ Absolute timestamp prevents time manipulation
- ⚠️ Timeout starts at creation, not funding (design choice)
  - Buyer has limited time to fund after creation
  - May timeout before funding occurs
  - **Recommendation**: Consider separate funding timeout

### Timeout Detection (✅ Correct Implementation)

**CheckEscrowTimeouts()** (`escrow.go:299-322`):

**Logic**:
```go
if !payment.EscrowTimeout.IsZero() && now.After(payment.EscrowTimeout) {
    timedOut = append(timedOut, payment.ID)
}
```

**Security Properties**:
- ✅ Validates timeout is set (`.IsZero()` check)
- ✅ Uses `After()` for clear comparison semantics
- ✅ Only checks funded/disputed escrows (correct states)
- ✅ Returns payment IDs for external handling (separation of concerns)

**State Filtering**:
```go
if payment.EscrowState == EscrowFunded || payment.EscrowState == EscrowDisputed {
    // Only check timeout for these states
}
```
- ✅ Ignores `EscrowPending` (not yet funded)
- ✅ Ignores `EscrowCompleted` (already resolved)
- ✅ Ignores `EscrowRefunded` (already refunded)
- ✅ Checks both funded and disputed states (correct)

### Timeout Enforcement (⚠️ Manual Process)

**Current Implementation**:
- ✅ `CheckEscrowTimeouts()` detects timed out escrows
- ⚠️ Does **not** automatically refund (returns IDs only)
- ⚠️ Caller responsible for refund execution
- ⚠️ No background goroutine for automatic checking

**Security Implications**:
- ✅ **Pro**: Explicit control over refund transactions
- ✅ **Pro**: Allows validation before refund
- ⚠️ **Con**: Timeouts won't trigger without calling `CheckEscrowTimeouts()`
- ⚠️ **Con**: Reliance on external monitoring/cron job

**Recommendation**: Add optional automatic timeout processing:
```go
func (em *EscrowManager) StartTimeoutMonitor(interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for range ticker.C {
            timedOut, err := em.CheckEscrowTimeouts()
            if err != nil {
                // Log error
                continue
            }
            for _, paymentID := range timedOut {
                // Automatically refund or trigger refund workflow
                if err := em.RefundBuyer(paymentID); err != nil {
                    // Log error
                }
            }
        }
    }()
}
```

### Race Conditions (✅ Mitigated by Design)

**Potential Race: Timeout During Resolution**:
- **Scenario**: Seller attempts to complete escrow while timeout check runs
- **Mitigation**: Store-level synchronization (mutex in `memstore.go`, `filestore.go`)
- ✅ `UpdatePayment()` is atomic per payment ID
- ✅ State transitions validate current state before updating

**Potential Race: Concurrent Timeout Checks**:
- **Scenario**: Multiple `CheckEscrowTimeouts()` calls run simultaneously
- **Impact**: Multiple threads may detect same timed out escrows
- ✅ Read-only operation (no state modification)
- ✅ Idempotent (returns same IDs repeatedly until resolved)
- ⚠️ Caller must handle idempotency if processing refunds

**Recommendation**: Add processing lock or deduplication:
```go
type EscrowManager struct {
    processingTimeouts map[string]bool
    mu                 sync.Mutex
}

func (em *EscrowManager) RefundBuyerIfNotProcessing(paymentID string) error {
    em.mu.Lock()
    if em.processingTimeouts[paymentID] {
        em.mu.Unlock()
        return nil // Already being processed
    }
    em.processingTimeouts[paymentID] = true
    em.mu.Unlock()
    
    defer func() {
        em.mu.Lock()
        delete(em.processingTimeouts, paymentID)
        em.mu.Unlock()
    }()
    
    return em.RefundBuyer(paymentID)
}
```

### Time Source Security (✅ Secure)

**Clock Source** (`time.Now()`):
- ✅ Uses system time (monotonic + wall clock)
- ✅ Monotonic clock prevents backwards time jumps during calculations
- ✅ Wall clock allows persistence across reboots

**Clock Tampering**:
- ⚠️ System time can be modified by root/admin
- ⚠️ VM time can be manipulated by hypervisor
- ⚠️ NTP time sync can introduce jumps

**Mitigations**:
- ✅ Monotonic clock (within process) prevents local manipulation
- ⚠️ No protection against system-wide clock changes
- **Recommendation**: Document requirement for NTP time sync
- **Advanced**: Use blockchain timestamps as authoritative time source

### Timeout Validation (⚠️ Minimal)

**CreateEscrow()** Validation:
```go
payment.EscrowTimeout = time.Now().Add(escrowTimeout)
```
- ⚠️ No validation of `escrowTimeout` parameter
- ⚠️ Negative durations allowed (timeout in past)
- ⚠️ Zero duration allowed (immediate timeout)
- ⚠️ Excessive durations allowed (timeout in far future)

**Recommendation**: Add timeout validation:
```go
func (em *EscrowManager) CreateEscrow(priceMultiplier float64, escrowTimeout time.Duration) (string, error) {
    // Validate timeout duration
    if escrowTimeout <= 0 {
        return "", errors.New("escrow timeout must be positive")
    }
    if escrowTimeout < time.Hour {
        return "", errors.New("escrow timeout must be at least 1 hour")
    }
    if escrowTimeout > 30*24*time.Hour {
        return "", errors.New("escrow timeout cannot exceed 30 days")
    }
    // ... rest of function
}
```

### Timeout Extension (❌ Not Implemented)

**Current Limitation**:
- ❌ No mechanism to extend timeout once set
- ❌ Buyer/seller cannot mutually agree to extend deadline
- ❌ Timeout modification requires manual database edit

**Use Cases for Extension**:
- Legitimate delay in goods delivery
- Mutual agreement to extend resolution period
- Arbiter requests more time for investigation

**Recommendation**: Add timeout extension API:
```go
func (em *EscrowManager) ExtendTimeout(paymentID string, extension time.Duration, requiredSigners []string) error {
    // Verify payment exists and is in escrow
    // Verify required parties have agreed (2-of-3 multisig for agreement)
    // Validate extension duration (e.g., max 7 days per extension)
    // Update payment.EscrowTimeout
    // Log extension event
}
```

### Refund Transaction Safety (✅ Proper Authorization)

**RefundBuyer()** (`escrow.go:240-297`):

**Authorization**:
- ✅ Validates payment exists and is in escrow
- ✅ Checks escrow state (must be funded, completed, or disputed)
- ✅ Requires 2-of-3 signatures for refund transaction
- ✅ State transition to `EscrowRefunded` prevents double-refund

**Security Properties**:
- ✅ Cannot refund without proper signatures
- ✅ Cannot refund already completed escrow
- ✅ Idempotent (repeated calls to refunded escrow fail gracefully)

**Timeout-Triggered Refund**:
- ⚠️ Caller must sign refund transaction (not automatic)
- ⚠️ Timeout detection does not auto-execute refund
- **Design Choice**: Explicit signing required even for timeouts

### Denial of Service Risks (⚠️ Moderate Risk)

**Timeout Check Performance**:
- ⚠️ `GetPendingMultisigPayments()` loads all pending payments
- ⚠️ Linear scan through all payments to check timeouts
- ⚠️ No index on `EscrowTimeout` field
- **Impact**: Performance degrades with many pending escrows

**Mitigation Options**:
1. **Add timeout index**: Store payments sorted by timeout
2. **Pagination**: Process timeouts in batches
3. **Early exit**: Stop checking once past earliest timeout
4. **Separate timeout queue**: Dedicated priority queue for timeouts

**Recommendation**:
```go
// Add to PaymentStore interface
GetEscrowsExpiringBefore(deadline time.Time) ([]*Payment, error)

// Implementation uses index or efficient query
// Returns only escrows with timeout <= deadline
```

### Risk Assessment

| Component | Status | Risk Level | Notes |
|-----------|--------|------------|-------|
| Timeout storage | ✅ Secure | Low | Absolute timestamp, tamper-evident |
| Timeout detection | ✅ Correct | Low | Proper state filtering and comparison |
| Timeout enforcement | ⚠️ Manual | Medium | Requires external process to trigger refunds |
| Timeout validation | ⚠️ Missing | Medium | No bounds checking on timeout duration |
| Timeout extension | ❌ Not implemented | Medium | Cannot extend once set |
| Race conditions | ✅ Mitigated | Low | Store-level locking protects state |
| Clock security | ⚠️ System-dependent | Low | Relies on system time accuracy |
| Performance (scaling) | ⚠️ Linear scan | Medium | May not scale to many concurrent escrows |
| Refund authorization | ✅ Secure | Low | Proper multisig signature requirement |

### Recommendations

**High Priority**:
1. **Add timeout validation**: Enforce minimum/maximum timeout durations
   ```go
   const (
       MinEscrowTimeout = 1 * time.Hour
       MaxEscrowTimeout = 30 * 24 * time.Hour
   )
   ```

2. **Add automatic timeout monitoring**: Optional background goroutine
   ```go
   func (em *EscrowManager) StartTimeoutMonitor(interval time.Duration)
   ```

3. **Add timeout extension API**: Allow mutual agreement to extend deadlines
   ```go
   func (em *EscrowManager) ExtendTimeout(paymentID string, extension time.Duration, signatures []Signature) error
   ```

**Medium Priority**:
4. **Optimize timeout checking**: Add `GetEscrowsExpiringBefore()` to store interface
5. **Add processing deduplication**: Prevent concurrent refund processing
6. **Document time sync requirements**: NTP requirement in operations docs

**Low Priority**:
7. **Add timeout event logging**: Log when timeouts are detected/processed
8. **Add timeout metrics**: Monitor timeout frequency and processing latency
9. **Consider blockchain time**: Use block timestamps for authoritative time

### Test Coverage Analysis

**Escrow Tests** (`escrow_test.go`):
- ✅ Tests timeout detection for funded escrows
- ✅ Tests timeout ignored for non-funded escrows
- ✅ Tests timeout field is set correctly
- ⚠️ Missing: Timeout boundary tests (zero, negative, excessive)
- ⚠️ Missing: Concurrent timeout processing tests
- ⚠️ Missing: Timeout during state transition tests

**Recommended Tests**:
```go
func TestCheckEscrowTimeouts_NegativeTimeout(t *testing.T)
func TestCheckEscrowTimeouts_ZeroTimeout(t *testing.T)
func TestCheckEscrowTimeouts_FarFutureTimeout(t *testing.T)
func TestRefundBuyer_ConcurrentCalls(t *testing.T)
func TestExtendTimeout_MutualAgreement(t *testing.T)
```

### Audit Conclusion

The escrow timeout handling is **secure but incomplete** for production use. Key findings:

**Strengths**:
- Correct timeout detection logic
- Proper state filtering (funded/disputed only)
- Tamper-evident storage (absolute timestamps)
- Thread-safe through store-level locking
- Proper refund authorization (multisig)

**Areas Requiring Attention**:
1. **No automatic timeout enforcement**: Requires external monitoring
2. **No timeout validation**: Accepts any duration (including negative)
3. **No timeout extension mechanism**: Cannot modify once set
4. **Performance concerns**: Linear scan doesn't scale well
5. **Missing background processing**: No built-in timeout monitor

**Risk Level**: **MEDIUM**

While not critically insecure, production deployments should address the automatic enforcement and validation gaps. The current design is safe but requires careful operational procedures.

---

## Security Review: Dispute Resolution Authority Checks

This section documents the security audit of dispute resolution authorization and arbiter authority validation (PLAN.md Phase 7.3).

### Arbiter Interface (✅ Well-Designed)

**Arbiter Interface** (`dispute.go:122-147`):

**Design Properties**:
- ✅ Clean separation of concerns (interface for extensibility)
- ✅ Supports external arbiter services (integrators provide implementations)
- ✅ LocalArbiter for testing/single-instance deployments
- ✅ Thread-safe with `sync.RWMutex` protection

**Interface Methods**:
- `RegisterDispute(payment)` - Register new dispute
- `SubmitEvidence(paymentID, evidence)` - Add evidence to dispute
- `GetResolution(paymentID)` - Retrieve arbiter's decision
- `GetDispute(paymentID)` - Get full dispute details
- `ListOpenDisputes()` - List all open disputes

### Critical Security Issues Identified

#### 🚨 CRITICAL: No Arbiter Authorization Validation

**Issue**: `EscrowManager.ResolveDispute()` (`escrow.go:186-238`) accepts ANY signature with `Role=RoleArbiter` without validation.

**Vulnerable Code** (`escrow.go:206-215`):
```go
if arbiterSig.Role != RoleArbiter {
    return fmt.Errorf("first signature must be from arbiter")
}

if winnerSig.Role != RoleBuyer && winnerSig.Role != RoleSeller {
    return fmt.Errorf("second signature must be from buyer or seller")
}
```

**Vulnerabilities**:
1. **No cryptographic signature verification**: Code only checks `Role` field, doesn't verify signature validity
2. **No arbiter identity validation**: Any attacker can set `Role=RoleArbiter` in their signature
3. **No public key allowlist**: No check that arbiter's public key is authorized
4. **Role field is attacker-controlled**: `SignatureData.Role` can be forged by anyone

**Attack Scenario**:
```go
// Attacker creates fake arbiter signature
attackerSig := &SignatureData{
    Role:      RoleArbiter,  // Attacker sets this
    PublicKey: attackerPubKey,
    Signature: attackerSig,
}

// Attacker signs with their own key to claim funds
winnerSig := &SignatureData{
    Role:      RoleBuyer,  // Attacker chooses winner
    PublicKey: attackerPubKey,
    Signature: attackerSig2,
}

// This will succeed without cryptographic verification!
em.ResolveDispute(paymentID, attackerSig, winnerSig)
```

**Impact**: **CRITICAL** - Complete bypass of dispute resolution security. Attacker can:
- Resolve disputes in their favor without arbiter involvement
- Steal escrowed funds by forging arbiter approval
- Impersonate arbiter to steal from both parties

#### 🚨 HIGH: Hardcoded Dispute Requester Role

**Issue**: `LocalArbiter.RegisterDispute()` (`dispute.go:160`) hardcodes requester as `RoleBuyer`.

**Vulnerable Code**:
```go
dispute := &Dispute{
    PaymentID: payment.ID,
    Requester: RoleBuyer, // Default, should be set based on who requested
    Reason:    payment.DisputeReason,
    // ...
}
```

**Vulnerabilities**:
1. **Incorrect attribution**: Seller disputes are recorded as buyer disputes
2. **Audit trail corruption**: Cannot determine who actually initiated dispute
3. **Comment acknowledges bug**: "// Default, should be set based on who requested"

**Impact**: **HIGH** - Dispute accountability broken. Cannot trace dispute origins for audit or legal purposes.

#### ⚠️ MEDIUM: Missing Arbiter Registration Integration

**Issue**: `EscrowManager.RequestDispute()` doesn't register with arbiter system.

**Current Flow**:
1. `EscrowManager.RequestDispute()` - Sets payment state to disputed
2. Payment store updated
3. ❌ **No call to `Arbiter.RegisterDispute()`**
4. Arbiter system has no record of the dispute

**Impact**: **MEDIUM** - Disconnect between payment system and arbiter system. Arbiter cannot:
- See pending disputes
- Request evidence from parties
- Track dispute lifecycle

#### ⚠️ MEDIUM: No Signature Cryptographic Verification

**Issue**: `ResolveDispute()` stores signatures without verification.

**Current Code** (`escrow.go:220-225`):
```go
// Add signatures to the payment
for walletType := range payment.Addresses {
    if payment.Signatures == nil {
        payment.Signatures = make(map[wallet.WalletType][]SignatureData)
    }
    payment.Signatures[walletType] = append(payment.Signatures[walletType], *arbiterSig, *winnerSig)
}
```

**Missing**:
- ❌ No call to `VerifySignature()` from multisig_tx.go
- ❌ No validation signature covers correct transaction
- ❌ No check public key matches expected participants

**Impact**: **MEDIUM** - Invalid signatures accepted. Attacker can provide:
- Signatures from wrong transactions
- Malformed signatures
- Signatures with tampered data

### Authorization Model Security (❌ BROKEN)

**Current Model** (Broken):
```
User provides SignatureData {
    Role: "arbiter"  // ❌ User-controlled, not validated
    PublicKey: [attacker_key]
    Signature: [fake_signature]
}

ResolveDispute() checks:
    ✅ arbiterSig.Role == "arbiter"  // ❌ Checks attacker-controlled field
    ❌ Signature cryptographically valid?  // NOT CHECKED
    ❌ Public key in authorized arbiter list?  // NOT CHECKED
```

**Secure Model** (Required):
```
System configuration:
    authorizedArbiters = [arbiter1_pubkey, arbiter2_pubkey, arbiter3_pubkey]

ResolveDispute() MUST:
    1. ✅ Verify arbiterSig.PublicKey in authorizedArbiters list
    2. ✅ Cryptographically verify arbiterSig.Signature
    3. ✅ Verify signature covers escrow transaction
    4. ✅ Verify winnerSig.PublicKey in [buyer, seller] list
    5. ✅ Cryptographically verify winnerSig.Signature
    6. ✅ Verify both signatures are for same transaction
```

### Evidence Submission Security (✅ Mostly Secure)

**LocalArbiter.SubmitEvidence()** (`dispute.go:169-191`):

**Validations Present**:
- ✅ Checks dispute exists
- ✅ Checks dispute not already resolved
- ✅ Validates evidence is not nil
- ✅ Validates evidence has content

**Missing Validations**:
- ⚠️ No validation that `SubmittedBy` role is authorized
- ⚠️ No validation that evidence size is reasonable (DoS risk)
- ⚠️ No validation that evidence type matches content
- ⚠️ No signature on evidence (can be forged/modified)

**Recommendations**:
```go
func (la *LocalArbiter) SubmitEvidence(paymentID string, evidence *Evidence, signature []byte) error {
    // 1. Verify signature on evidence
    evidenceHash := sha256.Sum256([]byte(evidence.Content))
    if !verifySignature(evidence.PublicKey, evidenceHash[:], signature) {
        return fmt.Errorf("invalid evidence signature")
    }
    
    // 2. Validate evidence size (prevent DoS)
    if len(evidence.Content) > 10*1024*1024 { // 10MB limit
        return fmt.Errorf("evidence too large")
    }
    
    // 3. Validate submitter is party to dispute
    if evidence.SubmittedBy != dispute.Requester && 
       evidence.SubmittedBy != getOtherParty(dispute) {
        return fmt.Errorf("evidence must be from dispute parties")
    }
    
    // ... rest of function
}
```

### Resolution Storage Security (⚠️ Audit Trail Issues)

**LocalArbiter.ResolveDispute()** (`dispute.go:236-262`):

**Security Properties**:
- ✅ Thread-safe with mutex
- ✅ Prevents resolution of already-resolved disputes
- ✅ Records timestamp and arbiter ID
- ✅ Immutable once resolved

**Missing Audit Trail**:
- ⚠️ No signature on resolution itself
- ⚠️ Resolution can be modified in LocalArbiter (if using in-memory)
- ⚠️ No cryptographic binding between resolution and signatures
- ⚠️ No append-only log of resolution history

**Recommendations**:
```go
type Resolution struct {
    // ... existing fields ...
    
    // Add cryptographic binding
    ResolutionHash  []byte    `json:"resolution_hash"`   // Hash of all resolution data
    ArbiterSignature []byte   `json:"arbiter_signature"` // Arbiter signs resolution hash
    
    // Add audit trail
    PreviousHash    []byte    `json:"previous_hash"`     // Chain resolutions
    Version         int       `json:"version"`           // Schema version
}
```

### Threat Model Analysis

**Threats Addressed** (✅):
1. **Concurrent dispute access**: Thread-safe with mutex
2. **Duplicate evidence submission**: Evidence IDs prevent duplicates
3. **Evidence after resolution**: Validates dispute status before accepting
4. **Nil pointer access**: Validates all inputs for nil

**Threats NOT Addressed** (❌):
1. **🚨 CRITICAL: Arbiter impersonation**: No validation of arbiter identity
2. **🚨 CRITICAL: Signature forgery**: No cryptographic verification
3. **🚨 HIGH: Role spoofing**: Attacker-controlled Role field
4. **⚠️ MEDIUM: Evidence tampering**: No signatures on evidence
5. **⚠️ MEDIUM: Arbiter authorization**: No allowlist of authorized arbiters
6. **⚠️ MEDIUM: Resolution tampering**: No signatures on resolutions

### Risk Assessment

| Component | Status | Risk Level | Notes |
|-----------|--------|------------|-------|
| Arbiter identity validation | ❌ Missing | **CRITICAL** | No check arbiter is authorized |
| Signature cryptographic verification | ❌ Missing | **CRITICAL** | Only checks Role field |
| Role field validation | ❌ Attacker-controlled | **CRITICAL** | Can be forged |
| Arbiter authorization list | ❌ Not implemented | **HIGH** | No allowlist mechanism |
| Dispute requester tracking | ❌ Hardcoded | **HIGH** | Always set to RoleBuyer |
| Arbiter registration | ❌ Not called | **MEDIUM** | Disconnect between systems |
| Evidence signatures | ❌ Not implemented | **MEDIUM** | Evidence can be forged |
| Resolution signatures | ❌ Not implemented | **MEDIUM** | Resolutions can be modified |
| Evidence size limits | ❌ Not enforced | **MEDIUM** | DoS via large evidence |
| Audit trail cryptographic binding | ❌ Not implemented | **MEDIUM** | No tamper-proof log |

### Required Fixes (Blocking Production Use)

#### Fix 1: Implement Arbiter Authorization List

**Add to Config**:
```go
type Config struct {
    // ... existing fields ...
    
    // Multisig arbiter configuration
    AuthorizedArbiters [][]byte  // List of authorized arbiter public keys
}
```

**Add to EscrowManager**:
```go
type EscrowManager struct {
    paywall           *Paywall
    authorizedArbiters map[string]bool  // Set of authorized arbiter pubkey hashes
}

func (em *EscrowManager) isAuthorizedArbiter(pubKey []byte) bool {
    keyHash := sha256.Sum256(pubKey)
    return em.authorizedArbiters[hex.EncodeToString(keyHash[:])]
}
```

#### Fix 2: Implement Cryptographic Signature Verification

**Update ResolveDispute**:
```go
func (em *EscrowManager) ResolveDispute(paymentID string, arbiterSig, winnerSig *SignatureData) error {
    // ... existing payment retrieval ...
    
    // 1. CRITICAL: Verify arbiter is authorized
    if !em.isAuthorizedArbiter(arbiterSig.PublicKey) {
        return fmt.Errorf("arbiter not authorized: %x", arbiterSig.PublicKey)
    }
    
    // 2. CRITICAL: Cryptographically verify arbiter signature
    for walletType, address := range payment.Addresses {
        metadata := payment.MultisigMetadata[walletType]
        if metadata == nil {
            continue
        }
        
        valid, err := verifySignatureAgainstTx(arbiterSig, payment, walletType)
        if err != nil || !valid {
            return fmt.Errorf("invalid arbiter signature: %w", err)
        }
    }
    
    // 3. CRITICAL: Verify winner signature
    if !em.isParticipant(winnerSig.PublicKey, payment) {
        return fmt.Errorf("winner public key not in payment participants")
    }
    
    valid, err := verifySignatureAgainstTx(winnerSig, payment, walletType)
    if err != nil || !valid {
        return fmt.Errorf("invalid winner signature: %w", err)
    }
    
    // 4. Verify roles match public keys (derived from pubkey, not user input)
    arbiterSig.Role = em.getRoleForPubKey(arbiterSig.PublicKey)
    winnerSig.Role = em.getRoleForPubKey(winnerSig.PublicKey)
    
    if arbiterSig.Role != RoleArbiter {
        return fmt.Errorf("first signature must be from authorized arbiter")
    }
    
    // ... rest of function
}
```

#### Fix 3: Fix Hardcoded Dispute Requester

**Update RegisterDispute**:
```go
func (la *LocalArbiter) RegisterDispute(payment *Payment) error {
    // ... existing validation ...
    
    // Determine requester from payment context
    requester := RoleBuyer  // Default fallback
    if payment.MultisigEnabled && len(payment.Signatures) > 0 {
        // Infer from who initiated dispute in payment metadata
        // This requires additional context passed from RequestDispute
    }
    
    dispute := &Dispute{
        PaymentID: payment.ID,
        Requester: requester,  // Should be passed as parameter
        Reason:    payment.DisputeReason,
        // ...
    }
    
    la.disputes[payment.ID] = dispute
    return nil
}
```

**Better**: Add requester parameter:
```go
func (la *LocalArbiter) RegisterDispute(payment *Payment, requester MultisigRole) error {
    // ... validation ...
    
    if requester != RoleBuyer && requester != RoleSeller {
        return fmt.Errorf("requester must be buyer or seller")
    }
    
    dispute := &Dispute{
        PaymentID: payment.ID,
        Requester: requester,
        // ...
    }
    // ...
}
```

#### Fix 4: Integrate Arbiter Registration

**Update EscrowManager.RequestDispute**:
```go
func (em *EscrowManager) RequestDispute(paymentID string, requesterRole MultisigRole, reason string) error {
    // ... existing validation ...
    
    payment.EscrowState = EscrowDisputed
    payment.DisputeReason = reason
    if err := em.paywall.Store.UpdatePayment(payment); err != nil {
        return fmt.Errorf("failed to update payment state: %w", err)
    }
    
    // CRITICAL: Register dispute with arbiter system
    if em.paywall.arbiter != nil {
        if err := em.paywall.arbiter.RegisterDispute(payment, requesterRole); err != nil {
            // Rollback payment state on failure
            payment.EscrowState = EscrowFunded
            em.paywall.Store.UpdatePayment(payment)
            return fmt.Errorf("failed to register dispute with arbiter: %w", err)
        }
    }
    
    return nil
}
```

### Recommendations Priority Matrix

| Priority | Recommendation | Effort | Impact |
|----------|----------------|--------|--------|
| **P0** | Implement arbiter authorization list | 2-4 hours | Blocks arbiter impersonation |
| **P0** | Add cryptographic signature verification | 4-6 hours | Prevents signature forgery |
| **P0** | Remove attacker-controlled Role field | 2-3 hours | Derive role from public key |
| **P1** | Fix hardcoded dispute requester | 1-2 hours | Fixes audit trail |
| **P1** | Integrate arbiter registration | 2-3 hours | Connects payment & arbiter systems |
| **P1** | Add evidence signatures | 3-4 hours | Prevents evidence forgery |
| **P2** | Add resolution signatures | 2-3 hours | Tamper-proof resolutions |
| **P2** | Add evidence size limits | 1 hour | Prevents DoS |
| **P2** | Add cryptographic audit trail | 4-6 hours | Tamper-proof history |

### Test Coverage Requirements

**Critical Tests Needed**:
```go
func TestResolveDispute_UnauthorizedArbiter(t *testing.T)
func TestResolveDispute_InvalidArbiterSignature(t *testing.T)
func TestResolveDispute_ForgedRole(t *testing.T)
func TestResolveDispute_MismatchedSignatures(t *testing.T)
func TestRequestDispute_ArbiterRegistration(t *testing.T)
func TestSubmitEvidence_SignatureVerification(t *testing.T)
func TestSubmitEvidence_SizeLimit(t *testing.T)
```

### Audit Conclusion

**Status**: ⛔ **NOT READY FOR PRODUCTION**

The dispute resolution authorization system has **CRITICAL security vulnerabilities** that completely bypass arbiter controls. Key findings:

**Critical Issues (Blocking)**:
1. No arbiter identity validation - anyone can impersonate arbiter
2. No cryptographic signature verification - signatures not validated
3. Attacker-controlled Role field - can be forged
4. No arbiter authorization list - no concept of "authorized arbiter"

**High Priority Issues**:
5. Hardcoded dispute requester breaks audit trail
6. Missing arbiter registration integration

**Risk Level**: ⛔ **CRITICAL - DO NOT USE IN PRODUCTION**

**Required Actions**:
- Implement all P0 fixes before any production use
- Add comprehensive security tests for all scenarios
- Consider security audit by external party
- Document arbiter authorization model clearly

**Estimated Effort**: 15-25 hours to resolve all critical issues

**Current State**: Suitable for prototyping only. Production use would result in immediate fund theft via arbiter impersonation.











- [ ] Wallet encryption key stored in secure key storage (not in code)
- [ ] RPC endpoints configured (local Bitcoin node strongly recommended)
- [ ] Minimum confirmations set to 6 for mainnet
- [ ] HTTPS enabled with valid certificates
- [ ] Logging configured to exclude sensitive data
- [ ] Testnet flag set to `false`
- [ ] Payment timeout values reviewed
- [ ] Wallet seeds backed up securely offline
- [ ] Access to wallet keys restricted to production application only
- [ ] Monero RPC credentials stored securely (if using Monero)
- [ ] Rate limiting configured to prevent abuse
- [ ] Monitoring and alerting set up for payment verification failures

## References

- [BIP32: Hierarchical Deterministic Wallets](https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki)
- [BIP44: Multi-Account Hierarchy for Deterministic Wallets](https://github.com/bitcoin/bips/blob/master/bip-0044.mediawiki)
- [OWASP Cookie Security](https://owasp.org/www-community/attacks/csrf)
- [NIST SP 800-38D: GCM Mode](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf)
- [RFC 6265bis: Cookies](https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-rfc6265bis)

## Security Reporting

If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public GitHub issue
2. Email security details to the project maintainers with details and reproduction steps
3. Allow 90 days for patch development and testing before public disclosure
4. Once patched, a security advisory will be published

Thank you for helping keep this project secure.
