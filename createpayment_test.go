package paywall

import (
	"fmt"
	"testing"
	"time"
	
	"github.com/opd-ai/paywall/wallet"
)

// TestPaywall_CreatePayment tests the newly implemented CreatePayment method
func TestPaywall_CreatePayment(t *testing.T) {
	// Create test paywall with memory store
	pw, err := NewPaywall(Config{
		PriceInBTC:       0.001,
		PriceInXMR:       0.01,
		TestNet:          true,
		Store:            NewMemoryStore(),
		PaymentTimeout:   time.Hour,
		MinConfirmations: 1,
		XMRUser:          "test",
		XMRPassword:      "testpass123",
		XMRRPC:           "http://localhost:18081",
	})
	
	// XMR wallet creation will fail (expected), but BTC should work
	if err != nil {
		t.Fatalf("NewPaywall() failed: %v", err)
	}
	defer pw.Close()

	// Test successful payment creation
	t.Run("SuccessfulPaymentCreation", func(t *testing.T) {
		payment, err := pw.CreatePayment()
		if err != nil {
			t.Fatalf("CreatePayment() failed: %v", err)
		}

		// Validate payment structure
		if payment == nil {
			t.Fatal("CreatePayment() returned nil payment")
		}

		if payment.ID == "" {
			t.Error("Payment ID should not be empty")
		}

		if len(payment.ID) != 32 { // 16 bytes hex encoded = 32 chars
			t.Errorf("Payment ID length = %d, expected 32", len(payment.ID))
		}

		if payment.Status != StatusPending {
			t.Errorf("Payment status = %s, expected %s", payment.Status, StatusPending)
		}

		if payment.Confirmations != 0 {
			t.Errorf("Payment confirmations = %d, expected 0", payment.Confirmations)
		}

		// Check timing
		if payment.CreatedAt.IsZero() {
			t.Error("Payment CreatedAt should be set")
		}

		if payment.ExpiresAt.IsZero() {
			t.Error("Payment ExpiresAt should be set")
		}

		expectedExpiry := payment.CreatedAt.Add(time.Hour)
		if payment.ExpiresAt.Before(expectedExpiry.Add(-time.Second)) || 
		   payment.ExpiresAt.After(expectedExpiry.Add(time.Second)) {
			t.Errorf("Payment expiry time incorrect: got %v, expected ~%v", 
				payment.ExpiresAt, expectedExpiry)
		}

		// Check addresses and amounts
		if payment.Addresses == nil {
			t.Fatal("Payment addresses map should not be nil")
		}

		if payment.Amounts == nil {
			t.Fatal("Payment amounts map should not be nil")
		}

		// Bitcoin should always be present (XMR may fail in test env)
		btcAddr, hasBTC := payment.Addresses[wallet.Bitcoin]
		if !hasBTC {
			t.Error("Payment should have Bitcoin address")
		}

		if btcAddr == "" {
			t.Error("Bitcoin address should not be empty")
		}

		btcAmount, hasBTCAmount := payment.Amounts[wallet.Bitcoin]
		if !hasBTCAmount {
			t.Error("Payment should have Bitcoin amount")
		}

		if btcAmount != 0.001 {
			t.Errorf("Bitcoin amount = %f, expected 0.001", btcAmount)
		}

		// Verify payment was stored
		storedPayment, err := pw.Store.GetPayment(payment.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve stored payment: %v", err)
		}

		if storedPayment == nil {
			t.Fatal("Stored payment should not be nil")
		}

		if storedPayment.ID != payment.ID {
			t.Errorf("Stored payment ID = %s, expected %s", storedPayment.ID, payment.ID)
		}
	})

	// Test multiple payments have unique IDs
	t.Run("UniquePaymentIDs", func(t *testing.T) {
		payment1, err := pw.CreatePayment()
		if err != nil {
			t.Fatalf("First CreatePayment() failed: %v", err)
		}

		payment2, err := pw.CreatePayment()
		if err != nil {
			t.Fatalf("Second CreatePayment() failed: %v", err)
		}

		if payment1.ID == payment2.ID {
			t.Error("Payment IDs should be unique")
		}

		// Check both payments are stored separately
		stored1, _ := pw.Store.GetPayment(payment1.ID)
		stored2, _ := pw.Store.GetPayment(payment2.ID)

		if stored1 == nil || stored2 == nil {
			t.Error("Both payments should be stored")
		}
	})
}

// TestPaywall_CreatePayment_ErrorCases tests error scenarios
func TestPaywall_CreatePayment_ErrorCases(t *testing.T) {
	t.Run("NoWalletsEnabled", func(t *testing.T) {
		// Create paywall with empty wallets map to simulate no enabled wallets
		pw := &Paywall{
			HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
			Store:            NewMemoryStore(),
			prices:           make(map[wallet.WalletType]float64),
			paymentTimeout:   time.Hour,
			minConfirmations: 1,
		}

		payment, err := pw.CreatePayment()
		if err == nil {
			t.Error("CreatePayment() should fail with no wallets enabled")
		}

		if payment != nil {
			t.Error("CreatePayment() should return nil payment on error")
		}

		expectedErr := "no wallets enabled for payment"
		if err.Error() != expectedErr {
			t.Errorf("Error message = %q, expected %q", err.Error(), expectedErr)
		}
	})
}

// TestPaywall_CreatePayment_RaceConditionFix tests that address indexes are properly rolled back
// when payment storage fails, preventing gaps in the HD wallet derivation path
func TestPaywall_CreatePayment_RaceConditionFix(t *testing.T) {
	// Create paywall with a failing store to test rollback
	failingStore := &FailingStore{}
	
	pw, err := NewPaywall(Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            failingStore,
		PaymentTimeout:   time.Hour,
		MinConfirmations: 1,
	})
	if err != nil {
		t.Fatalf("NewPaywall() failed: %v", err)
	}
	defer pw.Close()

	// Get the initial index for Bitcoin wallet
	btcWallet := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet)
	initialIndex := btcWallet.GetNextIndex()

	// Attempt to create payment - should fail due to store failure
	_, err = pw.CreatePayment()
	if err == nil {
		t.Fatal("Expected CreatePayment() to fail with FailingStore")
	}

	// Verify that the address index was rolled back
	currentIndex := btcWallet.GetNextIndex()
	if currentIndex != initialIndex {
		t.Errorf("Address index not rolled back properly: initial=%d, current=%d", initialIndex, currentIndex)
	}
}

// FailingStore is a test store that always fails CreatePayment to test rollback
type FailingStore struct{}

func (fs *FailingStore) CreatePayment(payment *Payment) error {
	return fmt.Errorf("simulated storage failure")
}

func (fs *FailingStore) GetPayment(id string) (*Payment, error) {
	return nil, fmt.Errorf("not implemented")
}

func (fs *FailingStore) GetPaymentByAddress(address string) (*Payment, error) {
	return nil, fmt.Errorf("not implemented")
}

func (fs *FailingStore) UpdatePayment(payment *Payment) error {
	return fmt.Errorf("not implemented")
}

func (fs *FailingStore) ListPendingPayments() ([]*Payment, error) {
	return nil, fmt.Errorf("not implemented")
}

func (fs *FailingStore) Close() error {
	return nil
}
