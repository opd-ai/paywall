package paywall

import (
	"sync"
	"testing"
	"time"
)

func TestMetricsCollector_MultisigOperations(t *testing.T) {
	m := NewMetricsCollector()

	m.IncrementMultisigAddressGenerated()
	m.IncrementMultisigAddressGenerated()
	m.IncrementPartialSignatureSubmitted()
	m.IncrementMultisigTransactionCompleted()

	snapshot := m.Snapshot()

	if snapshot.MultisigAddressGenerated != 2 {
		t.Errorf("expected 2 multisig addresses generated, got %d", snapshot.MultisigAddressGenerated)
	}
	if snapshot.PartialSignatureSubmitted != 1 {
		t.Errorf("expected 1 partial signature submitted, got %d", snapshot.PartialSignatureSubmitted)
	}
	if snapshot.MultisigTransactionCompleted != 1 {
		t.Errorf("expected 1 multisig transaction completed, got %d", snapshot.MultisigTransactionCompleted)
	}
}

func TestMetricsCollector_EscrowStateTransitions(t *testing.T) {
	m := NewMetricsCollector()

	m.IncrementEscrowCreated()
	m.IncrementEscrowFunded()
	m.IncrementEscrowCompleted()
	m.IncrementEscrowDisputed()
	m.IncrementEscrowDisputeResolved()

	snapshot := m.Snapshot()

	if snapshot.EscrowCreated != 1 {
		t.Errorf("expected 1 escrow created, got %d", snapshot.EscrowCreated)
	}
	if snapshot.EscrowFunded != 1 {
		t.Errorf("expected 1 escrow funded, got %d", snapshot.EscrowFunded)
	}
	if snapshot.EscrowCompleted != 1 {
		t.Errorf("expected 1 escrow completed, got %d", snapshot.EscrowCompleted)
	}
	if snapshot.EscrowDisputed != 1 {
		t.Errorf("expected 1 escrow disputed, got %d", snapshot.EscrowDisputed)
	}
	if snapshot.EscrowDisputeResolved != 1 {
		t.Errorf("expected 1 escrow dispute resolved, got %d", snapshot.EscrowDisputeResolved)
	}
}

func TestMetricsCollector_DisputeResolutionTiming(t *testing.T) {
	m := NewMetricsCollector()

	// Record 3 dispute resolutions
	m.RecordDisputeResolutionDuration(100 * time.Millisecond)
	m.RecordDisputeResolutionDuration(200 * time.Millisecond)
	m.RecordDisputeResolutionDuration(300 * time.Millisecond)

	snapshot := m.Snapshot()

	if snapshot.DisputeResolutionCount != 3 {
		t.Errorf("expected 3 dispute resolutions, got %d", snapshot.DisputeResolutionCount)
	}

	expectedTotal := int64(600)
	if snapshot.DisputeResolutionTotalMs != expectedTotal {
		t.Errorf("expected total %d ms, got %d ms", expectedTotal, snapshot.DisputeResolutionTotalMs)
	}

	expectedAvg := int64(200)
	if snapshot.DisputeResolutionAvgMs != expectedAvg {
		t.Errorf("expected avg %d ms, got %d ms", expectedAvg, snapshot.DisputeResolutionAvgMs)
	}
}

func TestMetricsCollector_PaymentOperations(t *testing.T) {
	m := NewMetricsCollector()

	m.IncrementPaymentCreated()
	m.IncrementPaymentCreated()
	m.IncrementPaymentConfirmed()
	m.IncrementPaymentExpired()

	snapshot := m.Snapshot()

	if snapshot.PaymentCreated != 2 {
		t.Errorf("expected 2 payments created, got %d", snapshot.PaymentCreated)
	}
	if snapshot.PaymentConfirmed != 1 {
		t.Errorf("expected 1 payment confirmed, got %d", snapshot.PaymentConfirmed)
	}
	if snapshot.PaymentExpired != 1 {
		t.Errorf("expected 1 payment expired, got %d", snapshot.PaymentExpired)
	}
}

func TestMetricsCollector_ErrorMetrics(t *testing.T) {
	m := NewMetricsCollector()

	m.IncrementSignatureVerificationFailed()
	m.IncrementTransactionBroadcastFailed()
	m.IncrementEscrowTimeoutTriggered()
	m.IncrementArbiterConsensusRequired()

	snapshot := m.Snapshot()

	if snapshot.SignatureVerificationFailed != 1 {
		t.Errorf("expected 1 signature verification failed, got %d", snapshot.SignatureVerificationFailed)
	}
	if snapshot.TransactionBroadcastFailed != 1 {
		t.Errorf("expected 1 transaction broadcast failed, got %d", snapshot.TransactionBroadcastFailed)
	}
	if snapshot.EscrowTimeoutTriggered != 1 {
		t.Errorf("expected 1 escrow timeout triggered, got %d", snapshot.EscrowTimeoutTriggered)
	}
	if snapshot.ArbiterConsensusRequired != 1 {
		t.Errorf("expected 1 arbiter consensus required, got %d", snapshot.ArbiterConsensusRequired)
	}
}

func TestMetricsCollector_PerformanceMetrics(t *testing.T) {
	m := NewMetricsCollector()

	m.RecordAddressGenerationDuration(50 * time.Millisecond)
	m.RecordAddressGenerationDuration(100 * time.Millisecond)
	m.RecordSignatureVerificationDuration(25 * time.Millisecond)
	m.RecordStateTransitionDuration(10 * time.Millisecond)

	snapshot := m.Snapshot()

	if snapshot.AddressGenerationDurationMs != 150 {
		t.Errorf("expected 150ms address generation, got %d ms", snapshot.AddressGenerationDurationMs)
	}
	if snapshot.SignatureVerificationDurationMs != 25 {
		t.Errorf("expected 25ms signature verification, got %d ms", snapshot.SignatureVerificationDurationMs)
	}
	if snapshot.StateTransitionDurationMs != 10 {
		t.Errorf("expected 10ms state transition, got %d ms", snapshot.StateTransitionDurationMs)
	}
}

func TestMetricsCollector_Reset(t *testing.T) {
	m := NewMetricsCollector()

	// Set some metrics
	m.IncrementPaymentCreated()
	m.IncrementEscrowCreated()
	m.RecordDisputeResolutionDuration(100 * time.Millisecond)

	// Verify metrics are set
	snapshot1 := m.Snapshot()
	if snapshot1.PaymentCreated == 0 {
		t.Error("expected payment created to be set before reset")
	}

	// Reset metrics
	m.Reset()

	// Verify all metrics are zero
	snapshot2 := m.Snapshot()
	if snapshot2.PaymentCreated != 0 {
		t.Errorf("expected payment created to be 0 after reset, got %d", snapshot2.PaymentCreated)
	}
	if snapshot2.EscrowCreated != 0 {
		t.Errorf("expected escrow created to be 0 after reset, got %d", snapshot2.EscrowCreated)
	}
	if snapshot2.DisputeResolutionCount != 0 {
		t.Errorf("expected dispute resolution count to be 0 after reset, got %d", snapshot2.DisputeResolutionCount)
	}
}

func TestMetricsCollector_ConcurrentAccess(t *testing.T) {
	m := NewMetricsCollector()

	// Run 100 goroutines that increment metrics concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.IncrementPaymentCreated()
			m.IncrementEscrowCreated()
			m.RecordDisputeResolutionDuration(10 * time.Millisecond)
		}()
	}

	wg.Wait()

	snapshot := m.Snapshot()

	if snapshot.PaymentCreated != 100 {
		t.Errorf("expected 100 payments created, got %d", snapshot.PaymentCreated)
	}
	if snapshot.EscrowCreated != 100 {
		t.Errorf("expected 100 escrows created, got %d", snapshot.EscrowCreated)
	}
	if snapshot.DisputeResolutionCount != 100 {
		t.Errorf("expected 100 dispute resolutions, got %d", snapshot.DisputeResolutionCount)
	}
}

func TestMetricsCollector_DisputeResolutionRingBuffer(t *testing.T) {
	m := NewMetricsCollector()

	// Record 150 dispute resolutions (more than the 100 buffer size)
	for i := 0; i < 150; i++ {
		m.RecordDisputeResolutionDuration(time.Duration(i+1) * time.Millisecond)
	}

	snapshot := m.Snapshot()

	if snapshot.DisputeResolutionCount != 150 {
		t.Errorf("expected 150 dispute resolutions, got %d", snapshot.DisputeResolutionCount)
	}

	// Total should be sum of 1+2+...+150 = 150*151/2 = 11325
	expectedTotal := int64(11325)
	if snapshot.DisputeResolutionTotalMs != expectedTotal {
		t.Errorf("expected total %d ms, got %d ms", expectedTotal, snapshot.DisputeResolutionTotalMs)
	}

	// Average = 11325 / 150 = 75.5 = 75 (integer division)
	expectedAvg := int64(75)
	if snapshot.DisputeResolutionAvgMs != expectedAvg {
		t.Errorf("expected avg %d ms, got %d ms", expectedAvg, snapshot.DisputeResolutionAvgMs)
	}
}

func BenchmarkMetricsCollector_IncrementCounter(b *testing.B) {
	m := NewMetricsCollector()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m.IncrementPaymentCreated()
	}
}

func BenchmarkMetricsCollector_RecordDuration(b *testing.B) {
	m := NewMetricsCollector()
	duration := 100 * time.Millisecond

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m.RecordDisputeResolutionDuration(duration)
	}
}

func BenchmarkMetricsCollector_Snapshot(b *testing.B) {
	m := NewMetricsCollector()

	// Set some metrics
	m.IncrementPaymentCreated()
	m.IncrementEscrowCreated()
	m.RecordDisputeResolutionDuration(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = m.Snapshot()
	}
}

func BenchmarkMetricsCollector_ConcurrentIncrements(b *testing.B) {
	m := NewMetricsCollector()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.IncrementPaymentCreated()
		}
	})
}
