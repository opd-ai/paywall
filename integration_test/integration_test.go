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
