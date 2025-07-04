package paywall

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// TestNewFileStoreWithConfig tests the new FileStoreConfig functionality
func TestNewFileStoreWithConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "filestore_config_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("StandardFileStore", func(t *testing.T) {
		config := FileStoreConfig{
			DataDir:       filepath.Join(tempDir, "standard"),
			EncryptionKey: nil, // No encryption
		}

		store, err := NewFileStoreWithConfig(config)
		if err != nil {
			t.Fatalf("NewFileStoreWithConfig() failed: %v", err)
		}

		// Should return a standard FileStore
		if _, ok := store.(*FileStore); !ok {
			t.Errorf("Expected *FileStore, got %T", store)
		}

		// Test basic functionality
		testPayment := &Payment{
			ID: "test-payment-123",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			},
			Amounts: map[wallet.WalletType]float64{
				wallet.Bitcoin: 0.001,
			},
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
			Status:    StatusPending,
		}

		err = store.CreatePayment(testPayment)
		if err != nil {
			t.Fatalf("CreatePayment() failed: %v", err)
		}

		retrieved, err := store.GetPayment("test-payment-123")
		if err != nil {
			t.Fatalf("GetPayment() failed: %v", err)
		}

		if retrieved == nil {
			t.Fatal("Retrieved payment should not be nil")
		}

		if retrieved.ID != testPayment.ID {
			t.Errorf("Retrieved payment ID = %s, expected %s", retrieved.ID, testPayment.ID)
		}
	})

	t.Run("EncryptedFileStore", func(t *testing.T) {
		encryptionKey, err := wallet.GenerateEncryptionKey()
		if err != nil {
			t.Fatalf("Failed to generate encryption key: %v", err)
		}

		config := FileStoreConfig{
			DataDir:       filepath.Join(tempDir, "encrypted"),
			EncryptionKey: encryptionKey,
		}

		store, err := NewFileStoreWithConfig(config)
		if err != nil {
			t.Fatalf("NewFileStoreWithConfig() failed: %v", err)
		}

		// Should return an EncryptedFileStore
		if _, ok := store.(*EncryptedFileStore); !ok {
			t.Errorf("Expected *EncryptedFileStore, got %T", store)
		}

		// Test basic functionality with encryption
		testPayment := &Payment{
			ID: "encrypted-payment-456",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
			},
			Amounts: map[wallet.WalletType]float64{
				wallet.Bitcoin: 0.002,
			},
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
			Status:    StatusPending,
		}

		err = store.CreatePayment(testPayment)
		if err != nil {
			t.Fatalf("CreatePayment() failed: %v", err)
		}

		retrieved, err := store.GetPayment("encrypted-payment-456")
		if err != nil {
			t.Fatalf("GetPayment() failed: %v", err)
		}

		if retrieved == nil {
			t.Fatal("Retrieved payment should not be nil")
		}

		if retrieved.ID != testPayment.ID {
			t.Errorf("Retrieved payment ID = %s, expected %s", retrieved.ID, testPayment.ID)
		}

		// Verify file is actually encrypted (should not contain readable JSON)
		encryptedFile := filepath.Join(config.DataDir, "encrypted-payment-456.enc")
		data, err := os.ReadFile(encryptedFile)
		if err != nil {
			t.Fatalf("Failed to read encrypted file: %v", err)
		}

		// Should not contain plaintext payment data
		if len(data) < 16 { // At least nonce size
			t.Error("Encrypted file too small")
		}

		// Should not contain readable JSON
		dataStr := string(data)
		if len(dataStr) > 0 && (dataStr[0] == '{' || dataStr[0] == '[') {
			t.Error("File appears to contain unencrypted JSON")
		}
	})

	t.Run("DefaultDirectory", func(t *testing.T) {
		config := FileStoreConfig{
			DataDir:       "", // Should use default
			EncryptionKey: nil,
		}

		store, err := NewFileStoreWithConfig(config)
		if err != nil {
			t.Fatalf("NewFileStoreWithConfig() failed: %v", err)
		}

		// Should create default directory
		if _, err := os.Stat("./payments"); os.IsNotExist(err) {
			t.Error("Default payments directory should be created")
		}

		// Clean up
		os.RemoveAll("./payments")

		_ = store // Ensure store is used
	})

	t.Run("InvalidEncryptionKeyLength", func(t *testing.T) {
		config := FileStoreConfig{
			DataDir:       filepath.Join(tempDir, "invalid"),
			EncryptionKey: []byte("too_short"), // Invalid length
		}

		store, err := NewFileStoreWithConfig(config)
		if err == nil {
			t.Error("NewFileStoreWithConfig() should fail with invalid key length")
		}

		if store != nil {
			t.Error("Store should be nil on error")
		}

		expectedErr := "encryption key must be 32 bytes, got 9"
		if err.Error() != expectedErr {
			t.Errorf("Error message = %q, expected %q", err.Error(), expectedErr)
		}
	})

	t.Run("DirectoryCreationFailure", func(t *testing.T) {
		// Use an invalid path that cannot be created
		config := FileStoreConfig{
			DataDir:       "/root/invalid/readonly/path",
			EncryptionKey: nil,
		}

		store, err := NewFileStoreWithConfig(config)
		if err == nil {
			t.Error("NewFileStoreWithConfig() should fail with invalid directory")
		}

		if store != nil {
			t.Error("Store should be nil on error")
		}
	})
}
