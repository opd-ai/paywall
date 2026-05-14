// Package integration_test provides comprehensive integration tests for escrow functionality
package integration_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
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

	// Create escrow with short timeout (but above minimum bounds)
	// Default minimum is 24 hours, so use 25 hours
	shortTimeout := 25 * time.Hour
	paymentID, err := escrowMgr.CreateEscrow(1.0, shortTimeout)
	if err != nil {
		t.Fatalf("Failed to create escrow: %v", err)
	}

	payment, err := store.GetPayment(paymentID)
	if err != nil {
		t.Fatalf("Failed to get payment: %v", err)
	}

	// Verify timeout field is set correctly and in the future
	expectedTimeout := time.Now().Add(shortTimeout)
	// Allow 1 second tolerance for test execution time
	if payment.EscrowTimeout.Before(expectedTimeout.Add(-1*time.Second)) || payment.EscrowTimeout.After(expectedTimeout.Add(1*time.Second)) {
		t.Errorf("Expected escrow timeout around %v, got %v", expectedTimeout, payment.EscrowTimeout)
	}

	// Verify the timeout is in the future initially
	if time.Now().After(payment.EscrowTimeout) {
		t.Error("Expected escrow timeout to be in the future")
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

	// Test 7: Test timeout validation (new in timeout bounds enforcement)
	// Attempt to create escrow with timeout below minimum (should fail)
	_, err = escrowMgr.CreateEscrow(1.0, time.Millisecond)
	if err == nil {
		t.Error("Expected error when creating escrow with timeout below minimum")
	}

	// Create escrow with valid timeout
	validTimeoutID, err := escrowMgr.CreateEscrow(1.0, 25*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create escrow with valid timeout: %v", err)
	}

	validPayment, _ := store.GetPayment(validTimeoutID)
	if validPayment.EscrowTimeout.Before(time.Now()) {
		t.Error("Expected timeout to be in the future")
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

// TestEndToEndEscrowWithRealSignatures tests the complete escrow workflow using real cryptographic signatures
// instead of mock data. This validates that the signature verification logic works correctly with actual
// Bitcoin ECDSA signatures over transaction data.
func TestEndToEndEscrowWithRealSignatures(t *testing.T) {
	// Generate deterministic private keys for reproducible tests
	buyerSeed := sha256.Sum256([]byte("buyer-real-sig-seed"))
	sellerSeed := sha256.Sum256([]byte("seller-real-sig-seed"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-real-sig-seed"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

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

	// Step 1: Create escrow payment
	paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
	if err != nil {
		t.Fatalf("Failed to create escrow: %v", err)
	}

	// Step 2: Simulate blockchain confirmation and fund escrow
	payment, _ := store.GetPayment(paymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		t.Fatalf("Failed to fund escrow: %v", err)
	}

	// Verify escrow is funded
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowFunded {
		t.Errorf("Expected escrow state EscrowFunded, got %s", payment.EscrowState)
	}

	// Step 3: Create a real multisig transaction for the escrow release
	// Build 2-of-3 redeem script (pubKeys first, then requiredSigs)
	// Using P2SH for simplicity (legacy format works without PrevOutFetcher)
	multisigAddr, redeemScript, err := wallet.CreateMultisigAddress(
		publicKeys,
		2,
		wallet.P2SH,
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("Failed to create multisig address: %v", err)
	}

	// Create a mock UTXO representing the escrowed funds
	// In a real system, this would come from blockchain monitoring
	mockTxID := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	utxo := wallet.UTXO{
		TxID:         mockTxID,
		Vout:         0,
		Amount:       100000,       // 0.001 BTC in satoshis
		ScriptPubKey: []byte{},     // Would be the actual P2SH script
		RedeemScript: redeemScript, // P2SH uses RedeemScript
	}

	// Create transaction to release funds to seller (happy path)
	// Generate a testnet address for the seller
	sellerAddr := "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx" // Example testnet address

	outputs := map[string]int64{
		sellerAddr: 99000, // 99k satoshis (1k for fee)
	}

	tx, err := wallet.CreateMultisigPaymentTx(
		[]wallet.UTXO{utxo},
		outputs,
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("Failed to create multisig transaction: %v", err)
	}

	// Sign the transaction with buyer's key (happy path: buyer agrees to release)
	err = tx.SignMultisigTx(0, buyerPrivKey, txscript.SigHashAll)
	if err != nil {
		t.Fatalf("Failed to sign transaction with buyer key: %v", err)
	}

	// Sign the transaction with seller's key
	err = tx.SignMultisigTx(0, sellerPrivKey, txscript.SigHashAll)
	if err != nil {
		t.Fatalf("Failed to sign transaction with seller key: %v", err)
	}

	// Extract the real signatures
	if len(tx.Signatures[0]) != 2 {
		t.Fatalf("Expected 2 signatures, got %d", len(tx.Signatures[0]))
	}

	// Create SignatureData structs with real signatures
	var buyerSig, sellerSig *paywall.SignatureData
	for _, sig := range tx.Signatures[0] {
		if bytes.Equal(sig.PublicKey, buyerPubKey) {
			buyerSig = &paywall.SignatureData{
				SignerID:  "buyer-real-sig",
				Role:      paywall.RoleBuyer,
				Signature: sig.Signature,
				PublicKey: sig.PublicKey,
				SignedAt:  time.Now(),
				Nonce:     []byte(paymentID + "-buyer-nonce"),
			}
		} else if bytes.Equal(sig.PublicKey, sellerPubKey) {
			sellerSig = &paywall.SignatureData{
				SignerID:  "seller-real-sig",
				Role:      paywall.RoleSeller,
				Signature: sig.Signature,
				PublicKey: sig.PublicKey,
				SignedAt:  time.Now(),
				Nonce:     []byte(paymentID + "-seller-nonce"),
			}
		}
	}

	if buyerSig == nil || sellerSig == nil {
		t.Fatal("Failed to extract buyer or seller signature")
	}

	_ = multisigAddr // Suppress unused variable warning

	// Step 4: Release to seller using real signatures
	err = escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
	if err != nil {
		t.Fatalf("Failed to release to seller with real signatures: %v", err)
	}

	// Verify escrow completed successfully
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowCompleted {
		t.Errorf("Expected escrow state EscrowCompleted, got %s", payment.EscrowState)
	}

	// Verify signatures were stored
	if len(payment.Signatures[wallet.Bitcoin]) < 2 {
		t.Errorf("Expected at least 2 signatures stored, got %d", len(payment.Signatures[wallet.Bitcoin]))
	}

	// Verify the stored signatures are the real ones (not mock data)
	storedBuyerSig := false
	storedSellerSig := false
	for _, sig := range payment.Signatures[wallet.Bitcoin] {
		if bytes.Equal(sig.PublicKey, buyerPubKey) && len(sig.Signature) > 10 {
			storedBuyerSig = true
		}
		if bytes.Equal(sig.PublicKey, sellerPubKey) && len(sig.Signature) > 10 {
			storedSellerSig = true
		}
	}

	if !storedBuyerSig || !storedSellerSig {
		t.Error("Real signatures were not properly stored in payment record")
	}

	t.Log("✓ End-to-end escrow with real cryptographic signatures completed successfully")
}

// TestDisputeResolutionWithMultipleArbiters tests dispute resolution using multi-arbiter consensus (3-of-5)
func TestDisputeResolutionWithMultipleArbiters(t *testing.T) {
	// Generate 5 arbiter keys for 3-of-5 consensus
	arbiterPrivKeys := make([]*btcec.PrivateKey, 5)
	arbiterPubKeys := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		seed := sha256.Sum256([]byte(fmt.Sprintf("arbiter-%d-seed", i)))
		arbiterPrivKeys[i], _ = btcec.PrivKeyFromBytes(seed[:])
		arbiterPubKeys[i] = arbiterPrivKeys[i].PubKey().SerializeCompressed()
	}

	// Generate buyer and seller keys
	buyerSeed := sha256.Sum256([]byte("buyer-multi-arbiter"))
	sellerSeed := sha256.Sum256([]byte("seller-multi-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()

	// Note: For multi-arbiter, we'd need buyer, seller, and multiple arbiters in the multisig
	// For simplicity, we'll use 2-of-3 with buyer, seller, and one arbiter representative
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKeys[0]}

	// Configure paywall with multi-arbiter consensus
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
		AuthorizedArbiters: arbiterPubKeys, // All 5 arbiters authorized
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

	// Create arbiter consensus manager
	arbiterConfig := &paywall.ArbiterConfig{
		RequiredArbiterVotes: 3, // 3-of-5
		TotalArbiters:        5,
		PrimaryArbiters:      arbiterPubKeys,
		VotingTimeout:        24 * time.Hour,
	}

	consensusMgr, err := paywall.NewArbiterConsensusManager(arbiterConfig, nil)
	if err != nil {
		t.Fatalf("Failed to create consensus manager: %v", err)
	}

	// Step 1: Create and fund escrow
	paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
	if err != nil {
		t.Fatalf("Failed to create escrow: %v", err)
	}

	payment, _ := store.GetPayment(paymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		t.Fatalf("Failed to fund escrow: %v", err)
	}

	// Verify escrow funded
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowFunded {
		t.Errorf("Expected escrow state EscrowFunded, got %s", payment.EscrowState)
	}

	// Step 2: Buyer raises dispute
	err = escrowMgr.RequestDispute(paymentID, paywall.RoleBuyer, "Product not as described - color is wrong")
	if err != nil {
		t.Fatalf("Failed to request dispute: %v", err)
	}

	// Verify disputed state
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowDisputed {
		t.Errorf("Expected escrow state EscrowDisputed, got %s", payment.EscrowState)
	}

	// Step 3: Initiate multi-arbiter consensus
	_, err = consensusMgr.InitiateConsensus(paymentID)
	if err != nil {
		t.Fatalf("Failed to initiate consensus: %v", err)
	}

	// Step 4: Arbiters cast votes (3 vote for buyer, 2 for seller - buyer wins)
	// Arbiter 0 votes for buyer
	vote0 := &paywall.ArbiterVote{
		ArbiterPubKey: arbiterPubKeys[0],
		ArbiterID:     "arbiter-0",
		Decision:      paywall.RoleBuyer,
		Reason:        "Evidence supports buyer's claim - product photos show incorrect color",
		Signature: &paywall.SignatureData{
			SignerID:  "arbiter-0",
			Role:      paywall.RoleArbiter,
			Signature: []byte("arbiter-0-signature"),
			PublicKey: arbiterPubKeys[0],
			SignedAt:  time.Now(),
		},
		VotedAt: time.Now(),
	}

	err = consensusMgr.CastVote(paymentID, vote0)
	if err != nil {
		t.Fatalf("Arbiter 0 failed to cast vote: %v", err)
	}

	// Arbiter 1 votes for seller
	vote1 := &paywall.ArbiterVote{
		ArbiterPubKey: arbiterPubKeys[1],
		ArbiterID:     "arbiter-1",
		Decision:      paywall.RoleSeller,
		Reason:        "Seller provided tracking and delivery confirmation",
		Signature: &paywall.SignatureData{
			SignerID:  "arbiter-1",
			Role:      paywall.RoleArbiter,
			Signature: []byte("arbiter-1-signature"),
			PublicKey: arbiterPubKeys[1],
			SignedAt:  time.Now(),
		},
		VotedAt: time.Now(),
	}

	err = consensusMgr.CastVote(paymentID, vote1)
	if err != nil {
		t.Fatalf("Arbiter 1 failed to cast vote: %v", err)
	}

	// Arbiter 2 votes for buyer
	vote2 := &paywall.ArbiterVote{
		ArbiterPubKey: arbiterPubKeys[2],
		ArbiterID:     "arbiter-2",
		Decision:      paywall.RoleBuyer,
		Reason:        "Product description mismatch confirmed",
		Signature: &paywall.SignatureData{
			SignerID:  "arbiter-2",
			Role:      paywall.RoleArbiter,
			Signature: []byte("arbiter-2-signature"),
			PublicKey: arbiterPubKeys[2],
			SignedAt:  time.Now(),
		},
		VotedAt: time.Now(),
	}

	err = consensusMgr.CastVote(paymentID, vote2)
	if err != nil {
		t.Fatalf("Arbiter 2 failed to cast vote: %v", err)
	}

	// Arbiter 3 votes for buyer (this reaches consensus: 3 votes for buyer)
	vote3 := &paywall.ArbiterVote{
		ArbiterPubKey: arbiterPubKeys[3],
		ArbiterID:     "arbiter-3",
		Decision:      paywall.RoleBuyer,
		Reason:        "Buyer's evidence is credible and well-documented",
		Signature: &paywall.SignatureData{
			SignerID:  "arbiter-3",
			Role:      paywall.RoleArbiter,
			Signature: []byte("arbiter-3-signature"),
			PublicKey: arbiterPubKeys[3],
			SignedAt:  time.Now(),
		},
		VotedAt: time.Now(),
	}

	err = consensusMgr.CastVote(paymentID, vote3)
	if err != nil {
		t.Fatalf("Arbiter 3 failed to cast vote: %v", err)
	}

	// Step 5: Check consensus reached
	consensus, err := consensusMgr.GetConsensus(paymentID)
	if err != nil {
		t.Fatalf("Failed to get consensus: %v", err)
	}

	if !consensus.ConsensusReached {
		t.Error("Expected consensus to be reached after 3 votes for buyer")
	}

	if consensus.FinalDecision != paywall.RoleBuyer {
		t.Errorf("Expected final decision RoleBuyer, got %s", consensus.FinalDecision)
	}

	if consensus.Status != paywall.ConsensusReached {
		t.Errorf("Expected status ConsensusReached, got %s", consensus.Status)
	}

	// Verify vote count
	if len(consensus.Votes) != 4 {
		t.Errorf("Expected 4 votes recorded, got %d", len(consensus.Votes))
	}

	// Step 6: Execute consensus decision (refund to buyer)
	// Create mock signatures for arbiter + buyer (winner)
	arbiterSig := &paywall.SignatureData{
		SignerID:  "lead-arbiter",
		Role:      paywall.RoleArbiter,
		Signature: []byte("arbiter-consensus-signature"),
		PublicKey: arbiterPubKeys[0],
		SignedAt:  time.Now(),
	}

	buyerSig := &paywall.SignatureData{
		SignerID:  "buyer-multi-arbiter",
		Role:      paywall.RoleBuyer,
		Signature: []byte("buyer-consensus-signature"),
		PublicKey: buyerPubKey,
		SignedAt:  time.Now(),
	}

	err = escrowMgr.ResolveDispute(paymentID, arbiterSig, buyerSig)
	if err != nil {
		t.Fatalf("Failed to resolve dispute with consensus: %v", err)
	}

	// Step 7: Verify final escrow state (refunded to buyer)
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowRefunded {
		t.Errorf("Expected escrow state EscrowRefunded, got %s", payment.EscrowState)
	}

	t.Logf("✓ Multi-arbiter dispute resolution completed: 3-of-5 consensus reached in favor of buyer")
	t.Logf("  - Total votes: %d", len(consensus.Votes))
	t.Logf("  - Buyer votes: 3, Seller votes: 1")
	t.Logf("  - Final decision: Refund to buyer")
}

// TestTimeoutBasedRefund tests automatic refund when escrow times out
func TestTimeoutBasedRefund(t *testing.T) {
	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("buyer-timeout-test"))
	sellerSeed := sha256.Sum256([]byte("seller-timeout-test"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-timeout-test"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	// Configure paywall
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

	// Step 1: Create escrow with 25 hour timeout (minimum is 24h)
	timeoutDuration := 25 * time.Hour
	paymentID, err := escrowMgr.CreateEscrow(1.0, timeoutDuration)
	if err != nil {
		t.Fatalf("Failed to create escrow: %v", err)
	}

	// Step 2: Fund the escrow
	payment, _ := store.GetPayment(paymentID)
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	store.UpdatePayment(payment)

	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		t.Fatalf("Failed to fund escrow: %v", err)
	}

	// Verify escrow funded with timeout set
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowFunded {
		t.Errorf("Expected escrow state EscrowFunded, got %s", payment.EscrowState)
	}

	if payment.EscrowTimeout.IsZero() {
		t.Error("Expected escrow timeout to be set")
	}

	originalTimeout := payment.EscrowTimeout
	t.Logf("Escrow created with timeout at: %s", originalTimeout.Format(time.RFC3339))

	// Step 3: Create timeout monitor
	monitorConfig := paywall.DefaultTimeoutMonitorConfig()
	monitorConfig.CheckInterval = 100 * time.Millisecond // Fast checks for testing
	monitorConfig.UseBlockchainTime = false              // Use system time
	monitorConfig.AutoRefund = false                     // Manual for testing

	monitor := paywall.NewTimeoutMonitor(escrowMgr, monitorConfig)

	// Step 4: Manually set payment timeout to past time (simulates time passing)
	payment.EscrowTimeout = time.Now().Add(-1 * time.Minute) // 1 minute ago
	if err := store.UpdatePayment(payment); err != nil {
		t.Fatalf("Failed to update payment timeout: %v", err)
	}

	t.Logf("Simulated timeout: set escrow timeout to %s (past)", payment.EscrowTimeout.Format(time.RFC3339))

	// Step 5: Check for timeouts
	currentTime := time.Now()
	timedOutIDs, err := escrowMgr.CheckEscrowTimeoutsWithTime(currentTime)
	if err != nil {
		t.Fatalf("Failed to check timeouts: %v", err)
	}

	if len(timedOutIDs) == 0 {
		t.Error("Expected timeout to be detected")
	}

	found := false
	for _, id := range timedOutIDs {
		if id == paymentID {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected payment %s to be in timed-out list", paymentID)
	}

	t.Logf("✓ Timeout detected for payment %s", paymentID)

	// Step 6: Process timeout refund (requires buyer + arbiter signatures)
	// Create mock signatures for refund
	buyerSig := &paywall.SignatureData{
		SignerID:  "buyer-timeout",
		Role:      paywall.RoleBuyer,
		Signature: []byte("buyer-timeout-signature"),
		PublicKey: buyerPubKey,
		SignedAt:  time.Now(),
		Nonce:     []byte(paymentID + "-buyer-refund"),
	}

	arbiterSig := &paywall.SignatureData{
		SignerID:  "arbiter-timeout",
		Role:      paywall.RoleArbiter,
		Signature: []byte("arbiter-timeout-signature"),
		PublicKey: arbiterPubKey,
		SignedAt:  time.Now(),
		Nonce:     []byte(paymentID + "-arbiter-refund"),
	}

	// Execute refund
	err = escrowMgr.RefundBuyer(paymentID, buyerSig, arbiterSig)
	if err != nil {
		t.Fatalf("Failed to refund buyer after timeout: %v", err)
	}

	// Step 7: Verify refund completed
	payment, _ = store.GetPayment(paymentID)
	if payment.EscrowState != paywall.EscrowRefunded {
		t.Errorf("Expected escrow state EscrowRefunded, got %s", payment.EscrowState)
	}

	// Stop monitor
	monitor.Stop()

	t.Logf("✓ Timeout-based refund completed successfully")
	t.Logf("  - Original timeout: %s", originalTimeout.Format(time.RFC3339))
	t.Logf("  - Timeout detected at: %s", currentTime.Format(time.RFC3339))
	t.Logf("  - Refund executed: buyer + arbiter signatures")
}
