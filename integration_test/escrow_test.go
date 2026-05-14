// Package integration_test provides comprehensive integration tests for escrow functionality
package integration_test

import (
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// generateTestPublicKeys creates valid compressed public keys for testing
// Returns buyer, seller, arbiter public keys that are valid secp256k1 curve points
func generateTestPublicKeys() ([]byte, []byte, []byte) {
	// Generate valid secp256k1 private keys and derive public keys
	// Use deterministic "seeds" for reproducible tests
	buyerSeed := sha256.Sum256([]byte("buyer-test-seed"))
	sellerSeed := sha256.Sum256([]byte("seller-test-seed"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-test-seed"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	return buyerPubKey, sellerPubKey, arbiterPubKey
}

// mockSignatureData creates a mock signature for testing escrow workflows
func mockSignatureData(role paywall.MultisigRole, pubKey []byte) *paywall.SignatureData {
	return &paywall.SignatureData{
		SignerID:  string(role) + "-test-signer",
		Role:      role,
		Signature: []byte("test-signature-" + string(role)),
		PublicKey: pubKey,
		SignedAt:  time.Now(),
	}
}

// TestEndToEnd2of3EscrowFlow tests the complete 2-of-3 escrow workflow
// This covers: creation, funding, release to seller (happy path)
func TestEndToEnd2of3EscrowFlow(t *testing.T) {
	// Setup: Use valid compressed public keys
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	// Configure paywall with 2-of-3 multisig
	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole: paywall.RoleBuyer,
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	// Create escrow manager
	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Step 1: Create escrow payment
	paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
	if err != nil {
		t.Fatalf("Failed to create escrow: %v", err)
	}

	if paymentID == "" {
		t.Fatal("Payment ID should not be empty")
	}

	// Verify payment was created with correct initial state
	payment, err := store.GetPayment(paymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}

	if payment.EscrowState != paywall.EscrowPending {
		t.Errorf("Expected escrow state EscrowPending, got %s", payment.EscrowState)
	}

	if !payment.MultisigEnabled {
		t.Error("Payment should have multisig enabled")
	}

	if payment.RequiredSignatures[wallet.Bitcoin] != 2 {
		t.Errorf("Expected 2 required signatures, got %d", payment.RequiredSignatures[wallet.Bitcoin])
	}

	// Step 2: Simulate blockchain confirmation (payment received)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	if err := store.UpdatePayment(payment); err != nil {
		t.Fatalf("Failed to update payment status: %v", err)
	}

	// Step 3: Fund the escrow
	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		t.Fatalf("Failed to fund escrow: %v", err)
	}

	// Verify escrow is now funded
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowFunded {
		t.Errorf("Expected escrow state EscrowFunded, got %s", payment.EscrowState)
	}

	// Step 4: Happy path - Release to seller (buyer + seller signatures)
	buyerSig := mockSignatureData(paywall.RoleBuyer, buyerPubKey)
	sellerSig := mockSignatureData(paywall.RoleSeller, sellerPubKey)

	err = escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
	if err != nil {
		t.Fatalf("Failed to release to seller: %v", err)
	}

	// Verify escrow is completed
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowCompleted {
		t.Errorf("Expected escrow state EscrowCompleted, got %s", payment.EscrowState)
	}

	// Verify signatures were stored
	if len(payment.Signatures[wallet.Bitcoin]) < 2 {
		t.Errorf("Expected at least 2 signatures, got %d", len(payment.Signatures[wallet.Bitcoin]))
	}
}

// TestEscrowDisputeResolutionFlow tests the dispute resolution path
func TestEscrowDisputeResolutionFlow(t *testing.T) {
	// Setup: Use valid compressed public keys
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole:       paywall.RoleBuyer,
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Create and fund escrow
	paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
	if err != nil {
		t.Fatalf("Failed to create escrow: %v", err)
	}

	// Simulate funding
	payment, _ := store.GetPayment(paymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		t.Fatalf("Failed to fund escrow: %v", err)
	}

	// Step 1: Buyer requests dispute
	err = escrowMgr.RequestDispute(paymentID, paywall.RoleBuyer, "Product not as described")
	if err != nil {
		t.Fatalf("Failed to request dispute: %v", err)
	}

	// Verify dispute state
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowDisputed {
		t.Errorf("Expected escrow state EscrowDisputed, got %s", payment.EscrowState)
	}

	if payment.DisputeReason != "Product not as described" {
		t.Errorf("Expected dispute reason to be set, got %s", payment.DisputeReason)
	}

	// Step 2: Arbiter resolves in favor of buyer (arbiter + buyer signatures)
	arbiterSig := mockSignatureData(paywall.RoleArbiter, arbiterPubKey)
	buyerSig := mockSignatureData(paywall.RoleBuyer, buyerPubKey)

	err = escrowMgr.ResolveDispute(paymentID, arbiterSig, buyerSig)
	if err != nil {
		t.Fatalf("Failed to resolve dispute: %v", err)
	}

	// Verify resolution resulted in refund
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowRefunded {
		t.Errorf("Expected escrow state EscrowRefunded (buyer won), got %s", payment.EscrowState)
	}

	// Verify signatures were stored
	if len(payment.Signatures[wallet.Bitcoin]) < 2 {
		t.Errorf("Expected at least 2 signatures, got %d", len(payment.Signatures[wallet.Bitcoin]))
	}
}

// TestEscrowDisputeResolutionInFavorOfSeller tests arbiter ruling for seller
func TestEscrowDisputeResolutionInFavorOfSeller(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole:       paywall.RoleSeller,
		AuthorizedArbiters: [][]byte{arbiterPubKey}, // Add arbiter to authorized list
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Create and fund escrow with dispute
	paymentID, _ := escrowMgr.CreateEscrow(1.0, time.Hour*72)

	payment, _ := store.GetPayment(paymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	escrowMgr.FundEscrow(paymentID)
	escrowMgr.RequestDispute(paymentID, paywall.RoleSeller, "Buyer refusing to accept delivery")

	// Arbiter resolves in favor of seller (arbiter + seller signatures)
	arbiterSig := mockSignatureData(paywall.RoleArbiter, arbiterPubKey)
	sellerSig := mockSignatureData(paywall.RoleSeller, sellerPubKey)

	err = escrowMgr.ResolveDispute(paymentID, arbiterSig, sellerSig)
	if err != nil {
		t.Fatalf("Failed to resolve dispute: %v", err)
	}

	// Verify resolution resulted in completion (seller won)
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowCompleted {
		t.Errorf("Expected escrow state EscrowCompleted (seller won), got %s", payment.EscrowState)
	}
}

// TestEscrowRefundFlow tests the refund path (timeout or mutual agreement)
func TestEscrowRefundFlow(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole: paywall.RoleBuyer,
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Create and fund escrow
	paymentID, _ := escrowMgr.CreateEscrow(1.0, time.Hour*72)

	payment, _ := store.GetPayment(paymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	escrowMgr.FundEscrow(paymentID)

	// Mutual agreement to refund (buyer + seller signatures)
	buyerSig := mockSignatureData(paywall.RoleBuyer, buyerPubKey)
	sellerSig := mockSignatureData(paywall.RoleSeller, sellerPubKey)

	err = escrowMgr.RefundBuyer(paymentID, buyerSig, sellerSig)
	if err != nil {
		t.Fatalf("Failed to refund buyer: %v", err)
	}

	// Verify refund completed
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowRefunded {
		t.Errorf("Expected escrow state EscrowRefunded, got %s", payment.EscrowState)
	}
}

// TestEscrowInvalidStateTransitions tests that invalid state changes are rejected
func TestEscrowInvalidStateTransitions(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole: paywall.RoleBuyer,
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	paymentID, _ := escrowMgr.CreateEscrow(1.0, time.Hour*72)

	// Try to release without funding - should fail
	buyerSig := mockSignatureData(paywall.RoleBuyer, buyerPubKey)
	sellerSig := mockSignatureData(paywall.RoleSeller, sellerPubKey)

	err = escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
	if err == nil {
		t.Error("Expected error when releasing unfunded escrow, got nil")
	}

	// Try to fund without confirmation - should fail
	err = escrowMgr.FundEscrow(paymentID)
	if err == nil {
		t.Error("Expected error when funding unconfirmed payment, got nil")
	}

	// Try to dispute pending escrow - should fail
	err = escrowMgr.RequestDispute(paymentID, paywall.RoleBuyer, "Test dispute")
	if err == nil {
		t.Error("Expected error when disputing unfunded escrow, got nil")
	}
}

// TestEscrowTimeoutHandling tests timeout scenarios
func TestEscrowTimeoutHandling(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole: paywall.RoleBuyer,
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Create escrow with very short timeout
	paymentID, _ := escrowMgr.CreateEscrow(1.0, time.Millisecond)

	payment, _ := store.GetPayment(paymentID)

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	// Verify timeout field is in the past
	if time.Now().Before(payment.EscrowTimeout) {
		t.Error("Expected escrow timeout to be in the past")
	}

	// In a real implementation, a background goroutine would handle timeouts
	// For now, we just verify the timeout field is set correctly
}

// TestConcurrentMultisigOperations tests thread safety of concurrent multisig operations
func TestConcurrentMultisigOperations(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole: paywall.RoleBuyer,
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Create multiple escrow payments concurrently
	const numConcurrent = 10
	paymentIDs := make(chan string, numConcurrent)
	errors := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func(index int) {
			paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
			if err != nil {
				errors <- err
				return
			}
			paymentIDs <- paymentID
			errors <- nil
		}(i)
	}

	// Collect results
	createdPayments := make([]string, 0, numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		if err := <-errors; err != nil {
			t.Errorf("Concurrent create failed: %v", err)
		} else {
			createdPayments = append(createdPayments, <-paymentIDs)
		}
	}

	// Verify all payments were created with unique IDs
	if len(createdPayments) != numConcurrent {
		t.Errorf("Expected %d payments, got %d", numConcurrent, len(createdPayments))
	}

	uniqueIDs := make(map[string]bool)
	for _, id := range createdPayments {
		if uniqueIDs[id] {
			t.Errorf("Duplicate payment ID: %s", id)
		}
		uniqueIDs[id] = true
	}

	// Test concurrent updates to the same payment
	testPaymentID := createdPayments[0]

	// Mark payment as funded
	payment, _ := store.GetPayment(testPaymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)
	escrowMgr.FundEscrow(testPaymentID)

	// Simulate concurrent access to the same payment
	const numConcurrentReads = 20
	readErrors := make(chan error, numConcurrentReads)

	for i := 0; i < numConcurrentReads; i++ {
		go func() {
			_, err := store.GetPayment(testPaymentID)
			readErrors <- err
		}()
	}

	// Verify all reads succeeded
	for i := 0; i < numConcurrentReads; i++ {
		if err := <-readErrors; err != nil {
			t.Errorf("Concurrent read failed: %v", err)
		}
	}

	// Test concurrent signature submissions with proper synchronization
	// Note: In production, signature collection should be serialized or use atomic operations
	const numSignatures = 5
	sigErrors := make(chan error, numSignatures)
	var sigMutex sync.Mutex

	for i := 0; i < numSignatures; i++ {
		go func(index int) {
			// Lock to ensure atomic read-modify-write
			sigMutex.Lock()
			defer sigMutex.Unlock()

			payment, err := store.GetPayment(testPaymentID)
			if err != nil {
				sigErrors <- err
				return
			}

			// Add a signature
			if payment.Signatures == nil {
				payment.Signatures = make(map[wallet.WalletType][]paywall.SignatureData)
			}

			sig := paywall.SignatureData{
				SignerID:  string(paywall.RoleBuyer) + "-" + string(rune(index)),
				Role:      paywall.RoleBuyer,
				Signature: []byte("test-sig"),
				PublicKey: buyerPubKey,
				SignedAt:  time.Now(),
			}

			payment.Signatures[wallet.Bitcoin] = append(payment.Signatures[wallet.Bitcoin], sig)
			sigErrors <- store.UpdatePayment(payment)
		}(i)
	}

	// Verify no errors in concurrent signature submissions
	for i := 0; i < numSignatures; i++ {
		if err := <-sigErrors; err != nil {
			t.Errorf("Concurrent signature submission failed: %v", err)
		}
	}
}

// TestFailureRecoveryAndRollback tests error handling and state recovery
func TestFailureRecoveryAndRollback(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := paywall.NewMemoryStore()
	config := paywall.Config{
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
		MultisigRole:       paywall.RoleBuyer,
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Test 1: Attempt to fund an escrow without confirmation
	paymentID, _ := escrowMgr.CreateEscrow(1.0, time.Hour*72)

	err = escrowMgr.FundEscrow(paymentID)
	if err == nil {
		t.Error("Expected error when funding unconfirmed payment, got nil")
	}

	// Verify payment state is still pending (not corrupted)
	payment, _ := store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowPending {
		t.Errorf("Expected state EscrowPending after failed fund, got %s", payment.EscrowState)
	}

	// Test 2: Recover from failed fund by properly confirming
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		t.Errorf("Expected successful fund after confirmation, got: %v", err)
	}

	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowFunded {
		t.Errorf("Expected state EscrowFunded, got %s", payment.EscrowState)
	}

	// Test 3: Attempt invalid state transition
	err = escrowMgr.RequestDispute(paymentID, paywall.RoleArbiter, "Invalid requester")
	if err == nil {
		t.Error("Expected error when arbiter requests dispute, got nil")
	}

	// Verify state unchanged
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowFunded {
		t.Errorf("Expected state to remain EscrowFunded, got %s", payment.EscrowState)
	}

	// Test 4: Valid dispute followed by invalid resolution attempt
	err = escrowMgr.RequestDispute(paymentID, paywall.RoleBuyer, "Valid dispute")
	if err != nil {
		t.Fatalf("Valid dispute request failed: %v", err)
	}

	// Try to resolve with wrong signatures
	buyerSig := mockSignatureData(paywall.RoleBuyer, buyerPubKey)
	sellerSig := mockSignatureData(paywall.RoleSeller, sellerPubKey)

	err = escrowMgr.ResolveDispute(paymentID, buyerSig, sellerSig) // Wrong: need arbiter
	if err == nil {
		t.Error("Expected error when resolving without arbiter signature, got nil")
	}

	// Verify state unchanged
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowDisputed {
		t.Errorf("Expected state to remain EscrowDisputed, got %s", payment.EscrowState)
	}

	// Test 5: Proper recovery with correct signatures
	arbiterSig := mockSignatureData(paywall.RoleArbiter, arbiterPubKey)
	err = escrowMgr.ResolveDispute(paymentID, arbiterSig, buyerSig)
	if err != nil {
		t.Fatalf("Valid dispute resolution failed: %v", err)
	}

	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowRefunded {
		t.Errorf("Expected state EscrowRefunded, got %s", payment.EscrowState)
	}

	// Test 6: Attempt operation on completed escrow
	err = escrowMgr.RequestDispute(paymentID, paywall.RoleSeller, "Too late")
	if err == nil {
		t.Error("Expected error when disputing completed escrow, got nil")
	}

	// Test 7: Test timeout handling
	shortTimeoutID, _ := escrowMgr.CreateEscrow(1.0, time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	shortPayment, _ := store.GetPayment(shortTimeoutID)
	if time.Now().Before(shortPayment.EscrowTimeout) {
		t.Error("Expected timeout to be in the past")
	}

	// Test 8: Multiple failure-recovery cycles
	cycleID, _ := escrowMgr.CreateEscrow(1.0, time.Hour*72)

	// Cycle through multiple failed attempts
	for i := 0; i < 3; i++ {
		err = escrowMgr.FundEscrow(cycleID)
		if err == nil {
			t.Error("Expected error on unfunded escrow")
		}

		cyclePayment, _ := store.GetPayment(cycleID)
		if cyclePayment.EscrowState != paywall.EscrowPending {
			t.Errorf("Cycle %d: State corrupted after failure", i)
		}
	}

	// Finally succeed
	cyclePayment, _ := store.GetPayment(cycleID)
	cyclePayment.Status = paywall.StatusConfirmed
	cyclePayment.Confirmations = 3
	store.UpdatePayment(cyclePayment)

	err = escrowMgr.FundEscrow(cycleID)
	if err != nil {
		t.Errorf("Expected successful fund after multiple retries: %v", err)
	}
}
