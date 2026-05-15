package paywall

import (
	"testing"
	"time"

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

	em := &EscrowManager{paywall: pw}
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

	em := &EscrowManager{paywall: pw}
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

	em := &EscrowManager{paywall: pw}
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

	em := &EscrowManager{paywall: pw}
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

	em := &EscrowManager{paywall: pw}
	config := DefaultTimeoutMonitorConfig()
	monitor := NewTimeoutMonitor(em, config)

	paymentID := "test-payment"

	// Mark as processing
	monitor.processing[paymentID] = true

	// Try to process again (should be skipped)
	err := monitor.processTimeout(paymentID)
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

	em := &EscrowManager{paywall: pw}

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

	em := &EscrowManager{paywall: pw}

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

func TestTimeoutMonitor_IntegrationTest(t *testing.T) {
	store := NewMemoryStore()

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em := &EscrowManager{paywall: pw}

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
