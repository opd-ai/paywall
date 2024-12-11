package main

import (
	"log"
	"net/http"
	"time"

	"flag"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

var seed = flag.String("seed", "", "Sequence of bytes to use as a seed for the wallet")

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

// Minimal in-memory store implementation (required)
type MemoryStore struct{}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (m *MemoryStore) CreatePayment(p *paywall.Payment) error                    { return nil }
func (m *MemoryStore) GetPayment(id string) (*paywall.Payment, error)            { return nil, nil }
func (m *MemoryStore) UpdatePayment(p *paywall.Payment) error                    { return nil }
func (m *MemoryStore) ListPendingPayments() ([]*paywall.Payment, error)          { return nil, nil }
func (m *MemoryStore) GetPaymentByAddress(addr string) (*paywall.Payment, error) { return nil, nil }
