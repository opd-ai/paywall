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

// TestMultisigPayment verifies multisig payment creation
func TestMultisigPayment(t *testing.T) {
	t.Skip("Multisig wallet implementation in progress - test will be enabled when complete")

	store := paywall.NewMemoryStore()
	key1, key2, key3 := make([]byte, 33), make([]byte, 33), make([]byte, 33)
	copy(key1, []byte{0x02})
	copy(key2, []byte{0x03})
	copy(key3, []byte{0x04})

	config := paywall.Config{
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
}
