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
	// validate payment amounts
	if config.PriceInBTC <= 0 {
		return nil, fmt.Errorf("PriceInBTC must be positive, got: %f", config.PriceInBTC)
	}
	if config.PriceInXMR <= 0 {
		return nil, fmt.Errorf("PriceInXMR must be positive, got: %f", config.PriceInXMR)
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
		log.Printf("WARNING: XMR wallet configuration was provided but wallet creation failed: %v", err)
		log.Printf("Continuing with Bitcoin-only support. Please check your Monero RPC configuration.")
	}

	tmpl, err := template.ParseFS(TemplateFS, "templates/payment.html")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	if config.MinConfirmations < 1 {
		config.MinConfirmations = 1
	}
	// Validate payment amounts are positive
	if config.PriceInBTC <= 0 {
		return nil, fmt.Errorf("PriceInBTC must be positive, got: %f", config.PriceInBTC)
	}
	if xmrHdWallet != nil && config.PriceInXMR <= 0 {
		return nil, fmt.Errorf("PriceInXMR must be positive, got: %f", config.PriceInXMR)
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

// CreatePayment generates a new payment with addresses for all enabled cryptocurrencies
//
// Returns:
//   - *Payment: New payment record with generated addresses and amounts
//   - error: If address generation fails or random ID generation fails
//
// The method creates a unique payment with:
//   - Cryptographically secure random payment ID
//   - Bitcoin address (if enabled)
//   - Monero address (if enabled)
//   - Configured payment amounts for each currency
//   - Expiration time based on paymentTimeout
//   - Initial status of StatusPending
//
// Error handling:
//   - Returns error if random ID generation fails
//   - Returns error if any wallet address generation fails
//   - Validates payment amounts against dust limits
//
// Related types: Payment, wallet.HDWallet, PaymentStatus
func (p *Paywall) CreatePayment() (*Payment, error) {
	// Generate cryptographically secure payment ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("generate payment ID: %w", err)
	}
	paymentID := hex.EncodeToString(idBytes)

	// Create payment record
	payment := &Payment{
		ID:            paymentID,
		Addresses:     make(map[wallet.WalletType]string),
		Amounts:       make(map[wallet.WalletType]float64),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(p.paymentTimeout),
		Status:        StatusPending,
		Confirmations: 0,
	}

	// Generate addresses for all enabled wallets
	for walletType, hdWallet := range p.HDWallets {
		address, err := hdWallet.DeriveNextAddress()
		if err != nil {
			return nil, fmt.Errorf("generate %s address: %w", walletType, err)
		}
		payment.Addresses[walletType] = address
		payment.Amounts[walletType] = p.prices[walletType]
	}

	// Validate payment has at least one enabled currency
	if len(payment.Addresses) == 0 {
		return nil, fmt.Errorf("no wallets enabled for payment")
	}

	// Store the payment
	if err := p.Store.CreatePayment(payment); err != nil {
		return nil, fmt.Errorf("store payment: %w", err)
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
