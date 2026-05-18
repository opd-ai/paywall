package paywall

import (
	"sync"
	"time"

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
// Returns a deep copy to prevent concurrent modification issues.
//
// Parameters:
//   - id: Payment identifier
//
// Returns:
//   - *Payment: Payment record deep copy if found, nil if not found
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPayment(id string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.payments[id]
	if !exists {
		return nil, nil
	}

	// Return a deep copy to prevent concurrent modification of shared state
	return deepCopyPayment(p), nil
}

// deepCopyPayment creates a deep copy of a payment to prevent shared mutable state
func deepCopyPayment(p *Payment) *Payment {
	if p == nil {
		return nil
	}

	paymentCopy := *p
	paymentCopy.Addresses = copyAddresses(p.Addresses)
	paymentCopy.Amounts = copyAmounts(p.Amounts)
	paymentCopy.MultisigMetadata = copyMultisigMetadata(p.MultisigMetadata)
	paymentCopy.RequiredSignatures = copyRequiredSignatures(p.RequiredSignatures)
	paymentCopy.Signatures = copySignatures(p.Signatures)
	paymentCopy.StateTransitionHistory = copyStateHistory(p.StateTransitionHistory)

	return &paymentCopy
}

func copyAddresses(src map[wallet.WalletType]string) map[wallet.WalletType]string {
	if src == nil {
		return nil
	}
	dst := make(map[wallet.WalletType]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyAmounts(src map[wallet.WalletType]float64) map[wallet.WalletType]float64 {
	if src == nil {
		return nil
	}
	dst := make(map[wallet.WalletType]float64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyMultisigMetadata(src map[wallet.WalletType]*wallet.MultisigMetadata) map[wallet.WalletType]*wallet.MultisigMetadata {
	if src == nil {
		return nil
	}
	dst := make(map[wallet.WalletType]*wallet.MultisigMetadata, len(src))
	for k, v := range src {
		if v != nil {
			metaCopy := *v
			metaCopy.RedeemScript = copyBytes(v.RedeemScript)
			metaCopy.PublicKeys = copyPublicKeys(v.PublicKeys)
			dst[k] = &metaCopy
		}
	}
	return dst
}

func copyPublicKeys(src [][]byte) [][]byte {
	if src == nil {
		return nil
	}
	dst := make([][]byte, len(src))
	for i, pk := range src {
		dst[i] = copyBytes(pk)
	}
	return dst
}

func copyRequiredSignatures(src map[wallet.WalletType]int) map[wallet.WalletType]int {
	if src == nil {
		return nil
	}
	dst := make(map[wallet.WalletType]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copySignatures(src map[wallet.WalletType][]SignatureData) map[wallet.WalletType][]SignatureData {
	if src == nil {
		return nil
	}
	dst := make(map[wallet.WalletType][]SignatureData, len(src))
	for k, sigs := range src {
		if sigs != nil {
			sigsCopy := make([]SignatureData, len(sigs))
			for i, sig := range sigs {
				sigsCopy[i] = sig
				sigsCopy[i].Signature = copyBytes(sig.Signature)
				sigsCopy[i].PublicKey = copyBytes(sig.PublicKey)
				sigsCopy[i].Nonce = copyBytes(sig.Nonce)
			}
			dst[k] = sigsCopy
		}
	}
	return dst
}

func copyBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

func copyStateHistory(src []StateTransitionHistory) []StateTransitionHistory {
	if src == nil {
		return nil
	}
	dst := make([]StateTransitionHistory, len(src))
	copy(dst, src)
	return dst
}

// UpdatePayment updates an existing payment record.
//
// Parameters:
//   - p: Payment record with updated fields
//
// Returns:
//   - error: ErrVersionConflict if the payment was concurrently modified, nil otherwise
func (m *MemoryStore) UpdatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Optimistic locking: check that the version matches what's in storage
	existingPayment, exists := m.payments[p.ID]
	if !exists {
		// Payment doesn't exist, cannot update
		return nil
	}

	// Check version for concurrent modification detection
	if existingPayment.Version != p.Version {
		return ErrVersionConflict
	}

	// Increment version before storing the updated payment
	p.Version++
	m.payments[p.ID] = p
	return nil
}

// ListPendingPayments returns all pending payment records.
//
// Returns:
//   - []*Payment: Slice of payments with 1 or fewer confirmations
//   - error: Always nil in this implementation
func (m *MemoryStore) ListPendingPayments() ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var payments []*Payment
	for _, p := range m.payments {
		if p.Confirmations <= 1 {
			payments = append(payments, deepCopyPayment(p))
		}
	}
	return payments, nil
}

// GetPaymentByAddress retrieves a payment record by Bitcoin address.
// Returns a deep copy to prevent concurrent modification.
//
// Parameters:
//   - addr: Bitcoin address associated with the payment
//
// Returns:
//   - *Payment: Payment record deep copy if found, nil if not found
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPaymentByAddress(addr string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.payments {
		if p.Addresses[wallet.Bitcoin] == addr || p.Addresses[wallet.Monero] == addr {
			return deepCopyPayment(p), nil
		}
	}
	return nil, nil
}

// GetPendingMultisigPayments returns all pending payments that have multisig enabled.
//
// Returns:
//   - []*Payment: Slice of pending multisig payments
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPendingMultisigPayments() ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var payments []*Payment
	for _, p := range m.payments {
		if p.MultisigEnabled && p.Status == StatusPending {
			payments = append(payments, deepCopyPayment(p))
		}
	}
	return payments, nil
}

// GetEscrowsExpiringBefore returns escrow payments expiring before the deadline.
// This enables efficient timeout checking without scanning all payments.
// Note: In MemoryStore this still does a linear scan, but the interface
// allows FileStore to use indexed queries for better performance.
//
// Parameters:
//   - deadline: Time threshold - returns escrows expiring before this time
//
// Returns:
//   - []*Payment: Slice of escrow payments with EscrowTimeout before deadline
//   - error: Always nil in this implementation
func (m *MemoryStore) GetEscrowsExpiringBefore(deadline time.Time) ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var expiring []*Payment
	for _, p := range m.payments {
		if !p.MultisigEnabled {
			continue
		}
		if p.EscrowState != EscrowFunded && p.EscrowState != EscrowDisputed {
			continue
		}
		// Check if timeout is before deadline
		if !p.EscrowTimeout.IsZero() && p.EscrowTimeout.Before(deadline) {
			expiring = append(expiring, deepCopyPayment(p))
		}
	}
	return expiring, nil
}
