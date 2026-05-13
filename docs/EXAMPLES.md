# Examples

## Basic Bitcoin-Only Paywall

The simplest example: require Bitcoin payment for content access using testnet for development.

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	// Create paywall with minimal configuration
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:      0.0001,                 // 0.0001 BTC (~$5 at market rates)
		TestNet:         true,                   // Use Bitcoin testnet for testing
		Store:           paywall.NewMemoryStore(), // Store payments in memory
		PaymentTimeout:  24 * time.Hour,         // Payment valid for 24 hours
		MinConfirmations: 1,                     // Accept after 1 confirmation (testnet)
	})
	if err != nil {
		log.Fatal(err)
	}
	defer pw.Close()

	// Protected content handler
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("This is your premium content!\n"))
		w.Write([]byte("Thanks for paying with Bitcoin.\n"))
	})

	// Apply paywall middleware to protect the route
	http.Handle("/premium", pw.Middleware(protected))

	// Also provide a status page for debugging
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Paywall is running"))
	})

	log.Println("Paywall running on http://localhost:8000")
	log.Println("Try: curl http://localhost:8000/premium")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
```

**Run it**:
```bash
go run example.go
# Visit: http://localhost:8000/premium
# You'll see a payment page with Bitcoin address and QR code
```

## Production Bitcoin Paywall with Persistent Storage

For a production system, use encrypted file storage and mainnet configuration.

```go
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	// Load encryption key from environment (generated once, stored securely)
	encryptionKey := os.Getenv("PAYWALL_ENCRYPTION_KEY")
	if encryptionKey == "" {
		log.Fatal("PAYWALL_ENCRYPTION_KEY environment variable not set")
	}

	// Create encrypted file storage for persistent payment records
	store, err := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
		DataDir:       "/var/lib/paywall/payments",
		EncryptionKey: encryptionKey,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Production configuration: Bitcoin mainnet, 6 confirmations
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:       0.001,                // 0.001 BTC required
		TestNet:          false,                // Use Bitcoin mainnet
		Store:            store,                // Encrypted persistent storage
		PaymentTimeout:   24 * time.Hour,       // Payment window
		MinConfirmations: 6,                    // Standard confirmation threshold
	})
	if err != nil {
		log.Fatal(err)
	}
	defer pw.Close()

	// Protect premium article
	articleHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
	<title>Premium Article</title>
	<style>
		body { font-family: sans-serif; max-width: 800px; margin: 40px auto; }
		.content { background: #f0f0f0; padding: 20px; border-radius: 8px; }
	</style>
</head>
<body>
	<h1>The Future of Bitcoin Payments</h1>
	<div class="content">
		<p>This is premium content protected by the opd-ai/paywall system.</p>
		<p>You successfully paid with Bitcoin! Thank you for supporting independent journalism.</p>
		<p>Check back in 24 hours for the next article, or subscribe for unlimited access.</p>
	</div>
</body>
</html>
		`))
	})

	// Apply paywall middleware
	http.Handle("/article/future-of-bitcoin", pw.Middleware(articleHandler))

	// Health check endpoint (no paywall required)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	log.Println("Production paywall running on https://example.com")
	log.Fatal(http.ListenAndServeTLS(
		":443",
		"/etc/letsencrypt/live/example.com/fullchain.pem",
		"/etc/letsencrypt/live/example.com/privkey.pem",
		nil,
	))
}
```

**Setup**:
```bash
# Generate encryption key (do this once)
export PAYWALL_ENCRYPTION_KEY=$(openssl rand -hex 32)

# Save it securely (e.g., in 1Password, HashiCorp Vault, AWS Secrets Manager)
# DO NOT commit to Git

# Run with HTTPS
sudo go run production_example.go
```

## Dual-Currency Paywall (Bitcoin + Monero)

Accept both Bitcoin and Monero payments for maximum flexibility and privacy.

```go
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	// Encryption for storage
	encryptionKey := os.Getenv("PAYWALL_ENCRYPTION_KEY")
	store, _ := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
		DataDir:       "./payments",
		EncryptionKey: encryptionKey,
	})

	// Monero RPC credentials (from environment)
	xmrUser := os.Getenv("XMR_WALLET_USER")
	xmrPass := os.Getenv("XMR_WALLET_PASS")
	if xmrUser == "" || xmrPass == "" {
		log.Fatal("XMR_WALLET_USER and XMR_WALLET_PASS required for Monero support")
	}

	// Accept both Bitcoin and Monero
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:     0.001,                  // 0.001 BTC
		PriceInXMR:     0.01,                   // 0.01 XMR (equivalent value)
		TestNet:        true,                   // Test on both networks
		Store:          store,
		PaymentTimeout: 24 * time.Hour,
		MinConfirmations: 1,
		XMRUser:        xmrUser,
		XMRPassword:    xmrPass,
		XMRRPC:         "http://localhost:18081", // Local Monero wallet RPC
	})
	if err != nil {
		log.Fatal(err)
	}
	defer pw.Close()

	// Protected content
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`
<h1>Premium Privacy Content</h1>
<p>You paid using Bitcoin or Monero for this exclusive content.</p>
<p>Your payment ensures creator freedom and user privacy.</p>
		`))
	})

	http.Handle("/video/private", pw.Middleware(protected))

	log.Println("Dual-currency paywall running")
	log.Println("BTC address: (generated per payment)")
	log.Println("XMR address: (generated per payment)")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
```

**Setup Monero RPC**:
```bash
# Start Monero wallet RPC with authentication
monero-wallet-rpc \
    --wallet-file /path/to/wallet \
    --password "wallet_password" \
    --daemon-address http://127.0.0.1:18081 \
    --rpc-bind-port 18081 \
    --rpc-bind-ip 127.0.0.1 \
    --rpc-login paywall_user:monero_rpc_password

# Then run the paywall
export XMR_WALLET_USER="paywall_user"
export XMR_WALLET_PASS="monero_rpc_password"
go run dual_currency.go
```

## Multiple Protected Routes

Protect multiple routes with a single paywall instance.

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	pw, _ := paywall.NewPaywall(paywall.Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          paywall.NewMemoryStore(),
		PaymentTimeout: 24 * time.Hour,
		MinConfirmations: 1,
	})
	defer pw.Close()

	// Multiple protected content handlers
	article := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Article content"))
	})

	video := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Video content"))
	})

	podcast := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Podcast content"))
	})

	// Protect multiple routes with same paywall
	http.Handle("/article", pw.Middleware(article))
	http.Handle("/video", pw.Middleware(video))
	http.Handle("/podcast", pw.Middleware(podcast))

	// Public routes (no paywall)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome! Buy access to premium content."))
	})

	http.HandleFunc("/pricing", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("All content: 0.001 BTC"))
	})

	log.Fatal(http.ListenAndServe(":8000", nil))
}
```

**Routes**:
- `/` — Free welcome page
- `/pricing` — Free pricing info
- `/article` — Protected (paywall required)
- `/video` — Protected (paywall required)
- `/podcast` — Protected (paywall required)

## API Monetization: Per-Endpoint Pricing

Charge different prices for different API endpoints.

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	store := paywall.NewMemoryStore()

	// Tier 1: Basic API access ($0.001 BTC)
	basicPaywall, _ := paywall.NewPaywall(paywall.Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: 720 * time.Hour, // 30 days
	})
	defer basicPaywall.Close()

	// Tier 2: Premium API access ($0.01 BTC)
	premiumPaywall, _ := paywall.NewPaywall(paywall.Config{
		PriceInBTC:     0.01,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: 720 * time.Hour,
	})
	defer premiumPaywall.Close()

	// Basic endpoint: prediction API
	basicAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"prediction": "sunny", "confidence": 0.85}`))
	})

	// Premium endpoint: custom training API
	premiumAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model_id": "custom_xyz", "status": "training", "eta": "2 hours"}`))
	})

	// Public endpoints
	http.HandleFunc("/api/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("API Documentation\n\nBasic: 0.001 BTC\nPremium: 0.01 BTC"))
	})

	// Protected endpoints with different prices
	http.Handle("/api/v1/predict", basicPaywall.Middleware(basicAPI))
	http.Handle("/api/v2/train-model", premiumPaywall.Middleware(premiumAPI))

	log.Println("API Server running on port 8000")
	log.Println("/api/docs - Free documentation")
	log.Println("/api/v1/predict - 0.001 BTC")
	log.Println("/api/v2/train-model - 0.01 BTC")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
```

## Reverse Proxy Pattern (Protect Existing Services)

Use paywall as a reverse proxy to protect an existing application without modifying it.

See [example/reverseproxy/](../example/reverseproxy/) for a complete reverse proxy implementation that:
- Accepts user requests on `localhost:8000`
- Checks payment status via paywall middleware
- Forwards paid requests to backend service on `localhost:3000`
- Returns payment page for unpaid requests

**Quick start**:
```bash
# Terminal 1: Start backend service
python3 -m http.server 3000

# Terminal 2: Start paywall reverse proxy
go run example/reverseproxy/main.go

# Terminal 3: Access via paywall
curl http://localhost:8000/
# Get: Payment page (requires Bitcoin payment)
```

## Testing Without a Full Node

For development, use public blockchain testnet endpoints. The paywall automatically handles endpoint selection:

```go
// No configuration needed for endpoints - paywall selects automatically
pw, _ := paywall.NewPaywall(paywall.Config{
    PriceInBTC: 0.0001,
    TestNet:    true,
    // Paywall will use public endpoints:
    // - blockchain.info (testnet)
    // - BlockCypher (testnet)
    // - Other public explorers
})
```

To test locally without internet:
```bash
# Run a local Bitcoin testnet node
bitcoind -testnet -regtest -rpcuser=test -rpcpassword=test

# Generate blocks
bitcoin-cli -testnet -regtest generatetoaddress 101 "tb1q..."

# Then use local RPC
# (Requires modifying paywall to accept custom RPC endpoint)
```

## Custom Store Implementation

Implement a custom payment store (e.g., PostgreSQL, MongoDB) by implementing the `PaymentStore` interface.

```go
package main

import "github.com/opd-ai/paywall"

// PostgreSQL payment store example
type PostgresPaymentStore struct {
	db *sql.DB
}

// Implement PaymentStore interface
func (s *PostgresPaymentStore) CreatePayment(p *paywall.Payment) error {
	_, err := s.db.Exec(
		"INSERT INTO payments (id, address, amount_btc, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)",
		p.ID, p.Addresses[0], p.Amounts[0], p.CreatedAt, p.ExpiresAt,
	)
	return err
}

func (s *PostgresPaymentStore) GetPaymentByID(id string) (*paywall.Payment, error) {
	// SELECT from database
	// return &payment, nil
}

func (s *PostgresPaymentStore) UpdatePayment(p *paywall.Payment) error {
	// UPDATE database
	return nil
}

func (s *PostgresPaymentStore) ListPendingPayments() ([]*paywall.Payment, error) {
	// SELECT payments WHERE confirmations < 1
	// return payments, nil
}

func (s *PostgresPaymentStore) GetPaymentByAddress(address string) (*paywall.Payment, error) {
	// SELECT from database WHERE address = $1
	// return &payment, nil
}

func (s *PostgresPaymentStore) GetPaymentsByIDs(ids []string) ([]*paywall.Payment, error) {
	// SELECT from database WHERE id IN (...)
	// return payments, nil
}

// Then use it:
func main() {
	db := openPostgresConnection()
	store := &PostgresPaymentStore{db: db}

	pw, _ := paywall.NewPaywall(paywall.Config{
		PriceInBTC: 0.001,
		Store:      store, // Use custom store
	})
	defer pw.Close()
}
```

## Integration with Web Frameworks

### Using with Gin

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/opd-ai/paywall"
)

func main() {
	pw, _ := paywall.NewPaywall(/* config */)
	defer pw.Close()

	router := gin.Default()

	// Convert paywall middleware to Gin middleware
	paymentMiddleware := func(c *gin.Context) {
		w := gin.ResponseWriter(c.Writer) // Gin writer, implements http.ResponseWriter
		r := c.Request

		paymentID, _ := c.Cookie("__Host-payment")
		if paymentID == "" {
			// No payment, show payment page
			c.HTML(200, "payment.html", gin.H{
				"address": "...",
			})
			c.Abort()
			return
		}

		// Verify payment
		payment, _ := pw.Store.GetPaymentByID(paymentID)
		if payment != nil && payment.Status == "confirmed" {
			c.Next()
			return
		}

		c.HTML(200, "payment.html", gin.H{
			"address": "...",
		})
		c.Abort()
	}

	protected := router.Group("/premium")
	protected.Use(paymentMiddleware)
	protected.GET("/article", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"content": "Premium article",
		})
	})

	router.Run(":8000")
}
```

### Using with Echo

```go
package main

import (
	"github.com/labstack/echo/v4"
	"github.com/opd-ai/paywall"
)

func main() {
	pw, _ := paywall.NewPaywall(/* config */)
	defer pw.Close()

	e := echo.New()

	paymentCheck := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			paymentID, _ := c.Cookie("__Host-payment")
			if paymentID == "" {
				return c.String(402, "Payment required")
			}
			return next(c)
		}
	}

	e.GET("/premium", func(c echo.Context) error {
		return c.String(200, "Premium content")
	}, paymentCheck)

	e.Start(":8000")
}
```

## Debugging

### Check Payment Status

```go
// Get a specific payment
payment, _ := pw.Store.GetPaymentByID(paymentID)
log.Printf("Payment %s: %+v", paymentID, payment)
// Output: Payment abc123: {ID:abc123 Address:... Confirmations:3 Status:confirmed ...}
```

### List Pending Payments

```go
// Find all payments waiting for confirmation
pending, _ := pw.Store.ListPendingPayments()
log.Printf("Pending payments: %d", len(pending))
for _, p := range pending {
	log.Printf("  %s: %d confirmations", p.ID, p.Confirmations)
}
```

### Monitor Verification Status

The paywall background goroutine verifies payments continuously. Monitor logs for:

```
// Expected logs
2026/05/12 10:00:00 Payment ID abc123 confirmed: 6 confirmations
2026/05/12 10:00:05 Payment ID xyz789 expired: no confirmations

// Error logs indicate problems
2026/05/12 10:00:10 ERROR: Failed to verify payment abc123: blockchain timeout
```

## Next Steps

- See [CONFIGURATION.md](CONFIGURATION.md) for detailed configuration options
- See [SECURITY.md](SECURITY.md) for security best practices
- See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues
- See [example/](../example/) for complete runnable examples
