// paywall.go
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

//go:embed templates/payment.html
var TemplateFS embed.FS

type Config struct {
	PriceInBTC       float64
	PaymentTimeout   time.Duration
	MinConfirmations int
	TestNet          bool
	Store            PaymentStore
}

type Paywall struct {
	HDWallet         *wallet.HDWallet
	store            PaymentStore
	priceInBTC       float64
	paymentTimeout   time.Duration
	minConfirmations int
	template         *template.Template
}

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

	return &Paywall{
		HDWallet:         hdWallet,
		store:            config.Store,
		priceInBTC:       config.PriceInBTC,
		paymentTimeout:   config.PaymentTimeout,
		minConfirmations: config.MinConfirmations,
		template:         tmpl,
	}, nil
}

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

// Helper function to generate payment ID
func generatePaymentID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
