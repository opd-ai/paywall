// Package main provides an example implementation of a Bitcoin paywall-protected HTTP server
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// Command-line flag for wallet seed initialization
var seed = flag.String("seed", "", "Sequence of bytes to use as a seed for the wallet")

// main initializes and runs a Bitcoin paywall-protected HTTP server.
// It demonstrates:
// - Wallet creation and persistence
// - Paywall configuration
// - HTTP middleware integration
// - Basic payment tracking
//
// The server protects content at /protected endpoint with Bitcoin payments

func main() {
	flag.Parse()
	key, err := wallet.GenerateEncryptionKey()
	if err != nil {
		log.Fatal(err)
	}

	config := wallet.StorageConfig{
		DataDir:       "./paywallet",
		EncryptionKey: key,
	}

	// Initialize paywall with minimal config
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:     0.001,            // 0.001 BTC
		TestNet:        true,             // Use testnet
		Store:          NewMemoryStore(), // Required for payment tracking
		PaymentTimeout: time.Hour * 24,
	})
	// Attempt to load wallet from disk, if it fails store the new one
	if HDWallet, err := wallet.LoadFromFile(config); err != nil {
		// Save newly generated wallet
		if err := pw.HDWallet.SaveToFile(config); err != nil {
			log.Fatal(err)
		}
	} else {
		// Load stored wallet from disk
		pw.HDWallet = HDWallet
	}

	// Protected content handler
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Protected content"))
	})

	// Apply paywall middleware
	http.Handle("/protected", pw.Middleware(protected))

	log.Println("Server starting on :8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}

// MemoryStore implements paywall.Store interface for in-memory payment tracking.
// This is a minimal implementation for demonstration purposes.
//
// Warning: Data is not persisted and will be lost on server restart
type MemoryStore struct{}

// NewMemoryStore creates a new in-memory payment store instance.
//
// Returns:
//   - *MemoryStore: Empty payment store
//
// Related: paywall.Store interface
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// CreatePayment stores a new payment record.
//
// Parameters:
//   - p: Payment record to store
//
// Returns:
//   - error: Always returns nil in this implementation
//
// Note: This is a minimal implementation that doesn't actually store data
func (m *MemoryStore) CreatePayment(p *paywall.Payment) error { return nil }

// GetPayment retrieves a payment record by ID.
//
// Parameters:
//   - id: Payment identifier
//
// Returns:
//   - *paywall.Payment: Always nil in this implementation
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPayment(id string) (*paywall.Payment, error) { return nil, nil }

// UpdatePayment updates an existing payment record.
//
// Parameters:
//   - p: Payment record with updated fields
//
// Returns:
//   - error: Always nil in this implementation
func (m *MemoryStore) UpdatePayment(p *paywall.Payment) error { return nil }

// ListPendingPayments returns all pending payment records.
//
// Returns:
//   - []*paywall.Payment: Always nil in this implementation
//   - error: Always nil in this implementation
func (m *MemoryStore) ListPendingPayments() ([]*paywall.Payment, error) { return nil, nil }

// GetPaymentByAddress retrieves a payment record by Bitcoin address.
//
// Parameters:
//   - addr: Bitcoin address associated with the payment
//
// Returns:
//   - *paywall.Payment: Always nil in this implementation
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPaymentByAddress(addr string) (*paywall.Payment, error) { return nil, nil }
