# Project Overview

The opd-ai/paywall is a production-ready Bitcoin and Monero paywall implementation in Go, designed to help creative workers join the cryptocurrency economy by controlling their own content distribution platforms with minimal barriers to entry. The project provides secure Bitcoin HD wallet implementation, flexible payment tracking and verification, easy-to-use HTTP middleware, and multiple storage backends with AES-256 encrypted wallet storage. It serves digital content creators, artists, subscription services, and API monetization use cases by implementing real-time payment verification with mobile-friendly QR code UIs and both testnet and mainnet support.

The target audience includes developers building paywalled content systems, creative workers seeking cryptocurrency payment integration, and businesses wanting to monetize APIs or digital content without traditional payment processors. The project emphasizes security, simplicity, and self-sovereignty while supporting both Bitcoin (for compatibility) and Monero (for privacy).

## Technical Stack

- **Primary Language**: Go 1.23.2
- **Cryptocurrency Libraries**: 
  - btcsuite/btcd v0.24.2 (Bitcoin HD wallet and network operations)
  - monero-ecosystem/go-monero-rpc-client (Monero RPC integration)
  - opd-ai/wileedot v0.0.0-20241217172720 (rate limiting)
  - sethvargo/go-limiter v1.0.0 (rate limiting middleware)
- **Cryptography**: golang.org/x/crypto v0.31.0 (AES-256-GCM encryption)
- **Web Framework**: Standard library net/http with custom middleware patterns
- **Storage**: File-based JSON storage with AES-256 encryption, in-memory store for testing
- **Build Tools**: Standard Go toolchain with Makefile for formatting (gofumpt) and building
- **Frontend**: Embedded HTML templates with QR code generation via qrcode.min.js

## Code Assistance Guidelines

1. **Follow BIP32/44 HD Wallet Standards**: All Bitcoin wallet operations must comply with BIP32/44 hierarchical deterministic wallet specifications. Use proper key derivation paths, implement secure seed generation with crypto/rand, and maintain deterministic address generation patterns as seen in `wallet/btc_hd_wallet.go`.

2. **Implement Thread-Safe Payment Operations**: All payment store operations require proper mutex protection following the patterns in `memstore.go` and `filestore.go`. Use `sync.RWMutex` for read-heavy operations and `sync.Mutex` for write operations. Ensure concurrent access safety for payment creation, updates, and retrieval.

3. **Maintain Secure Cookie Handling**: Follow the middleware pattern in `middleware.go` using `__Host-` prefixed cookies with `Secure: true`, `HttpOnly: true`, and `SameSite: http.SameSiteStrictMode`. Implement proper cookie expiration and validation for payment session management.

4. **Use Structured Error Handling**: Implement comprehensive error wrapping with `fmt.Errorf("context: %w", err)` patterns. Provide specific error messages for wallet operations, payment verification, and storage failures. Follow the error handling patterns in `paywall.go` and `verification.go`.

5. **Embed Static Assets Properly**: Use Go's `embed` package for templates and static files as demonstrated with `TemplateFS` and `QrcodeJs` in `paywall.go`. Ensure embedded assets are properly validated and provide fallback behavior when resources fail to load.

6. **Implement Cryptocurrency Network Abstraction**: Support both Bitcoin and Monero through the `wallet.HDWallet` interface pattern. Maintain separate client implementations while providing unified payment tracking. Follow the multi-currency approach in `types.go` with `map[wallet.WalletType]` patterns.

7. **Apply Defense-in-Depth Security**: Validate all user inputs, implement secure random ID generation, use AES-256-GCM encryption for sensitive data storage, and maintain separation between testnet and mainnet operations. Follow security patterns in `construct.go` and wallet storage implementations.

## Project Context

- **Domain**: Cryptocurrency payment processing for digital content protection, enabling creators to monetize content through Bitcoin and Monero payments without traditional payment processors
- **Architecture**: HTTP middleware-based architecture with embedded wallet functionality, persistent storage abstraction, and real-time blockchain monitoring for payment verification
- **Key Directories**:
  - `/wallet/` - Bitcoin HD wallet and Monero RPC client implementations with Base58 encoding
  - `/templates/` - Embedded HTML payment page templates with QR code generation
  - `/example/` - Reference implementations including basic server and reverse proxy patterns
  - `/migration/` - Wallet encryption and migration utilities
  - `/docs/` - Project documentation including foundation and marketing materials
- **Configuration**: Environment-based configuration for Monero RPC credentials (`XMR_WALLET_USER`, `XMR_WALLET_PASS`), file-based wallet persistence with AES-256 encryption, and flexible storage backend selection

## Quality Standards

- **Testing Requirements**: While no formal test suite exists currently, implement table-driven tests for all payment verification logic, wallet operations, and storage backends. Maintain high code coverage for security-critical cryptocurrency handling functions. Test both testnet and mainnet scenarios.
- **Code Review Criteria**: All cryptocurrency-related code requires security review focusing on key management, payment validation, and blockchain interaction. Validate proper error handling in payment flows and ensure thread-safety in concurrent operations.
- **Documentation Standards**: Maintain comprehensive godoc documentation for all exported functions, especially payment processing and wallet operations. Include security considerations, parameter validation requirements, and related type references in function documentation. Follow the detailed documentation patterns in existing files.
- **Security Requirements**: Mandatory use of crypto/rand for all random generation, proper input validation for addresses and amounts, secure storage of wallet data with AES-256 encryption, and separation of testnet/mainnet configurations.
- **Cryptocurrency Best Practices**: Follow BIP standards for Bitcoin operations, implement proper dust limit validation, maintain accurate transaction confirmation tracking, and provide clear payment expiration handling. Support both privacy-focused (Monero) and compatibility-focused (Bitcoin) payment options.
