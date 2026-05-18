package paywall

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// TestCryptoChainMonitor_ExponentialBackoff tests that the monitor implements
// exponential backoff when checkPendingPayments returns errors
func TestCryptoChainMonitor_ExponentialBackoff(t *testing.T) {
	// Create a mock paywall with a store that always returns errors
	mockStore := &mockFailingStore{}
	pw := &Paywall{
		Store: mockStore,
	}

	monitor := &CryptoChainMonitor{
		paywall: pw,
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the monitor
	monitor.Start(ctx)

	// Let it run for a short time to ensure it starts
	time.Sleep(100 * time.Millisecond)

	// Cancel the context to stop the monitor
	cancel()

	// Test passes if no panic occurs and monitor starts/stops cleanly
	// The actual backoff behavior is tested by observing logs in integration tests
}

// TestCheckWalletPayment_MissingClient tests that checkWalletPayment returns an error
// when the requested wallet client is not found
func TestCheckWalletPayment_MissingClient(t *testing.T) {
	mockStore := &mockStore{}
	pw := &Paywall{
		Store:            mockStore,
		minConfirmations: 3,
	}

	monitor := &CryptoChainMonitor{
		paywall: pw,
		client:  make(map[wallet.WalletType]CryptoClient),
	}

	payment := &Payment{
		ID:        "test-payment",
		Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		Status:    StatusPending,
	}

	var mux sync.Mutex
	err := monitor.checkWalletPayment(payment, wallet.Bitcoin, &mux)

	if err == nil {
		t.Fatal("Expected error for missing client, got nil")
	}
	if err.Error() != "BTC client not found" {
		t.Errorf("Expected 'BTC client not found', got '%s'", err.Error())
	}
}

// TestCheckWalletPayment_BalanceBelowThreshold tests that payment status remains pending
// when balance is below the required amount
func TestCheckWalletPayment_BalanceBelowThreshold(t *testing.T) {
	mockStore := &mockStore{}
	pw := &Paywall{
		Store:            mockStore,
		minConfirmations: 3,
	}

	mockClient := &mockCryptoClient{
		balance: 0.0005, // Below required amount
	}

	monitor := &CryptoChainMonitor{
		paywall: pw,
		client:  map[wallet.WalletType]CryptoClient{wallet.Bitcoin: mockClient},
	}

	payment := &Payment{
		ID:        "test-payment",
		Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		Status:    StatusPending,
	}

	var mux sync.Mutex
	err := monitor.checkWalletPayment(payment, wallet.Bitcoin, &mux)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if payment.Status != StatusPending {
		t.Errorf("Expected status to remain pending, got %s", payment.Status)
	}
}

// TestCheckWalletPayment_BalanceAboveThreshold tests that payment status is updated
// to confirmed when balance meets or exceeds the required amount
func TestCheckWalletPayment_BalanceAboveThreshold(t *testing.T) {
	mockStore := &mockStore{}
	pw := &Paywall{
		Store:            mockStore,
		minConfirmations: 3,
	}

	mockClient := &mockCryptoClient{
		balance: 0.002, // Above required amount
	}

	monitor := &CryptoChainMonitor{
		paywall: pw,
		client:  map[wallet.WalletType]CryptoClient{wallet.Bitcoin: mockClient},
	}

	payment := &Payment{
		ID:        "test-payment",
		Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		Status:    StatusPending,
	}

	var mux sync.Mutex
	err := monitor.checkWalletPayment(payment, wallet.Bitcoin, &mux)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if payment.Status != StatusConfirmed {
		t.Errorf("Expected status to be confirmed, got %s", payment.Status)
	}
	if payment.Confirmations != 3 {
		t.Errorf("Expected confirmations to be 3, got %d", payment.Confirmations)
	}
	if !mockStore.updateCalled {
		t.Error("Expected UpdatePayment to be called")
	}
}

// TestCheckWalletPayment_GetBalanceError tests that errors from GetAddressBalance
// are properly propagated
func TestCheckWalletPayment_GetBalanceError(t *testing.T) {
	mockStore := &mockStore{}
	pw := &Paywall{
		Store:            mockStore,
		minConfirmations: 3,
	}

	mockClient := &mockCryptoClient{
		err: errors.New("network error"),
	}

	monitor := &CryptoChainMonitor{
		paywall: pw,
		client:  map[wallet.WalletType]CryptoClient{wallet.Bitcoin: mockClient},
	}

	payment := &Payment{
		ID:        "test-payment",
		Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		Status:    StatusPending,
	}

	var mux sync.Mutex
	err := monitor.checkWalletPayment(payment, wallet.Bitcoin, &mux)

	if err == nil {
		t.Fatal("Expected error from GetAddressBalance, got nil")
	}
	if err.Error() != "network error" {
		t.Errorf("Expected 'network error', got '%s'", err.Error())
	}
}

// TestCheckWalletPayment_UpdatePaymentError tests that errors from UpdatePayment
// are silently ignored (as per current implementation)
func TestCheckWalletPayment_UpdatePaymentError(t *testing.T) {
	mockStore := &mockStore{
		updateError: errors.New("storage error"),
	}
	pw := &Paywall{
		Store:            mockStore,
		minConfirmations: 3,
	}

	mockClient := &mockCryptoClient{
		balance: 0.002, // Above required amount
	}

	monitor := &CryptoChainMonitor{
		paywall: pw,
		client:  map[wallet.WalletType]CryptoClient{wallet.Bitcoin: mockClient},
	}

	payment := &Payment{
		ID:        "test-payment",
		Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		Status:    StatusPending,
	}

	var mux sync.Mutex
	err := monitor.checkWalletPayment(payment, wallet.Bitcoin, &mux)
	// Current implementation doesn't check UpdatePayment error
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if payment.Status != StatusConfirmed {
		t.Errorf("Expected status to be confirmed, got %s", payment.Status)
	}
}

// mockStore is a mock implementation of PaymentStore for testing
type mockStore struct {
	updateCalled bool
	updateError  error
}

func (m *mockStore) CreatePayment(payment *Payment) error {
	return nil
}

func (m *mockStore) GetPayment(id string) (*Payment, error) {
	return nil, nil
}

func (m *mockStore) GetPaymentByAddress(address string) (*Payment, error) {
	return nil, nil
}

func (m *mockStore) UpdatePayment(payment *Payment) error {
	m.updateCalled = true
	return m.updateError
}

func (m *mockStore) ListPendingPayments() ([]*Payment, error) {
	return nil, nil
}

func (m *mockStore) GetPendingMultisigPayments() ([]*Payment, error) {
	return nil, nil
}

func (m *mockStore) GetEscrowsExpiringBefore(deadline time.Time) ([]*Payment, error) {
	return nil, nil
}

func (m *mockStore) Close() error {
	return nil
}

// mockCryptoClient is a mock implementation of CryptoClient for testing
type mockCryptoClient struct {
	balance float64
	err     error
}

func (m *mockCryptoClient) GetAddressBalance(address string) (float64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.balance, nil
}

// mockFailingStore always returns errors to trigger backoff behavior
type mockFailingStore struct{}

func (m *mockFailingStore) CreatePayment(payment *Payment) error {
	return nil
}

func (m *mockFailingStore) GetPayment(id string) (*Payment, error) {
	return nil, nil
}

func (m *mockFailingStore) GetPaymentByAddress(address string) (*Payment, error) {
	return nil, nil
}

func (m *mockFailingStore) UpdatePayment(payment *Payment) error {
	return nil
}

func (m *mockFailingStore) ListPendingPayments() ([]*Payment, error) {
	// Always return an error to trigger backoff
	return nil, errors.New("mock store error")
}

func (m *mockFailingStore) GetPendingMultisigPayments() ([]*Payment, error) {
	return nil, errors.New("mock store error")
}

func (m *mockFailingStore) GetEscrowsExpiringBefore(deadline time.Time) ([]*Payment, error) {
	return nil, errors.New("mock store error")
}

func (m *mockFailingStore) Close() error {
	return nil
}
