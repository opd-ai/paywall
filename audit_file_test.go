package paywall

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileAuditLogger(t *testing.T) {
	// Create temp directory for test
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}
	defer logger.Close()

	if logger == nil {
		t.Fatal("NewFileAuditLogger() returned nil")
	}

	// Verify file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Audit log file was not created")
	}
}

func TestFileAuditLogger_LogAction(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}
	defer logger.Close()

	entry := &AuditLogEntry{
		PaymentID:     "test-payment-1",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
		ActorRole:     RoleBuyer,
		Metadata:      map[string]string{"test": "data"},
	}

	entryID, err := logger.LogAction(entry)
	if err != nil {
		t.Fatalf("LogAction() failed: %v", err)
	}

	if entryID == "" {
		t.Error("LogAction() returned empty entry ID")
	}

	// Verify entry was persisted by creating a new logger
	logger.Close()
	logger2, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("Failed to reopen audit log: %v", err)
	}
	defer logger2.Close()

	allEntries, err := logger2.GetAllEntries()
	if err != nil {
		t.Fatalf("GetAllEntries() failed: %v", err)
	}

	if len(allEntries) != 1 {
		t.Fatalf("Expected 1 entry after reload, got %d", len(allEntries))
	}

	if allEntries[0].PaymentID != "test-payment-1" {
		t.Errorf("Expected paymentID test-payment-1, got %s", allEntries[0].PaymentID)
	}
}

func TestFileAuditLogger_GetAuditTrail(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}
	defer logger.Close()

	// Log multiple entries for different payments
	entries := []*AuditLogEntry{
		{
			PaymentID:     "payment-1",
			Action:        AuditActionCreate,
			PreviousState: EscrowNone,
			NewState:      EscrowPending,
			ActorRole:     RoleBuyer,
		},
		{
			PaymentID:     "payment-2",
			Action:        AuditActionCreate,
			PreviousState: EscrowNone,
			NewState:      EscrowPending,
			ActorRole:     RoleBuyer,
		},
		{
			PaymentID:     "payment-1",
			Action:        AuditActionFund,
			PreviousState: EscrowPending,
			NewState:      EscrowFunded,
			ActorRole:     RoleBuyer,
		},
	}

	for _, entry := range entries {
		if _, err := logger.LogAction(entry); err != nil {
			t.Fatalf("LogAction() failed: %v", err)
		}
	}

	// Retrieve trail for payment-1
	trail, err := logger.GetAuditTrail("payment-1")
	if err != nil {
		t.Fatalf("GetAuditTrail() failed: %v", err)
	}

	if len(trail) != 2 {
		t.Fatalf("Expected 2 entries for payment-1, got %d", len(trail))
	}

	// Verify entries are in chronological order
	if trail[0].Action != AuditActionCreate {
		t.Errorf("Expected first action to be Create, got %s", trail[0].Action)
	}
	if trail[1].Action != AuditActionFund {
		t.Errorf("Expected second action to be Fund, got %s", trail[1].Action)
	}
}

func TestFileAuditLogger_GetAllEntries(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}
	defer logger.Close()

	// Log multiple entries
	for i := 0; i < 5; i++ {
		entry := &AuditLogEntry{
			PaymentID:     "test-payment",
			Action:        AuditActionCreate,
			PreviousState: EscrowNone,
			NewState:      EscrowPending,
			ActorRole:     RoleBuyer,
		}
		if _, err := logger.LogAction(entry); err != nil {
			t.Fatalf("LogAction() failed: %v", err)
		}
	}

	allEntries, err := logger.GetAllEntries()
	if err != nil {
		t.Fatalf("GetAllEntries() failed: %v", err)
	}

	if len(allEntries) != 5 {
		t.Fatalf("Expected 5 entries, got %d", len(allEntries))
	}
}

func TestFileAuditLogger_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	// Create logger and write entries
	logger1, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}

	entry1 := &AuditLogEntry{
		PaymentID:     "persist-test",
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
		ActorRole:     RoleBuyer,
		Timestamp:     time.Now(),
	}
	_, err = logger1.LogAction(entry1)
	if err != nil {
		t.Fatalf("LogAction() failed: %v", err)
	}
	logger1.Close()

	// Create new logger and verify entries were persisted
	logger2, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("Failed to reopen audit log: %v", err)
	}
	defer logger2.Close()

	trail, err := logger2.GetAuditTrail("persist-test")
	if err != nil {
		t.Fatalf("GetAuditTrail() failed: %v", err)
	}

	if len(trail) != 1 {
		t.Fatalf("Expected 1 persisted entry, got %d", len(trail))
	}

	if trail[0].PaymentID != "persist-test" {
		t.Errorf("Persisted entry has wrong payment ID: %s", trail[0].PaymentID)
	}
}

func TestFileAuditLogger_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}
	defer logger.Close()

	// Launch multiple goroutines writing concurrently
	numGoroutines := 10
	entriesPerGoroutine := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()
			for j := 0; j < entriesPerGoroutine; j++ {
				entry := &AuditLogEntry{
					PaymentID:     "concurrent-test",
					Action:        AuditActionCreate,
					PreviousState: EscrowNone,
					NewState:      EscrowPending,
					ActorRole:     RoleBuyer,
				}
				if _, err := logger.LogAction(entry); err != nil {
					t.Errorf("Concurrent LogAction() failed: %v", err)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all entries were written
	trail, err := logger.GetAuditTrail("concurrent-test")
	if err != nil {
		t.Fatalf("GetAuditTrail() failed: %v", err)
	}

	expected := numGoroutines * entriesPerGoroutine
	if len(trail) != expected {
		t.Errorf("Expected %d entries from concurrent writes, got %d", expected, len(trail))
	}
}

func TestFileAuditLogger_EmptyPaymentID(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "audit.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed: %v", err)
	}
	defer logger.Close()

	_, err = logger.GetAuditTrail("")
	if err == nil {
		t.Error("GetAuditTrail() should return error for empty payment ID")
	}
}

func TestFileAuditLogger_DirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	// Use nested directory that doesn't exist
	logPath := filepath.Join(tempDir, "logs", "audit", "escrow.log")

	logger, err := NewFileAuditLogger(logPath)
	if err != nil {
		t.Fatalf("NewFileAuditLogger() failed to create nested directories: %v", err)
	}
	defer logger.Close()

	// Verify directory was created
	dir := filepath.Dir(logPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}
}
