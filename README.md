# Go Bitcoin Paywall

[![Go Report Card](https://goreportcard.com/badge/github.com/opd-ai/paywall)](https://goreportcard.com/report/github.com/opd-ai/paywall)
[![GoDoc](https://godoc.org/github.com/opd-ai/paywall?status.svg)](https://godoc.org/github.com/opd-ai/paywall)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A secure, production-ready Bitcoin paywall implementation in Go, designed to help creative workers join the Bitcoin economy by controlling their own content distribution platforms with minimal barriers to entry.

## Features

- 🔒 Secure Bitcoin HD wallet implementation
- 🔒 Support for Monero wallets via RPC interface
- 💰 Flexible payment tracking and verification
- 🌐 Easy-to-use HTTP middleware
- 💾 Multiple storage backends (Memory, File)
- 🔑 AES-256 encrypted wallet storage
- ⚡ Real-time payment verification
- 📱 Mobile-friendly payment UI with QR codes
- 🧪 Testnet support for development

## Installation

```bash
go get github.com/opd-ai/paywall
```

## Quick Start

```go
package main

import (
    "log"
    "net/http"
    "time"
    
    "github.com/opd-ai/paywall"
)

func main() {
    // Initialize paywall with minimal config
    pw, err := paywall.NewPaywall(paywall.Config{
        PriceInBTC:     0.001,                    // 0.001 BTC
        TestNet:        true,                     // Use testnet
        Store:          paywall.NewMemoryStore(), // In-memory storage
        PaymentTimeout: time.Hour * 24,           // 24-hour payment window
    })
    if err != nil {
        log.Fatal(err)
    }

    // Protected content handler
    protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Protected content"))
    })

    // Apply paywall middleware
    http.Handle("/protected", pw.Middleware(protected))

    log.Fatal(http.ListenAndServe(":8000", nil))
}
```

## Documentation

### Configuration

```go
type Config struct {
    PriceInBTC     float64
    TestNet        bool
    Store          Store
    PaymentTimeout time.Duration
}
```

### Storage Options

- `NewMemoryStore()`: In-memory payment tracking (default)
- `NewFileStore()`: Filesystem-based persistent storage

### Wallet Management

```go
// Generate new wallet encryption key
key, err := wallet.GenerateEncryptionKey()

// Configure wallet storage
config := wallet.StorageConfig{
    DataDir:       "./paywallet",
    EncryptionKey: key,
}

// Save/load wallet
err = pw.HDWallet.SaveToFile(config)
wallet, err := wallet.LoadFromFile(config)
```

## Security Features

- Secure cookie handling with SameSite=Strict
- AES-256-GCM wallet encryption
- Cryptographically secure random payment IDs
- Base58Check address encoding
- Proper error handling and input validation

## Use Cases

Perfect for:
- Digital content creators
- Artists selling digital works
- Subscription-based services
- Pay-per-view content
- API monetization
- Digital downloads

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

Re: support for other cryptocurrency, we will consider other currencies, but we consider Monero to be the only good cryptocurrency.

This is because Monero **is** the only good cryptocurrency.

Bitcoin is supported out of expediency, Ethereum may also be worth supporting.
We're not going to focus on shitcoins.

## Support the Project

If you find this project useful, consider supporting the developer:

Monero Address: `43H3Uqnc9rfEsJjUXZYmam45MbtWmREFSANAWY5hijY4aht8cqYaT2BCNhfBhua5XwNdx9Tb6BEdt4tjUHJDwNW5H7mTiwe`
Bitcoin Address: `bc1qew5kx0srtp8c4hlpw8ax0gllhnpsnp9ylthpas`

## License

MIT License - see LICENSE file for details

## Credits

Created and maintained by the OPD AI team.