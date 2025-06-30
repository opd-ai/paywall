// Package paywall implements a Bitcoin payment system for protecting web content
package paywall

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"os"
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
	// PriceInXMR is the amount in Monero required for access
	PriceInXMR float64
	// PaymentTimeout is the duration after which pending payments expire
	PaymentTimeout time.Duration
	// MinConfirmations is the required number of blockchain confirmations
	MinConfirmations int
	// TestNet determines whether to use Bitcoin testnet (true) or mainnet (false)
	TestNet bool
	// Store implements the payment persistence interface
	Store PaymentStore
	// XMRUser is the monero-rpc username
	XMRUser string
	// XMRPassword is the monero-rpc password
	XMRPassword string
	// XMRRPC is the monero-rpc URL
	XMRRPC string
}

// Paywall manages Bitcoin payment processing and verification
// It generates payment addresses, tracks payment status, and validates transactions
// Related types: Config, Payment, PaymentStore, wallet.HDWallet
type Paywall struct {
	// HDWallets generates unique Bitcoin or XMR addresses for payments
	HDWallets map[wallet.WalletType]wallet.HDWallet
	// Store persists payment information
	Store PaymentStore
	// prices is the required payment amount in crypto per wallet
	prices map[wallet.WalletType]float64
	// paymentTimeout is how long payments can remain pending
	paymentTimeout time.Duration
	// minConfirmations is required blockchain confirmations
	minConfirmations int
	// template is the parsed payment page HTML template
	template *template.Template
	// monitor is the blockchain monitoring service
	monitor *CryptoChainMonitor
	// ctx is the context for monitoring goroutine
	ctx context.Context
	// cancel is the context cancellation function
	cancel context.CancelFunc
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
	// validate payment timeout
	if config.PaymentTimeout <= 0 {
		return nil, fmt.Errorf("payment timeout must be positive")
	}
	// Generate random seed for HD wallet
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("generate seed: %w", err)
	}

	hdWallet, err := wallet.NewBTCHDWallet(seed, config.TestNet, config.MinConfirmations)
	if err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	if config.XMRUser == "" {
		config.XMRUser = os.Getenv("XMR_WALLET_USER")
	}
	// Use secure environment variable handling
	if config.XMRPassword == "" {
		pass, exists := os.LookupEnv("XMR_WALLET_PASS")
		if !exists {
			return nil, fmt.Errorf("XMR wallet password not provided")
		}
		config.XMRPassword = pass
	}
	if config.XMRRPC == "" {
		config.XMRRPC = "http://127.0.0.1:18081"
	}
	// Add credential validation
	if config.XMRUser != "" && len(config.XMRUser) < 3 {
		return nil, fmt.Errorf("XMR RPC username must be at least 3 characters")
	}
	if config.XMRPassword != "" && len(config.XMRPassword) < 8 {
		return nil, fmt.Errorf("XMR RPC password must be at least 8 characters")
	}

	xmrHdWallet, err := wallet.NewMoneroWallet(wallet.MoneroConfig{
		RPCUser:     config.XMRUser,
		RPCURL:      config.XMRRPC,
		RPCPassword: config.XMRPassword,
	}, config.MinConfirmations)
	if err != nil {
		log.Printf("error creating XMR wallet %s,\n\tXMR will be disabled", err)
	}

	tmpl, err := template.ParseFS(TemplateFS, "templates/payment.html")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if config.MinConfirmations < 1 {
		config.MinConfirmations = 1
	}
	hdWallets := make(map[wallet.WalletType]wallet.HDWallet)
	hdWallets[wallet.WalletType(hdWallet.Currency())] = hdWallet
	if xmrHdWallet != nil {
		hdWallets[wallet.WalletType(xmrHdWallet.Currency())] = xmrHdWallet
	}
	prices := make(map[wallet.WalletType]float64)
	prices[wallet.WalletType(hdWallet.Currency())] = config.PriceInBTC
	if xmrHdWallet != nil {
		prices[wallet.WalletType(xmrHdWallet.Currency())] = config.PriceInXMR
	}
	// Create context with cancellation
	pctx, pcancel := context.WithCancel(context.Background())
	p := &Paywall{
		HDWallets:        hdWallets,
		Store:            config.Store,
		prices:           prices,
		paymentTimeout:   config.PaymentTimeout,
		minConfirmations: config.MinConfirmations,
		template:         tmpl,
		ctx:              pctx,
		cancel:           pcancel,
	}
	// Initialize monitor
	monitor := &CryptoChainMonitor{
		paywall: p,
		client:  make(map[wallet.WalletType]CryptoClient),
	}
	monitor.client[wallet.Bitcoin] = hdWallets[wallet.Bitcoin]
	if xmrHdWallet != nil {
		monitor.client[wallet.Monero] = hdWallets[wallet.Monero]
	}
	p.monitor = monitor
	p.monitor.Start(pctx)
	return p, nil
}

func (p *Paywall) Close() {
	p.cancel()
	p.monitor.Close()
}

func (p *Paywall) btcWalletAddress() (string, error) {
	return p.HDWallets[wallet.Bitcoin].GetAddress()
}

func (p *Paywall) xmrWalletAddress() (string, error) {
	if _, ok := p.HDWallets[wallet.Monero]; !ok {
		log.Printf("Warning: XMR wallet is not in use, your privacy is sub-optimal")
		return "", nil
	}
	xmrAddress, err := p.HDWallets[wallet.Monero].GetAddress()
	if err != nil {
		return "", fmt.Errorf("failed to get XMR address: %w", err)
	}
	return xmrAddress, nil
}

func (p *Paywall) addressMap() (map[wallet.WalletType]string, error) {
	btcAddress, err := p.btcWalletAddress()
	if err != nil {
		return nil, err
	}
	xmrAddress, err := p.xmrWalletAddress()
	if err != nil {
		return nil, err
	}
	addresses := make(map[wallet.WalletType]string)
	addresses[wallet.Bitcoin] = btcAddress
	if xmrAddress != "" {
		addresses[wallet.Monero] = xmrAddress
	}
	return addresses, nil
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
	addresses, err := p.addressMap()
	if err != nil {
		return nil, err
	}

	paymentID, err := generatePaymentID()
	if err != nil {
		return nil, err
	}

	payment := &Payment{
		ID:        paymentID,
		Addresses: addresses,
		Amounts:   p.prices,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(p.paymentTimeout),
		Status:    StatusPending,
	}

	if err := p.Store.CreatePayment(payment); err != nil {
		return nil, fmt.Errorf("failed to store payment: %w", err)
	}

	return payment, nil
}

// generatePaymentID creates a random 16-byte hex-encoded payment identifier
// Returns:
//   - string: A 32-character hexadecimal string
//   - error: If random generation fails
//
// This is an internal helper function that uses crypto/rand for secure randomness
func generatePaymentID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate secure random payment ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}
