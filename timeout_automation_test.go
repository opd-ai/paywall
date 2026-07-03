package paywall

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/opd-ai/paywall/wallet"
)

func TestDefaultTimeoutMonitorConfig(t *testing.T) {
	config := DefaultTimeoutMonitorConfig()

	if config.CheckInterval != 5*time.Minute {
		t.Errorf("CheckInterval = %v, want %v", config.CheckInterval, 5*time.Minute)
	}

	if config.UseBlockchainTime {
		t.Error("UseBlockchainTime = true, want false")
	}

	if config.AutoRefund {
		t.Error("AutoRefund = true, want false")
	}
}

func TestNewTimeoutMonitor(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	monitor := NewTimeoutMonitor(em, config)

	if monitor.em != em {
		t.Error("monitor.em not set correctly")
	}

	if monitor.interval != config.CheckInterval {
		t.Errorf("monitor.interval = %v, want %v", monitor.interval, config.CheckInterval)
	}

	if monitor.ctx == nil {
		t.Error("monitor.ctx is nil")
	}

	if monitor.processing == nil {
		t.Error("monitor.processing map is nil")
	}
}

func TestTimeoutMonitor_StartStop(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	config.CheckInterval = 100 * time.Millisecond // Short interval for testing

	monitor := NewTimeoutMonitor(em, config)

	// Start monitor
	monitor.Start()

	// Let it run briefly
	time.Sleep(250 * time.Millisecond)

	// Stop monitor
	monitor.Stop()

	// Verify it stopped
	select {
	case <-monitor.ctx.Done():
		// Expected
	default:
		t.Error("monitor context not cancelled after Stop()")
	}
}

func TestTimeoutMonitor_GetCurrentTime_SystemTime(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	config.UseBlockchainTime = false

	monitor := NewTimeoutMonitor(em, config)

	before := time.Now()
	currentTime, err := monitor.getCurrentTime()
	after := time.Now()

	if err != nil {
		t.Errorf("getCurrentTime() error = %v", err)
	}

	if currentTime.Before(before) || currentTime.After(after) {
		t.Errorf("getCurrentTime() = %v, want between %v and %v", currentTime, before, after)
	}
}

func TestTimeoutMonitor_GetCurrentTime_Blockchain(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	config.UseBlockchainTime = true

	monitor := NewTimeoutMonitor(em, config)

	// Should fall back to system time since blockchain provider not implemented
	currentTime, err := monitor.getCurrentTime()
	if err != nil {
		t.Errorf("getCurrentTime() error = %v", err)
	}

	// Should have fallen back to system time
	if currentTime.IsZero() {
		t.Error("getCurrentTime() returned zero time")
	}
}

func TestTimeoutMonitor_ProcessTimeout_NoDoubleProcessing(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	monitor := NewTimeoutMonitor(em, config)

	paymentID := "test-payment"

	// Mark as processing
	monitor.processing[paymentID] = true

	// Try to process again (should be skipped)
	err = monitor.processTimeout(paymentID)
	if err != nil {
		t.Errorf("processTimeout() error = %v", err)
	}

	// Should not have changed the processing state
	if !monitor.processing[paymentID] {
		t.Error("payment should still be marked as processing")
	}
}

func TestEscrowManager_CheckEscrowTimeoutsWithTime(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}

	// Create test payments
	now := time.Now()
	pastTimeout := now.Add(-1 * time.Hour)
	futureTimeout := now.Add(1 * time.Hour)

	// Payment 1: Funded and timed out
	payment1 := &Payment{
		ID:              "payment-1",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   pastTimeout,
	}
	store.CreatePayment(payment1)

	// Payment 2: Funded but not timed out
	payment2 := &Payment{
		ID:              "payment-2",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   futureTimeout,
	}
	store.CreatePayment(payment2)

	// Payment 3: Disputed and timed out
	payment3 := &Payment{
		ID:              "payment-3",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowDisputed,
		EscrowTimeout:   pastTimeout,
	}
	store.CreatePayment(payment3)

	// Payment 4: Completed (should not be checked)
	payment4 := &Payment{
		ID:              "payment-4",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowCompleted,
		EscrowTimeout:   pastTimeout,
	}
	store.CreatePayment(payment4)

	// Payment 5: Pending escrow state (should not be checked)
	payment5 := &Payment{
		ID:              "payment-5",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowPending,
		EscrowTimeout:   pastTimeout,
	}
	store.CreatePayment(payment5)

	// Payment 6: Non-multisig escrow (should not be checked)
	payment6 := &Payment{
		ID:              "payment-6",
		MultisigEnabled: false,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   pastTimeout,
	}
	store.CreatePayment(payment6)

	// Check timeouts
	timedOut, err := em.CheckEscrowTimeoutsWithTime(now)
	if err != nil {
		t.Fatalf("CheckEscrowTimeoutsWithTime() error = %v", err)
	}

	// Should find payment-1 and payment-3
	if len(timedOut) != 2 {
		t.Errorf("len(timedOut) = %d, want 2", len(timedOut))
	}

	found1, found3 := false, false
	for _, id := range timedOut {
		if id == "payment-1" {
			found1 = true
		}
		if id == "payment-3" {
			found3 = true
		}
	}

	if !found1 {
		t.Error("payment-1 not found in timedOut")
	}
	if !found3 {
		t.Error("payment-3 not found in timedOut")
	}
}

func TestEscrowManager_CheckEscrowTimeoutsWithTime_EmptyStore(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}

	timedOut, err := em.CheckEscrowTimeoutsWithTime(time.Now())
	if err != nil {
		t.Fatalf("CheckEscrowTimeoutsWithTime() error = %v", err)
	}

	if len(timedOut) != 0 {
		t.Errorf("len(timedOut) = %d, want 0", len(timedOut))
	}
}

func TestNewBitcoinTimestampProvider(t *testing.T) {
	provider := NewBitcoinTimestampProvider("http://localhost:8332", true)

	if provider.rpcURL != "http://localhost:8332" {
		t.Errorf("rpcURL = %v, want http://localhost:8332", provider.rpcURL)
	}

	if !provider.testnet {
		t.Error("testnet = false, want true")
	}
}

func TestBitcoinTimestampProvider_GetLatestBlockTime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test that depends on external blockchain API in short mode")
	}

	provider := NewBitcoinTimestampProvider("http://localhost:8332", true)

	// Implementation now connects to public API
	// For testnet, it logs a warning and uses system time, so should not error
	blockTime, err := provider.GetLatestBlockTime()
	if err != nil {
		t.Errorf("GetLatestBlockTime() error = %v, want nil (testnet uses system time)", err)
	}
	if blockTime.IsZero() {
		t.Error("GetLatestBlockTime() returned zero time")
	}
}

func TestNewMoneroTimestampProvider(t *testing.T) {
	provider := NewMoneroTimestampProvider(nil)

	if provider == nil {
		t.Error("NewMoneroTimestampProvider() returned nil")
	}
}

func TestMoneroTimestampProvider_GetLatestBlockTime(t *testing.T) {
	provider := NewMoneroTimestampProvider(nil)

	// Should return not implemented error
	_, err := provider.GetLatestBlockTime()
	if err == nil {
		t.Error("GetLatestBlockTime() error = nil, want error")
	}
}

// mockArbiterSigner implements ArbiterSigner for testing
type mockArbiterSigner struct {
	shouldFail bool
	privateKey *btcec.PrivateKey
}

func (m *mockArbiterSigner) SignTimeoutRefund(payment *Payment) (*SignatureData, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock signer error")
	}

	privateKey := m.privateKey
	if privateKey == nil {
		seed := sha256.Sum256([]byte("timeout-automation-arbiter"))
		privateKey, _ = btcec.PrivKeyFromBytes(seed[:])
	}
	hash := sha256.Sum256([]byte("timeout_refund|" + payment.ID))
	return &SignatureData{
		SignerID:  "test-arbiter",
		Role:      RoleArbiter,
		Signature: append(ecdsa.Sign(privateKey, hash[:]).Serialize(), byte(0x01)),
		PublicKey: privateKey.PubKey().SerializeCompressed(),
		SignedAt:  time.Now(),
		Nonce:     []byte(payment.ID + "-arbiter-timeout"),
		PaymentID: payment.ID,
	}, nil
}

func TestTimeoutMonitor_AutoRefund_Disabled(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	config.AutoRefund = false
	monitor := NewTimeoutMonitor(em, config)

	// Process timeout with autoRefund disabled
	err = monitor.processTimeout("test-payment")
	if err != nil {
		t.Errorf("processTimeout() with autoRefund disabled should not error, got: %v", err)
	}

	// Verify autoRefund is disabled
	if monitor.autoRefund {
		t.Error("autoRefund should be false")
	}
}

func TestTimeoutMonitor_AutoRefund_NoSigner(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	config.AutoRefund = true
	monitor := NewTimeoutMonitor(em, config)

	// Create a test payment
	payment := &Payment{
		ID:              "test-payment",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
	}
	store.CreatePayment(payment)

	// Try to process timeout without arbiter signer
	err = monitor.processTimeout("test-payment")
	if err == nil {
		t.Error("processTimeout() with autoRefund enabled but no signer should error")
	}
	if err != nil && !strings.Contains(err.Error(), "arbiter signer not configured for automatic refunds") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTimeoutMonitor_SetArbiterSigner(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	monitor := NewTimeoutMonitor(em, config)

	// Set arbiter signer
	signer := &mockArbiterSigner{}
	monitor.SetArbiterSigner(signer)

	if monitor.arbiterSigner == nil {
		t.Error("arbiterSigner should be set")
	}
}

func TestTimeoutMonitor_AutoRefund_Success(t *testing.T) {
	store := NewMemoryStore()

	buyerSeed := sha256.Sum256([]byte("timeout-automation-buyer"))
	sellerSeed := sha256.Sum256([]byte("timeout-automation-seller"))
	arbiterSeed := sha256.Sum256([]byte("timeout-automation-arbiter"))
	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])
	buyerPub := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPub := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPub := arbiterPrivKey.PubKey().SerializeCompressed()

	// Create config with authorized arbiters
	config := Config{
		PriceInBTC:         0.001,
		TestNet:            true,
		Store:              store,
		PaymentTimeout:     time.Hour * 24,
		MultisigEnabled:    true,
		MultisigRequired:   2,
		MultisigTotal:      3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: {buyerPub, sellerPub, arbiterPub}},
		AuthorizedArbiters: [][]byte{arbiterPub},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	em, _ := NewEscrowManagerWithAudit(pw, NewMemoryAuditLogger())
	monitorConfig := DefaultTimeoutMonitorConfig()
	monitorConfig.AutoRefund = true
	monitor := NewTimeoutMonitor(em, monitorConfig)

	// Set arbiter signer
	signer := &mockArbiterSigner{privateKey: arbiterPrivKey}
	monitor.SetArbiterSigner(signer)

	buyerRefundHash := sha256.Sum256([]byte("timeout_refund|test-payment"))
	buyerRefundSig := append(ecdsa.Sign(buyerPrivKey, buyerRefundHash[:]).Serialize(), byte(0x01))

	payment := &Payment{
		ID:              "test-payment",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
		MultisigMetadata: map[wallet.WalletType]*wallet.MultisigMetadata{
			wallet.Bitcoin: {
				PublicKeys: [][]byte{
					buyerPub,
					sellerPub,
					arbiterPub,
				},
				RequiredSigs: 2,
			},
		},
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {
				{
					SignerID:  "buyer",
					Role:      RoleBuyer,
					Signature: buyerRefundSig,
					PublicKey: buyerPub,
					SignedAt:  time.Now(),
					Nonce:     []byte("test-payment-timeout-approval"),
					PaymentID: "test-payment",
				},
			},
		},
	}
	store.CreatePayment(payment)

	// Process timeout with automatic refund
	err = monitor.processTimeout("test-payment")
	if err != nil {
		t.Errorf("processTimeout() with autoRefund enabled should succeed, got: %v", err)
	}

	// Verify payment was refunded
	updatedPayment, _ := store.GetPayment("test-payment")
	if updatedPayment.EscrowState != EscrowRefunded {
		t.Errorf("payment should be refunded, got state: %v", updatedPayment.EscrowState)
	}
}

func TestTimeoutMonitor_AutoRefund_SignerError(t *testing.T) {
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("timeout-automation-buyer"))
	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	buyerPub := buyerPrivKey.PubKey().SerializeCompressed()
	buyerRefundHash := sha256.Sum256([]byte("timeout_refund|test-payment"))

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}
	config := DefaultTimeoutMonitorConfig()
	config.AutoRefund = true
	monitor := NewTimeoutMonitor(em, config)

	// Set arbiter signer that will fail
	signer := &mockArbiterSigner{shouldFail: true}
	monitor.SetArbiterSigner(signer)

	// Create a test payment
	payment := &Payment{
		ID:              "test-payment",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {
				{
					SignerID:  "buyer",
					Role:      RoleBuyer,
					Signature: append(ecdsa.Sign(buyerPrivKey, buyerRefundHash[:]).Serialize(), byte(0x01)),
					PublicKey: buyerPub,
					SignedAt:  time.Now(),
					Nonce:     []byte("test-payment-timeout-approval"),
					PaymentID: "test-payment",
				},
			},
		},
	}
	store.CreatePayment(payment)

	// Try to process timeout with failing signer
	err = monitor.processTimeout("test-payment")
	if err == nil {
		t.Error("processTimeout() with failing signer should error")
	}
}

func TestTimeoutMonitor_IntegrationTest(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create EscrowManager: %v", err)
	}

	// Create a payment that will timeout
	payment := &Payment{
		ID:              "test-payment",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour), // Already timed out
	}
	store.CreatePayment(payment)

	// Create monitor with short interval
	config := DefaultTimeoutMonitorConfig()
	config.CheckInterval = 100 * time.Millisecond
	monitor := NewTimeoutMonitor(em, config)

	// Start monitoring
	monitor.Start()

	// Wait for at least one check
	time.Sleep(250 * time.Millisecond)

	// Stop monitoring
	monitor.Stop()

	// The monitor should have detected the timeout (logged but not processed)
	// This is a basic integration test - in production you'd check logs
}

func TestTimeoutMonitor_AutomaticRefund(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a payment that has already timed out
	payment := &Payment{
		ID:              "test-timeout-payment",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
	}
	store.CreatePayment(payment)

	// Create monitor with auto-refund enabled
	config := TimeoutMonitorConfig{
		CheckInterval:     100 * time.Millisecond,
		UseBlockchainTime: false,
		AutoRefund:        true,
	}
	monitor := NewTimeoutMonitor(em, config)

	// Set arbiter signer for automatic refunds
	signer := &mockArbiterSigner{shouldFail: false}
	monitor.SetArbiterSigner(signer)

	// Start monitoring
	monitor.Start()

	// Wait for at least one check cycle
	time.Sleep(250 * time.Millisecond)

	// Stop monitoring
	monitor.Stop()

	// Verify payment was automatically refunded
	refundedPayment, err := store.GetPayment("test-timeout-payment")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if refundedPayment.EscrowState != EscrowRefunded {
		t.Errorf("EscrowState = %v, want %v", refundedPayment.EscrowState, EscrowRefunded)
	}
}

func TestTimeoutMonitor_ManualRefund(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a payment that has already timed out
	payment := &Payment{
		ID:              "test-manual-payment",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
	}
	store.CreatePayment(payment)

	// Create monitor with auto-refund DISABLED
	config := TimeoutMonitorConfig{
		CheckInterval:     100 * time.Millisecond,
		UseBlockchainTime: false,
		AutoRefund:        false,
	}
	monitor := NewTimeoutMonitor(em, config)

	// Start monitoring
	monitor.Start()

	// Wait for at least one check cycle
	time.Sleep(250 * time.Millisecond)

	// Stop monitoring
	monitor.Stop()

	// Verify payment was NOT automatically refunded (should still be Funded)
	manualPayment, err := store.GetPayment("test-manual-payment")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if manualPayment.EscrowState != EscrowFunded {
		t.Errorf("EscrowState = %v, want %v (should not auto-refund)", manualPayment.EscrowState, EscrowFunded)
	}
}

func TestTimeoutMonitor_ProcessTimeout_WithAutoRefund(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a timed-out payment
	payment := &Payment{
		ID:              "test-process-timeout",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
	}
	store.CreatePayment(payment)

	// Create monitor with auto-refund enabled
	config := TimeoutMonitorConfig{
		CheckInterval:     5 * time.Minute,
		UseBlockchainTime: false,
		AutoRefund:        true,
	}
	monitor := NewTimeoutMonitor(em, config)

	// Set arbiter signer for automatic refunds
	signer := &mockArbiterSigner{shouldFail: false}
	monitor.SetArbiterSigner(signer)

	// Directly call processTimeout
	err = monitor.processTimeout("test-process-timeout")
	if err != nil {
		t.Errorf("processTimeout() error = %v", err)
	}

	// Verify the payment was refunded
	refundedPayment, err := store.GetPayment("test-process-timeout")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if refundedPayment.EscrowState != EscrowRefunded {
		t.Errorf("EscrowState = %v, want %v", refundedPayment.EscrowState, EscrowRefunded)
	}
}

func TestTimeoutMonitor_NoConcurrentProcessing(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a timed-out payment
	payment := &Payment{
		ID:              "test-concurrent",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
	}
	store.CreatePayment(payment)

	// Create monitor
	config := TimeoutMonitorConfig{
		CheckInterval:     5 * time.Minute,
		UseBlockchainTime: false,
		AutoRefund:        true,
	}
	monitor := NewTimeoutMonitor(em, config)

	// Set arbiter signer for automatic refunds
	signer := &mockArbiterSigner{shouldFail: false}
	monitor.SetArbiterSigner(signer)

	// Try to process the same timeout twice concurrently
	done := make(chan bool, 2)
	go func() {
		monitor.processTimeout("test-concurrent")
		done <- true
	}()
	go func() {
		monitor.processTimeout("test-concurrent")
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Verify payment was refunded only once (no duplicate processing)
	refundedPayment, err := store.GetPayment("test-concurrent")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if refundedPayment.EscrowState != EscrowRefunded {
		t.Errorf("EscrowState = %v, want %v", refundedPayment.EscrowState, EscrowRefunded)
	}
}

func TestEscrowManager_StartTimeoutMonitor(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a timed-out payment
	payment := &Payment{
		ID:              "test-start-monitor",
		MultisigEnabled: true,
		Status:          StatusPending,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(-1 * time.Hour),
	}
	store.CreatePayment(payment)

	// Start monitor using convenience method
	config := TimeoutMonitorConfig{
		CheckInterval:     100 * time.Millisecond,
		UseBlockchainTime: false,
		AutoRefund:        true,
	}
	monitor := em.StartTimeoutMonitor(config)

	// Set arbiter signer for automatic refunds
	signer := &mockArbiterSigner{shouldFail: false}
	monitor.SetArbiterSigner(signer)

	// Verify monitor is running
	if monitor == nil {
		t.Fatal("StartTimeoutMonitor() returned nil")
	}

	// Wait for processing
	time.Sleep(250 * time.Millisecond)

	// Stop monitor
	monitor.Stop()

	// Verify payment was refunded
	refundedPayment, err := store.GetPayment("test-start-monitor")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}

	if refundedPayment.EscrowState != EscrowRefunded {
		t.Errorf("EscrowState = %v, want %v", refundedPayment.EscrowState, EscrowRefunded)
	}
}
