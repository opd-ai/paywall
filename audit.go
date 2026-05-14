// Package paywall implements audit logging for escrow operations
package paywall

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// AuditLogger defines the interface for audit trail operations
// Implementations must ensure append-only semantics and thread-safety
type AuditLogger interface {
	// LogAction records an action in the audit trail
	// Returns the audit entry ID and any error encountered
	LogAction(entry *AuditLogEntry) (string, error)

	// GetAuditTrail retrieves all audit entries for a payment
	// Returns entries in chronological order
	GetAuditTrail(paymentID string) ([]*AuditLogEntry, error)

	// GetAllEntries retrieves all audit log entries
	// Returns entries in chronological order
	GetAllEntries() ([]*AuditLogEntry, error)
}

// MemoryAuditLogger is an in-memory implementation of AuditLogger
// WARNING: Audit logs are not persisted and will be lost on restart
// Use only for testing; production should use persistent storage
type MemoryAuditLogger struct {
	mu      sync.RWMutex
	entries []*AuditLogEntry
}

// NewMemoryAuditLogger creates a new in-memory audit logger
func NewMemoryAuditLogger() *MemoryAuditLogger {
	return &MemoryAuditLogger{
		entries: make([]*AuditLogEntry, 0),
	}
}

// LogAction records an action in the audit trail.
// Automatically generates an ID and timestamp if not provided.
// Returns the audit entry ID.
//
// Parameters:
//   - entry: The audit log entry to record
//
// Returns:
//   - string: The unique ID assigned to this entry
//   - error: Any error encountered during logging
//
// Thread-safety: Protected by write lock
func (m *MemoryAuditLogger) LogAction(entry *AuditLogEntry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("audit entry cannot be nil")
	}

	// Generate ID if not provided
	if entry.ID == "" {
		id, err := generateAuditID()
		if err != nil {
			return "", fmt.Errorf("failed to generate audit ID: %w", err)
		}
		entry.ID = id
	}

	// Set timestamp if not provided
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Append-only: create defensive copy and append
	entryCopy := *entry
	m.entries = append(m.entries, &entryCopy)

	return entry.ID, nil
}

// GetAuditTrail retrieves all audit entries for a specific payment.
// Returns entries in chronological order (oldest first).
//
// Parameters:
//   - paymentID: The payment ID to filter by
//
// Returns:
//   - []*AuditLogEntry: Slice of audit entries for the payment
//   - error: Always nil in this implementation
//
// Thread-safety: Protected by read lock
func (m *MemoryAuditLogger) GetAuditTrail(paymentID string) ([]*AuditLogEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var trail []*AuditLogEntry
	for _, entry := range m.entries {
		if entry.PaymentID == paymentID {
			// Return defensive copy
			entryCopy := *entry
			trail = append(trail, &entryCopy)
		}
	}

	return trail, nil
}

// GetAllEntries retrieves all audit log entries.
// Returns entries in chronological order (oldest first).
//
// Returns:
//   - []*AuditLogEntry: Slice of all audit entries
//   - error: Always nil in this implementation
//
// Thread-safety: Protected by read lock
func (m *MemoryAuditLogger) GetAllEntries() ([]*AuditLogEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return defensive copies
	result := make([]*AuditLogEntry, len(m.entries))
	for i, entry := range m.entries {
		entryCopy := *entry
		result[i] = &entryCopy
	}

	return result, nil
}

// generateAuditID creates a unique identifier for an audit log entry
func generateAuditID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "audit_" + hex.EncodeToString(bytes), nil
}
