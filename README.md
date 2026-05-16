# Go Bitcoin Paywall

[![Go Report Card](https://goreportcard.com/badge/github.com/opd-ai/paywall)](https://goreportcard.com/report/github.com/opd-ai/paywall)
[![GoDoc](https://godoc.org/github.com/opd-ai/paywall?status.svg)](https://godoc.org/github.com/opd-ai/paywall)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A secure, production-ready Bitcoin paywall implementation in Go, designed to help creative workers join the Bitcoin economy by controlling their own content distribution platforms with minimal barriers to entry.

## Features=[]

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

    // Call close when your program shuts down to terminate the payment check routine co
    defer pw.Close()
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
    PriceInBTC       float64           // Price in Bitcoin
    PriceInXMR       float64           // Price in Monero (optional)
    TestNet          bool              // Use testnet networks
    Store            PaymentStore      // Storage backend
    PaymentTimeout   time.Duration     // Payment expiration time
    MinConfirmations int               // Required blockchain confirmations
    XMRUser          string            // Monero wallet RPC username (optional)
    XMRPassword      string            // Monero wallet RPC password (optional)
    XMRRPC           string            // Monero RPC endpoint URL (optional)
}
```

**Bitcoin-Only vs Multi-Currency Configuration**:
- **Bitcoin-only**: Only `PriceInBTC` is required. XMR fields (XMRUser, XMRPassword, XMRRPC, PriceInXMR) are optional and can be omitted.
- **Multi-currency**: To enable Monero support, provide all XMR fields. The paywall will automatically fail over to Bitcoin-only mode if Monero RPC connection fails, with a warning logged.

### Storage Options

- `NewMemoryStore()`: In-memory payment tracking (default)
- `NewFileStore()`: Filesystem-based persistent storage

### Wallet Management

#### BIP39 Mnemonic Support

User-friendly wallet backup with 12 or 24-word phrases:

```go
// Generate new mnemonic (24 words recommended)
mnemonic, err := wallet.GenerateMnemonic(wallet.Mnemonic24Words)
// Example: "abandon ability able about above absent absorb abstract..."

// Create wallet from mnemonic
wallet, err := wallet.NewBTCHDWalletFromMnemonic(
    mnemonic,
    "",    // Optional passphrase for extra security
    true,  // Testnet
    1,     // Min confirmations
)

// Restore wallet from backed-up mnemonic
seed, err := wallet.ImportFromMnemonic(mnemonic, "")
wallet, err := wallet.NewBTCHDWallet(seed[:32], testnet, 1)

// Validate user-entered mnemonic
if !wallet.ValidateMnemonic(userInput) {
    fmt.Println("Invalid mnemonic phrase")
}
```

**Mnemonic Backup Best Practices**:
- **Write down the mnemonic** on paper (never store digitally unencrypted)
- Use 24 words for maximum security (256-bit entropy)
- Optional passphrase adds "25th word" protection
- Test recovery before trusting with real funds
- Store backup in secure location (fireproof safe, safety deposit box)

#### File-Based Wallet Storage

```go
// Generate new wallet encryption key
key, err := wallet.GenerateEncryptionKey()

// Configure wallet storage
config := wallet.StorageConfig{
    DataDir:       "./paywallet",
    EncryptionKey: key,
}

// Save Bitcoin wallet (type assertion required)
btcWallet := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet)
err = btcWallet.SaveToFile(config)

// Load wallet from file
loadedWallet, err := wallet.LoadFromFile(config)
```

**Note on Wallet Recovery**: You can now use BIP39 mnemonics for wallet recovery! The mnemonic provides full wallet recovery including the seed. However, the `nextIndex` counter (tracking which addresses have been used) is not stored in the mnemonic. To preserve address history, back up both the mnemonic AND the encrypted wallet files. If you lose the wallet file but have the mnemonic, addresses will regenerate from the beginning, which may cause address reuse if previous addresses received payments.

#### Monero Multisig Support

The paywall now supports Monero multisig wallets for escrow and multi-party payment scenarios. Monero multisig setup is a multi-step process requiring coordination between all participants.

**2-of-3 Multisig Setup Example**:

```go
// Create wallets for all three participants
wallet1, _ := wallet.NewMoneroWallet(config1, 1)
wallet2, _ := wallet.NewMoneroWallet(config2, 1)
wallet3, _ := wallet.NewMoneroWallet(config3, 1)

// Step 1: Each participant prepares multisig
info1, _ := wallet1.PrepareMultisig(2) // threshold = 2
info2, _ := wallet2.PrepareMultisig(2)
info3, _ := wallet3.PrepareMultisig(2)

// Step 2: Each participant makes multisig with others' info
round2Info1, addr1, _ := wallet1.MakeMultisig([]string{info2, info3}, 2)
round2Info2, addr2, _ := wallet2.MakeMultisig([]string{info1, info3}, 2)
round2Info3, addr3, _ := wallet3.MakeMultisig([]string{info1, info2}, 2)

// Step 3: Finalize multisig (required for M-of-N where M < N)
wallet1.FinalizeMultisig([]string{round2Info2, round2Info3})
wallet2.FinalizeMultisig([]string{round2Info1, round2Info3})
wallet3.FinalizeMultisig([]string{round2Info1, round2Info2})

// Now wallets are ready for multisig payments
// Use with EscrowManager for marketplace/subscription scenarios
```

**Using Monero Multisig with Escrow**:

```go
// After multisig setup, create escrow payment
escrowMgr := paywall.NewEscrowManager(pw)
payment, _ := escrowMgr.CreateEscrow(
    0.1,              // 0.1 XMR
    wallet.Monero,    // Use Monero multisig
    buyerPubKey,
    sellerPubKey,
    arbiterPubKey,
    2,                // 2-of-3 signatures required
)

// The payment uses the Monero multisig address
// Transaction signing requires coordination via ExportMultisigInfo/ImportMultisigInfo
```

**Important Notes**:
- Monero multisig addresses are wallet-level, not per-payment like subaddresses
- Each wallet must complete the full setup process before use
- For M-of-N multisig (e.g., 2-of-3), the FinalizeMultisig step is required
- Transaction signing requires additional coordination rounds (ExportMultisigInfo/ImportMultisigInfo)
- Keep multisig setup info secure - loss of info may make funds unrecoverable

### Storage Backends

#### Available Storage Options

#### Memory Store
- Volatile in-memory storage
- Suitable for testing and development
- Data is lost on service restart

#### File Store
- Persistent filesystem storage
- Default location: ./paywallet
- AES-256 encrypted storage
- Suitable for production use

### Configuration Example

```go
// Memory Store
store := paywall.NewMemoryStore()

// File Store (simple)
store := paywall.NewFileStore("./payments")

// File Store with encryption (recommended for production)
encryptionKey, err := wallet.GenerateEncryptionKey()
if err != nil {
    log.Fatal(err)
}

store, err := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir:       "/var/lib/paywall/data",
    EncryptionKey: encryptionKey, // Optional: enables AES-256 encryption
})
if err != nil {
    log.Fatal(err)
}
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