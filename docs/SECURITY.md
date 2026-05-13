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
