package migrations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// createTestPayment creates a sample payment for testing
func createTestPayment(id string) *paywall.Payment {
	return &paywall.Payment{
		ID: id,
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wallet.Monero:  "4AdUndXHHZ6cfufTMvppY6JwXNouMBzSkbLYfpAV5Usx3skxNgYeYTRJ5mAkH3TZgdqXVVGHHzZfvWVpcL5mKa1Q8v8Dj8Z",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
			wallet.Monero:  0.01,
		},
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(time.Hour),
		Status:        paywall.StatusPending,
		Confirmations: 0,
	}
}

// setupTestDirectory creates a temporary directory with test files
func setupTestDirectory(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "migration_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// createTestJSONFile creates a JSON payment file for testing
func createTestJSONFile(t *testing.T, dir, id string, payment *paywall.Payment) {
	t.Helper()

	data, err := json.Marshal(payment)
	if err != nil {
		t.Fatalf("Failed to marshal payment: %v", err)
	}

	filePath := filepath.Join(dir, id+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
}

func TestEncryptExisting_Success(t *testing.T) {
	tests := []struct {
		name          string
		paymentIDs    []string
		expectSuccess int
	}{
		{
			name:          "single payment",
			paymentIDs:    []string{"payment1"},
			expectSuccess: 1,
		},
		{
			name:          "multiple payments",
			paymentIDs:    []string{"payment1", "payment2", "payment3"},
			expectSuccess: 3,
		},
		{
			name:          "empty directory",
			paymentIDs:    []string{},
			expectSuccess: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test directory
			baseDir, cleanup := setupTestDirectory(t)
			defer cleanup()

			keyPath := filepath.Join(baseDir, "test.key")

			// Create test payment files
			for _, id := range tt.paymentIDs {
				payment := createTestPayment(id)
				createTestJSONFile(t, baseDir, id, payment)
			}

			// Run encryption
			err := EncryptExisting(keyPath, baseDir)
			if err != nil {
				t.Fatalf("EncryptExisting failed: %v", err)
			}

			// Verify encrypted files were created
			for _, id := range tt.paymentIDs {
				encPath := filepath.Join(baseDir, id+".enc")
				if _, err := os.Stat(encPath); os.IsNotExist(err) {
					t.Errorf("Expected encrypted file %s was not created", encPath)
				}
			}

			// Verify original JSON files still exist
			for _, id := range tt.paymentIDs {
				jsonPath := filepath.Join(baseDir, id+".json")
				if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
					t.Errorf("Original JSON file %s should still exist", jsonPath)
				}
			}
		})
	}
}

func TestEncryptExisting_SkipAlreadyEncrypted(t *testing.T) {
	// Setup test directory
	baseDir, cleanup := setupTestDirectory(t)
	defer cleanup()

	keyPath := filepath.Join(baseDir, "test.key")
	paymentID := "payment1"

	// Create test payment file
	payment := createTestPayment(paymentID)
	createTestJSONFile(t, baseDir, paymentID, payment)

	// Create existing encrypted file
	encPath := filepath.Join(baseDir, paymentID+".enc")
	if err := os.WriteFile(encPath, []byte("existing encrypted data"), 0644); err != nil {
		t.Fatalf("Failed to create existing encrypted file: %v", err)
	}

	// Record original encrypted file content
	originalContent, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("Failed to read original encrypted file: %v", err)
	}

	// Run encryption
	err = EncryptExisting(keyPath, baseDir)
	if err != nil {
		t.Fatalf("EncryptExisting failed: %v", err)
	}

	// Verify encrypted file was not overwritten
	newContent, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("Failed to read encrypted file after migration: %v", err)
	}

	if string(originalContent) != string(newContent) {
		t.Error("Existing encrypted file was overwritten when it should have been skipped")
	}
}

func TestEncryptExisting_InvalidKeyPath(t *testing.T) {
	// Setup test directory
	baseDir, cleanup := setupTestDirectory(t)
	defer cleanup()

	// Use invalid key path (directory that doesn't exist and can't be created)
	keyPath := "/invalid/path/that/cannot/be/created/test.key"

	// Create test payment file
	payment := createTestPayment("payment1")
	createTestJSONFile(t, baseDir, "payment1", payment)

	// Run encryption - should fail due to invalid key path
	err := EncryptExisting(keyPath, baseDir)
	if err == nil {
		t.Error("Expected EncryptExisting to fail with invalid key path, but it succeeded")
	}
}

func TestEncryptExisting_InvalidBaseDirectory(t *testing.T) {
	// Use non-existent base directory
	baseDir := "/nonexistent/directory"
	keyPath := "/tmp/test.key"

	// Run encryption - should fail due to invalid base directory
	err := EncryptExisting(keyPath, baseDir)
	if err == nil {
		t.Error("Expected EncryptExisting to fail with invalid base directory, but it succeeded")
	}
}

func TestEncryptExisting_CorruptedJSONFile(t *testing.T) {
	// Setup test directory
	baseDir, cleanup := setupTestDirectory(t)
	defer cleanup()

	keyPath := filepath.Join(baseDir, "test.key")

	// Create valid payment file
	validPayment := createTestPayment("valid_payment")
	createTestJSONFile(t, baseDir, "valid_payment", validPayment)

	// Create corrupted JSON file
	corruptedPath := filepath.Join(baseDir, "corrupted_payment.json")
	if err := os.WriteFile(corruptedPath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("Failed to create corrupted JSON file: %v", err)
	}

	// Run encryption - should handle corrupted file gracefully
	err := EncryptExisting(keyPath, baseDir)
	if err != nil {
		t.Fatalf("EncryptExisting failed: %v", err)
	}

	// Verify valid payment was encrypted
	validEncPath := filepath.Join(baseDir, "valid_payment.enc")
	if _, err := os.Stat(validEncPath); os.IsNotExist(err) {
		t.Error("Valid payment should have been encrypted despite corrupted file")
	}

	// Verify corrupted payment was not encrypted
	corruptedEncPath := filepath.Join(baseDir, "corrupted_payment.enc")
	if _, err := os.Stat(corruptedEncPath); !os.IsNotExist(err) {
		t.Error("Corrupted payment should not have been encrypted")
	}
}

func TestEncryptExisting_NonJSONFiles(t *testing.T) {
	// Setup test directory
	baseDir, cleanup := setupTestDirectory(t)
	defer cleanup()

	keyPath := filepath.Join(baseDir, "test.key")

	// Create non-JSON files
	testFiles := []struct {
		name    string
		content string
	}{
		{"readme.txt", "This is a text file"},
		{"config.yaml", "key: value"},
		{"script.sh", "#!/bin/bash\necho hello"},
		{"data.csv", "col1,col2\nval1,val2"},
	}

	for _, file := range testFiles {
		filePath := filepath.Join(baseDir, file.name)
		if err := os.WriteFile(filePath, []byte(file.content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file.name, err)
		}
	}

	// Create one valid payment file
	payment := createTestPayment("payment1")
	createTestJSONFile(t, baseDir, "payment1", payment)

	// Run encryption
	err := EncryptExisting(keyPath, baseDir)
	if err != nil {
		t.Fatalf("EncryptExisting failed: %v", err)
	}

	// Verify only the JSON file was processed
	paymentEncPath := filepath.Join(baseDir, "payment1.enc")
	if _, err := os.Stat(paymentEncPath); os.IsNotExist(err) {
		t.Error("Payment JSON file should have been encrypted")
	}

	// Verify non-JSON files were not processed
	for _, file := range testFiles {
		encPath := filepath.Join(baseDir, file.name[:len(file.name)-len(filepath.Ext(file.name))]+".enc")
		if _, err := os.Stat(encPath); !os.IsNotExist(err) {
			t.Errorf("Non-JSON file %s should not have been processed", file.name)
		}
	}
}

func TestEncryptExisting_EmptyKeyPath(t *testing.T) {
	// Setup test directory
	baseDir, cleanup := setupTestDirectory(t)
	defer cleanup()

	// Create test payment file
	payment := createTestPayment("payment1")
	createTestJSONFile(t, baseDir, "payment1", payment)

	// Run encryption with empty key path (should use default)
	err := EncryptExisting("", baseDir)
	if err != nil {
		t.Fatalf("EncryptExisting failed with empty key path: %v", err)
	}

	// Verify encrypted file was created
	encPath := filepath.Join(baseDir, "payment1.enc")
	if _, err := os.Stat(encPath); os.IsNotExist(err) {
		t.Error("Expected encrypted file was not created with empty key path")
	}

	// Verify default key was created
	defaultKeyPath := "./keys/store.key"
	defer os.RemoveAll("./keys") // Cleanup default key directory

	if _, err := os.Stat(defaultKeyPath); os.IsNotExist(err) {
		t.Error("Default key file should have been created")
	}
}

func TestEncryptExisting_MixedScenario(t *testing.T) {
	// Setup test directory
	baseDir, cleanup := setupTestDirectory(t)
	defer cleanup()

	keyPath := filepath.Join(baseDir, "test.key")

	// Create various types of files:
	// 1. Valid payment that should be encrypted
	validPayment := createTestPayment("valid_payment")
	createTestJSONFile(t, baseDir, "valid_payment", validPayment)

	// 2. Payment that already has encrypted version
	existingPayment := createTestPayment("existing_payment")
	createTestJSONFile(t, baseDir, "existing_payment", existingPayment)
	existingEncPath := filepath.Join(baseDir, "existing_payment.enc")
	if err := os.WriteFile(existingEncPath, []byte("existing encrypted data"), 0644); err != nil {
		t.Fatalf("Failed to create existing encrypted file: %v", err)
	}

	// 3. Corrupted JSON file
	corruptedPath := filepath.Join(baseDir, "corrupted.json")
	if err := os.WriteFile(corruptedPath, []byte("{corrupted"), 0644); err != nil {
		t.Fatalf("Failed to create corrupted file: %v", err)
	}

	// 4. Non-JSON file
	txtPath := filepath.Join(baseDir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("readme content"), 0644); err != nil {
		t.Fatalf("Failed to create text file: %v", err)
	}

	// Run encryption
	err := EncryptExisting(keyPath, baseDir)
	if err != nil {
		t.Fatalf("EncryptExisting failed: %v", err)
	}

	// Verify results
	// 1. Valid payment should be encrypted
	validEncPath := filepath.Join(baseDir, "valid_payment.enc")
	if _, err := os.Stat(validEncPath); os.IsNotExist(err) {
		t.Error("Valid payment should have been encrypted")
	}

	// 2. Existing encrypted file should remain unchanged
	existingContent, err := os.ReadFile(existingEncPath)
	if err != nil || string(existingContent) != "existing encrypted data" {
		t.Error("Existing encrypted file should remain unchanged")
	}

	// 3. Corrupted file should not be encrypted
	corruptedEncPath := filepath.Join(baseDir, "corrupted.enc")
	if _, err := os.Stat(corruptedEncPath); !os.IsNotExist(err) {
		t.Error("Corrupted file should not have been encrypted")
	}

	// 4. Non-JSON file should not be processed
	txtEncPath := filepath.Join(baseDir, "readme.enc")
	if _, err := os.Stat(txtEncPath); !os.IsNotExist(err) {
		t.Error("Non-JSON file should not have been processed")
	}
}
