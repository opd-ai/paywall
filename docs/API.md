# API Reference

Complete reference for the opd-ai/paywall public API.

## Table of Contents

- [Main Package](#main-package)
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
