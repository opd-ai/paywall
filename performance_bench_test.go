package paywall

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/opd-ai/paywall/wallet"
)

// Benchmark multisig address generation vs single-sig
// Run with: go test -bench=BenchmarkAddressGeneration -benchmem

// BenchmarkSingleSigAddressGeneration benchmarks traditional single-signature address generation
func BenchmarkSingleSigAddressGeneration(b *testing.B) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,
		// No multisig configuration - single-sig mode
		MultisigEnabled: false,
	}

	pw, err := NewPaywall(config)
	if err != nil {
		b.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := pw.CreatePayment()
		if err != nil {
			b.Fatalf("Failed to create payment: %v", err)
		}
	}
}

// BenchmarkMultisigAddressGeneration benchmarks 2-of-3 multisig address generation
func BenchmarkMultisigAddressGeneration(b *testing.B) {
	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("bench-buyer"))
	sellerSeed := sha256.Sum256([]byte("bench-seller"))
	arbiterSeed := sha256.Sum256([]byte("bench-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	publicKeys := [][]byte{
		buyerPrivKey.PubKey().SerializeCompressed(),
		sellerPrivKey.PubKey().SerializeCompressed(),
		arbiterPrivKey.PubKey().SerializeCompressed(),
	}

	store := NewMemoryStore()
	config := Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,

		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: publicKeys,
		},
		MultisigRole: RoleBuyer,
	}

	pw, err := NewPaywall(config)
	if err != nil {
		b.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := pw.CreatePayment()
		if err != nil {
			b.Fatalf("Failed to create payment: %v", err)
		}
	}
}

// BenchmarkSignatureCreation benchmarks partial signature creation for multisig
func BenchmarkSignatureCreation(b *testing.B) {
	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("bench-buyer"))
	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()

	// Dummy transaction data
	txData := []byte("test transaction data for benchmarking signature creation")
	paymentID := "benchmark-payment-id"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sig := &SignatureData{
			SignerID:  "buyer-bench",
			Role:      RoleBuyer,
			Signature: txData, // In real usage, this would be an ECDSA signature
			PublicKey: buyerPubKey,
			SignedAt:  time.Now(),
			Nonce:     []byte("nonce-" + paymentID),
			PaymentID: paymentID,
		}
		_ = sig
	}
}

// BenchmarkSignatureVerification benchmarks signature validation
func BenchmarkSignatureVerification(b *testing.B) {
	// Setup
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("bench-buyer"))
	sellerSeed := sha256.Sum256([]byte("bench-seller"))
	arbiterSeed := sha256.Sum256([]byte("bench-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	publicKeys := [][]byte{
		buyerPrivKey.PubKey().SerializeCompressed(),
		sellerPrivKey.PubKey().SerializeCompressed(),
		arbiterPrivKey.PubKey().SerializeCompressed(),
	}

	config := Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,

		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: publicKeys,
		},
		MultisigRole:       RoleBuyer,
		AuthorizedArbiters: [][]byte{arbiterPrivKey.PubKey().SerializeCompressed()},
	}

	pw, _ := NewPaywall(config)
	defer pw.Close()

	escrowMgr, _ := NewEscrowManager(pw)

	// Create a payment
	paymentID, _ := escrowMgr.CreateEscrow(0.001, time.Hour*72)

	// Create test signature
	buyerSig := &SignatureData{
		SignerID:  "buyer",
		Role:      RoleBuyer,
		Signature: []byte("test-signature"),
		PublicKey: buyerPrivKey.PubKey().SerializeCompressed(),
		SignedAt:  time.Now(),
		Nonce:     []byte(paymentID + "-nonce"),
		PaymentID: paymentID,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Benchmark signature validation logic
		payment, _ := store.GetPayment(paymentID)
		if payment != nil {
			_ = escrowMgr.validateSignatureData(buyerSig, payment)
		}
	}
}

// BenchmarkPaymentVerification benchmarks payment confirmation checking
func BenchmarkPaymentVerificationSingleSig(b *testing.B) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:      0.001,
		TestNet:         true,
		Store:           store,
		PaymentTimeout:  time.Hour * 24,
		MultisigEnabled: false,
	}

	pw, _ := NewPaywall(config)
	defer pw.Close()

	// Create test payment
	payment, _ := pw.CreatePayment()

	// Mock payment as having confirmations
	payment.Status = StatusConfirmed
	payment.Confirmations = 6
	store.UpdatePayment(payment)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Verification involves fetching payment and checking status
		p, _ := store.GetPayment(payment.ID)
		_ = p.Status == StatusConfirmed && p.Confirmations >= 6
	}
}

// BenchmarkPaymentVerificationMultisig benchmarks multisig payment verification
func BenchmarkPaymentVerificationMultisig(b *testing.B) {
	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("bench-buyer"))
	sellerSeed := sha256.Sum256([]byte("bench-seller"))
	arbiterSeed := sha256.Sum256([]byte("bench-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	publicKeys := [][]byte{
		buyerPrivKey.PubKey().SerializeCompressed(),
		sellerPrivKey.PubKey().SerializeCompressed(),
		arbiterPrivKey.PubKey().SerializeCompressed(),
	}

	store := NewMemoryStore()
	config := Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,

		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: publicKeys,
		},
		MultisigRole: RoleBuyer,
	}

	pw, _ := NewPaywall(config)
	defer pw.Close()

	// Create test payment
	payment, _ := pw.CreatePayment()

	// Mock payment as confirmed
	payment.Status = StatusConfirmed
	payment.Confirmations = 6
	store.UpdatePayment(payment)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Verification for multisig includes additional checks
		p, _ := store.GetPayment(payment.ID)
		isValid := p.Status == StatusConfirmed &&
			p.Confirmations >= 6 &&
			p.MultisigEnabled
		_ = isValid
	}
}

// BenchmarkEscrowStateTransition benchmarks state machine transitions
func BenchmarkEscrowStateTransition(b *testing.B) {
	validator := NewEscrowStateValidator()
	payment := &Payment{
		ID:          "bench-payment",
		EscrowState: EscrowPending,
		Version:     1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset for each iteration
		payment.EscrowState = EscrowPending
		payment.Version = 1
		payment.StateTransitionHistory = nil

		// Perform transition
		validator.ValidateAndRecordTransition(payment, EscrowFunded, "buyer", "funding")
	}
}

// BenchmarkStoreOperations benchmarks payment store operations
func BenchmarkStoreCreatePayment(b *testing.B) {
	store := NewMemoryStore()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		paymentID, _ := generatePaymentID()
		payment := &Payment{
			ID: paymentID,
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "test-address",
			},
			Amounts: map[wallet.WalletType]float64{
				wallet.Bitcoin: 0.001,
			},
			Status:      StatusPending,
			EscrowState: EscrowPending,
			Version:     1,
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		store.CreatePayment(payment)
	}
}

// BenchmarkStoreGetPayment benchmarks payment retrieval
func BenchmarkStoreGetPayment(b *testing.B) {
	store := NewMemoryStore()

	// Create test payment
	paymentID, _ := generatePaymentID()
	payment := &Payment{
		ID: paymentID,
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "test-address",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
		},
		Status:      StatusPending,
		EscrowState: EscrowPending,
		Version:     1,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	store.CreatePayment(payment)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		store.GetPayment(paymentID)
	}
}

// BenchmarkStoreUpdatePayment benchmarks payment updates
func BenchmarkStoreUpdatePayment(b *testing.B) {
	store := NewMemoryStore()

	// Create test payment
	paymentID, _ := generatePaymentID()
	payment := &Payment{
		ID: paymentID,
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "test-address",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
		},
		Status:      StatusPending,
		EscrowState: EscrowPending,
		Version:     1,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	store.CreatePayment(payment)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		payment.Confirmations = i
		payment.Version++
		store.UpdatePayment(payment)
	}
}

// Benchmark comparison: multisig vs single-sig full flow
func BenchmarkFullPaymentFlowSingleSig(b *testing.B) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:      0.001,
		TestNet:         true,
		Store:           store,
		PaymentTimeout:  time.Hour * 24,
		MultisigEnabled: false,
	}

	pw, _ := NewPaywall(config)
	defer pw.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create payment
		payment, _ := pw.CreatePayment()

		// Simulate confirmation
		payment.Status = StatusConfirmed
		payment.Confirmations = 6
		store.UpdatePayment(payment)
	}
}

func BenchmarkFullPaymentFlowMultisig(b *testing.B) {
	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("bench-buyer"))
	sellerSeed := sha256.Sum256([]byte("bench-seller"))
	arbiterSeed := sha256.Sum256([]byte("bench-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	publicKeys := [][]byte{
		buyerPrivKey.PubKey().SerializeCompressed(),
		sellerPrivKey.PubKey().SerializeCompressed(),
		arbiterPrivKey.PubKey().SerializeCompressed(),
	}

	store := NewMemoryStore()
	config := Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,

		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: publicKeys,
		},
		MultisigRole: RoleBuyer,
	}

	pw, _ := NewPaywall(config)
	defer pw.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create payment
		payment, _ := pw.CreatePayment()

		// Simulate confirmation
		payment.Status = StatusConfirmed
		payment.Confirmations = 6
		store.UpdatePayment(payment)
	}
}
