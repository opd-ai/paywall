# Configuration

## Overview

The opd-ai/paywall system is configured via the `Config` struct, which controls payment amounts, payment storage, blockchain confirmation requirements, and cryptocurrency network selection. This guide covers all configuration options, recommended values, and common setup patterns.

## Configuration Struct

```go
type Config struct {
    PriceInBTC       float64           // Price in Bitcoin (e.g., 0.001 for 0.001 BTC)
    PriceInXMR       float64           // Price in Monero (e.g., 0.01 for 0.01 XMR)
    PaymentTimeout   time.Duration     // Duration to wait for payment (e.g., 24 * time.Hour)
    MinConfirmations int               // Blockchain confirmations required (e.g., 6)
    TestNet          bool              // true = Bitcoin testnet, false = mainnet
    Store            PaymentStore      // Where to store payment records (Memory/File/EncryptedFile)
    XMRUser          string            // Monero RPC username (optional, from env if not provided)
    XMRPassword      string            // Monero RPC password (optional, from env if not provided)
    XMRRPC           string            // Monero RPC URL (optional, default: http://127.0.0.1:18081)
}
```

## Price Configuration

### Bitcoin Amounts

Bitcoin prices are specified in decimal BTC. Common values:

| Use Case | BTC | USD (@ $45k) | XMR |
|----------|-----|--------------|-----|
| Free tier | 0 | $0 | 0 |
| Article | 0.0001 | $4.50 | 0.001 |
| Video | 0.001 | $45 | 0.01 |
| Course | 0.01 | $450 | 0.1 |
| Premium | 0.1 | $4,500 | 1.0 |

**Configuration example**:

```go
config := paywall.Config{
    PriceInBTC:     0.001,  // 0.001 BTC
    TestNet:        true,   // Use testnet for development
    Store:          paywall.NewMemoryStore(),
    PaymentTimeout: 24 * time.Hour,
    MinConfirmations: 1,
}
```

### Monero Amounts

Monero prices are specified in decimal XMR. Monero provides:
- **Privacy**: Transactions don't reveal sender/receiver
- **Fungibility**: All XMR coins are truly equivalent
- **Lower fees**: Network fees typically 0.0001-0.001 XMR

Common Monero amounts:

| Amount | Use | Notes |
|--------|-----|-------|
| 0.001 | Article | Very low value, proof of payment |
| 0.01 | Small content | Typical microservice price |
| 0.1 | Medium content | $10-20 USD equivalent |
| 1.0+ | Premium content | Significant value, consider payment tracking |

**Configuration example**:

```go
config := paywall.Config{
    PriceInBTC:     0.001,
    PriceInXMR:     0.01,  // Accept both Bitcoin and Monero
    TestNet:        true,
    Store:          paywall.NewFileStore("./payments"),
    PaymentTimeout: 24 * time.Hour,
    MinConfirmations: 1,
}
```

### Dust Limits

The system enforces minimum amounts to prevent "dust" (uneconomic) payments:

- **Bitcoin minimum**: 0.00001 BTC (1 satoshi × 1000, accounts for network fees)
- **Monero minimum**: 0.0001 XMR

Prices below these limits will cause `NewPaywall()` to return an error:

```go
config := paywall.Config{
    PriceInBTC: 0.000001,  // TOO LOW!
}
pw, err := paywall.NewPaywall(config)
// Error: PriceInBTC 0.000001 is below dust limit (minimum: 0.00001)
```

## Payment Timeout

`PaymentTimeout` specifies how long a payment can remain pending before expiring and being removed from the system.

**Recommended values**:

| Scenario | Timeout | Rationale |
|----------|---------|-----------|
| Development | 5 minutes | Quick iteration |
| Testnet | 1 hour | Real-world but test |
| Production (volatile) | 12-24 hours | Accommodate network delays |
| Production (stable) | 72 hours | Reduce expiration false positives |

**Configuration**:

```go
import "time"

config := paywall.Config{
    PriceInBTC:     0.001,
    PaymentTimeout: 24 * time.Hour,  // Payment valid for 24 hours
}
```

**How it works**:
1. Payment created at time T
2. User generates payment page (with QR code)
3. User sends funds to generated address
4. System monitors blockchain for payment
5. If payment received before (T + PaymentTimeout), mark as confirmed
6. If payment NOT received after (T + PaymentTimeout), remove from system

**Developer note**: Longer timeouts increase storage overhead (more pending payments stored). Shorter timeouts may reject legitimate slow payments or network confirmations.

## Minimum Confirmations

`MinConfirmations` specifies how many blockchain confirmations are required before a payment is considered finalized.

### Bitcoin Confirmation Levels

| Confirmations | Time | Security | Risk | Use Case |
|---------------|------|----------|------|----------|
| 1 | ~10 min | ~99.9% | Double-spend possible | Testing, low-value |
| 3 | ~30 min | ~99.99% | Double-spend unlikely | Medium value |
| 6 | ~60 min | ~99.999% | Industry standard, "irreversible" | Production, high-value |
| 12+ | ~120+ min | ~99.9999%+ | Maximum security | Very high value |

### Monero Confirmation Levels

Monero transactions are final after 10 blocks (blocks added every ~2 minutes):

| Confirmations | Time | Setup |
|---------------|------|-------|
| 1-10 | 2-20 min | Development, testing |
| 10 | ~20 min | Standard finality |
| 20+ | ~40 min | Maximum confidence |

### Configuration Recommendations

**Development/Testnet**:
```go
config := paywall.Config{
    MinConfirmations: 1,  // Accept immediately for testing
    TestNet:         true,
}
```

**Production/Mainnet (recommended)**:
```go
config := paywall.Config{
    MinConfirmations: 6,  // Bitcoin standard (~60 min)
    TestNet:         false,
}
```

**High-security/High-value**:
```go
config := paywall.Config{
    MinConfirmations: 12,  // Maximum security
    TestNet:         false,
}
```

## Network Selection (TestNet vs MainNet)

### Bitcoin TestNet

Use for development and testing. TestNet Bitcoin has no monetary value.

```go
config := paywall.Config{
    TestNet: true,
    // Addresses generated at path: m/44'/1'/... (coin_type=1)
}
```

**Characteristics**:
- 10-minute average block time (same as mainnet)
- No real value - funds are worthless
- Coins available from faucets: testnet-faucet.mempool.space
- Testnet3 is the current official testnet

**When to use**:
- Development
- Integration testing
- Demo/PoC systems
- User testing before production

### Bitcoin MainNet

Use for production systems handling real payments.

```go
config := paywall.Config{
    TestNet: false,
    // Addresses generated at path: m/44'/0'/... (coin_type=0)
}
```

**Characteristics**:
- Real Bitcoin value
- Real blockchain confirmations
- Real network fees
- Transaction finality crucial

**CRITICAL**: Never enable `TestNet: true` in production. Users will not verify payments on testnet and attackers could send fake testnet coins.

### Switching Networks (DO NOT DO THIS)

Changing `TestNet` value changes the HD wallet derivation path. Do NOT switch networks with an existing wallet:

```go
// BAD: Wallet created on testnet
wallet1 := paywall.NewWallet(seed, true)   // m/44'/1'/...
// Then later: switched to mainnet
wallet2 := paywall.NewWallet(seed, false)  // m/44'/0'/...
// Result: Different addresses, existing payments not found, confusion
```

Instead, create separate wallet instances for testnet and mainnet.

## Storage Configuration

The system supports multiple storage backends for payment records.

### Memory Store (Testing)

Payments stored in memory only. Lost when application stops.

```go
import "github.com/opd-ai/paywall"

store := paywall.NewMemoryStore()
config := paywall.Config{
    Store: store,
}
```

**Use cases**:
- Unit tests
- Development (no persistence needed)
- Single-request demos

**Characteristics**:
- ✅ No external dependencies (no filesystem, no database)
- ✅ Fast (everything in RAM)
- ❌ Data lost on restart
- ❌ No error recovery

### File Store (Persistent)

Payments stored as JSON files in a directory.

```go
store := paywall.NewFileStore("./payments")
config := paywall.Config{
    Store: store,
}
```

**Directory structure**:
```
./payments/
├── 1461540575152f03fedd677cb87cdc62.json
├── 26e996702e45a193ec644c6960f46663.json
└── 2e142588a68c343c2636fa51da9fb4b9.json
```

**Each file**:
```json
{
  "id": "1461540575152f03fedd677cb87cdc62",
  "address": "tb1q7enewxkdz7yy4ggv0kqr5ruvv8pdwdpm9ystzf",
  "amount_btc": 0.001,
  "amount_xmr": 0,
  "created_at": "2026-05-12T21:00:00Z",
  "expires_at": "2026-05-13T21:00:00Z",
  "confirmations": 1,
  "status": "confirmed"
}
```

**Use cases**:
- Small deployments
- Self-hosted systems
- Simple deployment (no database needed)

**Characteristics**:
- ✅ Persistent (survives restarts)
- ✅ Simple to inspect (JSON files)
- ✅ No database required
- ⚠️ Single-file performance limits (thousands of payments OK, millions slow)
- ❌ No encryption by default

### Encrypted File Store (Recommended for Production)

Same as File Store but with AES-256-GCM encryption.

```go
import "github.com/opd-ai/paywall"

// Generate encryption key (do this once, store securely)
key, _ := paywall.GenerateEncryptionKey()

store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "./encrypted_payments",
    EncryptionKey: key,
})

config := paywall.Config{
    Store: store,
}
```

**Files are encrypted**:
```
./encrypted_payments/
├── 1461540575152f03fedd677cb87cdc62.enc
├── 26e996702e45a193ec644c6960f46663.enc
└── (binary encrypted data, not human readable)
```

**Use cases**:
- Production systems with sensitive payment data
- Systems in Cloud environments
- Compliance requirements for data protection

**Characteristics**:
- ✅ Persistent with encryption
- ✅ AES-256-GCM prevents tampering
- ✅ No database required
- ✅ Files not human-readable
- ❌ Requires key management (don't lose the key!)

#### Encryption Key Generation

```go
// Generate a new 256-bit encryption key
key, err := paywall.GenerateEncryptionKey()
if err != nil {
    log.Fatal(err)
}

// Key is 32 bytes (256 bits) in hex format
// Example output: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"

// Store securely (environment variable, key vault, etc.)
// DO NOT hardcode in source code
// DO NOT commit to Git
// DO NOT log

// Retrieve from environment
encryptionKey := os.Getenv("PAYWALL_ENCRYPTION_KEY")
```

**Key storage options**:
- Environment variable (for development/containers)
- HashiCorp Vault (for enterprise)
- AWS Secrets Manager (for AWS deployments)
- Azure Key Vault (for Azure deployments)
- Kubernetes Secrets (for Kubernetes deployments)

## Monero RPC Configuration

If accepting Monero payments, configure the Monero wallet RPC connection.

### Configuration Methods

**1. Explicit config**:
```go
config := paywall.Config{
    PriceInXMR:  0.01,
    XMRUser:    "monero_rpc_user",
    XMRPassword: "monero_rpc_password",
    XMRRPC:     "http://localhost:18081",  // Default port
}
```

**2. Environment variables** (if not provided in config):
```bash
export XMR_WALLET_USER="monero_rpc_user"
export XMR_WALLET_PASS="secure_password_min_8_chars"
# XMR_WALLET_RPC defaults to http://127.0.0.1:18081
```

Then:
```go
config := paywall.Config{
    PriceInXMR: 0.01,
    // XMRUser, XMRPassword loaded from env automatically
}
```

### Monero Wallet Setup

```bash
# Start Monero wallet RPC
# Method 1: Using monero-wallet-rpc binary
monero-wallet-rpc \
    --wallet-file /path/to/wallet/name \
    --password "wallet_password" \
    --daemon-address http://node.moneroworld.com:18089 \
    --rpc-bind-port 18081 \
    --trusted-daemon \
    --public-node  # Only if exposing publicly

# Method 2: Using monero-wallet-cli with --rpc-bind-address
monero-wallet-cli \
    --wallet-file /path/to/wallet \
    --password "wallet_password" \
    --daemon-address http://127.0.0.1:18081 \
    start_mining \
    --threads 4
```

### Monero Daemon Connection

The Monero wallet RPC connects to a Monero daemon node. Configure the daemon URL:

**Public network nodes** (use with caution - not recommended for production):
- `http://node.moneroworld.com:18089`
- `http://opennode.com:18089`
- Advantages: Easy, no setup
- Disadvantages: Privacy concerns, depend on uptime of others

**Local node** (recommended for production):
```bash
monerod --daemon-bind-ip 127.0.0.1 --daemon-bind-port 18081
```

Advantages:
- Privacy: Only you see transactions
- Reliability: Under your control
- Security: No network exposure needed

## Example Configurations

### Bitcoin-Only Development

```go
pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC:     0.0001,
    TestNet:        true,
    Store:          paywall.NewMemoryStore(),
    PaymentTimeout: 5 * time.Minute,
    MinConfirmations: 1,
})
defer pw.Close()
```

### Bitcoin-Only Production (with persistent storage)

```go
store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "/var/lib/paywall",
    EncryptionKey: os.Getenv("PAYWALL_KEY"),
})

pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC:     0.001,
    TestNet:        false,
    Store:          store,
    PaymentTimeout: 24 * time.Hour,
    MinConfirmations: 6,
})
defer pw.Close()
```

### Dual-Currency Production (Bitcoin + Monero)

```go
store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "/var/lib/paywall",
    EncryptionKey: os.Getenv("PAYWALL_KEY"),
})

pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC:     0.001,
    PriceInXMR:     0.01,
    TestNet:        false,
    Store:          store,
    PaymentTimeout: 24 * time.Hour,
    MinConfirmations: 6,
    XMRUser:        os.Getenv("XMR_WALLET_USER"),
    XMRPassword:    os.Getenv("XMR_WALLET_PASS"),
    XMRRPC:         "http://localhost:18081",
})
defer pw.Close()
```

## Validation Rules

The `NewPaywall()` function validates configuration values:

| Field | Rules | Error | Example |
|-------|-------|-------|---------|
| PriceInBTC | > 0 OR = 0 (if not used) | Must be positive if > 0 | ✅ 0.001 or ✅ 0 |
| PriceInBTC | ≥ 0.00001 if > 0 | Below dust limit | ❌ 0.000001 |
| PriceInXMR | > 0 if XMR configured | Must be positive if XMR used | ✅ 0.01 |
| PriceInXMR | ≥ 0.0001 if > 0 | Below dust limit | ❌ 0.00001 |
| PaymentTimeout | > 0 | Must be positive | ✅ 24*time.Hour |
| MinConfirmations | ≥ 0 | Not validated (0 = no wait) | ✅ 6 |
| Store | not nil | Required | ❌ nil (must provide) |

## Environment Variable Reference

| Variable | Purpose | Required | Example |
|----------|---------|----------|---------|
| `XMR_WALLET_USER` | Monero RPC username | If using Monero | `paywall_user` |
| `XMR_WALLET_PASS` | Monero RPC password | If using Monero | `secure_password` |
| `PAYWALL_ENCRYPTION_KEY` | Encryption key for file storage | For encrypted storage | `a1b2c3d4...` |

## Testing Configuration

For unit tests and integration tests:

```go
// Minimal config for testing
store := paywall.NewMemoryStore()
config := paywall.Config{
    PriceInBTC:     0.0001,
    TestNet:        true,
    Store:          store,
    PaymentTimeout: 5 * time.Minute,
    MinConfirmations: 1,
}
pw, _ := paywall.NewPaywall(config)
defer pw.Close()
```

## Multisig Configuration (Optional)

Multisig (multi-signature) support allows creating payment addresses that require multiple signatures to spend funds, enabling escrow and dispute resolution workflows.

### When to Use Multisig

**Use cases**:
- **Escrow**: Buyer, seller, and arbiter each hold one key (2-of-3 multisig)
- **Marketplace**: Platform holds one key, seller holds one, dispute resolution service holds third
- **High-value**: Require approval from multiple parties before funds can be moved
- **Business**: Require multiple executive signatures for withdrawals

**Single-sig is simpler**: For most content paywalls, standard single-signature addresses are sufficient and simpler to implement.

### Configuration Fields

```go
type Config struct {
    // ... existing fields ...

    // MultisigEnabled enables multisig address generation (default: false)
    MultisigEnabled     bool

    // MultisigRequired is m in m-of-n (number of signatures required)
    // Must be >= 2 and <= MultisigTotal when MultisigEnabled=true
    MultisigRequired    int

    // MultisigTotal is n in m-of-n (total number of signers)
    // Must be >= MultisigRequired when MultisigEnabled=true
    MultisigTotal       int

    // ParticipantPubKeys are public keys for all participants per wallet type
    // Map keys: wallet.Bitcoin or wallet.Monero
    // Each slice must contain MultisigTotal public keys
    ParticipantPubKeys  map[wallet.WalletType][][]byte

    // MultisigRole identifies this instance's role (optional)
    // Values: wallet.RoleBuyer, wallet.RoleSeller, wallet.RoleArbiter
    MultisigRole        MultisigRole
}
```

### Example: 2-of-3 Escrow Configuration

#### Step 1: Generate Keys for Each Participant

Each participant generates their own wallet and exports public keys:

```go
// Buyer's setup
buyerSeed := make([]byte, 32)
rand.Read(buyerSeed)
buyerWallet, _ := wallet.NewBTCHDWallet(buyerSeed, false, 6)
buyerPubKey := buyerWallet.GetPublicKey() // Implement as needed

// Seller's setup (similar process)
sellerPubKey := ... // Seller exports their public key

// Arbiter's setup (similar process)
arbiterPubKey := ... // Arbiter exports their public key
```

#### Step 2: Configure Paywall with Multisig

Each participant configures their paywall instance with all public keys:

```go
config := paywall.Config{
    PriceInBTC:       0.01,
    TestNet:          false,
    Store:            paywall.NewFileStore("./payments"),
    PaymentTimeout:   72 * time.Hour,  // Longer timeout for escrow
    MinConfirmations: 6,

    // Multisig configuration
    MultisigEnabled:  true,
    MultisigRequired: 2,  // Need 2 signatures to spend
    MultisigTotal:    3,  // Out of 3 total signers

    ParticipantPubKeys: map[wallet.WalletType][][]byte{
        wallet.Bitcoin: {
            buyerPubKey,
            sellerPubKey,
            arbiterPubKey,
        },
    },

    MultisigRole: wallet.RoleBuyer,  // This instance is the buyer
}

pw, err := paywall.NewPaywall(config)
if err != nil {
    log.Fatal(err)
}
defer pw.Close()
```

### Validation Rules for Multisig

When `MultisigEnabled: true`, additional validation applies:

| Field | Rule | Error Example |
|-------|------|---------------|
| MultisigRequired | >= 2 | ❌ MultisigRequired: 1 |
| MultisigTotal | >= MultisigRequired | ❌ Total=2, Required=3 |
| ParticipantPubKeys | Not nil when enabled | ❌ Must provide keys |
| ParticipantPubKeys | Length = MultisigTotal per wallet | ❌ Expected 3 keys, got 2 |
| ParticipantPubKeys | No empty keys | ❌ Key at index 1 is empty |

### Bitcoin vs Monero Multisig

**Bitcoin multisig**:
- Uses P2SH (legacy) or P2WSH (SegWit) addresses
- Requires collecting signatures from M participants
- Signatures can be collected off-chain, then broadcast together
- Well-established, widely supported

**Monero multisig**:
- Uses native Monero multisig via RPC
- Requires wallet synchronization between participants
- More coordination steps (PrepareMultisig → MakeMultisig → FinalizeMultisig)
- Provides privacy benefits

### Common Multisig Configurations

| Configuration | Use Case | Security | Complexity |
|---------------|----------|----------|------------|
| 2-of-2 | Joint accounts | High (both must approve) | Low |
| 2-of-3 | Escrow with arbiter | Medium (dispute resolution) | Medium |
| 3-of-5 | Business board | Medium (majority approval) | High |
| 5-of-9 | High-security custody | Very high | Very high |

### Single-Sig vs Multisig Comparison

| Feature | Single-Sig | Multisig |
|---------|------------|----------|
| Setup complexity | Simple | Complex (key coordination) |
| Transaction speed | Fast | Slower (signature collection) |
| Security | Single key risk | Distributed key risk |
| Use case | Content paywalls | Escrow, disputes |
| Recommended | Default for most users | Only when needed |

### Backward Compatibility

Multisig is **completely optional**. When `MultisigEnabled: false` (default):
- System behaves exactly as before
- No multisig validation performed
- Standard single-signature addresses generated
- Existing code continues to work unchanged

## Troubleshooting Configuration

**Error**: `PriceInBTC must be positive`
- **Cause**: PriceInBTC is 0 or negative
- **Fix**: Set PriceInBTC to a positive value (e.g., 0.001)

**Error**: `XMR wallet password not provided`
- **Cause**: XMR fields provided but password missing
- **Fix**: Either:
  1. Set `config.XMRPassword` explicitly, OR
  2. Set `XMR_WALLET_PASS` environment variable, OR
  3. Remove XMR configuration entirely if only using Bitcoin

**Error**: `PriceInBTC below dust limit`
- **Cause**: Price is less than 0.00001 BTC
- **Fix**: Increase price to at least 0.00001 BTC

**Error**: `payment timeout must be positive`
- **Cause**: PaymentTimeout is 0 or negative
- **Fix**: Set to a reasonable duration (e.g., 24*time.Hour)

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for more issues.
