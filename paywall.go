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

	"github.com/btcsuite/btcd/chaincfg"
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

	// Bitcoin RPC configuration (optional - for transaction broadcasting)

	// BTCRPCHost is the Bitcoin RPC server address (e.g., "localhost:18332" for testnet)
	// If empty, transaction broadcasting will be disabled
	BTCRPCHost string
	// BTCRPCUser is the Bitcoin RPC username for authentication
	BTCRPCUser string
	// BTCRPCPass is the Bitcoin RPC password for authentication
	BTCRPCPass string
	// BTCDisableTLS disables TLS verification for Bitcoin RPC (testnet only, insecure)
	BTCDisableTLS bool

	// Multisig configuration (optional - defaults to single-signature mode)

	// MultisigEnabled enables multisig address generation for payments.
	// When false (default), standard single-signature addresses are used.
	// When true, MultisigRequired, MultisigTotal, and ParticipantPubKeys must be provided.
	MultisigEnabled bool

	// MultisigRequired is the number of signatures required (m in m-of-n multisig).
	// Must be >= 2 and <= MultisigTotal when MultisigEnabled is true.
	// Ignored when MultisigEnabled is false.
	MultisigRequired int

	// MultisigTotal is the total number of signers (n in m-of-n multisig).
	// Must be >= MultisigRequired when MultisigEnabled is true.
	// Ignored when MultisigEnabled is false.
	MultisigTotal int

	// ParticipantPubKeys contains public keys for all multisig participants per wallet type.
	// The map keys are wallet types (Bitcoin, Monero) and values are slices of public key bytes.
	// Total number of keys per wallet type must equal MultisigTotal.
	// Required when MultisigEnabled is true, ignored otherwise.
	ParticipantPubKeys map[wallet.WalletType][][]byte

	// MultisigRole identifies this instance's role in multisig transactions.
	// Used for escrow and dispute resolution coordination.
	// Common values: RoleBuyer, RoleSeller, RoleArbiter.
	// Optional: only needed for escrow/dispute resolution workflows.
	MultisigRole MultisigRole

	// AuthorizedArbiters contains the public keys of arbiters authorized to resolve disputes.
	// Each arbiter's public key (in compressed or uncompressed format) grants them
	// permission to participate in dispute resolution.
	// Optional: only required for escrow workflows with dispute resolution.
	// If nil or empty, any arbiter signature will be rejected (no arbitration possible).
	AuthorizedArbiters [][]byte

	// Escrow timeout configuration (optional - for escrow workflows)

	// MinEscrowTimeout is the minimum allowed escrow timeout duration.
	// Prevents creating escrows with unreasonably short timeouts that could
	// bypass proper dispute resolution. Defaults to 24 hours if zero.
	MinEscrowTimeout time.Duration

	// MaxEscrowTimeout is the maximum allowed escrow timeout duration.
	// Prevents creating escrows with unreasonably long timeouts that could
	// lock funds indefinitely. Defaults to 90 days if zero.
	MaxEscrowTimeout time.Duration

	// Multi-arbiter consensus configuration (optional - for advanced dispute resolution)

	// EnableMultiArbiterConsensus enables multi-arbiter voting for dispute resolution.
	// When true, disputes require consensus from multiple arbiters (e.g., 3-of-5).
	// When false (default), single arbiter resolution is used.
	EnableMultiArbiterConsensus bool

	// RequiredArbiterVotes is how many arbiters must agree for consensus (e.g., 3 in 3-of-5).
	// Must be >= 2 when EnableMultiArbiterConsensus is true.
	RequiredArbiterVotes int

	// TotalArbiters is the total number of arbiters in the pool (e.g., 5 in 3-of-5).
	// Must be >= RequiredArbiterVotes when EnableMultiArbiterConsensus is true.
	TotalArbiters int

	// PrimaryArbiters contains public keys of primary arbiters for dispute resolution.
	// Required when EnableMultiArbiterConsensus is true. Length must equal TotalArbiters.
	PrimaryArbiters [][]byte

	// FallbackArbiters contains public keys of backup arbiters used when primary arbiters are unavailable.
	// Optional: if empty, no fallback mechanism is available.
	FallbackArbiters [][]byte

	// ArbiterVotingTimeout is how long arbiters have to vote on a dispute.
	// Defaults to 48 hours if zero. After timeout, fallback arbiters may be activated.
	ArbiterVotingTimeout time.Duration

	// Dispute anti-spam configuration (optional - for preventing griefing attacks)

	// DisputeFeePercent is the percentage of escrow amount charged as a dispute fee.
	// Example: 0.05 means 5% of the escrow amount. Defaults to 0 (no fee).
	// The fee discourages frivolous disputes. Winner of dispute gets fee refunded.
	DisputeFeePercent float64

	// MaxDisputesPerPeriod limits how many disputes a user can file in a time window.
	// Defaults to 0 (unlimited). Recommended: 3-5 disputes per DisputePeriod.
	MaxDisputesPerPeriod int

	// DisputePeriod is the time window for dispute rate limiting.
	// Defaults to 30 days. Used with MaxDisputesPerPeriod to prevent abuse.
	DisputePeriod time.Duration

	// MaxEvidenceSizeBytes is the maximum total evidence size per dispute.
	// Defaults to 10 MB. Prevents DoS via large evidence uploads.
	MaxEvidenceSizeBytes int64

	// ExtendEscrowOnDispute is the additional time added to escrow timeout when dispute is filed.
	// Defaults to 7 days. Prevents exploiting timeout during dispute resolution.
	ExtendEscrowOnDispute time.Duration
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

	// Multisig configuration (optional - defaults to single-signature mode)

	// multisigEnabled indicates whether multisig addresses should be used for payments
	multisigEnabled bool
	// multisigRequired is the number of signatures required (m in m-of-n multisig)
	multisigRequired int
	// multisigTotal is the total number of signers (n in m-of-n multisig)
	multisigTotal int
	// participantPubKeys contains public keys for all multisig participants per wallet type
	participantPubKeys map[wallet.WalletType][][]byte
	// multisigRole identifies this instance's role in multisig transactions (buyer/seller/arbiter)
	multisigRole MultisigRole
	// authorizedArbiters contains public keys of arbiters authorized for dispute resolution
	authorizedArbiters [][]byte

	// Escrow timeout configuration (optional - for escrow workflows)

	// minEscrowTimeout is the minimum allowed escrow timeout duration
	minEscrowTimeout time.Duration
	// maxEscrowTimeout is the maximum allowed escrow timeout duration
	maxEscrowTimeout time.Duration

	// Multi-arbiter consensus (optional - for advanced dispute resolution)

	// consensusManager handles multi-arbiter voting for disputes
	consensusManager *ArbiterConsensusManager

	// Dispute anti-spam configuration (optional - for preventing griefing attacks)

	// disputeFeePercent is the percentage of escrow amount charged as a dispute fee
	disputeFeePercent float64
	// maxDisputesPerPeriod limits disputes per user in time window
	maxDisputesPerPeriod int
	// disputePeriod is the time window for dispute rate limiting
	disputePeriod time.Duration
	// maxEvidenceSizeBytes is the maximum evidence size per dispute
	maxEvidenceSizeBytes int64
	// extendEscrowOnDispute is additional time added when dispute is filed
	extendEscrowOnDispute time.Duration
	// disputeHistory tracks dispute counts per participant for rate limiting
	disputeHistory map[string][]time.Time

	// Transaction broadcasting (optional - for multisig workflows)

	// btcBroadcaster handles Bitcoin transaction broadcasting to the network
	// Initialized if BTCRPCHost is provided in config
	btcBroadcaster *BTCBroadcaster
	// xmrBroadcaster handles Monero transaction broadcasting to the network
	// Initialized if XMR RPC config is provided
	xmrBroadcaster *XMRBroadcaster
}

func validateConfig(config *Config) error {
	if config.PaymentTimeout <= 0 {
		return fmt.Errorf("payment timeout must be positive, got: %s (hint: use time.Hour*24 for 24 hours)", config.PaymentTimeout)
	}

	if config.PriceInBTC < 0 {
		return fmt.Errorf("PriceInBTC must be positive, got: %.8f BTC (hint: set PriceInBTC: 0.0001 or leave at 0 to disable Bitcoin payments)", config.PriceInBTC)
	}

	if config.PriceInXMR < 0 {
		return fmt.Errorf("PriceInXMR must be positive, got: %.8f XMR (hint: set PriceInXMR: 0.01 or leave at 0 to disable Monero payments)", config.PriceInXMR)
	}

	if config.PriceInBTC <= 0 && config.PriceInXMR <= 0 {
		return fmt.Errorf("configuration error: PriceInBTC and PriceInXMR are both zero - at least one cryptocurrency price must be set (hint: set PriceInBTC: 0.0001 or PriceInXMR: 0.01)")
	}

	const minBTCDustLimit = 0.00001
	const minXMRDustLimit = 0.0001
	if config.PriceInBTC > 0 && config.PriceInBTC <= minBTCDustLimit {
		return fmt.Errorf("PriceInBTC %.8f is below dust limit (minimum: %.5f BTC). Dust payments are rejected by the Bitcoin network. Please increase the price", config.PriceInBTC, minBTCDustLimit)
	}

	if config.PriceInXMR > 0 && config.PriceInXMR <= minXMRDustLimit {
		return fmt.Errorf("PriceInXMR %.8f is below dust limit (minimum: %.4f XMR). Dust payments are rejected by the Monero network. Please increase the price", config.PriceInXMR, minXMRDustLimit)
	}

	if config.PriceInXMR > 0 && (config.XMRUser == "" || config.XMRPassword == "" || config.XMRRPC == "") {
		return fmt.Errorf("Monero price set (%.8f XMR) but credentials missing. Required: XMRUser, XMRPassword, and XMRRPC (hint: set XMRUser from XMR_WALLET_USER env, XMRPassword from XMR_WALLET_PASS env, XMRRPC: 'http://localhost:18081')", config.PriceInXMR)
	}

	if (config.XMRUser != "" || config.XMRPassword != "" || config.XMRRPC != "") && config.PriceInXMR <= 0 {
		return fmt.Errorf("Monero RPC credentials provided but PriceInXMR is zero. Set PriceInXMR to enable Monero payments (hint: PriceInXMR: 0.01)")
	}

	if config.MultisigEnabled {
		if config.MultisigRequired < 2 {
			return fmt.Errorf("MultisigRequired must be at least 2 for multisig, got: %d (hint: for 2-of-3 multisig, set MultisigRequired: 2, MultisigTotal: 3)", config.MultisigRequired)
		}
		if config.MultisigTotal < config.MultisigRequired {
			return fmt.Errorf("MultisigTotal (%d) must be >= MultisigRequired (%d). Example: for 2-of-3, set MultisigRequired: 2, MultisigTotal: 3", config.MultisigTotal, config.MultisigRequired)
		}
		if config.ParticipantPubKeys == nil {
			return fmt.Errorf("ParticipantPubKeys required when MultisigEnabled is true (hint: provide public keys for all %d participants)", config.MultisigTotal)
		}
		for walletType, pubKeys := range config.ParticipantPubKeys {
			if len(pubKeys) != config.MultisigTotal {
				return fmt.Errorf("ParticipantPubKeys for %s: expected %d keys (MultisigTotal), got %d. Ensure you provide exactly %d public keys", walletType, config.MultisigTotal, len(pubKeys), config.MultisigTotal)
			}
			for i, key := range pubKeys {
				if len(key) == 0 {
					return fmt.Errorf("ParticipantPubKeys for %s: key at index %d is empty. Each participant must have a non-empty public key", walletType, i)
				}
			}
		}
	}

	if config.MinConfirmations < 1 {
		config.MinConfirmations = 1
	}

	if config.Store == nil {
		return fmt.Errorf("Store is required (hint: use paywall.NewMemoryStore() for testing or paywall.NewFileStore() for production)")
	}

	return nil
}

func initializeWallets(config Config) (map[wallet.WalletType]wallet.HDWallet, map[wallet.WalletType]float64, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, nil, fmt.Errorf("generate seed: %w", err)
	}

	hdWallet, err := wallet.NewBTCHDWallet(seed, config.TestNet, config.MinConfirmations)
	if err != nil {
		return nil, nil, fmt.Errorf("create wallet: %w", err)
	}

	if config.MultisigEnabled {
		if pubKeys, ok := config.ParticipantPubKeys[wallet.Bitcoin]; ok {
			if err := hdWallet.EnableMultisig(pubKeys, config.MultisigRequired); err != nil {
				return nil, nil, fmt.Errorf("enable multisig on Bitcoin wallet: %w", err)
			}
		}
	}

	if config.XMRUser != "" || config.XMRPassword != "" || config.XMRRPC != "" || config.PriceInXMR > 0 {
		if config.XMRUser == "" {
			config.XMRUser = os.Getenv("XMR_WALLET_USER")
		}
		if config.XMRPassword == "" {
			pass, exists := os.LookupEnv("XMR_WALLET_PASS")
			if !exists {
				return nil, nil, fmt.Errorf("XMR wallet password not provided")
			}
			config.XMRPassword = pass
		}
		if config.XMRRPC == "" {
			config.XMRRPC = "http://127.0.0.1:18081"
		}
		if config.XMRUser != "" && len(config.XMRUser) < 3 {
			return nil, nil, fmt.Errorf("XMR RPC username must be at least 3 characters")
		}
		if config.XMRPassword != "" && len(config.XMRPassword) < 8 {
			return nil, nil, fmt.Errorf("XMR RPC password must be at least 8 characters")
		}
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

	return hdWallets, prices, nil
}

func setupMultisig(config Config) (*ArbiterConsensusManager, error) {
	if !config.EnableMultiArbiterConsensus {
		return nil, nil
	}

	arbiterConfig := &ArbiterConfig{
		RequiredArbiterVotes: config.RequiredArbiterVotes,
		TotalArbiters:        config.TotalArbiters,
		PrimaryArbiters:      config.PrimaryArbiters,
		FallbackArbiters:     config.FallbackArbiters,
		VotingTimeout:        config.ArbiterVotingTimeout,
	}
	reputationTracker := NewArbiterReputationTracker()
	consensusManager, err := NewArbiterConsensusManager(arbiterConfig, reputationTracker)
	if err != nil {
		return nil, fmt.Errorf("failed to create arbiter consensus manager: %w", err)
	}
	return consensusManager, nil
}

func startBackgroundWorkers(p *Paywall, hdWallets map[wallet.WalletType]wallet.HDWallet) {
	monitor := &CryptoChainMonitor{
		paywall: p,
		client:  make(map[wallet.WalletType]CryptoClient),
	}
	monitor.client[wallet.Bitcoin] = hdWallets[wallet.Bitcoin]
	if xmrWallet, ok := hdWallets[wallet.Monero]; ok {
		monitor.client[wallet.Monero] = xmrWallet
	}
	p.monitor = monitor
	p.monitor.Start(p.ctx)
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
	if err := validateConfig(&config); err != nil {
		return nil, err
	}

	hdWallets, prices, err := initializeWallets(config)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.ParseFS(TemplateFS, "templates/payment.html")
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	pctx, pcancel := context.WithCancel(context.Background())

	minEscrowTimeout := config.MinEscrowTimeout
	if minEscrowTimeout <= 0 {
		minEscrowTimeout = 24 * time.Hour
	}
	maxEscrowTimeout := config.MaxEscrowTimeout
	if maxEscrowTimeout <= 0 {
		maxEscrowTimeout = 90 * 24 * time.Hour
	}

	p := &Paywall{
		HDWallets:             hdWallets,
		Store:                 config.Store,
		prices:                prices,
		paymentTimeout:        config.PaymentTimeout,
		minConfirmations:      config.MinConfirmations,
		template:              tmpl,
		ctx:                   pctx,
		cancel:                pcancel,
		multisigEnabled:       config.MultisigEnabled,
		multisigRequired:      config.MultisigRequired,
		multisigTotal:         config.MultisigTotal,
		participantPubKeys:    config.ParticipantPubKeys,
		multisigRole:          config.MultisigRole,
		authorizedArbiters:    config.AuthorizedArbiters,
		minEscrowTimeout:      minEscrowTimeout,
		maxEscrowTimeout:      maxEscrowTimeout,
		disputeFeePercent:     config.DisputeFeePercent,
		maxDisputesPerPeriod:  config.MaxDisputesPerPeriod,
		disputePeriod:         config.DisputePeriod,
		maxEvidenceSizeBytes:  config.MaxEvidenceSizeBytes,
		extendEscrowOnDispute: config.ExtendEscrowOnDispute,
		disputeHistory:        make(map[string][]time.Time),
	}

	if p.disputePeriod <= 0 {
		p.disputePeriod = 30 * 24 * time.Hour
	}
	if p.maxEvidenceSizeBytes <= 0 {
		p.maxEvidenceSizeBytes = 10 * 1024 * 1024
	}
	if p.extendEscrowOnDispute <= 0 {
		p.extendEscrowOnDispute = 7 * 24 * time.Hour
	}

	p.consensusManager, err = setupMultisig(config)
	if err != nil {
		pcancel()
		return nil, err
	}

	// Initialize transaction broadcasters if RPC config is provided
	if config.BTCRPCHost != "" {
		chainParams, err := getChaincfgParams(config.TestNet)
		if err != nil {
			pcancel()
			return nil, fmt.Errorf("failed to get chain params: %w", err)
		}
		btcBroadcaster, err := NewBTCBroadcaster(
			config.BTCRPCHost,
			config.BTCRPCUser,
			config.BTCRPCPass,
			!config.BTCDisableTLS,
			chainParams,
		)
		if err != nil {
			log.Printf("Warning: failed to initialize Bitcoin broadcaster: %v", err)
			// Don't fail initialization - broadcasting is optional
		} else {
			p.btcBroadcaster = btcBroadcaster
			log.Printf("Bitcoin transaction broadcaster initialized (RPC: %s)", config.BTCRPCHost)
		}
	}

	// Initialize Monero broadcaster if XMR config is provided
	if config.XMRRPC != "" && config.XMRUser != "" && config.XMRPassword != "" {
		xmrBroadcaster, err := NewXMRBroadcaster(
			config.XMRRPC,
			config.XMRUser,
			config.XMRPassword,
		)
		if err != nil {
			log.Printf("Warning: failed to initialize Monero broadcaster: %v", err)
			// Don't fail initialization - broadcasting is optional
		} else {
			p.xmrBroadcaster = xmrBroadcaster
			log.Printf("Monero transaction broadcaster initialized (RPC: %s)", config.XMRRPC)
		}
	}

	startBackgroundWorkers(p, hdWallets)
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

// getChaincfgParams returns the appropriate Bitcoin chain parameters
func getChaincfgParams(testnet bool) (*chaincfg.Params, error) {
	if testnet {
		return &chaincfg.TestNet3Params, nil
	}
	return &chaincfg.MainNetParams, nil
}

// GetBTCBroadcaster returns the Bitcoin transaction broadcaster if configured
// Returns nil if Bitcoin RPC was not configured or initialization failed
// Users should call this after NewPaywall to set up MultisigCoordinator
func (p *Paywall) GetBTCBroadcaster() *BTCBroadcaster {
	return p.btcBroadcaster
}

// GetXMRBroadcaster returns the Monero transaction broadcaster if configured
// Returns nil if Monero RPC was not configured or initialization failed
// Users should call this after NewPaywall to set up MultisigCoordinator
func (p *Paywall) GetXMRBroadcaster() *XMRBroadcaster {
	return p.xmrBroadcaster
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

	// Initialize multisig fields if multisig is enabled
	if p.multisigEnabled {
		payment.MultisigEnabled = true
		payment.MultisigMetadata = make(map[wallet.WalletType]*wallet.MultisigMetadata)
		payment.RequiredSignatures = make(map[wallet.WalletType]int)
		payment.Signatures = make(map[wallet.WalletType][]SignatureData)
	}

	// Generate addresses for all enabled wallets
	// Track which wallets had addresses generated for rollback on failure
	var generatedWallets []wallet.WalletType
	for walletType, hdWallet := range p.HDWallets {
		var address string
		var err error

		// Use multisig address if enabled, otherwise use standard HD derivation
		if p.multisigEnabled {
			// Get participant public keys for this wallet type
			pubKeys, ok := p.participantPubKeys[walletType]
			if !ok || len(pubKeys) == 0 {
				// Skip this wallet type if no multisig keys configured
				continue
			}

			// Generate multisig address with metadata
			var metadata *wallet.MultisigMetadata
			address, metadata, err = hdWallet.DeriveMultisigAddress(pubKeys, p.multisigRequired)
			if err != nil {
				// Rollback any previously generated addresses
				p.rollbackAddressGeneration(generatedWallets)
				return nil, fmt.Errorf("generate multisig %s address: %w", walletType, err)
			}

			// Store multisig metadata in payment
			payment.MultisigMetadata[walletType] = metadata
			payment.RequiredSignatures[walletType] = p.multisigRequired
		} else {
			// Standard single-signature address derivation
			address, err = hdWallet.DeriveNextAddress()
			if err != nil {
				// Rollback any previously generated addresses
				p.rollbackAddressGeneration(generatedWallets)
				return nil, fmt.Errorf("generate %s address: %w", walletType, err)
			}
		}

		payment.Addresses[walletType] = address
		payment.Amounts[walletType] = p.prices[walletType]
		generatedWallets = append(generatedWallets, walletType)
	}

	// Validate payment has at least one enabled currency
	if len(payment.Addresses) == 0 {
		return nil, fmt.Errorf("no wallets enabled for payment")
	}

	// Store the payment
	if err := p.Store.CreatePayment(payment); err != nil {
		// Rollback address generation on storage failure
		p.rollbackAddressGeneration(generatedWallets)
		return nil, fmt.Errorf("store payment: %w", err)
	}

	return payment, nil
}

// rollbackAddressGeneration decrements the address index for wallets that had addresses generated
// This is used to maintain atomic payment creation by rolling back on failures
func (p *Paywall) rollbackAddressGeneration(walletTypes []wallet.WalletType) {
	for _, walletType := range walletTypes {
		if hdWallet, exists := p.HDWallets[walletType]; exists {
			// Call rollback method on each wallet that had an address generated
			switch w := hdWallet.(type) {
			case *wallet.BTCHDWallet:
				w.RollbackLastAddress()
			case *wallet.MoneroHDWallet:
				w.RollbackLastAddress()
			}
		}
	}
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

// IsAuthorizedArbiter checks if a public key is in the authorized arbiters list
// Parameters:
//   - pubKey: The public key to check (compressed or uncompressed format)
//
// Returns:
//   - bool: true if the public key is authorized, false otherwise
//
// This method performs a byte-wise comparison of the provided public key
// against all authorized arbiter keys. Used for validating arbiter signatures
// in dispute resolution workflows.
//
// Related types: SignatureData, MultisigRole
func (p *Paywall) IsAuthorizedArbiter(pubKey []byte) bool {
	if len(p.authorizedArbiters) == 0 {
		return false
	}
	for _, authorizedKey := range p.authorizedArbiters {
		if bytesEqual(pubKey, authorizedKey) {
			return true
		}
	}
	return false
}

// AddAuthorizedArbiter adds a public key to the list of authorized arbiters
// Parameters:
//   - pubKey: The public key to authorize (compressed or uncompressed format)
//
// Returns:
//   - error: If the public key is invalid or already exists
//
// This method validates that the public key is not empty and not already
// in the authorized list before adding it. The public key should be in
// the same format as used in multisig participant keys.
//
// Related types: SignatureData, MultisigRole
func (p *Paywall) AddAuthorizedArbiter(pubKey []byte) error {
	if len(pubKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}
	if p.IsAuthorizedArbiter(pubKey) {
		return fmt.Errorf("arbiter already authorized")
	}
	p.authorizedArbiters = append(p.authorizedArbiters, pubKey)
	return nil
}

// RemoveAuthorizedArbiter removes a public key from the authorized arbiters list
// Parameters:
//   - pubKey: The public key to remove
//
// Returns:
//   - error: If the public key is not found in the authorized list
//
// This method searches for the public key in the authorized arbiters list
// and removes it if found. Returns an error if the key is not authorized.
//
// Related types: SignatureData, MultisigRole
func (p *Paywall) RemoveAuthorizedArbiter(pubKey []byte) error {
	for i, authorizedKey := range p.authorizedArbiters {
		if bytesEqual(pubKey, authorizedKey) {
			// Remove by swapping with last element and truncating
			p.authorizedArbiters[i] = p.authorizedArbiters[len(p.authorizedArbiters)-1]
			p.authorizedArbiters = p.authorizedArbiters[:len(p.authorizedArbiters)-1]
			return nil
		}
	}
	return fmt.Errorf("arbiter not found in authorized list")
}

// GetAuthorizedArbiters returns a copy of the authorized arbiters list
// Returns:
//   - [][]byte: A slice containing copies of all authorized arbiter public keys
//
// This method returns a defensive copy to prevent external modification
// of the internal authorized arbiters list.
//
// Related types: SignatureData, MultisigRole
func (p *Paywall) GetAuthorizedArbiters() [][]byte {
	if len(p.authorizedArbiters) == 0 {
		return nil
	}
	// Return defensive copy
	result := make([][]byte, len(p.authorizedArbiters))
	for i, key := range p.authorizedArbiters {
		result[i] = make([]byte, len(key))
		copy(result[i], key)
	}
	return result
}

// GetConsensusManager returns the arbiter consensus manager if multi-arbiter mode is enabled
// Returns nil if multi-arbiter consensus is not configured
func (p *Paywall) GetConsensusManager() *ArbiterConsensusManager {
	return p.consensusManager
}

// GetReputationTracker returns the arbiter reputation tracker if multi-arbiter mode is enabled
// Returns nil if multi-arbiter consensus is not configured
func (p *Paywall) GetReputationTracker() *ArbiterReputationTracker {
	if p.consensusManager == nil {
		return nil
	}
	return p.consensusManager.reputationTracker
}

// getRoleForPubKey derives a participant's role from their public key position
// in the participant list. Returns the role based on key position:
//   - Index 0: RoleBuyer (the party paying for goods/services)
//   - Index 1: RoleSeller (the party providing goods/services)
//   - Index 2: RoleArbiter (the neutral third party for disputes)
//
// This prevents role spoofing by verifying the role against the canonical
// participant list rather than trusting user-provided role fields.
//
// Parameters:
//   - pubKey: The public key to look up
//   - walletType: The wallet type (Bitcoin/Monero) to search
//
// Returns:
//   - MultisigRole: The derived role for the public key
//   - error: If the key is not found in the participant list
//
// Related types: MultisigRole, SignatureData
func (p *Paywall) getRoleForPubKey(pubKey []byte, walletType wallet.WalletType) (MultisigRole, error) {
	if !p.multisigEnabled || p.participantPubKeys == nil {
		return "", fmt.Errorf("multisig not enabled or participant keys not configured")
	}

	participants, ok := p.participantPubKeys[walletType]
	if !ok {
		return "", fmt.Errorf("no participants configured for wallet type %s", walletType)
	}

	// Find the public key in the participant list
	for i, participantKey := range participants {
		if bytesEqual(pubKey, participantKey) {
			// Map index to role: 0=buyer, 1=seller, 2=arbiter
			switch i {
			case 0:
				return RoleBuyer, nil
			case 1:
				return RoleSeller, nil
			case 2:
				return RoleArbiter, nil
			default:
				return "", fmt.Errorf("participant at index %d has no defined role", i)
			}
		}
	}

	return "", fmt.Errorf("public key not found in participant list for wallet type %s", walletType)
}

// bytesEqual performs a constant-time comparison of two byte slices
// to prevent timing attacks when comparing sensitive data like keys
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	result := byte(0)
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
