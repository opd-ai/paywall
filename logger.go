package paywall

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// LogLevel represents the severity of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// StructuredLogger provides structured logging for paywall operations
// Logs are emitted in JSON format for easy parsing and analysis
type StructuredLogger struct {
	writer     io.Writer
	minLevel   LogLevel
	jsonOutput bool
}

// LogEntry represents a single structured log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     LogLevel               `json:"level"`
	Event     string                 `json:"event"`
	Message   string                 `json:"message,omitempty"`
	PaymentID string                 `json:"payment_id,omitempty"`
	EscrowID  string                 `json:"escrow_id,omitempty"`
	Amount    float64                `json:"amount,omitempty"`
	Currency  wallet.WalletType      `json:"currency,omitempty"`
	Role      Role                   `json:"role,omitempty"`
	State     EscrowState            `json:"state,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(writer io.Writer, minLevel LogLevel, jsonOutput bool) *StructuredLogger {
	if writer == nil {
		writer = os.Stdout
	}
	return &StructuredLogger{
		writer:     writer,
		minLevel:   minLevel,
		jsonOutput: jsonOutput,
	}
}

// NewDefaultLogger creates a logger with sensible defaults (stdout, INFO level, JSON output)
func NewDefaultLogger() *StructuredLogger {
	return NewStructuredLogger(os.Stdout, LogLevelInfo, true)
}

func (l *StructuredLogger) shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		LogLevelDebug: 0,
		LogLevelInfo:  1,
		LogLevelWarn:  2,
		LogLevelError: 3,
	}
	return levels[level] >= levels[l.minLevel]
}

func (l *StructuredLogger) log(entry LogEntry) {
	if !l.shouldLog(entry.Level) {
		return
	}

	entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	if l.jsonOutput {
		data, err := json.Marshal(entry)
		if err != nil {
			log.Printf("ERROR: failed to marshal log entry: %v", err)
			return
		}
		fmt.Fprintln(l.writer, string(data))
	} else {
		// Human-readable format
		msg := fmt.Sprintf("[%s] %s - %s", entry.Timestamp, entry.Level, entry.Event)
		if entry.Message != "" {
			msg += ": " + entry.Message
		}
		if entry.PaymentID != "" {
			msg += fmt.Sprintf(" (payment=%s)", entry.PaymentID)
		}
		if entry.EscrowID != "" {
			msg += fmt.Sprintf(" (escrow=%s)", entry.EscrowID)
		}
		fmt.Fprintln(l.writer, msg)
	}
}

// Multisig Events

func (l *StructuredLogger) LogMultisigAddressGenerated(paymentID, address string, walletType wallet.WalletType, required, total int) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "multisig_address_generated",
		Message:   "Generated multisig address",
		PaymentID: paymentID,
		Currency:  walletType,
		Data: map[string]interface{}{
			"address":  address,
			"required": required,
			"total":    total,
		},
	})
}

func (l *StructuredLogger) LogPartialSignatureSubmitted(paymentID string, role Role, signatureIndex int) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "partial_signature_submitted",
		Message:   "Partial signature submitted",
		PaymentID: paymentID,
		Role:      role,
		Data: map[string]interface{}{
			"signature_index": signatureIndex,
		},
	})
}

func (l *StructuredLogger) LogPartialSignatureVerified(paymentID string, role Role, signatureIndex int) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "partial_signature_verified",
		Message:   "Partial signature verified successfully",
		PaymentID: paymentID,
		Role:      role,
		Data: map[string]interface{}{
			"signature_index": signatureIndex,
		},
	})
}

func (l *StructuredLogger) LogSignatureThresholdReached(paymentID string, requiredSigs, collectedSigs int) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "signature_threshold_reached",
		Message:   "Signature threshold reached for transaction",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"required_signatures":  requiredSigs,
			"collected_signatures": collectedSigs,
		},
	})
}

func (l *StructuredLogger) LogMultisigTransactionBroadcast(paymentID, txHash string, walletType wallet.WalletType) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "multisig_transaction_broadcast",
		Message:   "Multisig transaction broadcast to network",
		PaymentID: paymentID,
		Currency:  walletType,
		Data: map[string]interface{}{
			"tx_hash": txHash,
		},
	})
}

// Escrow Events

func (l *StructuredLogger) LogEscrowCreated(paymentID string, amount float64, currency wallet.WalletType, participants []Role) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "escrow_created",
		Message:   "Escrow created",
		PaymentID: paymentID,
		Amount:    amount,
		Currency:  currency,
		State:     EscrowPending,
		Data: map[string]interface{}{
			"participants": participants,
		},
	})
}

func (l *StructuredLogger) LogEscrowStateTransition(paymentID string, fromState, toState EscrowState, role Role) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "escrow_state_transition",
		Message:   fmt.Sprintf("Escrow state transition: %s -> %s", fromState, toState),
		PaymentID: paymentID,
		State:     toState,
		Role:      role,
		Data: map[string]interface{}{
			"from_state": fromState,
			"to_state":   toState,
		},
	})
}

func (l *StructuredLogger) LogEscrowFunded(paymentID, txHash string, amount float64, currency wallet.WalletType) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "escrow_funded",
		Message:   "Escrow funded with transaction",
		PaymentID: paymentID,
		Amount:    amount,
		Currency:  currency,
		State:     EscrowFunded,
		Data: map[string]interface{}{
			"tx_hash": txHash,
		},
	})
}

func (l *StructuredLogger) LogEscrowCompleted(paymentID string, releasedToRole Role) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "escrow_completed",
		Message:   "Escrow completed successfully",
		PaymentID: paymentID,
		State:     EscrowCompleted,
		Data: map[string]interface{}{
			"released_to": releasedToRole,
		},
	})
}

func (l *StructuredLogger) LogEscrowRefunded(paymentID string, refundedToRole Role) {
	l.log(LogEntry{
		Level:     LogLevelWarn,
		Event:     "escrow_refunded",
		Message:   "Escrow refunded",
		PaymentID: paymentID,
		State:     EscrowRefunded,
		Data: map[string]interface{}{
			"refunded_to": refundedToRole,
		},
	})
}

// Dispute Events

func (l *StructuredLogger) LogDisputeInitiated(paymentID string, initiatedBy Role, reason string) {
	l.log(LogEntry{
		Level:     LogLevelWarn,
		Event:     "dispute_initiated",
		Message:   "Dispute initiated",
		PaymentID: paymentID,
		Role:      initiatedBy,
		State:     EscrowDisputed,
		Data: map[string]interface{}{
			"reason": reason,
		},
	})
}

func (l *StructuredLogger) LogArbiterVoteSubmitted(paymentID string, arbiterIndex int, votedFor Role) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "arbiter_vote_submitted",
		Message:   "Arbiter submitted vote",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"arbiter_index": arbiterIndex,
			"voted_for":     votedFor,
		},
	})
}

func (l *StructuredLogger) LogDisputeResolved(paymentID string, winner Role, consensusReached bool, resolutionTimeMs int64) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "dispute_resolved",
		Message:   "Dispute resolved",
		PaymentID: paymentID,
		Role:      winner,
		Data: map[string]interface{}{
			"winner":             winner,
			"consensus_reached":  consensusReached,
			"resolution_time_ms": resolutionTimeMs,
		},
	})
}

// Timeout Events

func (l *StructuredLogger) LogEscrowTimeout(paymentID string, state EscrowState, elapsedMs int64) {
	l.log(LogEntry{
		Level:     LogLevelWarn,
		Event:     "escrow_timeout",
		Message:   "Escrow timeout triggered",
		PaymentID: paymentID,
		State:     state,
		Data: map[string]interface{}{
			"elapsed_ms": elapsedMs,
		},
	})
}

func (l *StructuredLogger) LogTimeoutAutomation(paymentID string, actionTaken string) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "timeout_automation",
		Message:   "Automated timeout action executed",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"action": actionTaken,
		},
	})
}

// Error Events

func (l *StructuredLogger) LogSignatureVerificationFailed(paymentID string, role Role, reason string) {
	l.log(LogEntry{
		Level:     LogLevelError,
		Event:     "signature_verification_failed",
		Message:   "Signature verification failed",
		PaymentID: paymentID,
		Role:      role,
		Data: map[string]interface{}{
			"reason": reason,
		},
	})
}

func (l *StructuredLogger) LogTransactionBroadcastFailed(paymentID, txHash string, err error) {
	l.log(LogEntry{
		Level:     LogLevelError,
		Event:     "transaction_broadcast_failed",
		Message:   "Failed to broadcast transaction",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"tx_hash": txHash,
			"error":   err.Error(),
		},
	})
}

func (l *StructuredLogger) LogInvalidStateTransition(paymentID string, fromState, toState EscrowState) {
	l.log(LogEntry{
		Level:     LogLevelError,
		Event:     "invalid_state_transition",
		Message:   "Attempted invalid state transition",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"from_state": fromState,
			"to_state":   toState,
		},
	})
}

// Payment Events

func (l *StructuredLogger) LogPaymentCreated(paymentID string, amount float64, currency wallet.WalletType, multisigEnabled bool) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "payment_created",
		Message:   "Payment created",
		PaymentID: paymentID,
		Amount:    amount,
		Currency:  currency,
		Data: map[string]interface{}{
			"multisig_enabled": multisigEnabled,
		},
	})
}

func (l *StructuredLogger) LogPaymentConfirmed(paymentID string, confirmations int, txHash string) {
	l.log(LogEntry{
		Level:     LogLevelInfo,
		Event:     "payment_confirmed",
		Message:   "Payment confirmed",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"confirmations": confirmations,
			"tx_hash":       txHash,
		},
	})
}

func (l *StructuredLogger) LogPaymentExpired(paymentID string, createdAt time.Time) {
	l.log(LogEntry{
		Level:     LogLevelWarn,
		Event:     "payment_expired",
		Message:   "Payment expired",
		PaymentID: paymentID,
		Data: map[string]interface{}{
			"created_at": createdAt.Format(time.RFC3339),
			"age_hours":  time.Since(createdAt).Hours(),
		},
	})
}
