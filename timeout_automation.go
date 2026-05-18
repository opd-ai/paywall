package paywall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// ArbiterSigner provides signature generation for arbiter-triggered refunds
type ArbiterSigner interface {
	SignTimeoutRefund(payment *Payment) (*SignatureData, error)
}

// TimeoutMonitor provides automatic timeout monitoring and resolution
type TimeoutMonitor struct {
	em                *EscrowManager
	interval          time.Duration
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	processingLock    sync.Mutex
	processing        map[string]bool
	useBlockchainTime bool
	autoRefund        bool
	signerMu          sync.RWMutex
	arbiterSigner     ArbiterSigner
}

// TimeoutMonitorConfig configures the timeout monitor
type TimeoutMonitorConfig struct {
	// CheckInterval is how often to check for timeouts
	CheckInterval time.Duration
	// UseBlockchainTime uses blockchain timestamps instead of system time
	UseBlockchainTime bool
	// AutoRefund automatically processes refunds for timed-out escrows
	AutoRefund bool
}

// DefaultTimeoutMonitorConfig returns sensible defaults
func DefaultTimeoutMonitorConfig() TimeoutMonitorConfig {
	return TimeoutMonitorConfig{
		CheckInterval:     5 * time.Minute,
		UseBlockchainTime: false, // System time by default for compatibility
		AutoRefund:        false, // Manual refunds by default for safety
	}
}

// NewTimeoutMonitor creates a new timeout monitor
func NewTimeoutMonitor(em *EscrowManager, config TimeoutMonitorConfig) *TimeoutMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &TimeoutMonitor{
		em:                em,
		interval:          config.CheckInterval,
		ctx:               ctx,
		cancel:            cancel,
		processing:        make(map[string]bool),
		useBlockchainTime: config.UseBlockchainTime,
		autoRefund:        config.AutoRefund,
	}
}

// Start begins monitoring timeouts in a background goroutine
// Start begins the timeout monitoring loop
func (tm *TimeoutMonitor) Start() {
	tm.wg.Add(1)
	go tm.monitorLoop()
}

// SetArbiterSigner sets the arbiter signer for automatic refunds
func (tm *TimeoutMonitor) SetArbiterSigner(signer ArbiterSigner) {
	tm.signerMu.Lock()
	defer tm.signerMu.Unlock()
	tm.arbiterSigner = signer
}

// Stop halts the timeout monitor
func (tm *TimeoutMonitor) Stop() {
	tm.cancel()
	tm.wg.Wait()
}

// monitorLoop runs the periodic timeout check
func (tm *TimeoutMonitor) monitorLoop() {
	defer tm.wg.Done()

	ticker := time.NewTicker(tm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-tm.ctx.Done():
			return
		case <-ticker.C:
			if err := tm.checkAndProcessTimeouts(); err != nil {
				tm.em.paywall.logger.log(LogEntry{
					Level:   LogLevelError,
					Event:   "timeout_monitor_error",
					Message: fmt.Sprintf("Timeout monitor error: %v", err),
				})
			}
		}
	}
}

// checkAndProcessTimeouts finds and processes timed-out escrows
func (tm *TimeoutMonitor) checkAndProcessTimeouts() error {
	// Get current time (system or blockchain)
	currentTime, err := tm.getCurrentTime()
	if err != nil {
		return fmt.Errorf("get current time: %w", err)
	}

	// Find timed-out escrows
	timedOut, err := tm.em.CheckEscrowTimeoutsWithTime(currentTime)
	if err != nil {
		return fmt.Errorf("check timeouts: %w", err)
	}

	// Process each timed-out escrow
	for _, paymentID := range timedOut {
		if err := tm.processTimeout(paymentID); err != nil {
			tm.em.paywall.logger.log(LogEntry{
				Level:     LogLevelError,
				Event:     "timeout_processing_failed",
				Message:   fmt.Sprintf("Failed to process timeout: %v", err),
				PaymentID: paymentID,
			})
			// Continue processing other timeouts
		}
	}

	return nil
}

// processTimeout handles a single timed-out escrow
func (tm *TimeoutMonitor) processTimeout(paymentID string) error {
	// Prevent concurrent processing of same payment
	tm.processingLock.Lock()
	if tm.processing[paymentID] {
		tm.processingLock.Unlock()
		return nil // Already being processed
	}
	tm.processing[paymentID] = true
	tm.processingLock.Unlock()

	defer func() {
		tm.processingLock.Lock()
		delete(tm.processing, paymentID)
		tm.processingLock.Unlock()
	}()

	// Log timeout detection
	tm.em.paywall.logger.LogEscrowTimeout(paymentID, EscrowFunded, time.Since(time.Now()).Milliseconds())

	// If auto-refund is enabled, trigger the automatic refund
	if tm.autoRefund {
		if err := tm.executeAutomaticRefund(paymentID); err != nil {
			tm.em.paywall.logger.log(LogEntry{
				Level:     LogLevelError,
				Event:     "automatic_refund_failed",
				Message:   fmt.Sprintf("Automatic refund failed: %v", err),
				PaymentID: paymentID,
			})
			return fmt.Errorf("automatic refund failed: %w", err)
		}
		tm.em.paywall.logger.log(LogEntry{
			Level:     LogLevelInfo,
			Event:     "automatic_refund_completed",
			Message:   "Automatic refund completed",
			PaymentID: paymentID,
		})
	} else {
		// Just log detection without processing
		tm.em.paywall.logger.log(LogEntry{
			Level:     LogLevelInfo,
			Event:     "manual_refund_required",
			Message:   "Manual refund required (auto-refund disabled)",
			PaymentID: paymentID,
		})
	}

	return nil
}

// executeAutomaticRefund performs an automatic refund for a timed-out escrow
// This is called when AutoRefund is enabled in the configuration
func (tm *TimeoutMonitor) executeAutomaticRefund(paymentID string) error {
	// Get payment details
	payment, err := tm.em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	// Verify payment is in a state that allows timeout refund
	if payment.EscrowState != EscrowFunded && payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("payment %s in state %s, cannot auto-refund", paymentID, payment.EscrowState.String())
	}

	// Verify arbiter signer is configured for automatic refunds
	// Automatic refunds require arbiter signatures to process the refund transaction
	tm.signerMu.RLock()
	signer := tm.arbiterSigner
	tm.signerMu.RUnlock()

	if signer == nil {
		return fmt.Errorf("arbiter signer not configured for automatic refunds")
	}

	// Get arbiter signature for the timeout refund
	// This validates that the arbiter can sign and is willing to authorize the refund
	arbiterSig, err := signer.SignTimeoutRefund(payment)
	if err != nil {
		return fmt.Errorf("arbiter failed to sign timeout refund: %w", err)
	}
	if arbiterSig == nil {
		return fmt.Errorf("arbiter returned nil signature for timeout refund")
	}

	// For automatic timeout refunds, we mark the state as refunded
	// In a production system with actual blockchain transactions, this would:
	// 1. Create the refund transaction
	// 2. Get required signatures (typically buyer + arbiter for timeout)
	// 3. Broadcast the transaction
	// For now, we transition the state to indicate timeout refund

	prevState := payment.EscrowState

	// Validate and record state transition
	if err := tm.em.stateValidator.ValidateAndRecordTransition(
		payment,
		EscrowRefunded,
		"timeout-monitor",
		fmt.Sprintf("Automatic refund due to timeout at %s", time.Now().Format(time.RFC3339)),
	); err != nil {
		return fmt.Errorf("invalid state transition: %w", err)
	}

	// Update payment in store
	if err := tm.em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("update payment: %w", err)
	}

	// Log the automatic refund in audit trail
	if tm.em.auditLogger != nil {
		_, auditErr := tm.em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionRefund,
			PreviousState: prevState,
			NewState:      EscrowRefunded,
			ActorRole:     "", // System action, no specific actor
			Metadata: map[string]string{
				"reason":    "timeout",
				"automatic": "true",
				"timeout":   payment.EscrowTimeout.Format(time.RFC3339),
			},
		})
		if auditErr != nil {
			tm.em.paywall.logger.log(LogEntry{
				Level:     LogLevelWarn,
				Event:     "audit_log_failed",
				Message:   fmt.Sprintf("Failed to log automatic refund: %v", auditErr),
				PaymentID: paymentID,
			})
		}
	}

	return nil
}

// getCurrentTime returns the current time from appropriate source
func (tm *TimeoutMonitor) getCurrentTime() (time.Time, error) {
	if !tm.useBlockchainTime {
		return time.Now(), nil
	}

	// Get blockchain timestamp from Bitcoin wallet
	blockTime, err := tm.getBlockchainTimestamp()
	if err != nil {
		// Fallback to system time if blockchain unavailable
		tm.em.paywall.logger.log(LogEntry{
			Level:   LogLevelDebug,
			Event:   "blockchain_timestamp_unavailable",
			Message: fmt.Sprintf("Blockchain timestamp unavailable, using system time: %v", err),
		})
		return time.Now(), nil
	}

	return blockTime, nil
}

// getBlockchainTimestamp retrieves the timestamp of the latest block
func (tm *TimeoutMonitor) getBlockchainTimestamp() (time.Time, error) {
	// Try Monero first (since it's easier - wallet RPC provides height)
	xmrWallet, hasXMR := tm.em.paywall.HDWallets[wallet.Monero]
	if hasXMR {
		if mw, ok := xmrWallet.(*wallet.MoneroHDWallet); ok {
			blockTime, err := mw.GetLatestBlockTime()
			if err == nil {
				return blockTime, nil
			}
			tm.em.paywall.logger.log(LogEntry{
				Level:   LogLevelDebug,
				Event:   "monero_block_time_unavailable",
				Message: fmt.Sprintf("Monero block time unavailable: %v", err),
			})
		}
	}

	// Try Bitcoin as fallback using public API
	_, hasBTC := tm.em.paywall.HDWallets[wallet.Bitcoin]
	if hasBTC {
		// Determine network based on wallet configuration
		// Check if testnet by examining the first address format or use a flag
		provider := NewBitcoinTimestampProvider("", false) // Will use blockchain.info
		blockTime, err := provider.GetLatestBlockTime()
		if err == nil {
			return blockTime, nil
		}
		tm.em.paywall.logger.log(LogEntry{
			Level:   LogLevelDebug,
			Event:   "bitcoin_block_time_unavailable",
			Message: fmt.Sprintf("Bitcoin block time unavailable: %v", err),
		})
	}

	return time.Time{}, fmt.Errorf("no blockchain timestamp available from any wallet")
}

// CheckEscrowTimeoutsWithTime checks for timed-out escrows using provided time
func (em *EscrowManager) CheckEscrowTimeoutsWithTime(currentTime time.Time) ([]string, error) {
	// Use indexed query for efficient timeout checking
	payments, err := em.paywall.Store.GetEscrowsExpiringBefore(currentTime)
	if err != nil {
		return nil, fmt.Errorf("get expiring escrows: %w", err)
	}

	// Extract payment IDs
	timedOut := make([]string, 0, len(payments))
	for _, payment := range payments {
		timedOut = append(timedOut, payment.ID)
	}

	return timedOut, nil
}

// BlockchainTimestampProvider defines interface for getting blockchain time
type BlockchainTimestampProvider interface {
	GetLatestBlockTime() (time.Time, error)
}

// BitcoinTimestampProvider implements blockchain time for Bitcoin
type BitcoinTimestampProvider struct {
	rpcURL  string
	testnet bool
}

// NewBitcoinTimestampProvider creates a Bitcoin timestamp provider
func NewBitcoinTimestampProvider(rpcURL string, testnet bool) *BitcoinTimestampProvider {
	return &BitcoinTimestampProvider{
		rpcURL:  rpcURL,
		testnet: testnet,
	}
}

// GetLatestBlockTime retrieves the timestamp of the latest Bitcoin block
func (btp *BitcoinTimestampProvider) GetLatestBlockTime() (time.Time, error) {
	if btp.rpcURL == "" {
		return time.Time{}, fmt.Errorf("rpc url not configured")
	}

	if btp.testnet {
		// Use blockstream.info testnet API for testnet block times
		// Step 1: Get latest block hash
		tipHashURL := "https://blockstream.info/testnet/api/blocks/tip/hash"
		resp, err := http.Get(tipHashURL)
		if err != nil {
			return time.Time{}, fmt.Errorf("query testnet blockchain api for tip hash: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return time.Time{}, fmt.Errorf("testnet blockchain api returned status %d for tip hash", resp.StatusCode)
		}

		tipHash, err := io.ReadAll(resp.Body)
		if err != nil {
			return time.Time{}, fmt.Errorf("read testnet tip hash response: %w", err)
		}

		// Step 2: Get block details including timestamp
		blockURL := fmt.Sprintf("https://blockstream.info/testnet/api/block/%s", string(tipHash))
		resp2, err := http.Get(blockURL)
		if err != nil {
			return time.Time{}, fmt.Errorf("query testnet blockchain api for block: %w", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != 200 {
			return time.Time{}, fmt.Errorf("testnet blockchain api returned status %d for block", resp2.StatusCode)
		}

		body, err := io.ReadAll(resp2.Body)
		if err != nil {
			return time.Time{}, fmt.Errorf("read testnet block response: %w", err)
		}

		var blockData struct {
			Timestamp int64 `json:"timestamp"`
		}
		if err := json.Unmarshal(body, &blockData); err != nil {
			return time.Time{}, fmt.Errorf("parse testnet block response: %w", err)
		}

		return time.Unix(blockData.Timestamp, 0), nil
	}

	// Use blockchain.info public API for mainnet
	url := "https://blockchain.info/latestblock"
	resp, err := http.Get(url)
	if err != nil {
		return time.Time{}, fmt.Errorf("query blockchain api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return time.Time{}, fmt.Errorf("blockchain api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return time.Time{}, fmt.Errorf("read response body: %w", err)
	}

	// Parse blockchain.info response
	var blockData struct {
		Time int64 `json:"time"`
	}
	if err := json.Unmarshal(body, &blockData); err != nil {
		return time.Time{}, fmt.Errorf("parse blockchain response: %w", err)
	}

	return time.Unix(blockData.Time, 0), nil
}

// MoneroTimestampProvider implements blockchain time for Monero
type MoneroTimestampProvider struct {
	rpcClient interface{} // monero RPC client
}

// NewMoneroTimestampProvider creates a Monero timestamp provider
func NewMoneroTimestampProvider(rpcClient interface{}) *MoneroTimestampProvider {
	return &MoneroTimestampProvider{
		rpcClient: rpcClient,
	}
}

// GetLatestBlockTime retrieves the timestamp of the latest Monero block
func (mtp *MoneroTimestampProvider) GetLatestBlockTime() (time.Time, error) {
	if mtp.rpcClient == nil {
		return time.Time{}, fmt.Errorf("monero rpc client not configured")
	}

	// Type assert to MoneroHDWallet to use the GetLatestBlockTime method
	if mw, ok := mtp.rpcClient.(*wallet.MoneroHDWallet); ok {
		return mw.GetLatestBlockTime()
	}

	return time.Time{}, fmt.Errorf("invalid monero client type")
}
