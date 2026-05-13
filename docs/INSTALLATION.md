# Installation

## Prerequisites

Before installing opd-ai/paywall, ensure you have the following:

### System Requirements

- **Go 1.23.2 or later** — Download from [golang.org](https://go.dev/dl)
- **256-bit entropy available** — `/dev/urandom` must be readable and functional (required for `crypto/rand`)
- **Linux, macOS, or Windows** — All major platforms supported

### Bitcoin Node (Optional)

For testnet development without a local node:
- Public Bitcoin testnet RPC endpoints are included as fallback
- No local node configuration required for basic testing

For mainnet production or private testnet:
- Bitcoin Core node running in testnet or mainnet mode
- RPC endpoint accessible (default: http://127.0.0.1:18332 for testnet)
- RPC credentials configured (if required)

### Monero Integration (Optional)

For Monero payment support:
- Monero wallet RPC service running
- Default endpoint: `http://127.0.0.1:18081`
- RPC credentials (username, password)
- Wallet must be open and synced

For Bitcoin-only paywall operation:
- No Monero setup required

## Installation Steps

### 1. Install the Package

```bash
go get github.com/opd-ai/paywall
```

### 2. Verify Installation

```bash
go list -m github.com/opd-ai/paywall
# Output: github.com/opd-ai/paywall v0.0.0-...
```

### 3. Create Project Structure (Optional)

For a new project using the paywall:

```bash
mkdir -p myapp/paywallet
cd myapp
go mod init example.com/myapp
go get github.com/opd-ai/paywall
```

### 4. Generate Wallet Encryption Key

For persistent payment storage with encryption:

```bash
# Run from your go project
go run -c 'package main; import ("fmt"; "github.com/opd-ai/paywall/wallet"); func main() {
  key, _ := wallet.GenerateEncryptionKey()
  fmt.Printf("Generated key: %x\n", key)
}'
```

Store this key securely in environment variables or a secrets manager.

## Configuration

### Bitcoin-Only Configuration

No additional setup required. Example from README:

```go
package main

import (
    "github.com/opd-ai/paywall"
    "net/http"
)

func main() {
    pw, err := paywall.NewPaywall(paywall.Config{
        PriceInBTC:     0.001,
        TestNet:        true,
        Store:          paywall.NewMemoryStore(),
        PaymentTimeout: time.Hour * 24,
    })
    if err != nil {
        log.Fatal(err)
    }

    http.Handle("/protected", pw.Middleware(
        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.Write([]byte("Protected content"))
        }),
    ))
    http.ListenAndServe(":8000", nil)
}
```

### Monero Integration Configuration

Set environment variables before running:

```bash
export XMR_WALLET_USER="rpc_user"      # Monero wallet RPC username
export XMR_WALLET_PASS="rpc_password"  # Monero wallet RPC password
# Optional: export XMR_RPC="http://rpc.monero.com:18081"

go run main.go
```

Then in config:

```go
pw, err := paywall.NewPaywall(paywall.Config{
    PriceInBTC:  0.001,
    PriceInXMR:  0.05,
    TestNet:     false,
    Store:       paywall.NewMemoryStore(),
    // XMRUser, XMRPassword, XMRRPC loaded from env vars
})
```

### Persistent Storage Configuration

For production with encrypted wallet storage:

```go
encryptionKey, _ := wallet.GenerateEncryptionKey()
store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "/var/lib/paywall/data",
    EncryptionKey: encryptionKey,
})

pw, err := paywall.NewPaywall(paywall.Config{
    PriceInBTC: 0.001,
    TestNet:    false,
    Store:      store,
})
```

## Wallet Initialization

### First-Time Setup

When `NewPaywall()` is called for the first time:

1. A new BTC HD wallet is generated from a secure random seed
2. Monero wallet is connected (if configured)
3. Payment storage backend is initialized
4. Background payment verification goroutine starts

### Wallet Persistence

Wallets are NOT automatically saved to disk. To persist:

```go
btcWallet := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet)
err := btcWallet.SaveToFile(wallet.StorageConfig{
    DataDir:       "./paywallet",
    EncryptionKey: encryptionKey,
})
```

or use the FileStoreWithConfig approach above.

### Loading Existing Wallet

```go
loadedWallet, err := wallet.LoadFromFile(wallet.StorageConfig{
    DataDir:       "./paywallet",
    EncryptionKey: encryptionKey,
})
```

## Deployment

### Development (Testnet)

```bash
# Run with testnet (no real money)
go run main.go
```

### Production (Mainnet)

```bash
# 1. Update config
config.TestNet = false
config.PriceInBTC = 0.01  # Real bitcoin amount

# 2. Use encrypted storage
store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "/var/lib/paywall/data",
    EncryptionKey: encryptionKey,
})

# 3. Set environment for Monero (if used)
export XMR_WALLET_USER="prod_user"
export XMR_WALLET_PASS="secure_password"

# 4. Run behind HTTPS (required for secure cookies)
go run main.go
```

### Docker Deployment

```dockerfile
FROM golang:1.23

WORKDIR /app
COPY . .

RUN go build -o paywall main.go

CMD ["./paywall"]
```

Environment variables:
```yaml
environment:
  - XMR_WALLET_USER=rpc_user
  - XMR_WALLET_PASS=rpc_password
  - ENCRYPTION_KEY=<your-256-bit-key-hex>
```

### Systemd Service

```ini
[Unit]
Description=Bitcoin/Monero Paywall Service
After=network.target

[Service]
Type=simple
User=paywall
WorkingDirectory=/var/lib/paywall
ExecStart=/usr/local/bin/paywall

Environment="XMR_WALLET_USER=rpc_user"
Environment="XMR_WALLET_PASS=rpc_password"
Restart=on-failure
RestartSec=10s

[Install]
WantedBy=multi-user.target
```

## Troubleshooting Installation

### "crypto/rand failed" on Startup

**Symptom**: Program exits with `crypto/rand.Int failed: cannot initialize wallet securely`

**Solution**: This is a critical security failure indicating system entropy is exhausted. The system refuses to proceed to prevent security degradation.

- Check `/dev/urandom` is available: `ls -l /dev/urandom`
- In containers, ensure adequate entropy: check Docker `--cpus` limits
- Run `cat /proc/sys/kernel/random/entropy_avail` (Linux) — should be > 1000

### "XMR wallet password not provided"

**Symptom**: Configuration error when trying to use Bitcoin-only paywall

**Solution**: XMR environment variables are only required if Monero configuration is provided.

For Bitcoin-only:
```go
// This should work without XMR env vars
pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC: 0.001,
    // Don't set PriceInXMR, XMRUser, XMRPassword
})
```

### "address already in use" when binding to port

**Symptom**: `listen tcp: address already in use`

**Solution**: 
```bash
# Find process using port 8000
lsof -i :8000

# Kill the process
kill <PID>

# Run on different port
go run main.go -port 8001
```

## Next Steps

- See [CONFIGURATION.md](CONFIGURATION.md) for all configuration options
- See [EXAMPLES.md](EXAMPLES.md) for code examples
- See [SECURITY.md](SECURITY.md) for security considerations
- See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues
