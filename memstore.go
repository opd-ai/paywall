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

	// Copy the struct
	paymentCopy := *p

	// Deep copy maps and slices
	if p.Addresses != nil {
		paymentCopy.Addresses = make(map[wallet.WalletType]string, len(p.Addresses))
		for k, v := range p.Addresses {
			paymentCopy.Addresses[k] = v
		}
	}

	if p.Amounts != nil {
		paymentCopy.Amounts = make(map[wallet.WalletType]float64, len(p.Amounts))
		for k, v := range p.Amounts {
			paymentCopy.Amounts[k] = v
		}
	}

	if p.MultisigMetadata != nil {
		paymentCopy.MultisigMetadata = make(map[wallet.WalletType]*wallet.MultisigMetadata, len(p.MultisigMetadata))
		for k, v := range p.MultisigMetadata {
			if v != nil {
				metaCopy := *v
				// Deep copy nested slices in metadata
				if v.RedeemScript != nil {
					metaCopy.RedeemScript = make([]byte, len(v.RedeemScript))
					copy(metaCopy.RedeemScript, v.RedeemScript)
				}
				if v.PublicKeys != nil {
					metaCopy.PublicKeys = make([][]byte, len(v.PublicKeys))
					for i, pk := range v.PublicKeys {
						if pk != nil {
							metaCopy.PublicKeys[i] = make([]byte, len(pk))
							copy(metaCopy.PublicKeys[i], pk)
						}
					}
				}
				paymentCopy.MultisigMetadata[k] = &metaCopy
			}
		}
	}

	if p.RequiredSignatures != nil {
		paymentCopy.RequiredSignatures = make(map[wallet.WalletType]int, len(p.RequiredSignatures))
		for k, v := range p.RequiredSignatures {
			paymentCopy.RequiredSignatures[k] = v
		}
	}

	if p.Signatures != nil {
		paymentCopy.Signatures = make(map[wallet.WalletType][]SignatureData, len(p.Signatures))
		for k, sigs := range p.Signatures {
			if sigs != nil {
				sigsCopy := make([]SignatureData, len(sigs))
				for i, sig := range sigs {
					sigsCopy[i] = sig
					// Deep copy byte slices in SignatureData
					if sig.Signature != nil {
						sigsCopy[i].Signature = make([]byte, len(sig.Signature))
						copy(sigsCopy[i].Signature, sig.Signature)
					}
					if sig.PublicKey != nil {
						sigsCopy[i].PublicKey = make([]byte, len(sig.PublicKey))
						copy(sigsCopy[i].PublicKey, sig.PublicKey)
					}
					if sig.Nonce != nil {
						sigsCopy[i].Nonce = make([]byte, len(sig.Nonce))
						copy(sigsCopy[i].Nonce, sig.Nonce)
					}
				}
				paymentCopy.Signatures[k] = sigsCopy
			}
		}
	}

	if p.StateTransitionHistory != nil {
		paymentCopy.StateTransitionHistory = make([]StateTransitionHistory, len(p.StateTransitionHistory))
		copy(paymentCopy.StateTransitionHistory, p.StateTransitionHistory)
	}

	return &paymentCopy
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

// GetPaymentsByMultisigAddress finds payments by multisig address.
//
// Parameters:
//   - address: The multisig address to search for
//
// Returns:
//   - []*Payment: Slice of payments associated with the address
//   - error: Always nil in this implementation
func (m *MemoryStore) GetPaymentsByMultisigAddress(address string) ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var payments []*Payment
	for _, p := range m.payments {
		if !p.MultisigEnabled {
			continue
		}
		// Check if any wallet address matches
		for _, addr := range p.Addresses {
			if addr == address {
				payments = append(payments, deepCopyPayment(p))
				break
			}
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
		// Only check escrow-enabled payments
		if p.EscrowState == EscrowNone {
			continue
		}
		// Only check active escrow states (not completed/refunded)
		if p.EscrowState == EscrowCompleted || p.EscrowState == EscrowRefunded {
			continue
		}
		// Check if timeout is before deadline
		if !p.EscrowTimeout.IsZero() && p.EscrowTimeout.Before(deadline) {
			expiring = append(expiring, deepCopyPayment(p))
		}
	}
	return expiring, nil
}

