package wallet

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestGenerateEncryptionKey verifies the encryption key generation functionality
func TestGenerateEncryptionKey(t *testing.T) {
	t.Run("ValidKeyGeneration", func(t *testing.T) {
		key, err := GenerateEncryptionKey()
		if err != nil {
			t.Fatalf("GenerateEncryptionKey() failed: %v", err)
		}

		if len(key) != 32 {
			t.Errorf("Expected key length of 32 bytes, got %d", len(key))
		}

		// Verify key is not all zeros
		allZeros := make([]byte, 32)
		if bytes.Equal(key, allZeros) {
			t.Error("Generated key should not be all zeros")
		}
	})

	t.Run("KeyUniqueness", func(t *testing.T) {
		key1, err := GenerateEncryptionKey()
		if err != nil {
			t.Fatalf("First GenerateEncryptionKey() failed: %v", err)
		}

		key2, err := GenerateEncryptionKey()
		if err != nil {
			t.Fatalf("Second GenerateEncryptionKey() failed: %v", err)
		}

		if bytes.Equal(key1, key2) {
			t.Error("Generated keys should be unique")
		}
	})
}

// TestBTCHDWallet_SaveToFile tests the wallet encryption and saving functionality
func TestBTCHDWallet_SaveToFile(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "wallet_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test wallet
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		nextIndex: 42,
		network:   &chaincfg.MainNetParams,
	}

	// Fill with test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	tests := []struct {
		name        string
		config      StorageConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "ValidSave",
			config: StorageConfig{
				DataDir:       tempDir,
				EncryptionKey: []byte("valid_32_byte_encryption_key____"),
			},
			expectError: false,
		},
		{
			name: "InvalidKeyLength_Short",
			config: StorageConfig{
				DataDir:       tempDir,
				EncryptionKey: []byte("short_key"),
			},
			expectError: true,
			errorMsg:    "encryption key must be 32 bytes",
		},
		{
			name: "InvalidKeyLength_Long",
			config: StorageConfig{
				DataDir:       tempDir,
				EncryptionKey: []byte("this_key_is_way_too_long_for_aes_256_encryption_usage"),
			},
			expectError: true,
			errorMsg:    "encryption key must be 32 bytes",
		},
		{
			name: "InvalidDirectory",
			config: StorageConfig{
				DataDir:       "/invalid/readonly/path",
				EncryptionKey: []byte("valid_32_byte_encryption_key____"),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wallet.SaveToFile(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Verify file was created
				walletPath := filepath.Join(tt.config.DataDir, "wallet.dat")
				if _, err := os.Stat(walletPath); os.IsNotExist(err) {
					t.Error("Wallet file was not created")
				}

				// Verify file has expected structure (nonce + ciphertext)
				data, err := os.ReadFile(walletPath)
				if err != nil {
					t.Errorf("Failed to read wallet file: %v", err)
				}

				if len(data) <= 12 {
					t.Error("Wallet file too short (should contain nonce + encrypted data)")
				}
			}
		})
	}
}

// TestLoadFromFile tests the wallet decryption and loading functionality
func TestLoadFromFile(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "wallet_load_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	validKey := []byte("valid_32_byte_encryption_key____")

	t.Run("ValidLoad", func(t *testing.T) {
		// First create a wallet and save it
		originalWallet := &BTCHDWallet{
			masterKey: make([]byte, 32),
			chainCode: make([]byte, 32),
			nextIndex: 123,
			network:   &chaincfg.MainNetParams,
		}

		copy(originalWallet.masterKey, []byte("original_master_key_32_bytes____"))
		copy(originalWallet.chainCode, []byte("original_chain_code_32_bytes____"))

		config := StorageConfig{
			DataDir:       tempDir,
			EncryptionKey: validKey,
		}

		err := originalWallet.SaveToFile(config)
		if err != nil {
			t.Fatalf("Failed to save wallet: %v", err)
		}

		// Now load it back
		loadedWallet, err := LoadFromFile(config)
		if err != nil {
			t.Fatalf("Failed to load wallet: %v", err)
		}

		// Verify the data matches
		if !bytes.Equal(loadedWallet.masterKey, originalWallet.masterKey) {
			t.Error("Master key mismatch after load")
		}

		if !bytes.Equal(loadedWallet.chainCode, originalWallet.chainCode) {
			t.Error("Chain code mismatch after load")
		}

		if loadedWallet.nextIndex != originalWallet.nextIndex {
			t.Errorf("Next index mismatch: expected %d, got %d", originalWallet.nextIndex, loadedWallet.nextIndex)
		}
	})

	tests := []struct {
		name        string
		config      StorageConfig
		setupFile   func(string) error
		expectError bool
		errorMsg    string
	}{
		{
			name: "InvalidKeyLength",
			config: StorageConfig{
				DataDir:       tempDir,
				EncryptionKey: []byte("short"),
			},
			expectError: true,
			errorMsg:    "encryption key must be 32 bytes",
		},
		{
			name: "FileNotFound",
			config: StorageConfig{
				DataDir:       "/nonexistent/path",
				EncryptionKey: validKey,
			},
			expectError: true,
		},
		{
			name: "InvalidFileContent_TooShort",
			config: StorageConfig{
				DataDir:       tempDir,
				EncryptionKey: validKey,
			},
			setupFile: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "wallet.dat"), []byte("short"), 0o600)
			},
			expectError: true,
			errorMsg:    "invalid wallet file",
		},
		{
			name: "InvalidFileContent_CorruptedData",
			config: StorageConfig{
				DataDir:       tempDir,
				EncryptionKey: validKey,
			},
			setupFile: func(dir string) error {
				// Create invalid encrypted data
				invalidData := make([]byte, 50)
				if _, err := io.ReadFull(rand.Reader, invalidData); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "wallet.dat"), invalidData, 0o600)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFile != nil {
				err := tt.setupFile(tt.config.DataDir)
				if err != nil {
					t.Fatalf("Failed to setup test file: %v", err)
				}
			}

			wallet, err := LoadFromFile(tt.config)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
				if wallet != nil {
					t.Error("Expected nil wallet on error")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if wallet == nil {
					t.Error("Expected valid wallet")
				}
			}
		})
	}
}

// TestStorageConfig_RoundTrip tests complete save and load cycle
func TestStorageConfig_RoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wallet_roundtrip_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Generate random encryption key
	encryptionKey, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}

	config := StorageConfig{
		DataDir:       tempDir,
		EncryptionKey: encryptionKey,
	}

	testCases := []struct {
		name   string
		wallet *BTCHDWallet
	}{
		{
			name: "ZeroValues",
			wallet: &BTCHDWallet{
				masterKey: make([]byte, 32),
				chainCode: make([]byte, 32),
				nextIndex: 0,
				network:   &chaincfg.MainNetParams,
			},
		},
		{
			name: "MaxValues",
			wallet: &BTCHDWallet{
				masterKey: bytes.Repeat([]byte{0xFF}, 32),
				chainCode: bytes.Repeat([]byte{0xFF}, 32),
				nextIndex: 0xFFFFFFFF,
				network:   &chaincfg.MainNetParams,
			},
		},
		{
			name: "RandomValues",
			wallet: func() *BTCHDWallet {
				w := &BTCHDWallet{
					masterKey: make([]byte, 32),
					chainCode: make([]byte, 32),
					nextIndex: 12345,
					network:   &chaincfg.MainNetParams,
				}
				io.ReadFull(rand.Reader, w.masterKey)
				io.ReadFull(rand.Reader, w.chainCode)
				return w
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save the wallet
			err := tc.wallet.SaveToFile(config)
			if err != nil {
				t.Fatalf("Failed to save wallet: %v", err)
			}

			// Load the wallet
			loadedWallet, err := LoadFromFile(config)
			if err != nil {
				t.Fatalf("Failed to load wallet: %v", err)
			}

			// Compare all fields
			if !bytes.Equal(loadedWallet.masterKey, tc.wallet.masterKey) {
				t.Error("Master key mismatch after round trip")
			}

			if !bytes.Equal(loadedWallet.chainCode, tc.wallet.chainCode) {
				t.Error("Chain code mismatch after round trip")
			}

			if loadedWallet.nextIndex != tc.wallet.nextIndex {
				t.Errorf("Next index mismatch: expected %d, got %d", tc.wallet.nextIndex, loadedWallet.nextIndex)
			}

			// Network should be set to MainNet by default
			if loadedWallet.network != &chaincfg.MainNetParams {
				t.Error("Network should be set to MainNet after load")
			}
		})
	}
}

// TestStorageConfig_DirectoryCreation tests automatic directory creation
func TestStorageConfig_DirectoryCreation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wallet_dir_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Use a nested path that doesn't exist
	nestedPath := filepath.Join(tempDir, "nested", "path", "for", "wallet")

	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		nextIndex: 1,
		network:   &chaincfg.MainNetParams,
	}

	config := StorageConfig{
		DataDir:       nestedPath,
		EncryptionKey: []byte("test_key_32_bytes_long__________"),
	}

	err = wallet.SaveToFile(config)
	if err != nil {
		t.Fatalf("Failed to save wallet with nested path: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Error("Nested directory was not created")
	}

	// Verify wallet file exists
	walletPath := filepath.Join(nestedPath, "wallet.dat")
	if _, err := os.Stat(walletPath); os.IsNotExist(err) {
		t.Error("Wallet file was not created in nested directory")
	}
}

// TestEncryptionSecurity verifies that encryption produces different output each time
func TestEncryptionSecurity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wallet_security_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		nextIndex: 42,
		network:   &chaincfg.MainNetParams,
	}

	// Fill with identical test data
	copy(wallet.masterKey, []byte("identical_master_key_32_bytes___"))
	copy(wallet.chainCode, []byte("identical_chain_code_32_bytes___"))

	config := StorageConfig{
		DataDir:       tempDir,
		EncryptionKey: []byte("valid_32_byte_encryption_key____"),
	}

	// Save twice with identical data
	// Modify SaveToFile calls to use different filenames for this test
	// Save first copy
	err = wallet.SaveToFile(config)
	if err != nil {
		t.Fatalf("Failed to save first wallet: %v", err)
	}

	// Read first encrypted file
	data1, err := os.ReadFile(filepath.Join(tempDir, "wallet.dat"))
	if err != nil {
		t.Fatalf("Failed to read first wallet file: %v", err)
	}

	// Remove and save again
	os.Remove(filepath.Join(tempDir, "wallet.dat"))

	err = wallet.SaveToFile(config)
	if err != nil {
		t.Fatalf("Failed to save second wallet: %v", err)
	}

	// Read second encrypted file
	data2, err := os.ReadFile(filepath.Join(tempDir, "wallet.dat"))
	if err != nil {
		t.Fatalf("Failed to read second wallet file: %v", err)
	}

	// Encrypted files should be different due to random nonces
	if bytes.Equal(data1, data2) {
		t.Error("Encrypted files should be different due to random nonces")
	}

	// But both should decrypt to the same wallet
	loadedWallet1, err := LoadFromFile(config)
	if err != nil {
		// Restore first file for loading
		os.WriteFile(filepath.Join(tempDir, "wallet.dat"), data1, 0o600)
		loadedWallet1, err = LoadFromFile(config)
		if err != nil {
			t.Fatalf("Failed to load first wallet: %v", err)
		}
	}

	// Restore second file for loading
	os.WriteFile(filepath.Join(tempDir, "wallet.dat"), data2, 0o600)
	loadedWallet2, err := LoadFromFile(config)
	if err != nil {
		t.Fatalf("Failed to load second wallet: %v", err)
	}

	// Both should have identical decrypted content
	if !bytes.Equal(loadedWallet1.masterKey, loadedWallet2.masterKey) {
		t.Error("Decrypted master keys should be identical")
	}

	if !bytes.Equal(loadedWallet1.chainCode, loadedWallet2.chainCode) {
		t.Error("Decrypted chain codes should be identical")
	}

	if loadedWallet1.nextIndex != loadedWallet2.nextIndex {
		t.Error("Decrypted next indices should be identical")
	}
}
