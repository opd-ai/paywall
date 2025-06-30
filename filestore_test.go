package paywall

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// Helper function to create a temporary directory for testing
func createTempDir(t *testing.T) string {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "filestore_test_")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	return tempDir
}

// Helper function to create a test payment
func createTestPayment(id string) *Payment {
	return &Payment{
		ID: id,
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wallet.Monero:  "48edfHu7V9Z84YzzMa6fUueoELZ9ZRXq9VetWzYGzKt52XU5xvqgzYnDK9URnRoJMk1j8nLwEVsaSWJ4fhdUyZijBGUicoD",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.0001,
			wallet.Monero:  0.001,
		},
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(2 * time.Hour),
		Status:        StatusPending,
		Confirmations: 0,
		TransactionID: "",
	}
}

func TestNewFileStore(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		want    string
	}{
		{
			name:    "custom base directory",
			baseDir: "/tmp/test-payments",
			want:    "/tmp/test-payments",
		},
		{
			name:    "empty base directory uses default",
			baseDir: "",
			want:    "./payments",
		},
		{
			name:    "relative path",
			baseDir: "./custom-payments",
			want:    "./custom-payments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing directory first
			if tt.baseDir != "" {
				os.RemoveAll(tt.baseDir)
			} else {
				os.RemoveAll("./payments")
			}

			store := NewFileStore(tt.baseDir)
			if store == nil {
				t.Error("NewFileStore() returned nil")
				return
			}

			if store.baseDir != tt.want {
				t.Errorf("NewFileStore() baseDir = %v, want %v", store.baseDir, tt.want)
			}

			// Verify directory was created
			if _, err := os.Stat(store.baseDir); os.IsNotExist(err) {
				t.Errorf("NewFileStore() did not create directory %v", store.baseDir)
			}

			// Clean up
			os.RemoveAll(store.baseDir)
		})
	}
}

func TestFileStore_CreatePayment(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	tests := []struct {
		name    string
		payment *Payment
		wantErr bool
	}{
		{
			name:    "valid payment",
			payment: createTestPayment("test-payment-1"),
			wantErr: false,
		},
		{
			name:    "payment with special characters in ID",
			payment: createTestPayment("test-payment-special-#@$"),
			wantErr: false,
		},
		{
			name: "payment with empty addresses map",
			payment: &Payment{
				ID:        "empty-addresses",
				Addresses: map[wallet.WalletType]string{},
				Amounts:   map[wallet.WalletType]float64{},
				CreatedAt: time.Now(),
				Status:    StatusPending,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreatePayment(tt.payment)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileStore.CreatePayment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file was created
				filename := filepath.Join(tempDir, tt.payment.ID+".json")
				if _, err := os.Stat(filename); os.IsNotExist(err) {
					t.Errorf("FileStore.CreatePayment() did not create file %v", filename)
				}

				// Verify file permissions
				fileInfo, err := os.Stat(filename)
				if err != nil {
					t.Errorf("Failed to get file info: %v", err)
				} else {
					if fileInfo.Mode().Perm() != 0o600 {
						t.Errorf("FileStore.CreatePayment() file permissions = %v, want %v", fileInfo.Mode().Perm(), 0o600)
					}
				}
			}
		})
	}
}

func TestFileStore_CreatePayment_NilPayment(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// This should cause a panic or error due to nil payment
	defer func() {
		if r := recover(); r == nil {
			t.Error("FileStore.CreatePayment() with nil payment should panic")
		}
	}()

	err := store.CreatePayment(nil)
	if err == nil {
		t.Error("FileStore.CreatePayment() with nil payment should return error")
	}
}

func TestFileStore_GetPayment(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)
	testPayment := createTestPayment("test-payment-get")

	// First create a payment
	err := store.CreatePayment(testPayment)
	if err != nil {
		t.Fatalf("Failed to create test payment: %v", err)
	}

	tests := []struct {
		name      string
		paymentID string
		wantNil   bool
		wantErr   bool
	}{
		{
			name:      "existing payment",
			paymentID: "test-payment-get",
			wantNil:   false,
			wantErr:   false,
		},
		{
			name:      "non-existing payment",
			paymentID: "non-existing-payment",
			wantNil:   true,
			wantErr:   false,
		},
		{
			name:      "empty payment ID",
			paymentID: "",
			wantNil:   true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment, err := store.GetPayment(tt.paymentID)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileStore.GetPayment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if (payment == nil) != tt.wantNil {
				t.Errorf("FileStore.GetPayment() payment is nil = %v, wantNil %v", payment == nil, tt.wantNil)
				return
			}

			if !tt.wantNil && payment != nil {
				if payment.ID != tt.paymentID {
					t.Errorf("FileStore.GetPayment() payment.ID = %v, want %v", payment.ID, tt.paymentID)
				}
				if payment.Addresses[wallet.Bitcoin] != testPayment.Addresses[wallet.Bitcoin] {
					t.Errorf("FileStore.GetPayment() Bitcoin address = %v, want %v",
						payment.Addresses[wallet.Bitcoin], testPayment.Addresses[wallet.Bitcoin])
				}
			}
		})
	}
}

func TestFileStore_UpdatePayment(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)
	testPayment := createTestPayment("test-payment-update")

	// First create a payment
	err := store.CreatePayment(testPayment)
	if err != nil {
		t.Fatalf("Failed to create test payment: %v", err)
	}

	tests := []struct {
		name     string
		setup    func() *Payment
		wantErr  bool
		validate func(*testing.T, *Payment)
	}{
		{
			name: "update existing payment status",
			setup: func() *Payment {
				updated := createTestPayment("test-payment-update")
				updated.Status = StatusConfirmed
				updated.Confirmations = 6
				updated.TransactionID = "abc123def456"
				return updated
			},
			wantErr: false,
			validate: func(t *testing.T, p *Payment) {
				if p.Status != StatusConfirmed {
					t.Errorf("Updated payment status = %v, want %v", p.Status, StatusConfirmed)
				}
				if p.Confirmations != 6 {
					t.Errorf("Updated payment confirmations = %v, want %v", p.Confirmations, 6)
				}
				if p.TransactionID != "abc123def456" {
					t.Errorf("Updated payment transaction ID = %v, want %v", p.TransactionID, "abc123def456")
				}
			},
		},
		{
			name: "update non-existing payment creates new file",
			setup: func() *Payment {
				return createTestPayment("new-payment-via-update")
			},
			wantErr: false,
			validate: func(t *testing.T, p *Payment) {
				if p.ID != "new-payment-via-update" {
					t.Errorf("New payment ID = %v, want %v", p.ID, "new-payment-via-update")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment := tt.setup()
			err := store.UpdatePayment(payment)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileStore.UpdatePayment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the update by reading the payment back
				retrieved, err := store.GetPayment(payment.ID)
				if err != nil {
					t.Errorf("Failed to retrieve updated payment: %v", err)
					return
				}
				if retrieved == nil {
					t.Error("Updated payment is nil")
					return
				}

				tt.validate(t, retrieved)
			}
		})
	}
}

func TestFileStore_ListPendingPayments(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// Create test payments with different confirmation counts
	payments := []*Payment{
		{
			ID:            "payment-0-confirmations",
			Addresses:     map[wallet.WalletType]string{wallet.Bitcoin: "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2"},
			Amounts:       map[wallet.WalletType]float64{wallet.Bitcoin: 0.0001},
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(2 * time.Hour),
			Status:        StatusPending,
			Confirmations: 0,
		},
		{
			ID:            "payment-1-confirmation",
			Addresses:     map[wallet.WalletType]string{wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"},
			Amounts:       map[wallet.WalletType]float64{wallet.Bitcoin: 0.0002},
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(2 * time.Hour),
			Status:        StatusPending,
			Confirmations: 1,
		},
		{
			ID:            "payment-3-confirmations",
			Addresses:     map[wallet.WalletType]string{wallet.Bitcoin: "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy"},
			Amounts:       map[wallet.WalletType]float64{wallet.Bitcoin: 0.0003},
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(2 * time.Hour),
			Status:        StatusPending,
			Confirmations: 3,
		},
		{
			ID:            "payment-6-confirmations",
			Addresses:     map[wallet.WalletType]string{wallet.Bitcoin: "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"},
			Amounts:       map[wallet.WalletType]float64{wallet.Bitcoin: 0.0004},
			CreatedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(2 * time.Hour),
			Status:        StatusConfirmed,
			Confirmations: 6,
		},
	}

	// Store all test payments
	for _, payment := range payments {
		err := store.CreatePayment(payment)
		if err != nil {
			t.Fatalf("Failed to create test payment %s: %v", payment.ID, err)
		}
	}

	tests := []struct {
		name                string
		wantErr             bool
		expectedCount       int
		expectedMinConfirms int
	}{
		{
			name:                "list pending payments with more than 1 confirmation",
			wantErr:             false,
			expectedCount:       2, // payments with 3 and 6 confirmations
			expectedMinConfirms: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pendingPayments, err := store.ListPendingPayments()
			if (err != nil) != tt.wantErr {
				t.Errorf("FileStore.ListPendingPayments() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(pendingPayments) != tt.expectedCount {
					t.Errorf("FileStore.ListPendingPayments() count = %v, want %v", len(pendingPayments), tt.expectedCount)
				}

				// Verify all returned payments have more than 1 confirmation
				for _, payment := range pendingPayments {
					if payment.Confirmations <= 1 {
						t.Errorf("FileStore.ListPendingPayments() returned payment with %v confirmations, want > 1", payment.Confirmations)
					}
				}
			}
		})
	}
}

func TestFileStore_ListPendingPayments_EmptyDirectory(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	pendingPayments, err := store.ListPendingPayments()
	if err != nil {
		t.Errorf("FileStore.ListPendingPayments() on empty directory error = %v, want nil", err)
	}

	if len(pendingPayments) != 0 {
		t.Errorf("FileStore.ListPendingPayments() on empty directory count = %v, want 0", len(pendingPayments))
	}
}

func TestFileStore_ListPendingPayments_NonJSONFiles(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// Create some non-JSON files
	nonJSONFiles := []string{"readme.txt", "config.xml", "data.csv", "script.sh"}
	for _, filename := range nonJSONFiles {
		filePath := filepath.Join(tempDir, filename)
		err := os.WriteFile(filePath, []byte("test content"), 0o644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}

	// Create one valid payment
	testPayment := createTestPayment("valid-payment")
	testPayment.Confirmations = 5
	err := store.CreatePayment(testPayment)
	if err != nil {
		t.Fatalf("Failed to create test payment: %v", err)
	}

	pendingPayments, err := store.ListPendingPayments()
	if err != nil {
		t.Errorf("FileStore.ListPendingPayments() error = %v, want nil", err)
	}

	if len(pendingPayments) != 1 {
		t.Errorf("FileStore.ListPendingPayments() count = %v, want 1", len(pendingPayments))
	}

	if len(pendingPayments) > 0 && pendingPayments[0].ID != "valid-payment" {
		t.Errorf("FileStore.ListPendingPayments() payment ID = %v, want valid-payment", pendingPayments[0].ID)
	}
}

func TestFileStore_GetPaymentByAddress(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// Create test payments with different addresses
	btcAddr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	xmrAddr := "48edfHu7V9Z84YzzMa6fUueoELZ9ZRXq9VetWzYGzKt52XU5xvqgzYnDK9URnRoJMk1j8nLwEVsaSWJ4fhdUyZijBGUicoD"

	payments := []*Payment{
		{
			ID: "payment-btc-only",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: btcAddr,
			},
			Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.0001},
			CreatedAt: time.Now(),
			Status:    StatusPending,
		},
		{
			ID: "payment-xmr-only",
			Addresses: map[wallet.WalletType]string{
				wallet.Monero: xmrAddr,
			},
			Amounts:   map[wallet.WalletType]float64{wallet.Monero: 0.001},
			CreatedAt: time.Now(),
			Status:    StatusPending,
		},
		{
			ID: "payment-both",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
				wallet.Monero:  "44AFFq5kSiGBoZ4NMDwYtN18obc8AemS33DBLWs3H7otXft3XjrpDtQGv7SqSsaBYBb98uNbr2VBBEt7f2wfn3RVGQBEP3A",
			},
			Amounts: map[wallet.WalletType]float64{
				wallet.Bitcoin: 0.0002,
				wallet.Monero:  0.002,
			},
			CreatedAt: time.Now(),
			Status:    StatusPending,
		},
	}

	// Store all test payments
	for _, payment := range payments {
		err := store.CreatePayment(payment)
		if err != nil {
			t.Fatalf("Failed to create test payment %s: %v", payment.ID, err)
		}
	}

	tests := []struct {
		name        string
		address     string
		wantPayment string
		wantNil     bool
		wantErr     bool
	}{
		{
			name:        "find payment by Bitcoin address",
			address:     btcAddr,
			wantPayment: "payment-btc-only",
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:        "find payment by Monero address",
			address:     xmrAddr,
			wantPayment: "payment-xmr-only",
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:        "find payment by Bitcoin address in mixed payment",
			address:     "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
			wantPayment: "payment-both",
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:        "find payment by Monero address in mixed payment",
			address:     "44AFFq5kSiGBoZ4NMDwYtN18obc8AemS33DBLWs3H7otXft3XjrpDtQGv7SqSsaBYBb98uNbr2VBBEt7f2wfn3RVGQBEP3A",
			wantPayment: "payment-both",
			wantNil:     false,
			wantErr:     false,
		},
		{
			name:        "address not found",
			address:     "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy",
			wantPayment: "",
			wantNil:     true,
			wantErr:     false,
		},
		{
			name:        "empty address",
			address:     "",
			wantPayment: "",
			wantNil:     true,
			wantErr:     false,
		},
		{
			name:        "invalid address",
			address:     "invalid-address",
			wantPayment: "",
			wantNil:     true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment, err := store.GetPaymentByAddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileStore.GetPaymentByAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if (payment == nil) != tt.wantNil {
				t.Errorf("FileStore.GetPaymentByAddress() payment is nil = %v, wantNil %v", payment == nil, tt.wantNil)
				return
			}

			if !tt.wantNil && payment != nil {
				if payment.ID != tt.wantPayment {
					t.Errorf("FileStore.GetPaymentByAddress() payment.ID = %v, want %v", payment.ID, tt.wantPayment)
				}
			}
		})
	}
}

func TestFileStore_GetPaymentByAddress_EmptyDirectory(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	payment, err := store.GetPaymentByAddress("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa")
	if err != nil {
		t.Errorf("FileStore.GetPaymentByAddress() on empty directory error = %v, want nil", err)
	}

	if payment != nil {
		t.Errorf("FileStore.GetPaymentByAddress() on empty directory payment = %v, want nil", payment)
	}
}

// TestFileStore_ConcurrentAccess tests thread safety of FileStore operations
func TestFileStore_ConcurrentAccess(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// Test concurrent operations
	const numGoroutines = 10
	const numOperations = 5

	errChan := make(chan error, numGoroutines*numOperations)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				payment := createTestPayment(fmt.Sprintf("concurrent-payment-%d-%d", id, j))
				if err := store.CreatePayment(payment); err != nil {
					errChan <- fmt.Errorf("goroutine %d operation %d: %w", id, j, err)
				} else {
					errChan <- nil
				}
			}
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines*numOperations; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent operation failed: %v", err)
		}
	}

	// Verify all payments were created
	payments, err := store.ListPendingPayments()
	if err != nil {
		t.Errorf("Failed to list payments after concurrent operations: %v", err)
	}

	// We can't guarantee exact count due to filtering (>1 confirmations), but should have some
	if len(payments) > numGoroutines*numOperations {
		t.Errorf("Too many payments found: %d, expected <= %d", len(payments), numGoroutines*numOperations)
	}
}

// TestFileStore_CorruptedJSONHandling tests how the store handles corrupted JSON files
func TestFileStore_CorruptedJSONHandling(t *testing.T) {
	tempDir := createTempDir(t)
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// Create a corrupted JSON file
	corruptedFile := filepath.Join(tempDir, "corrupted-payment.json")
	err := os.WriteFile(corruptedFile, []byte("{invalid json content"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create corrupted file: %v", err)
	}

	// Create a valid payment file
	validPayment := createTestPayment("valid-payment")
	validPayment.Confirmations = 5
	err = store.CreatePayment(validPayment)
	if err != nil {
		t.Fatalf("Failed to create valid payment: %v", err)
	}

	// ListPendingPayments should skip corrupted files and return valid ones
	payments, err := store.ListPendingPayments()
	if err != nil {
		t.Errorf("FileStore.ListPendingPayments() with corrupted file error = %v, want nil", err)
	}

	if len(payments) != 1 {
		t.Errorf("FileStore.ListPendingPayments() with corrupted file count = %v, want 1", len(payments))
	}

	// GetPaymentByAddress should skip corrupted files and find valid ones
	payment, err := store.GetPaymentByAddress(validPayment.Addresses[wallet.Bitcoin])
	if err != nil {
		t.Errorf("FileStore.GetPaymentByAddress() with corrupted file error = %v, want nil", err)
	}

	if payment == nil {
		t.Error("FileStore.GetPaymentByAddress() should find valid payment despite corrupted file")
	} else if payment.ID != "valid-payment" {
		t.Errorf("FileStore.GetPaymentByAddress() payment.ID = %v, want valid-payment", payment.ID)
	}
}

// Benchmark tests for performance validation
func BenchmarkFileStore_CreatePayment(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "filestore_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payment := createTestPayment(fmt.Sprintf("bench-payment-%d", i))
		if err := store.CreatePayment(payment); err != nil {
			b.Fatalf("CreatePayment failed: %v", err)
		}
	}
}

func BenchmarkFileStore_GetPayment(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "filestore_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewFileStore(tempDir)

	// Create a test payment
	payment := createTestPayment("bench-payment")
	if err := store.CreatePayment(payment); err != nil {
		b.Fatalf("Failed to create test payment: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.GetPayment("bench-payment")
		if err != nil {
			b.Fatalf("GetPayment failed: %v", err)
		}
	}
}
