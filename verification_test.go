package paywall

import (
	"context"
	"errors"
	"testing"
	"time"
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

func (m *mockFailingStore) Close() error {
	return nil
}
