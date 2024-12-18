// Package paywall implements a Bitcoin payment system for protecting web content
package paywall

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// TemplateFS embeds the payment page HTML template
//
//go:embed templates/payment.html
var TemplateFS embed.FS

// QrcodeJS embeds the QR code generation JavaScript library
//
//go:embed static/qrcode.min.js
var QrcodeJs embed.FS

// Config defines the configuration options for initializing a Paywall
// All fields are required unless otherwise noted
type Config struct {
	// PriceInBTC is the amount in Bitcoin required for access
	PriceInBTC float64
	// PaymentTimeout is the duration after which pending payments expire
	PaymentTimeout time.Duration
	// MinConfirmations is the required number of blockchain confirmations
	MinConfirmations int
	// TestNet determines whether to use Bitcoin testnet (true) or mainnet (false)
	TestNet bool
	// Store implements the payment persistence interface
	Store PaymentStore
}

// Paywall manages Bitcoin payment processing and verification
// It generates payment addresses, tracks payment status, and validates transactions
// Related types: Config, Payment, PaymentStore, wallet.HDWallet
type Paywall struct {
	// HDWallet generates unique Bitcoin addresses for payments
	HDWallet *wallet.HDWallet
	// store persists payment information
	store PaymentStore
	// priceInBTC is the required payment amount in Bitcoin
	priceInBTC float64
	// paymentTimeout is how long payments can remain pending
	paymentTimeout time.Duration
	// minConfirmations is required blockchain confirmations
	minConfirmations int
	// template is the parsed payment page HTML template
	template *template.Template
}

// NewPaywall creates and initializes a new Paywall instance
// Parameters:
//   - config: Configuration options for the paywall
//
// Returns:
//   - *Paywall: Initialized paywall instance
//   - error: If initialization fails
//
// Errors:
//   - If random seed generation fails
//   - If HD wallet creation fails
//   - If template parsing fails
//
// Related types: Config, Paywall
func NewPaywall(config Config) (*Paywall, error) {
	// Generate random seed for HD wallet
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("generate seed: %w", err)
	}

	hdWallet, err := wallet.NewHDWallet(seed, config.TestNet)
	if err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}

	tmpl, err := template.ParseFS(TemplateFS, "templates/payment.html")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if config.MinConfirmations < 1 {
		config.MinConfirmations = 1
	}

	return &Paywall{
		HDWallet:         hdWallet,
		store:            config.Store,
		priceInBTC:       config.PriceInBTC,
		paymentTimeout:   config.PaymentTimeout,
		minConfirmations: config.MinConfirmations,
		template:         tmpl,
	}, nil
}

// CreatePayment generates a new payment request
// It creates a unique Bitcoin address and payment record
// Returns:
//   - *Payment: The created payment record
//   - error: If address generation or storage fails
//
// The payment starts in StatusPending state and expires after paymentTimeout
// Related types: Payment, PaymentStatus
func (p *Paywall) CreatePayment() (*Payment, error) {
	address, err := p.HDWallet.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	payment := &Payment{
		ID:        generatePaymentID(),
		Address:   address,
		AmountBTC: p.priceInBTC,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(p.paymentTimeout),
		Status:    StatusPending,
	}

	if err := p.store.CreatePayment(payment); err != nil {
		return nil, fmt.Errorf("failed to store payment: %w", err)
	}

	return payment, nil
}

// generatePaymentID creates a random 16-byte hex-encoded payment identifier
// Returns:
//   - string: A 32-character hexadecimal string
//
// This is an internal helper function that uses crypto/rand for secure randomness
func generatePaymentID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
