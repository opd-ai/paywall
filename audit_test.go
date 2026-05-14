package paywall

import (
	"testing"

	"github.com/opd-ai/paywall/wallet"
)

func TestNewMemoryAuditLogger(t *testing.T) {
	logger := NewMemoryAuditLogger()
	if logger == nil {
		t.Fatal("NewMemoryAuditLogger() returned nil")
	}
}

func TestMemoryAuditLogger_LogAction(t *testing.T) {
	logger := NewMemoryAuditLogger()

	entry := &AuditLogEntry{
		PaymentID:     "payment-123",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
	}

	id, err := logger.LogAction(entry)
	if err != nil {
		t.Fatalf("LogAction() error = %v", err)
	}
	if id == "" {
		t.Error("LogAction() returned empty ID")
	}
	if entry.Timestamp.IsZero() {
		t.Error("LogAction() did not set timestamp")
	}
}

func TestMemoryAuditLogger_GetAuditTrail(t *testing.T) {
	logger := NewMemoryAuditLogger()

	// Log entries for multiple payments
	logger.LogAction(&AuditLogEntry{
		PaymentID:     "payment-1",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
	})

	logger.LogAction(&AuditLogEntry{
		PaymentID:     "payment-1",
		Action:        AuditActionFund,
		PreviousState: EscrowPending,
		NewState:      EscrowFunded,
	})

	logger.LogAction(&AuditLogEntry{
		PaymentID:     "payment-2",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
	})

	// Get audit trail for payment-1
	trail, err := logger.GetAuditTrail("payment-1")
	if err != nil {
		t.Fatalf("GetAuditTrail() error = %v", err)
	}

	if len(trail) != 2 {
		t.Errorf("GetAuditTrail() returned %d entries, want 2", len(trail))
	}

	// Verify entries are for the correct payment
	for _, entry := range trail {
		if entry.PaymentID != "payment-1" {
			t.Errorf("GetAuditTrail() entry has PaymentID = %s, want payment-1", entry.PaymentID)
		}
	}
}

func TestMemoryAuditLogger_GetAllEntries(t *testing.T) {
	logger := NewMemoryAuditLogger()

	// Log entries for multiple payments
	logger.LogAction(&AuditLogEntry{
		PaymentID:     "payment-1",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
	})

	logger.LogAction(&AuditLogEntry{
		PaymentID:     "payment-2",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
	})

	allEntries, err := logger.GetAllEntries()
	if err != nil {
		t.Fatalf("GetAllEntries() error = %v", err)
	}

	if len(allEntries) != 2 {
		t.Errorf("GetAllEntries() returned %d entries, want 2", len(allEntries))
	}
}

func TestEscrowManager_AuditLogging(t *testing.T) {
	// Create a simple escrow manager with an audit logger
	store := NewMemoryStore()
	pw := &Paywall{
		Store:           store,
		HDWallets:       make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled: true,
	}

	logger := NewMemoryAuditLogger()
	em, err := NewEscrowManagerWithAudit(pw, logger)
	if err != nil {
		t.Fatalf("NewEscrowManagerWithAudit() error = %v", err)
	}

	// Test logging state transitions directly
	paymentID := "test-payment-123"
	actor := []byte("test-actor-key-1234567890")

	// Log create
	em.logStateTransition(paymentID, AuditActionCreate, EscrowNone, EscrowPending,
		nil, "", nil, map[string]string{"test": "create"})

	// Log fund
	em.logStateTransition(paymentID, AuditActionFund, EscrowPending, EscrowFunded,
		actor, RoleBuyer, nil, nil)

	// Log dispute
	em.logStateTransition(paymentID, AuditActionDispute, EscrowFunded, EscrowDisputed,
		actor, RoleSeller, nil, map[string]string{"reason": "Product not as described"})

	// Verify audit trail
	trail, err := logger.GetAuditTrail(paymentID)
	if err != nil {
		t.Fatalf("GetAuditTrail() error = %v", err)
	}

	expectedActions := []AuditAction{AuditActionCreate, AuditActionFund, AuditActionDispute}
	if len(trail) != len(expectedActions) {
		t.Fatalf("GetAuditTrail() returned %d entries, want %d", len(trail), len(expectedActions))
	}

	// Verify action sequence
	for i, entry := range trail {
		if entry.Action != expectedActions[i] {
			t.Errorf("Audit trail[%d] action = %s, want %s", i, entry.Action, expectedActions[i])
		}
		if entry.PaymentID != paymentID {
			t.Errorf("Audit trail[%d] payment ID = %s, want %s", i, entry.PaymentID, paymentID)
		}
	}

	// Verify state transitions
	if trail[0].PreviousState != EscrowNone || trail[0].NewState != EscrowPending {
		t.Errorf("Create entry has wrong states: %v -> %v", trail[0].PreviousState, trail[0].NewState)
	}
	if trail[1].PreviousState != EscrowPending || trail[1].NewState != EscrowFunded {
		t.Errorf("Fund entry has wrong states: %v -> %v", trail[1].PreviousState, trail[1].NewState)
	}
	if trail[2].PreviousState != EscrowFunded || trail[2].NewState != EscrowDisputed {
		t.Errorf("Dispute entry has wrong states: %v -> %v", trail[2].PreviousState, trail[2].NewState)
	}

	// Verify metadata
	if trail[2].Metadata == nil || trail[2].Metadata["reason"] != "Product not as described" {
		t.Errorf("Dispute entry missing or incorrect reason metadata")
	}

	// Verify actor information
	if trail[1].ActorRole != RoleBuyer {
		t.Errorf("Fund entry actor role = %v, want RoleBuyer", trail[1].ActorRole)
	}
}

func TestMemoryAuditLogger_ConcurrentAccess(t *testing.T) {
	logger := NewMemoryAuditLogger()

	const numGoroutines = 10
	const entriesPerGoroutine = 5

	done := make(chan bool, numGoroutines)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			for j := 0; j < entriesPerGoroutine; j++ {
				entry := &AuditLogEntry{
					PaymentID:     "concurrent-test",
					Action:        AuditActionCreate,
					PreviousState: EscrowNone,
					NewState:      EscrowPending,
				}
				_, err := logger.LogAction(entry)
				if err != nil {
					t.Errorf("LogAction() error = %v", err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all entries were logged
	allEntries, _ := logger.GetAllEntries()
	expected := numGoroutines * entriesPerGoroutine
	if len(allEntries) != expected {
		t.Errorf("GetAllEntries() returned %d entries, want %d", len(allEntries), expected)
	}
}
