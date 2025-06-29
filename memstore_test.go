package paywall

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

func TestNewMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	
	if store == nil {
		t.Fatal("NewMemoryStore() returned nil")
	}
	
	if store.payments == nil {
		t.Error("NewMemoryStore() did not initialize payments map")
	}
	
	if len(store.payments) != 0 {
		t.Error("NewMemoryStore() should start with empty payments map")
	}
}

func TestMemoryStore_CreatePayment(t *testing.T) {
	store := NewMemoryStore()
	
	testCases := []struct {
		name    string
		payment *Payment
		wantErr bool
	}{
		{
			name: "valid payment",
			payment: &Payment{
				ID: "test-payment-1",
				Addresses: map[wallet.WalletType]string{
					wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
					wallet.Monero:  "48edfHu7V9Z9XdMHvY5UBj9CKdNgGzBCQVfv5QrMPTL",
				},
				Amounts: map[wallet.WalletType]float64{
					wallet.Bitcoin: 0.001,
					wallet.Monero:  0.05,
				},
				CreatedAt:     time.Now(),
				ExpiresAt:     time.Now().Add(24 * time.Hour),
				Status:        StatusPending,
				Confirmations: 0,
			},
			wantErr: false,
		},
		{
			name: "payment with empty ID",
			payment: &Payment{
				ID: "",
				Addresses: map[wallet.WalletType]string{
					wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
				},
				Status: StatusPending,
			},
			wantErr: false, // MemoryStore doesn't validate, just stores
		},
		{
			name: "nil payment",
			payment: nil,
			wantErr: false, // Will panic in real usage, but testing the behavior
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			
			// Test for panic with nil payment
			if tc.payment == nil {
				defer func() {
					if r := recover(); r == nil {
						t.Error("Expected panic with nil payment, but didn't panic")
					}
				}()
				err = store.CreatePayment(tc.payment)
				return
			}
			
			err = store.CreatePayment(tc.payment)
			
			if (err != nil) != tc.wantErr {
				t.Errorf("CreatePayment() error = %v, wantErr %v", err, tc.wantErr)
				return
			}
			
			if err == nil {
				// Verify payment was stored
				storedPayment, _ := store.GetPayment(tc.payment.ID)
				if storedPayment == nil {
					t.Error("Payment was not stored after CreatePayment()")
				} else if storedPayment.ID != tc.payment.ID {
					t.Errorf("Stored payment ID = %v, want %v", storedPayment.ID, tc.payment.ID)
				}
			}
		})
	}
}

func TestMemoryStore_GetPayment(t *testing.T) {
	store := NewMemoryStore()
	
	// Setup test data
	testPayment := &Payment{
		ID: "test-payment-get",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
		},
		Status: StatusPending,
	}
	
	// Store the payment first
	store.CreatePayment(testPayment)
	
	testCases := []struct {
		name      string
		id        string
		wantFound bool
		wantID    string
	}{
		{
			name:      "existing payment",
			id:        "test-payment-get",
			wantFound: true,
			wantID:    "test-payment-get",
		},
		{
			name:      "non-existing payment",
			id:        "non-existent",
			wantFound: false,
			wantID:    "",
		},
		{
			name:      "empty ID",
			id:        "",
			wantFound: false,
			wantID:    "",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payment, err := store.GetPayment(tc.id)
			
			if err != nil {
				t.Errorf("GetPayment() unexpected error = %v", err)
				return
			}
			
			if tc.wantFound {
				if payment == nil {
					t.Error("GetPayment() returned nil for existing payment")
					return
				}
				if payment.ID != tc.wantID {
					t.Errorf("GetPayment() ID = %v, want %v", payment.ID, tc.wantID)
				}
			} else {
				if payment != nil {
					t.Errorf("GetPayment() returned payment for non-existing ID: %v", payment.ID)
				}
			}
		})
	}
}

func TestMemoryStore_UpdatePayment(t *testing.T) {
	store := NewMemoryStore()
	
	// Setup initial payment
	initialPayment := &Payment{
		ID:            "test-payment-update",
		Status:        StatusPending,
		Confirmations: 0,
		TransactionID: "",
	}
	store.CreatePayment(initialPayment)
	
	// Update the payment
	updatedPayment := &Payment{
		ID:            "test-payment-update",
		Status:        StatusConfirmed,
		Confirmations: 3,
		TransactionID: "abcd1234",
	}
	
	err := store.UpdatePayment(updatedPayment)
	if err != nil {
		t.Errorf("UpdatePayment() unexpected error = %v", err)
		return
	}
	
	// Verify the update
	retrieved, _ := store.GetPayment("test-payment-update")
	if retrieved == nil {
		t.Fatal("Updated payment not found")
	}
	
	if retrieved.Status != StatusConfirmed {
		t.Errorf("Status not updated: got %v, want %v", retrieved.Status, StatusConfirmed)
	}
	
	if retrieved.Confirmations != 3 {
		t.Errorf("Confirmations not updated: got %v, want %v", retrieved.Confirmations, 3)
	}
	
	if retrieved.TransactionID != "abcd1234" {
		t.Errorf("TransactionID not updated: got %v, want %v", retrieved.TransactionID, "abcd1234")
	}
}

func TestMemoryStore_ListPendingPayments(t *testing.T) {
	store := NewMemoryStore()
	
	// Setup test payments with different confirmation counts
	testPayments := []*Payment{
		{
			ID:            "payment-0-confirmations",
			Status:        StatusPending,
			Confirmations: 0,
		},
		{
			ID:            "payment-1-confirmation",
			Status:        StatusPending,
			Confirmations: 1,
		},
		{
			ID:            "payment-2-confirmations",
			Status:        StatusPending,
			Confirmations: 2,
		},
		{
			ID:            "payment-5-confirmations",
			Status:        StatusConfirmed,
			Confirmations: 5,
		},
	}
	
	// Store all test payments
	for _, payment := range testPayments {
		store.CreatePayment(payment)
	}
	
	// Get pending payments (should only return those with >1 confirmation)
	pendingPayments, err := store.ListPendingPayments()
	if err != nil {
		t.Errorf("ListPendingPayments() unexpected error = %v", err)
		return
	}
	
	// Should return 2 payments (2 and 5 confirmations)
	expectedCount := 2
	if len(pendingPayments) != expectedCount {
		t.Errorf("ListPendingPayments() returned %v payments, want %v", len(pendingPayments), expectedCount)
	}
	
	// Verify the returned payments have >1 confirmation
	for _, payment := range pendingPayments {
		if payment.Confirmations <= 1 {
			t.Errorf("ListPendingPayments() returned payment with %v confirmations, should be > 1", payment.Confirmations)
		}
	}
	
	// Test with empty store
	emptyStore := NewMemoryStore()
	emptyPending, err := emptyStore.ListPendingPayments()
	if err != nil {
		t.Errorf("ListPendingPayments() on empty store unexpected error = %v", err)
	}
	if len(emptyPending) != 0 {
		t.Errorf("ListPendingPayments() on empty store returned %v payments, want 0", len(emptyPending))
	}
}

func TestMemoryStore_GetPaymentByAddress(t *testing.T) {
	store := NewMemoryStore()
	
	// Setup test payments with different addresses
	testPayments := []*Payment{
		{
			ID: "payment-btc",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
				wallet.Monero:  "48edfHu7V9Z9XdMHvY5UBj9CKdNgGzBCQVfv5QrMPTL",
			},
		},
		{
			ID: "payment-xmr-only",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN3", // Set a different Bitcoin address
				wallet.Monero:  "41edfHu7V9Z9XdMHvY5UBj9CKdNgGzBCQVfv5QrMPTL",
			},
		},
	}
	
	// Store test payments
	for _, payment := range testPayments {
		store.CreatePayment(payment)
	}
	
	testCases := []struct {
		name      string
		address   string
		wantFound bool
		wantID    string
	}{
		{
			name:      "existing Bitcoin address",
			address:   "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantFound: true,
			wantID:    "payment-btc",
		},
		{
			name:      "existing Monero address from first payment",
			address:   "48edfHu7V9Z9XdMHvY5UBj9CKdNgGzBCQVfv5QrMPTL",
			wantFound: true,
			wantID:    "payment-btc",
		},
		{
			name:      "existing Monero address from second payment",
			address:   "41edfHu7V9Z9XdMHvY5UBj9CKdNgGzBCQVfv5QrMPTL",
			wantFound: true,
			wantID:    "payment-xmr-only",
		},
		{
			name:      "non-existing address",
			address:   "1NonExistentAddressForTesting123456789",
			wantFound: false,
			wantID:    "",
		},
		{
			name:      "empty address",
			address:   "",
			wantFound: false,
			wantID:    "",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payment, err := store.GetPaymentByAddress(tc.address)
			
			if err != nil {
				t.Errorf("GetPaymentByAddress() unexpected error = %v", err)
				return
			}
			
			if tc.wantFound {
				if payment == nil {
					t.Error("GetPaymentByAddress() returned nil for existing address")
					return
				}
				if payment.ID != tc.wantID {
					t.Errorf("GetPaymentByAddress() ID = %v, want %v", payment.ID, tc.wantID)
				}
			} else {
				if payment != nil {
					t.Errorf("GetPaymentByAddress() returned payment for non-existing address: %v", payment.ID)
				}
			}
		})
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	store := NewMemoryStore()
	
	// Test concurrent read/write operations
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperationsPerGoroutine := 100
	
	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				payment := &Payment{
					ID:     fmt.Sprintf("payment-%d-%d", goroutineID, j),
					Status: StatusPending,
				}
				err := store.CreatePayment(payment)
				if err != nil {
					t.Errorf("Concurrent CreatePayment() error: %v", err)
				}
			}
		}(i)
	}
	
	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				// Try to read payments that might not exist yet
				_, err := store.GetPayment(fmt.Sprintf("payment-%d-%d", goroutineID, j))
				if err != nil {
					t.Errorf("Concurrent GetPayment() error: %v", err)
				}
			}
		}(i)
	}
	
	// Concurrent list operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperationsPerGoroutine; i++ {
			_, err := store.ListPendingPayments()
			if err != nil {
				t.Errorf("Concurrent ListPendingPayments() error: %v", err)
			}
		}
	}()
	
	wg.Wait()
	
	// Verify final state
	totalPayments := numGoroutines * numOperationsPerGoroutine
	
	// Count stored payments by trying to retrieve them
	foundCount := 0
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < numOperationsPerGoroutine; j++ {
			payment, _ := store.GetPayment(fmt.Sprintf("payment-%d-%d", i, j))
			if payment != nil {
				foundCount++
			}
		}
	}
	
	if foundCount != totalPayments {
		t.Errorf("Concurrent operations: found %d payments, expected %d", foundCount, totalPayments)
	}
}

// Need to add fmt import for the concurrent test
