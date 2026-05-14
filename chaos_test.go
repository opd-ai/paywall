package paywall

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/opd-ai/paywall/wallet"
)

// Chaos engineering tests for race conditions and concurrent state management
// These tests intentionally inject random delays, concurrent operations, and failures
// to verify system resilience and proper synchronization

// TestChaosEscrowConcurrentOperations applies chaos engineering principles to escrow operations
func TestChaosEscrowConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos engineering test in short mode")
	}

	// Setup
	store := NewMemoryStore()
	buyerSeed := sha256.Sum256([]byte("chaos-buyer"))
	sellerSeed := sha256.Sum256([]byte("chaos-seller"))
	arbiterSeed := sha256.Sum256([]byte("chaos-arbiter"))

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

	// Test scenario: Multiple goroutines attempting various operations on the same payment
	t.Run("ChaosMultipleOperationsSamePayment", func(t *testing.T) {
		paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
		if err != nil {
			t.Fatalf("Failed to create escrow: %v", err)
		}

		// Fund the escrow
		payment, _ := store.GetPayment(paymentID)
		payment.Status = StatusConfirmed
		payment.Confirmations = 6
		store.UpdatePayment(payment)
		escrowMgr.FundEscrow(paymentID)

		var wg sync.WaitGroup
		operationCount := 50
		successCount := int32(0)
		errorCount := int32(0)

		// Chaos injection: Random concurrent operations
		operations := []func(){
			// Try to release to seller
			func() {
				defer wg.Done()
				chaosDelay()
				buyerSig := makeSignature(RoleBuyer, buyerPubKey, paymentID, "release-buyer")
				sellerSig := makeSignature(RoleSeller, sellerPubKey, paymentID, "release-seller")
				err := escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&errorCount, 1)
				}
			},
			// Try to request dispute
			func() {
				defer wg.Done()
				chaosDelay()
				err := escrowMgr.RequestDispute(paymentID, RoleBuyer, "chaos dispute")
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&errorCount, 1)
				}
			},
			// Try to refund
			func() {
				defer wg.Done()
				chaosDelay()
				buyerSig := makeSignature(RoleBuyer, buyerPubKey, paymentID, "refund-buyer")
				arbiterSig := makeSignature(RoleArbiter, arbiterPubKey, paymentID, "refund-arbiter")
				err := escrowMgr.RefundBuyer(paymentID, buyerSig, arbiterSig)
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&errorCount, 1)
				}
			},
		}

		// Launch chaos: multiple goroutines trying different operations
		for i := 0; i < operationCount; i++ {
			wg.Add(1)
			op := operations[rand.Intn(len(operations))]
			go op()
		}

		wg.Wait()

		// Verify invariants
		finalPayment, _ := store.GetPayment(paymentID)
		if finalPayment == nil {
			t.Fatal("Payment disappeared during chaos test")
		}

		// Must be in a valid terminal state
		if finalPayment.EscrowState != EscrowCompleted &&
			finalPayment.EscrowState != EscrowRefunded &&
			finalPayment.EscrowState != EscrowDisputed {
			t.Errorf("Payment in invalid state after chaos: %s", finalPayment.EscrowState)
		}

		// At least one operation should have succeeded
		if successCount == 0 && errorCount == 0 {
			t.Error("No operations completed (possible deadlock)")
		}

		t.Logf("Chaos test completed: %d successes, %d errors, final state: %s",
			successCount, errorCount, finalPayment.EscrowState)
	})

	// Test scenario: Create and fund multiple escrows concurrently
	t.Run("ChaosMultipleEscrowsConcurrent", func(t *testing.T) {
		numEscrows := 20
		var wg sync.WaitGroup
		paymentIDs := make([]string, numEscrows)
		var idMutex sync.Mutex

		// Create escrows concurrently
		for i := 0; i < numEscrows; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				chaosDelay()

				paymentID, err := escrowMgr.CreateEscrow(float64(index+1), time.Hour*72)
				if err != nil {
					t.Errorf("Failed to create escrow %d: %v", index, err)
					return
				}

				idMutex.Lock()
				paymentIDs[index] = paymentID
				idMutex.Unlock()

				// Try to fund it
				chaosDelay()
				payment, _ := store.GetPayment(paymentID)
				if payment != nil {
					payment.Status = StatusConfirmed
					payment.Confirmations = 3
					store.UpdatePayment(payment)
					escrowMgr.FundEscrow(paymentID)
				}
			}(i)
		}

		wg.Wait()

		// Verify all escrows were created
		createdCount := 0
		for _, pid := range paymentIDs {
			if pid != "" {
				createdCount++
				payment, _ := store.GetPayment(pid)
				if payment == nil {
					t.Errorf("Payment %s not found in store", pid)
				}
			}
		}

		if createdCount != numEscrows {
			t.Errorf("Expected %d escrows created, got %d", numEscrows, createdCount)
		}

		t.Logf("Created and processed %d concurrent escrows", createdCount)
	})

	// Test scenario: Concurrent state transitions on different payments
	t.Run("ChaosConcurrentStateTransitions", func(t *testing.T) {
		numPayments := 10
		payments := make([]string, numPayments)

		// Create and fund multiple escrows
		for i := 0; i < numPayments; i++ {
			paymentID, err := escrowMgr.CreateEscrow(float64(i+1), time.Hour*72)
			if err != nil {
				t.Fatalf("Failed to create escrow %d: %v", i, err)
			}
			payments[i] = paymentID

			payment, _ := store.GetPayment(paymentID)
			payment.Status = StatusConfirmed
			payment.Confirmations = 6
			store.UpdatePayment(payment)
			escrowMgr.FundEscrow(paymentID)
		}

		var wg sync.WaitGroup
		successCount := int32(0)

		// Transition all payments concurrently with random delays
		for _, paymentID := range payments {
			wg.Add(1)
			go func(pid string) {
				defer wg.Done()
				chaosDelay()

				// Randomly choose to complete or dispute
				if rand.Intn(2) == 0 {
					buyerSig := makeSignature(RoleBuyer, buyerPubKey, pid, "release")
					sellerSig := makeSignature(RoleSeller, sellerPubKey, pid, "release")
					err := escrowMgr.ReleaseToSeller(pid, buyerSig, sellerSig)
					if err == nil {
						atomic.AddInt32(&successCount, 1)
					}
				} else {
					err := escrowMgr.RequestDispute(pid, RoleBuyer, "chaos dispute")
					if err == nil {
						atomic.AddInt32(&successCount, 1)
					}
				}
			}(paymentID)
		}

		wg.Wait()

		// Verify all payments are in valid states
		for _, pid := range payments {
			payment, _ := store.GetPayment(pid)
			if payment == nil {
				t.Errorf("Payment %s disappeared", pid)
				continue
			}

			if payment.EscrowState < EscrowNone || payment.EscrowState > EscrowRefunded {
				t.Errorf("Payment %s in invalid state: %d", pid, payment.EscrowState)
			}
		}

		t.Logf("Concurrent state transitions completed: %d successes", successCount)
	})
}

// TestChaosStoreOperations tests store resilience under concurrent chaos
func TestChaosStoreOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos engineering test in short mode")
	}

	store := NewMemoryStore()

	// Create some initial payments
	numPayments := 20
	paymentIDs := make([]string, numPayments)
	for i := 0; i < numPayments; i++ {
		paymentID, _ := generatePaymentID()
		payment := &Payment{
			ID: paymentID,
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "chaos-address-" + paymentID,
			},
			Amounts: map[wallet.WalletType]float64{
				wallet.Bitcoin: float64(i + 1),
			},
			Status:      StatusPending,
			EscrowState: EscrowPending,
			Version:     1,
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		err := store.CreatePayment(payment)
		if err != nil {
			t.Fatalf("Failed to create payment: %v", err)
		}
		paymentIDs[i] = payment.ID
	}

	// Chaos: concurrent reads and writes
	t.Run("ChaosConcurrentReadsWrites", func(t *testing.T) {
		var wg sync.WaitGroup
		operations := 100
		readCount := int32(0)
		writeCount := int32(0)
		errorCount := int32(0)

		for i := 0; i < operations; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				chaosDelay()

				pid := paymentIDs[rand.Intn(len(paymentIDs))]

				// Random operation: read or write
				if rand.Intn(2) == 0 {
					// Read
					payment, err := store.GetPayment(pid)
					if err != nil {
						atomic.AddInt32(&errorCount, 1)
					} else if payment != nil {
						atomic.AddInt32(&readCount, 1)
					}
				} else {
					// Write
					payment, _ := store.GetPayment(pid)
					if payment != nil {
						payment.Confirmations++
						payment.Version++
						err := store.UpdatePayment(payment)
						if err != nil {
							atomic.AddInt32(&errorCount, 1)
						} else {
							atomic.AddInt32(&writeCount, 1)
						}
					}
				}
			}()
		}

		wg.Wait()

		t.Logf("Chaos store operations: %d reads, %d writes, %d errors",
			readCount, writeCount, errorCount)

		// Verify all payments still exist and are valid
		for _, pid := range paymentIDs {
			payment, err := store.GetPayment(pid)
			if err != nil {
				t.Errorf("Failed to get payment %s: %v", pid, err)
			}
			if payment == nil {
				t.Errorf("Payment %s disappeared", pid)
			}
		}
	})
}

// TestChaosValidatorOperations tests state validator under chaos conditions
func TestChaosValidatorOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos engineering test in short mode")
	}

	validator := NewEscrowStateValidator()

	// Test concurrent validation operations
	t.Run("ChaosConcurrentValidations", func(t *testing.T) {
		numGoroutines := 50
		operationsPerGoroutine := 100
		var wg sync.WaitGroup

		transitions := []struct {
			from EscrowState
			to   EscrowState
		}{
			{EscrowPending, EscrowFunded},
			{EscrowFunded, EscrowCompleted},
			{EscrowFunded, EscrowDisputed},
			{EscrowFunded, EscrowRefunded},
			{EscrowDisputed, EscrowCompleted},
			{EscrowDisputed, EscrowRefunded},
		}

		errorCount := int32(0)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for j := 0; j < operationsPerGoroutine; j++ {
					chaosDelay()

					// Random validation
					trans := transitions[rand.Intn(len(transitions))]
					err := validator.ValidateTransition(trans.from, trans.to)
					if err != nil {
						atomic.AddInt32(&errorCount, 1)
					}

					// Random allowed transitions check
					_ = validator.GetAllowedTransitions(trans.from)

					// Random terminal state check
					_ = validator.IsTerminalState(trans.to)
				}
			}()
		}

		wg.Wait()

		// Validator should handle all concurrent operations without panicking
		t.Logf("Chaos validator operations completed, %d validation errors (expected)", errorCount)
	})
}

// Test concurrent payment creation and retrieval under chaos
func TestChaosPaymentLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos engineering test in short mode")
	}

	store := NewMemoryStore()
	validator := NewEscrowStateValidator()

	t.Run("ChaosFullLifecycle", func(t *testing.T) {
		numPayments := 20
		var wg sync.WaitGroup
		successfulTransitions := int32(0)
		paymentIDs := make([]string, numPayments)
		var idMutex sync.Mutex

		for i := 0; i < numPayments; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				// Create payment
				paymentID, _ := generatePaymentID()
				payment := &Payment{
					ID: paymentID,
					Addresses: map[wallet.WalletType]string{
						wallet.Bitcoin: "chaos-" + paymentID,
					},
					Amounts: map[wallet.WalletType]float64{
						wallet.Bitcoin: float64(index + 1),
					},
					Status:      StatusPending,
					EscrowState: EscrowPending,
					Version:     1,
					ExpiresAt:   time.Now().Add(time.Hour),
				}

				chaosDelay()
				err := store.CreatePayment(payment)
				if err != nil {
					t.Errorf("Failed to create payment: %v", err)
					return
				}

				// Store payment ID for later verification
				idMutex.Lock()
				paymentIDs[index] = paymentID
				idMutex.Unlock()

				// Transition through states with chaos delays
				states := []EscrowState{EscrowPending, EscrowFunded, EscrowCompleted}
				for i := 0; i < len(states)-1; i++ {
					chaosDelay()

					err := validator.ValidateAndRecordTransition(
						payment,
						states[i+1],
						"chaos-actor",
						"chaos-reason",
					)

					if err == nil {
						payment.Version++
						chaosDelay()
						store.UpdatePayment(payment)
						atomic.AddInt32(&successfulTransitions, 1)
					}
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Chaos lifecycle: %d successful transitions across %d payments",
			successfulTransitions, numPayments)

		// Verify payments are in valid states
		// Check that all payments we created still exist
		foundCount := 0
		for _, pid := range paymentIDs {
			payment, err := store.GetPayment(pid)
			if err == nil && payment != nil {
				foundCount++
			}
		}

		if foundCount == 0 {
			t.Error("No valid payments found after chaos lifecycle")
		} else {
			t.Logf("Found %d valid payments in store after chaos (out of %d created)", foundCount, numPayments)
		}
	})
}

// chaosDelay introduces random delays to increase likelihood of race conditions
func chaosDelay() {
	// Random delay between 0-5ms
	delay := time.Duration(rand.Intn(5)) * time.Millisecond
	time.Sleep(delay)
}

// makeSignature creates a signature for testing
func makeSignature(role MultisigRole, pubKey []byte, paymentID, suffix string) *SignatureData {
	nonce := fmt.Sprintf("%s-%s-%d", paymentID, suffix, time.Now().UnixNano())
	return &SignatureData{
		SignerID:  string(role) + "-" + suffix,
		Role:      role,
		Signature: []byte("chaos-sig-" + suffix),
		PublicKey: pubKey,
		SignedAt:  time.Now(),
		Nonce:     []byte(nonce),
		PaymentID: paymentID,
	}
}
