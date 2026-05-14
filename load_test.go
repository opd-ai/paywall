package paywall

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/opd-ai/paywall/wallet"
)

// Load testing for concurrent escrow operations
// These tests verify system behavior under sustained high load
// Run with: go test -run TestLoad -timeout 5m

// LoadTestMetrics tracks performance metrics during load tests
type LoadTestMetrics struct {
	OperationsCompleted int64
	OperationsFailed    int64
	TotalDuration       time.Duration
	StartTime           time.Time
	EndTime             time.Time
}

// RecordSuccess increments the successful operations counter
func (m *LoadTestMetrics) RecordSuccess() {
	atomic.AddInt64(&m.OperationsCompleted, 1)
}

// RecordFailure increments the failed operations counter
func (m *LoadTestMetrics) RecordFailure() {
	atomic.AddInt64(&m.OperationsFailed, 1)
}

// CalculateMetrics computes final statistics
func (m *LoadTestMetrics) CalculateMetrics() {
	m.EndTime = time.Now()
	m.TotalDuration = m.EndTime.Sub(m.StartTime)
}

// OperationsPerSecond returns the throughput rate
func (m *LoadTestMetrics) OperationsPerSecond() float64 {
	if m.TotalDuration.Seconds() == 0 {
		return 0
	}
	return float64(m.OperationsCompleted) / m.TotalDuration.Seconds()
}

// SuccessRate returns the percentage of successful operations
func (m *LoadTestMetrics) SuccessRate() float64 {
	total := m.OperationsCompleted + m.OperationsFailed
	if total == 0 {
		return 0
	}
	return (float64(m.OperationsCompleted) / float64(total)) * 100
}

// TestLoadEscrowCreation tests high-volume escrow creation
func TestLoadEscrowCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("load-buyer"))
	sellerSeed := sha256.Sum256([]byte("load-seller"))
	arbiterSeed := sha256.Sum256([]byte("load-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

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
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	t.Run("LoadCreate1000Escrows", func(t *testing.T) {
		numEscrows := 1000
		metrics := &LoadTestMetrics{StartTime: time.Now()}
		var wg sync.WaitGroup

		for i := 0; i < numEscrows; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				_, err := escrowMgr.CreateEscrow(float64(index+1)/1000, time.Hour*72)
				if err != nil {
					metrics.RecordFailure()
					t.Errorf("Failed to create escrow %d: %v", index, err)
				} else {
					metrics.RecordSuccess()
				}
			}(i)
		}

		wg.Wait()
		metrics.CalculateMetrics()

		// Report metrics
		t.Logf("Created %d escrows in %.2f seconds",
			metrics.OperationsCompleted, metrics.TotalDuration.Seconds())
		t.Logf("Throughput: %.2f escrows/second", metrics.OperationsPerSecond())
		t.Logf("Success rate: %.2f%%", metrics.SuccessRate())
		t.Logf("Failed operations: %d", metrics.OperationsFailed)

		// Verify minimum performance thresholds
		if metrics.SuccessRate() < 95.0 {
			t.Errorf("Success rate too low: %.2f%% (expected >= 95%%)", metrics.SuccessRate())
		}
	})
}

// TestLoadEscrowFullLifecycle tests complete escrow workflows under load
func TestLoadEscrowFullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("load-buyer"))
	sellerSeed := sha256.Sum256([]byte("load-seller"))
	arbiterSeed := sha256.Sum256([]byte("load-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

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
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	t.Run("LoadComplete500Escrows", func(t *testing.T) {
		numEscrows := 500
		metrics := &LoadTestMetrics{StartTime: time.Now()}
		var wg sync.WaitGroup

		for i := 0; i < numEscrows; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				// Create escrow
				paymentID, err := escrowMgr.CreateEscrow(float64(index+1)/1000, time.Hour*72)
				if err != nil {
					metrics.RecordFailure()
					return
				}

				// Fund escrow
				payment, _ := store.GetPayment(paymentID)
				if payment == nil {
					metrics.RecordFailure()
					return
				}
				payment.Status = StatusConfirmed
				payment.Confirmations = 6
				store.UpdatePayment(payment)

				err = escrowMgr.FundEscrow(paymentID)
				if err != nil {
					metrics.RecordFailure()
					return
				}

				// Complete escrow (release to seller)
				buyerSig := makeLoadSignature(RoleBuyer, buyerPubKey, paymentID, "release")
				sellerSig := makeLoadSignature(RoleSeller, sellerPubKey, paymentID, "release")
				err = escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
				if err != nil {
					metrics.RecordFailure()
				} else {
					metrics.RecordSuccess()
				}
			}(i)
		}

		wg.Wait()
		metrics.CalculateMetrics()

		// Report metrics
		t.Logf("Completed %d full escrow lifecycles in %.2f seconds",
			metrics.OperationsCompleted, metrics.TotalDuration.Seconds())
		t.Logf("Throughput: %.2f lifecycles/second", metrics.OperationsPerSecond())
		t.Logf("Success rate: %.2f%%", metrics.SuccessRate())
		t.Logf("Failed operations: %d", metrics.OperationsFailed)

		// Verify minimum performance thresholds
		if metrics.SuccessRate() < 90.0 {
			t.Errorf("Success rate too low: %.2f%% (expected >= 90%%)", metrics.SuccessRate())
		}
	})
}

// TestLoadDisputeResolution tests dispute handling under load
func TestLoadDisputeResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("load-buyer"))
	sellerSeed := sha256.Sum256([]byte("load-seller"))
	arbiterSeed := sha256.Sum256([]byte("load-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

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
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	t.Run("LoadResolve200Disputes", func(t *testing.T) {
		numDisputes := 200
		metrics := &LoadTestMetrics{StartTime: time.Now()}
		var wg sync.WaitGroup

		for i := 0; i < numDisputes; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				// Create and fund escrow
				paymentID, err := escrowMgr.CreateEscrow(float64(index+1)/1000, time.Hour*72)
				if err != nil {
					metrics.RecordFailure()
					return
				}

				payment, _ := store.GetPayment(paymentID)
				if payment == nil {
					metrics.RecordFailure()
					return
				}
				payment.Status = StatusConfirmed
				payment.Confirmations = 6
				store.UpdatePayment(payment)
				escrowMgr.FundEscrow(paymentID)

				// Request dispute
				err = escrowMgr.RequestDispute(paymentID, RoleBuyer, fmt.Sprintf("Load test dispute %d", index))
				if err != nil {
					metrics.RecordFailure()
					return
				}

				// Resolve dispute (alternate between buyer and seller wins)
				var arbiterSig, winnerSig *SignatureData
				if index%2 == 0 {
					// Buyer wins
					arbiterSig = makeLoadSignature(RoleArbiter, arbiterPubKey, paymentID, "resolve-arbiter")
					winnerSig = makeLoadSignature(RoleBuyer, buyerPubKey, paymentID, "resolve-buyer")
				} else {
					// Seller wins
					arbiterSig = makeLoadSignature(RoleArbiter, arbiterPubKey, paymentID, "resolve-arbiter")
					winnerSig = makeLoadSignature(RoleSeller, sellerPubKey, paymentID, "resolve-seller")
				}

				err = escrowMgr.ResolveDispute(paymentID, arbiterSig, winnerSig)
				if err != nil {
					metrics.RecordFailure()
				} else {
					metrics.RecordSuccess()
				}
			}(i)
		}

		wg.Wait()
		metrics.CalculateMetrics()

		// Report metrics
		t.Logf("Resolved %d disputes in %.2f seconds",
			metrics.OperationsCompleted, metrics.TotalDuration.Seconds())
		t.Logf("Throughput: %.2f disputes/second", metrics.OperationsPerSecond())
		t.Logf("Success rate: %.2f%%", metrics.SuccessRate())
		t.Logf("Failed operations: %d", metrics.OperationsFailed)

		// Verify minimum performance thresholds
		if metrics.SuccessRate() < 90.0 {
			t.Errorf("Success rate too low: %.2f%% (expected >= 90%%)", metrics.SuccessRate())
		}
	})
}

// TestLoadSustainedOperations tests system behavior under sustained load
func TestLoadSustainedOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("load-buyer"))
	sellerSeed := sha256.Sum256([]byte("load-seller"))
	arbiterSeed := sha256.Sum256([]byte("load-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

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
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	t.Run("LoadSustained30Seconds", func(t *testing.T) {
		duration := 30 * time.Second
		metrics := &LoadTestMetrics{StartTime: time.Now()}
		concurrency := 50
		var wg sync.WaitGroup

		// Stop signal
		stopChan := make(chan struct{})
		time.AfterFunc(duration, func() {
			close(stopChan)
		})

		// Launch workers
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				operationCount := 0
				for {
					select {
					case <-stopChan:
						return
					default:
						// Perform operation
						paymentID, err := escrowMgr.CreateEscrow(0.001, time.Hour*72)
						if err != nil {
							metrics.RecordFailure()
						} else {
							metrics.RecordSuccess()

							// Fund and complete some escrows
							if operationCount%3 == 0 {
								payment, _ := store.GetPayment(paymentID)
								if payment != nil {
									payment.Status = StatusConfirmed
									payment.Confirmations = 6
									store.UpdatePayment(payment)
									escrowMgr.FundEscrow(paymentID)

									buyerSig := makeLoadSignature(RoleBuyer, buyerPubKey, paymentID, "release")
									sellerSig := makeLoadSignature(RoleSeller, sellerPubKey, paymentID, "release")
									escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
								}
							}
						}
						operationCount++

						// Brief delay to avoid CPU saturation
						time.Sleep(10 * time.Millisecond)
					}
				}
			}(i)
		}

		wg.Wait()
		metrics.CalculateMetrics()

		// Report metrics
		t.Logf("Sustained load test: %d operations in %.2f seconds",
			metrics.OperationsCompleted, metrics.TotalDuration.Seconds())
		t.Logf("Throughput: %.2f operations/second", metrics.OperationsPerSecond())
		t.Logf("Success rate: %.2f%%", metrics.SuccessRate())
		t.Logf("Failed operations: %d", metrics.OperationsFailed)
		t.Logf("Concurrency: %d workers", concurrency)

		// Verify system stability under sustained load
		if metrics.SuccessRate() < 95.0 {
			t.Errorf("Success rate too low under sustained load: %.2f%% (expected >= 95%%)", metrics.SuccessRate())
		}

		// Verify throughput is reasonable
		if metrics.OperationsPerSecond() < 10.0 {
			t.Errorf("Throughput too low: %.2f ops/sec (expected >= 10)", metrics.OperationsPerSecond())
		}
	})
}

// makeLoadSignature creates a signature for load testing
func makeLoadSignature(role MultisigRole, pubKey []byte, paymentID, suffix string) *SignatureData {
	nonce := fmt.Sprintf("%s-%s-%d", paymentID, suffix, time.Now().UnixNano())
	return &SignatureData{
		SignerID:  string(role) + "-load-" + suffix,
		Role:      role,
		Signature: []byte("load-sig-" + suffix),
		PublicKey: pubKey,
		SignedAt:  time.Now(),
		Nonce:     []byte(nonce),
		PaymentID: paymentID,
	}
}

// BenchmarkEscrowCreation benchmarks escrow creation performance
func BenchmarkEscrowCreation(b *testing.B) {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		escrowMgr.CreateEscrow(0.001, time.Hour*72)
	}
}

// BenchmarkEscrowCompletion benchmarks full escrow lifecycle
func BenchmarkEscrowCompletion(b *testing.B) {
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("bench-buyer"))
	sellerSeed := sha256.Sum256([]byte("bench-seller"))
	arbiterSeed := sha256.Sum256([]byte("bench-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

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
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, _ := NewPaywall(config)
	defer pw.Close()

	escrowMgr, _ := NewEscrowManager(pw)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		paymentID, _ := escrowMgr.CreateEscrow(0.001, time.Hour*72)

		payment, _ := store.GetPayment(paymentID)
		payment.Status = StatusConfirmed
		payment.Confirmations = 6
		store.UpdatePayment(payment)
		escrowMgr.FundEscrow(paymentID)

		buyerSig := makeLoadSignature(RoleBuyer, buyerPubKey, paymentID, "bench")
		sellerSig := makeLoadSignature(RoleSeller, sellerPubKey, paymentID, "bench")
		escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
	}
}
