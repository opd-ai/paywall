package paywall

// MemoryStore implements Store interface for in-memory payment tracking.
// This is a minimal implementation for demonstration purposes.
//
// Warning: Data is not persisted and will be lost on server restart
type MemoryStore struct{}

// NewMemoryStore creates a new in-memory payment store instance.
//
// Returns:
//   - *MemoryStore: Empty payment store
//
// Related: Store interface
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// CreatePayment stores a new payment record.
//
// Parameters:
//   - p: Payment record to store
//
// Returns:
//   - error: Always returns nil in this implementation
//
// Note: This is a minimal implementation that doesn't actually store data
func (m *MemoryStore) CreatePayment(p *Payment) error { return nil }

// GetPayment retrieves a payment record by ID.
//
// Parameters:
//   - id: Payment identifier
//
// Returns:
//   - *Payment: Always nil in this implementation
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPayment(id string) (*Payment, error) { return nil, nil }

// UpdatePayment updates an existing payment record.
//
// Parameters:
//   - p: Payment record with updated fields
//
// Returns:
//   - error: Always nil in this implementation
func (m *MemoryStore) UpdatePayment(p *Payment) error { return nil }

// ListPendingPayments returns all pending payment records.
//
// Returns:
//   - []*Payment: Always nil in this implementation
//   - error: Always nil in this implementation
func (m *MemoryStore) ListPendingPayments() ([]*Payment, error) { return nil, nil }

// GetPaymentByAddress retrieves a payment record by Bitcoin address.
//
// Parameters:
//   - addr: Bitcoin address associated with the payment
//
// Returns:
//   - *Payment: Always nil in this implementation
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPaymentByAddress(addr string) (*Payment, error) { return nil, nil }
