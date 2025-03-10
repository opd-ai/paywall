# Crypto Paywall Reverse Proxy

A powerful reverse proxy service that adds cryptocurrency paywall protection to any HTTP application. This Go-based solution enables you to monetize API endpoints or web content by requiring Bitcoin (BTC) or Monero (XMR) payments before granting access.

## Features

- 🔒 Protect specific paths with cryptocurrency payments
- 💰 Accept payments in both Bitcoin and Monero
- ⚡ Rate limiting support out of the box
- 🔐 Optional SSL/TLS support via Let's Encrypt
- 🎯 Easy to configure and deploy
- 🌐 Works with any HTTP-based backend service

## Installation

```bash
# Clone the repository
git clone https://github.com/opd-ai/paywall
cd paywall/example/reverseproxy

# Build the binary
go build -o crypto-proxy
```

## Quick Start

```bash
# Basic usage - protect a local service
./crypto-proxy -target http://localhost:3000 -protected-path /api

# Using testnet for development
./crypto-proxy -target http://localhost:3000 -testnet -price-in-btc 0.0001
```

## Configuration Options

| Flag | Description | Default |
|------|-------------|---------|
| `-target` | Target server URL | `http://localhost:3000` |
| `-protected-path` | Path requiring payment | `/protected` |
| `-price-in-btc` | Price in BTC | `0.0001` |
| `-price-in-xmr` | Price in XMR | `0.01` |
| `-payment-timeout` | Payment timeout duration | `10m` |
| `-min-confirmations` | Required blockchain confirmations | `1` |
| `-testnet` | Use Bitcoin testnet | `false` |
| `-hostname` | Server hostname | `localhost` |
| `-port` | Server port | `8080` |
| `-letsencrypt` | Enable Let's Encrypt SSL | `false` |
| `-email` | Email for Let's Encrypt | `""` |
| `-cert-dir` | Certificate directory | `./` |
| `-tokens` | Rate limit tokens | `15` |
| `-interval` | Rate limit interval | `1m` |

## Advanced Usage Examples

### Protecting an API with SSL

```bash
./crypto-proxy \
  -target http://api.internal:8000 \
  -protected-path /v1 \
  -hostname api.example.com \
  -letsencrypt true \
  -email admin@example.com \
  -price-in-btc 0.001
```

### Development Setup with Testnet

```bash
./crypto-proxy \
  -target http://localhost:3000 \
  -testnet \
  -price-in-btc 0.0001 \
  -payment-timeout 5m \
  -min-confirmations 1
```

## Architecture

The proxy works by:

1. Intercepting requests to protected paths
2. Enforcing cryptocurrency payments via the paywall middleware
3. Forwarding validated requests to the target server
4. Managing rate limiting and SSL termination

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Setup

```bash
# Install dependencies
go mod download

# Run tests
go test ./...
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Security Considerations

- Always use SSL in production environments
- Configure appropriate rate limits
- Monitor blockchain confirmations based on payment amounts
- Keep private keys secure when using testnet/mainnet

## Acknowledgments

- Built with the [paywall](https://github.com/opd-ai/paywall) package
- Uses [wileedot](https://github.com/opd-ai/wileedot) for Let's Encrypt integration
- Rate limiting provided by [go-limiter](https://github.com/sethvargo/go-limiter)

---

For issues, feature requests, or support, please file an issue on the GitHub repository.