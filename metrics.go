package paywall

import (
	"sync"
	"time"
)

// MetricsCollector provides Prometheus-style metrics for paywall operations
// This implementation uses simple atomic counters and can be replaced with
// actual Prometheus instrumentation by importing prometheus/client_golang
type MetricsCollector struct {
	mu sync.RWMutex

	// Multisig operation counters
	multisigAddressGenerated      int64
	partialSignatureSubmitted     int64
	partialSignatureVerified      int64
	multisigTransactionCompleted  int64
	multisigTransactionBroadcast  int64

	// Escrow state transition counters
	escrowCreated                 int64
	escrowFunded                  int64
	escrowCompleted               int64
	escrowRefunded                int64
	escrowDisputed                int64
	escrowDisputeResolved         int64

	// Dispute resolution timing
	disputeResolutionDurations    []time.Duration
	disputeResolutionCount        int64
	disputeResolutionTotalMs      int64

	// Payment operations
	paymentCreated                int64
	paymentConfirmed              int64
	paymentExpired                int64

	// Error counters
	signatureVerificationFailed   int64
	transactionBroadcastFailed    int64
	escrowTimeoutTriggered        int64
	arbiterConsensusRequired      int64

	// Performance metrics
	addressGenerationDurationMs   int64
	signatureVerificationDurationMs int64
	stateTransitionDurationMs     int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		disputeResolutionDurations: make([]time.Duration, 0, 100),
	}
}

// Multisig Operation Metrics

func (m *MetricsCollector) IncrementMultisigAddressGenerated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.multisigAddressGenerated++
}

func (m *MetricsCollector) IncrementPartialSignatureSubmitted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partialSignatureSubmitted++
}

func (m *MetricsCollector) IncrementPartialSignatureVerified() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.partialSignatureVerified++
}

func (m *MetricsCollector) IncrementMultisigTransactionCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.multisigTransactionCompleted++
}

func (m *MetricsCollector) IncrementMultisigTransactionBroadcast() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.multisigTransactionBroadcast++
}

// Escrow State Transition Metrics

func (m *MetricsCollector) IncrementEscrowCreated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowCreated++
}

func (m *MetricsCollector) IncrementEscrowFunded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowFunded++
}

func (m *MetricsCollector) IncrementEscrowCompleted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowCompleted++
}

func (m *MetricsCollector) IncrementEscrowRefunded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowRefunded++
}

func (m *MetricsCollector) IncrementEscrowDisputed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowDisputed++
}

func (m *MetricsCollector) IncrementEscrowDisputeResolved() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowDisputeResolved++
}

// Dispute Resolution Timing Metrics

func (m *MetricsCollector) RecordDisputeResolutionDuration(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.disputeResolutionCount++
	durationMs := duration.Milliseconds()
	m.disputeResolutionTotalMs += durationMs
	
	// Keep last 100 durations for percentile calculations
	if len(m.disputeResolutionDurations) < 100 {
		m.disputeResolutionDurations = append(m.disputeResolutionDurations, duration)
	} else {
		// Ring buffer behavior - overwrite oldest
		idx := int(m.disputeResolutionCount-1) % 100
		m.disputeResolutionDurations[idx] = duration
	}
}

// Payment Operation Metrics

func (m *MetricsCollector) IncrementPaymentCreated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paymentCreated++
}

func (m *MetricsCollector) IncrementPaymentConfirmed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paymentConfirmed++
}

func (m *MetricsCollector) IncrementPaymentExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paymentExpired++
}

// Error Metrics

func (m *MetricsCollector) IncrementSignatureVerificationFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signatureVerificationFailed++
}

func (m *MetricsCollector) IncrementTransactionBroadcastFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transactionBroadcastFailed++
}

func (m *MetricsCollector) IncrementEscrowTimeoutTriggered() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrowTimeoutTriggered++
}

func (m *MetricsCollector) IncrementArbiterConsensusRequired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.arbiterConsensusRequired++
}

// Performance Metrics

func (m *MetricsCollector) RecordAddressGenerationDuration(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addressGenerationDurationMs += duration.Milliseconds()
}

func (m *MetricsCollector) RecordSignatureVerificationDuration(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signatureVerificationDurationMs += duration.Milliseconds()
}

func (m *MetricsCollector) RecordStateTransitionDuration(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateTransitionDurationMs += duration.Milliseconds()
}

// Snapshot returns a copy of current metrics
type MetricsSnapshot struct {
	// Multisig operations
	MultisigAddressGenerated     int64
	PartialSignatureSubmitted    int64
	PartialSignatureVerified     int64
	MultisigTransactionCompleted int64
	MultisigTransactionBroadcast int64

	// Escrow state transitions
	EscrowCreated         int64
	EscrowFunded          int64
	EscrowCompleted       int64
	EscrowRefunded        int64
	EscrowDisputed        int64
	EscrowDisputeResolved int64

	// Dispute resolution
	DisputeResolutionCount    int64
	DisputeResolutionAvgMs    int64
	DisputeResolutionTotalMs  int64

	// Payment operations
	PaymentCreated   int64
	PaymentConfirmed int64
	PaymentExpired   int64

	// Errors
	SignatureVerificationFailed int64
	TransactionBroadcastFailed  int64
	EscrowTimeoutTriggered      int64
	ArbiterConsensusRequired    int64

	// Performance
	AddressGenerationDurationMs      int64
	SignatureVerificationDurationMs  int64
	StateTransitionDurationMs        int64
}

func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgDisputeMs := int64(0)
	if m.disputeResolutionCount > 0 {
		avgDisputeMs = m.disputeResolutionTotalMs / m.disputeResolutionCount
	}

	return MetricsSnapshot{
		MultisigAddressGenerated:        m.multisigAddressGenerated,
		PartialSignatureSubmitted:       m.partialSignatureSubmitted,
		PartialSignatureVerified:        m.partialSignatureVerified,
		MultisigTransactionCompleted:    m.multisigTransactionCompleted,
		MultisigTransactionBroadcast:    m.multisigTransactionBroadcast,
		EscrowCreated:                   m.escrowCreated,
		EscrowFunded:                    m.escrowFunded,
		EscrowCompleted:                 m.escrowCompleted,
		EscrowRefunded:                  m.escrowRefunded,
		EscrowDisputed:                  m.escrowDisputed,
		EscrowDisputeResolved:           m.escrowDisputeResolved,
		DisputeResolutionCount:          m.disputeResolutionCount,
		DisputeResolutionAvgMs:          avgDisputeMs,
		DisputeResolutionTotalMs:        m.disputeResolutionTotalMs,
		PaymentCreated:                  m.paymentCreated,
		PaymentConfirmed:                m.paymentConfirmed,
		PaymentExpired:                  m.paymentExpired,
		SignatureVerificationFailed:     m.signatureVerificationFailed,
		TransactionBroadcastFailed:      m.transactionBroadcastFailed,
		EscrowTimeoutTriggered:          m.escrowTimeoutTriggered,
		ArbiterConsensusRequired:        m.arbiterConsensusRequired,
		AddressGenerationDurationMs:     m.addressGenerationDurationMs,
		SignatureVerificationDurationMs: m.signatureVerificationDurationMs,
		StateTransitionDurationMs:       m.stateTransitionDurationMs,
	}
}

// Reset clears all metrics (useful for testing)
func (m *MetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.multisigAddressGenerated = 0
	m.partialSignatureSubmitted = 0
	m.partialSignatureVerified = 0
	m.multisigTransactionCompleted = 0
	m.multisigTransactionBroadcast = 0
	m.escrowCreated = 0
	m.escrowFunded = 0
	m.escrowCompleted = 0
	m.escrowRefunded = 0
	m.escrowDisputed = 0
	m.escrowDisputeResolved = 0
	m.disputeResolutionDurations = make([]time.Duration, 0, 100)
	m.disputeResolutionCount = 0
	m.disputeResolutionTotalMs = 0
	m.paymentCreated = 0
	m.paymentConfirmed = 0
	m.paymentExpired = 0
	m.signatureVerificationFailed = 0
	m.transactionBroadcastFailed = 0
	m.escrowTimeoutTriggered = 0
	m.arbiterConsensusRequired = 0
	m.addressGenerationDurationMs = 0
	m.signatureVerificationDurationMs = 0
	m.stateTransitionDurationMs = 0
}
