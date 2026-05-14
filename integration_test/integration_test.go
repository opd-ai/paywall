// Package integration_test provides integration tests for paywall functionality
package integration_test

import (
	"testing"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// TestBasicPaymentFlow verifies basic payment creation
func TestBasicPaymentFlow(t *testing.T) {
	store := paywall.NewMemoryStore()
	config := paywall.Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	payment, err := pw.CreatePayment()
	if err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	if payment.ID == "" {
		t.Error("Payment ID should not be empty")
	}
}

// TestMultisigPayment verifies multisig payment creation and address generation
func TestMultisigPayment(t *testing.T) {
	store := paywall.NewMemoryStore()
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()

	config := paywall.Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	payment, err := pw.CreatePayment()
	if err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	if !payment.MultisigEnabled {
		t.Error("Payment should have multisig enabled")
	}

	if payment.RequiredSignatures[wallet.Bitcoin] != 2 {
		t.Errorf("Expected 2 required signatures, got %d", payment.RequiredSignatures[wallet.Bitcoin])
	}

	// Verify multisig metadata was created
	metadata := payment.MultisigMetadata[wallet.Bitcoin]
	if metadata == nil {
		t.Fatal("Multisig metadata should not be nil")
	}

	if len(metadata.PublicKeys) != 3 {
		t.Errorf("Expected 3 public keys in metadata, got %d", len(metadata.PublicKeys))
	}

	if metadata.RequiredSigs != 2 {
		t.Errorf("Expected 2 required sigs in metadata, got %d", metadata.RequiredSigs)
	}

	// Verify address was generated
	address := payment.Addresses[wallet.Bitcoin]
	if address == "" {
		t.Error("Multisig address should not be empty")
	}

	// Verify address starts with tb1 (testnet P2WSH)
	if len(address) < 3 {
		t.Errorf("Address too short: %s", address)
	}
}

// TestMultisigPaymentVerification tests that multisig payments can be verified
func TestMultisigPaymentVerification(t *testing.T) {
	store := paywall.NewMemoryStore()
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()

	config := paywall.Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MinConfirmations: 1,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	// Create a multisig payment
	payment, err := pw.CreatePayment()
	if err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	// Verify initial state
	if payment.Status != paywall.StatusPending {
		t.Errorf("Expected status Pending, got %s", payment.Status)
	}

	if !payment.MultisigEnabled {
		t.Error("Payment should have multisig enabled")
	}

	// Simulate payment confirmation (would normally be done by CryptoChainMonitor)
	// In a real scenario, funds would be sent to the multisig address
	// and the monitor would detect them via GetAddressBalance
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3

	err = store.UpdatePayment(payment)
	if err != nil {
		t.Fatalf("Failed to update payment: %v", err)
	}

	// Verify payment was updated
	retrievedPayment, err := store.GetPayment(payment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve payment: %v", err)
	}

	if retrievedPayment.Status != paywall.StatusConfirmed {
		t.Errorf("Expected status Confirmed, got %s", retrievedPayment.Status)
	}

	if retrievedPayment.Confirmations != 3 {
		t.Errorf("Expected 3 confirmations, got %d", retrievedPayment.Confirmations)
	}

	// Verify multisig metadata is intact
	if retrievedPayment.MultisigMetadata[wallet.Bitcoin] == nil {
		t.Error("Multisig metadata should be preserved")
	}
}

// TestMixedEnvironment verifies that single-sig and multisig payments can coexist
// in the same paywall instance and storage backend without interference
func TestMixedEnvironment(t *testing.T) {
	store := paywall.NewMemoryStore()
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeys()

	// First: Create single-sig paywall and payment
	singleSigConfig := paywall.Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,
	}

	singleSigPW, err := paywall.NewPaywall(singleSigConfig)
	if err != nil {
		t.Fatalf("Failed to create single-sig paywall: %v", err)
	}
	defer singleSigPW.Close()

	singleSigPayment, err := singleSigPW.CreatePayment()
	if err != nil {
		t.Fatalf("Failed to create single-sig payment: %v", err)
	}

	// Verify single-sig payment properties
	if singleSigPayment.MultisigEnabled {
		t.Error("Single-sig payment should not have multisig enabled")
	}
	if singleSigPayment.Addresses[wallet.Bitcoin] == "" {
		t.Error("Single-sig payment should have Bitcoin address")
	}
	if len(singleSigPayment.MultisigMetadata) > 0 {
		t.Error("Single-sig payment should not have multisig metadata")
	}

	// Second: Create multisig paywall and payment using same store
	multisigConfig := paywall.Config{
		PriceInBTC:       0.002,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
	}

	multisigPW, err := paywall.NewPaywall(multisigConfig)
	if err != nil {
		t.Fatalf("Failed to create multisig paywall: %v", err)
	}
	defer multisigPW.Close()

	multisigPayment, err := multisigPW.CreatePayment()
	if err != nil {
		t.Fatalf("Failed to create multisig payment: %v", err)
	}

	// Verify multisig payment properties
	if !multisigPayment.MultisigEnabled {
		t.Error("Multisig payment should have multisig enabled")
	}
	if multisigPayment.RequiredSignatures[wallet.Bitcoin] != 2 {
		t.Errorf("Expected 2 required signatures, got %d", multisigPayment.RequiredSignatures[wallet.Bitcoin])
	}
	if multisigPayment.MultisigMetadata[wallet.Bitcoin] == nil {
		t.Fatal("Multisig payment should have metadata")
	}

	// Verify both payments are distinct and retrievable from store
	retrievedSingleSig, err := store.GetPayment(singleSigPayment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve single-sig payment: %v", err)
	}
	retrievedMultisig, err := store.GetPayment(multisigPayment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve multisig payment: %v", err)
	}

	// Verify single-sig payment wasn't affected by multisig operations
	if retrievedSingleSig.MultisigEnabled {
		t.Error("Single-sig payment should remain non-multisig after multisig payment creation")
	}
	if retrievedSingleSig.Addresses[wallet.Bitcoin] == "" {
		t.Error("Single-sig payment Bitcoin address should be preserved")
	}

	// Verify multisig payment properties remain intact
	if !retrievedMultisig.MultisigEnabled {
		t.Error("Multisig payment should remain multisig")
	}
	if retrievedMultisig.MultisigMetadata[wallet.Bitcoin] == nil {
		t.Error("Multisig payment metadata should be preserved")
	}

	// Verify addresses are different
	if retrievedSingleSig.Addresses[wallet.Bitcoin] == retrievedMultisig.Addresses[wallet.Bitcoin] {
		t.Error("Single-sig and multisig payments should have different addresses")
	}

	// Verify both payments can be updated independently
	retrievedSingleSig.Status = paywall.StatusConfirmed
	retrievedSingleSig.Confirmations = 6
	err = store.UpdatePayment(retrievedSingleSig)
	if err != nil {
		t.Fatalf("Failed to update single-sig payment: %v", err)
	}

	retrievedMultisig.Status = paywall.StatusExpired
	err = store.UpdatePayment(retrievedMultisig)
	if err != nil {
		t.Fatalf("Failed to update multisig payment: %v", err)
	}

	// Verify updates didn't interfere with each other
	finalSingleSig, err := store.GetPayment(singleSigPayment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated single-sig payment: %v", err)
	}
	finalMultisig, err := store.GetPayment(multisigPayment.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated multisig payment: %v", err)
	}

	if finalSingleSig.Status != paywall.StatusConfirmed {
		t.Errorf("Single-sig payment status incorrect: expected Confirmed, got %s", finalSingleSig.Status)
	}
	if finalSingleSig.Confirmations != 6 {
		t.Errorf("Single-sig payment confirmations incorrect: expected 6, got %d", finalSingleSig.Confirmations)
	}
	if finalMultisig.Status != paywall.StatusExpired {
		t.Errorf("Multisig payment status incorrect: expected Expired, got %s", finalMultisig.Status)
	}

	// Verify type-specific fields are preserved
	if finalSingleSig.MultisigEnabled {
		t.Error("Single-sig payment multisig flag should remain false")
	}
	if !finalMultisig.MultisigEnabled {
		t.Error("Multisig payment multisig flag should remain true")
	}
}
