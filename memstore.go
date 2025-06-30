package paywall

import (
	"sync"

	"github.com/opd-ai/paywall/wallet"
)

// MemoryStore implements Store interface for in-memory payment tracking.
// This is a minimal implementation for demonstration purposes.
//
// Warning: Data is not persisted and will be lost on server restart
type MemoryStore struct {
	payments map[string]*Payment
	mu       sync.RWMutex
}

// NewMemoryStore creates a new in-memory payment store instance.
//
// Returns:
//   - *MemoryStore: Empty payment store
//
// Related: Store interface
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		payments: make(map[string]*Payment),
	}
}

// CreatePayment stores a new payment record.
//
// Parameters:
//   - p: Payment record to store
//
// Returns:
//   - error: Always returns nil in this implementation
func (m *MemoryStore) CreatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payments[p.ID] = p
	return nil
}

// GetPayment retrieves a payment record by ID.
//
// Parameters:
//   - id: Payment identifier
//
// Returns:
//   - *Payment: Payment record if found, nil if not found
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPayment(id string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.payments[id], nil
}

// UpdatePayment updates an existing payment record.
//
// Parameters:
//   - p: Payment record with updated fields
//
// Returns:
//   - error: Always nil in this implementation
func (m *MemoryStore) UpdatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payments[p.ID] = p
	return nil
}

// ListPendingPayments returns all pending payment records.
//
// Returns:
//   - []*Payment: Slice of payments with less than 1 confirmation
//   - error: Always nil in this implementation
func (m *MemoryStore) ListPendingPayments() ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var payments []*Payment
	for _, p := range m.payments {
		if p.Confirmations < 1 {
			payments = append(payments, p)
		}
	}
	return payments, nil
}

// GetPaymentByAddress retrieves a payment record by Bitcoin address.
//
// Parameters:
//   - addr: Bitcoin address associated with the payment
//
// Returns:
//   - *Payment: Payment record if found, nil if not found
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPaymentByAddress(addr string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.payments {
		if p.Addresses[wallet.Bitcoin] == addr || p.Addresses[wallet.Monero] == addr {
			return p, nil
		}
	}
	return nil, nil
}
