package paywall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

func TestStructuredLogger_LogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelWarn, true)

	// These should be filtered out (below WARN level)
	logger.LogPaymentCreated("test-1", 0.001, wallet.Bitcoin, false)
	logger.LogPartialSignatureVerified("test-2", RoleBuyer, 0)

	// This should be logged (WARN level)
	logger.LogEscrowTimeout("test-3", EscrowFunded, 5000)

	// This should be logged (ERROR level)
	logger.LogSignatureVerificationFailed("test-4", RoleSeller, "invalid signature")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 2 {
		t.Errorf("expected 2 log lines, got %d:\n%s", len(lines), output)
	}

	// Verify the logged events are the expected ones
	if !strings.Contains(output, "escrow_timeout") {
		t.Error("expected escrow_timeout event to be logged")
	}
	if !strings.Contains(output, "signature_verification_failed") {
		t.Error("expected signature_verification_failed event to be logged")
	}

	// Verify filtered events are not present
	if strings.Contains(output, "payment_created") {
		t.Error("payment_created should have been filtered out")
	}
	if strings.Contains(output, "partial_signature_verified") {
		t.Error("partial_signature_verified should have been filtered out")
	}
}

func TestStructuredLogger_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	paymentID := "test-payment-123"
	logger.LogPaymentCreated(paymentID, 0.001, wallet.Bitcoin, true)

	output := buf.String()
	
	// Verify it's valid JSON
	var entry LogEntry
	err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry)
	if err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, output)
	}

	// Verify fields
	if entry.Event != "payment_created" {
		t.Errorf("expected event 'payment_created', got '%s'", entry. Event)
	}
	if entry.PaymentID != paymentID {
		t.Errorf("expected payment_id '%s', got '%s'", paymentID, entry.PaymentID)
	}
	if entry.Amount != 0.001 {
		t.Errorf("expected amount 0.001, got %f", entry.Amount)
	}
	if entry.Currency != wallet.Bitcoin {
		t.Errorf("expected currency Bitcoin, got %s", entry.Currency)
	}
	if entry.Data["multisig_enabled"] != true {
		t.Error("expected multisig_enabled to be true")
	}
}

func TestStructuredLogger_HumanReadableOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, false)

	paymentID := "test-payment-123"
	logger.LogPaymentCreated(paymentID, 0.001, wallet.Bitcoin, true)

	output := buf.String()

	// Verify human-readable format contains key elements
	if !strings.Contains(output, "INFO") {
		t.Error("expected INFO level in output")
	}
	if !strings.Contains(output, "payment_created") {
		t.Error("expected payment_created event in output")
	}
	if !strings.Contains(output, paymentID) {
		t.Error("expected payment ID in output")
	}
}

func TestStructuredLogger_MultisigEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	paymentID := "test-payment-123"
	
	// Log various multisig events
	logger.LogMultisigAddressGenerated(paymentID, "2MzQw...", wallet.Bitcoin, 2, 3)
	logger.LogPartialSignatureSubmitted(paymentID, RoleBuyer, 0)
	logger.LogPartialSignatureVerified(paymentID, RoleBuyer, 0)
	logger.LogSignatureThresholdReached(paymentID, 2, 2)
	logger.LogMultisigTransactionBroadcast(paymentID, "abc123...", wallet.Bitcoin)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 5 {
		t.Errorf("expected 5 log lines, got %d", len(lines))
	}

	// Verify all events are present
	expectedEvents := []string{
		"multisig_address_generated",
		"partial_signature_submitted",
		"partial_signature_verified",
		"signature_threshold_reached",
		"multisig_transaction_broadcast",
	}

	for _, event := range expectedEvents {
		if !strings.Contains(output, event) {
			t.Errorf("expected event '%s' in output", event)
		}
	}
}

func TestStructuredLogger_EscrowEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	paymentID := "test-payment-123"
	
	// Log escrow lifecycle
	logger.LogEscrowCreated(paymentID, 0.001, wallet.Bitcoin, []MultisigRole{RoleBuyer, RoleSeller, RoleArbiter})
	logger.LogEscrowStateTransition(paymentID, EscrowPending, EscrowFunded, RoleBuyer)
	logger.LogEscrowFunded(paymentID, "tx123", 0.001, wallet.Bitcoin)
	logger.LogEscrowCompleted(paymentID, RoleSeller)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 4 {
		t.Errorf("expected 4 log lines, got %d", len(lines))
	}

	// Parse first line to verify structure
	var firstEntry LogEntry
	err := json.Unmarshal([]byte(lines[0]), &firstEntry)
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if firstEntry.Event != "escrow_created" {
		t.Errorf("expected escrow_created, got %s", firstEntry.Event)
	}
	if firstEntry.State != EscrowPending {
		t.Errorf("expected state EscrowPending, got %s", firstEntry.State)
	}
}

func TestStructuredLogger_DisputeEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	paymentID := "test-payment-123"
	
	logger.LogDisputeInitiated(paymentID, RoleBuyer, "Product not delivered")
	logger.LogArbiterVoteSubmitted(paymentID, 0, RoleSeller)
	logger.LogArbiterVoteSubmitted(paymentID, 1, RoleSeller)
	logger.LogDisputeResolved(paymentID, RoleSeller, true, 3600000)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 4 {
		t.Errorf("expected 4 log lines, got %d", len(lines))
	}

	// Verify dispute resolution contains expected data
	var resolvedEntry LogEntry
	err := json.Unmarshal([]byte(lines[3]), &resolvedEntry)
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if resolvedEntry.Event != "dispute_resolved" {
		t.Errorf("expected dispute_resolved, got %s", resolvedEntry.Event)
	}
	if resolvedEntry.Data["winner"] != string(RoleSeller) {
		t.Error("expected winner to be RoleSeller")
	}
	if resolvedEntry.Data["consensus_reached"] != true {
		t.Error("expected consensus_reached to be true")
	}
}

func TestStructuredLogger_ErrorEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelError, true)

	paymentID := "test-payment-123"
	
	logger.LogSignatureVerificationFailed(paymentID, RoleBuyer, "invalid signature format")
	logger.LogTransactionBroadcastFailed(paymentID, "tx123", fmt.Errorf("network error"))
	logger.LogInvalidStateTransition(paymentID, EscrowCompleted, EscrowPending)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("expected 3 log lines, got %d", len(lines))
	}

	// All should be ERROR level
	for _, line := range lines {
		var entry LogEntry
		err := json.Unmarshal([]byte(line), &entry)
		if err != nil {
			t.Fatalf("failed to parse JSON: %v", err)
		}
		if entry.Level != LogLevelError {
			t.Errorf("expected ERROR level, got %s", entry.Level)
		}
	}
}

func TestStructuredLogger_TimeoutEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	paymentID := "test-payment-123"
	
	logger.LogEscrowTimeout(paymentID, EscrowFunded, 86400000)
	logger.LogTimeoutAutomation(paymentID, "auto_refund_to_buyer")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 2 {
		t.Errorf("expected 2 log lines, got %d", len(lines))
	}

	// First should be WARN
	var timeoutEntry LogEntry
	err := json.Unmarshal([]byte(lines[0]), &timeoutEntry)
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if timeoutEntry.Level != LogLevelWarn {
		t.Errorf("expected WARN level, got %s", timeoutEntry.Level)
	}

	// Second should be INFO
	var automationEntry LogEntry
	err = json.Unmarshal([]byte(lines[1]), &automationEntry)
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if automationEntry.Level != LogLevelInfo {
		t.Errorf("expected INFO level, got %s", automationEntry.Level)
	}
}

func TestStructuredLogger_PaymentEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	paymentID := "test-payment-123"
	
	logger.LogPaymentCreated(paymentID, 0.001, wallet.Bitcoin, true)
	logger.LogPaymentConfirmed(paymentID, 6, "tx123")
	logger.LogPaymentExpired(paymentID, time.Now().Add(-25*time.Hour))

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("expected 3 log lines, got %d", len(lines))
	}

	// Verify payment confirmed has confirmations data
	var confirmedEntry LogEntry
	err := json.Unmarshal([]byte(lines[1]), &confirmedEntry)
	if err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if confirmedEntry.Data["confirmations"] != float64(6) {
		t.Errorf("expected 6 confirmations, got %v", confirmedEntry.Data["confirmations"])
	}
}

func TestStructuredLogger_DefaultLogger(t *testing.T) {
	logger := NewDefaultLogger()

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	if logger.minLevel != LogLevelInfo {
		t.Errorf("expected INFO min level, got %s", logger.minLevel)
	}
	if !logger.jsonOutput {
		t.Error("expected JSON output enabled")
	}
}

func BenchmarkStructuredLogger_LogEvent(b *testing.B) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelInfo, true)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.LogPaymentCreated("test-payment", 0.001, wallet.Bitcoin, true)
	}
}

func BenchmarkStructuredLogger_LogEventFiltered(b *testing.B) {
	var buf bytes.Buffer
	logger := NewStructuredLogger(&buf, LogLevelError, true)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// This should be filtered out (INFO < ERROR)
		logger.LogPaymentCreated("test-payment", 0.001, wallet.Bitcoin, true)
	}
}
