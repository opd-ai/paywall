# Cryptocurrency Wallet Implementation

![Go Version](https://img.shields.io/badge/go-%3E%3D1.19-blue)

A cryptocurrency wallet implementation supporting Bitcoin (BTC) and Monero (XMR) in Go. Features BIP32/44 compliant HD wallet functionality for Bitcoin and RPC-based wallet operations for Monero.

## Features

### Bitcoin Support
- BIP32/44 compliant HD wallet implementation
- Support for both mainnet and testnet networks
- Multiple public API endpoint failover system
- Local Bitcoin node connectivity with public node fallback
- Deterministic address generation and validation
- Thread-safe wallet operations
- Base58 encoding/decoding 
- Address balance checking
- Extensive API endpoint list with automatic failover

### Monero Support
- RPC-based wallet implementation
- Balance querying capabilities
- Subaddress generation
- Transaction confirmation tracking
- Integration with go-monero-rpc-client

### Core Features
- AES-256-GCM encrypted wallet storage
- Secure key derivation using HMAC-SHA512
- Basic error handling
- Thread-safe operations with mutex protection
- Automatic API endpoint selection and failover

## Installation

```bash
go get github.com/opd-ai/paywall/wallet
```

## Usage Examples

### Bitcoin HD Wallet

```go
// Create a new Bitcoin HD wallet
seed := make([]byte, 32)
rand.Read(seed)
btcWallet, err := wallet.NewBTCHDWallet(seed, false) // false for mainnet
if err != nil {
    log.Fatal(err)
}

// Generate new address
address, err := btcWallet.GetAddress()
if err != nil {
    log.Fatal(err)
}

// Check balance
balance, err := btcWallet.GetAddressBalance(address)
if err != nil {
    log.Fatal(err)
}
```

### Monero Wallet

```go
// Create a new Monero wallet
config := wallet.MoneroConfig{
    RPCURL:      "http://localhost:18081",
    RPCUser:     "user",
    RPCPassword: "password",
}

xmrWallet, err := wallet.NewMoneroWallet(config)
if err != nil {
    log.Fatal(err)
}

// Generate new address
address, err := xmrWallet.GetAddress()
if err != nil {
    log.Fatal(err)
}
```

### Secure Storage

```go
// Generate encryption key
key, err := wallet.GenerateEncryptionKey()
if err != nil {
    log.Fatal(err)
}

// Configure storage
config := wallet.StorageConfig{
    DataDir:       "/path/to/wallets",
    EncryptionKey: key,
}

// Save wallet
err = btcWallet.SaveToFile(config)
if err != nil {
    log.Fatal(err)
}

// Load wallet
loadedWallet, err := wallet.LoadFromFile(config)
if err != nil {
    log.Fatal(err)
}
```

## Project Structure

```
wallet/
├── address.go       # Bitcoin address handling and validation
├── base58.go        # Base58 encoding/decoding implementation
├── btc_hd_wallet.go # Bitcoin HD wallet implementation
├── hd_wallet.go     # Wallet interface definitions
├── storage.go       # Encrypted storage implementation
└── xmr_hd_wallet.go # Monero wallet implementation
```

## Security Features

### Key Management
- Master key generation using HMAC-SHA512
- BIP32/44 compliant key derivation
- AES-256-GCM encryption for stored data
- Secure random number generation for encryption keys

### Network Security
- Multiple API endpoints with automatic failover
- Local node priority with public node fallback
- Support for both HTTP and HTTPS endpoints
- Basic endpoint validation

### Storage Security
- AES-256-GCM encrypted wallet data
- Random nonce generation for encryption
- Basic file permissions management

## Development Requirements

- Go 1.19 or higher
- Access to Bitcoin/Monero nodes for testing
- Network connectivity for API access

## Testing

```bash
go test ./wallet/... -v
```

Testing requirements:
- Internet connectivity for API tests
- Local or remote node access for wallet operations
- Testnet access for integration tests

## Contributing

1. Fork the repository
2. Create your feature branch
3. Write tests for new functionality
4. Commit your changes
5. Push to the branch
6. Submit a pull request

## License

This project is available under the MIT License.

## Disclaimer

This implementation is provided for educational purposes. Production use requires thorough security review and testing.