package paywall

import (
	"fmt"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// BenchmarkEscrowManager_CreateEscrow benchmarks escrow payment creation
func BenchmarkEscrowManager_CreateEscrow(b *testing.B) {
	store := NewMemoryStore()

	key1, key2, key3 := make([]byte, 33), make([]byte, 33), make([]byte, 33)
	copy(key1, []byte{0x02})
	copy(key2, []byte{0x03})
	copy(key3, []byte{0x04})

	config := Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {key1, key2, key3},
		},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		b.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := NewEscrowManager(pw)
	if err != nil {
		b.Fatalf("Failed to create escrow manager: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		escrowMgr.CreateEscrow(1.0, time.Hour*48)
	}
}

// BenchmarkPayment_MultisigMetadata benchmarks multisig metadata operations
func BenchmarkPayment_MultisigMetadata(b *testing.B) {
	// Create payment with multisig metadata
	payment := &Payment{
		ID:               "test",
		MultisigEnabled:  true,
		MultisigMetadata: make(map[wallet.WalletType]*wallet.MultisigMetadata),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payment.MultisigMetadata[wallet.Bitcoin] = &wallet.MultisigMetadata{
			PublicKeys: [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)},
		}
	}
}

// BenchmarkPayment_SignatureCollection benchmarks signature append operations
func BenchmarkPayment_SignatureCollection(b *testing.B) {
	sigs := make([]SignatureData, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig := SignatureData{
			SignerID:  "signer",
			Role:      RoleBuyer,
			PublicKey: make([]byte, 33),
			Signature: make([]byte, 64),
			SignedAt:  time.Now(),
		}
		sigs = append(sigs, sig)
	}
}

// BenchmarkGetEscrowsExpiringBefore_10k benchmarks timeout checking with 10,000 escrows
func BenchmarkGetEscrowsExpiringBefore_10k(b *testing.B) {
	store := NewMemoryStore()

	// Populate store with 10,000 escrow payments
	now := time.Now()
	const numPayments = 10000

	for i := 0; i < numPayments; i++ {
		payment := &Payment{
			ID:              fmt.Sprintf("payment-%d", i),
			Status:          StatusPending,
			EscrowState:     EscrowFunded,
			EscrowTimeout:   now.Add(time.Duration(i) * time.Minute), // Stagger timeouts
			MultisigEnabled: true,
		}
		if err := store.CreatePayment(payment); err != nil {
			b.Fatalf("Failed to create payment: %v", err)
		}
	}

	// Deadline that will match ~50% of payments
	deadline := now.Add(time.Duration(numPayments/2) * time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		expiring, err := store.GetEscrowsExpiringBefore(deadline)
		if err != nil {
			b.Fatalf("GetEscrowsExpiringBefore failed: %v", err)
		}
		// Expect roughly 5000 results
		if len(expiring) < 4000 || len(expiring) > 6000 {
			b.Fatalf("Unexpected number of expiring escrows: %d", len(expiring))
		}
	}
}

// BenchmarkCheckEscrowTimeouts_10k benchmarks the full timeout check with 10,000 escrows
func BenchmarkCheckEscrowTimeouts_10k(b *testing.B) {
	store := NewMemoryStore()

	// Populate store with 10,000 escrow payments
	now := time.Now()
	const numPayments = 10000

	for i := 0; i < numPayments; i++ {
		payment := &Payment{
			ID:              fmt.Sprintf("payment-%d", i),
			Status:          StatusPending,
			EscrowState:     EscrowFunded,
			EscrowTimeout:   now.Add(time.Duration(i-5000) * time.Minute), // Half expired
			MultisigEnabled: true,
		}
		if err := store.CreatePayment(payment); err != nil {
			b.Fatalf("Failed to create payment: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		timedOut, err := store.GetEscrowsExpiringBefore(now)
		if err != nil {
			b.Fatalf("GetEscrowsExpiringBefore failed: %v", err)
		}
		// Expect roughly 5000 timed out payments
		if len(timedOut) < 4000 || len(timedOut) > 6000 {
			b.Fatalf("Unexpected number of timed out escrows: %d", len(timedOut))
		}
	}
}
