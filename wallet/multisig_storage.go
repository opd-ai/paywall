// Package wallet implements secure storage functionality for multisig wallets.
package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// MultisigStorageConfig defines configuration for multisig wallet persistence.
//
// Fields:
//   - DataDir: Directory path where multisig wallet files will be stored
//   - EncryptionKey: 32-byte key used for AES-256-GCM encryption (optional)
//   - WalletType: The type of wallet (Bitcoin, Monero)
//
// Security:
//   - DataDir should have appropriate filesystem permissions (0700)
//   - EncryptionKey is optional but recommended for production
//   - If EncryptionKey is nil, data is stored in plaintext JSON
type MultisigStorageConfig struct {
	DataDir       string
	EncryptionKey []byte     // Optional: 32-byte key for AES-256
	WalletType    WalletType // BTC or XMR
}

// MultisigWalletData contains the serializable state of a multisig wallet.
// This structure is saved to disk and loaded during wallet recovery.
//
// Related types: MultisigConfig, MultisigMetadata
type MultisigWalletData struct {
	// WalletType identifies the cryptocurrency (Bitcoin or Monero)
	WalletType WalletType `json:"wallet_type"`
	// Config contains the multisig configuration (m-of-n, public keys, etc.)
	Config *MultisigConfig `json:"config"`
	// Addresses maps generated multisig addresses to their metadata
	Addresses map[string]*MultisigMetadata `json:"addresses"`
	// Version is the schema version for forward compatibility
	Version int `json:"version"`
}

// MultisigStorage manages persistent storage of multisig wallet state.
// Thread-safe for concurrent access.
type MultisigStorage struct {
	config MultisigStorageConfig
	mu     sync.RWMutex
}

// NewMultisigStorage creates a new multisig storage manager.
//
// Parameters:
//   - config: Storage configuration including data directory and encryption key
//
// Returns:
//   - *MultisigStorage: Storage manager instance
//   - error: If configuration is invalid
//
// Related: SaveMultisigWallet, LoadMultisigWallet
func NewMultisigStorage(config MultisigStorageConfig) (*MultisigStorage, error) {
	if config.DataDir == "" {
		return nil, errors.New("data directory is required")
	}
	if config.WalletType == "" {
		return nil, errors.New("wallet type is required")
	}
	if config.EncryptionKey != nil && len(config.EncryptionKey) != 32 {
		return nil, errors.New("encryption key must be 32 bytes or nil")
	}

	return &MultisigStorage{
		config: config,
	}, nil
}

// SaveMultisigWallet persists multisig wallet state to disk.
//
// Parameters:
//   - data: The multisig wallet data to save
//
// Returns:
//   - error: If serialization, encryption, or file operations fail
//
// Security:
//   - Uses AES-256-GCM if encryption key is provided
//   - Generates random nonce for each save
//   - Sets restrictive file permissions (0600)
//   - Atomic write using temporary file + rename
//
// Related: LoadMultisigWallet
func (s *MultisigStorage) SaveMultisigWallet(data *MultisigWalletData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if data == nil {
		return errors.New("cannot save nil multisig wallet data")
	}

	// Set version if not already set
	if data.Version == 0 {
		data.Version = 1
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal multisig wallet data: %w", err)
	}

	// Encrypt if key is provided
	var finalData []byte
	if s.config.EncryptionKey != nil {
		encryptedData, err := s.encrypt(jsonData)
		if err != nil {
			return fmt.Errorf("failed to encrypt multisig wallet data: %w", err)
		}
		finalData = encryptedData
	} else {
		finalData = jsonData
	}

	// Ensure directory exists
	if err := os.MkdirAll(s.config.DataDir, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Build file path
	filename := fmt.Sprintf("multisig_%s.dat", s.config.WalletType)
	filePath := filepath.Join(s.config.DataDir, filename)

	// Atomic write: write to temp file, then rename
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, finalData, 0o600); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) // Clean up temp file on error
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// LoadMultisigWallet loads multisig wallet state from disk.
//
// Parameters:
//   - None
//
// Returns:
//   - *MultisigWalletData: The loaded multisig wallet data
//   - error: If file not found, decryption fails, or data is corrupt
//
// Security:
//   - Validates encryption key if data is encrypted
//   - Validates JSON schema version
//   - Returns error if file is tampered with (GCM authentication)
//
// Related: SaveMultisigWallet
func (s *MultisigStorage) LoadMultisigWallet() (*MultisigWalletData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build file path
	filename := fmt.Sprintf("multisig_%s.dat", s.config.WalletType)
	filePath := filepath.Join(s.config.DataDir, filename)

	// Read file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("multisig wallet file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to read multisig wallet file: %w", err)
	}

	// Decrypt if key is provided
	var jsonData []byte
	if s.config.EncryptionKey != nil {
		decryptedData, err := s.decrypt(fileData)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt multisig wallet data: %w", err)
		}
		jsonData = decryptedData
	} else {
		jsonData = fileData
	}

	// Unmarshal JSON
	var data MultisigWalletData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal multisig wallet data: %w", err)
	}

	// Validate version (forward compatibility check)
	if data.Version > 1 {
		return nil, fmt.Errorf("unsupported multisig wallet version: %d (current version: 1)", data.Version)
	}

	return &data, nil
}

// DeleteMultisigWallet removes multisig wallet data from disk.
//
// Parameters:
//   - None
//
// Returns:
//   - error: If file operations fail
//
// Security:
//   - Does not overwrite file contents (use secure deletion tools if needed)
//
// Related: SaveMultisigWallet
func (s *MultisigStorage) DeleteMultisigWallet() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := fmt.Sprintf("multisig_%s.dat", s.config.WalletType)
	filePath := filepath.Join(s.config.DataDir, filename)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted, not an error
		}
		return fmt.Errorf("failed to delete multisig wallet file: %w", err)
	}

	return nil
}

// MultisigWalletExists checks if a multisig wallet file exists on disk.
//
// Parameters:
//   - None
//
// Returns:
//   - bool: True if the wallet file exists
//   - error: If filesystem access fails (not if file doesn't exist)
//
// Related: LoadMultisigWallet
func (s *MultisigStorage) MultisigWalletExists() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := fmt.Sprintf("multisig_%s.dat", s.config.WalletType)
	filePath := filepath.Join(s.config.DataDir, filename)

	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat multisig wallet file: %w", err)
	}

	return true, nil
}

// encrypt encrypts data using AES-256-GCM with the configured encryption key.
func (s *MultisigStorage) encrypt(plaintext []byte) ([]byte, error) {
	if s.config.EncryptionKey == nil {
		return nil, errors.New("encryption key not configured")
	}

	block, err := aes.NewCipher(s.config.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Return nonce || ciphertext
	return append(nonce, ciphertext...), nil
}

// decrypt decrypts data using AES-256-GCM with the configured encryption key.
func (s *MultisigStorage) decrypt(data []byte) ([]byte, error) {
	if s.config.EncryptionKey == nil {
		return nil, errors.New("encryption key not configured")
	}

	if len(data) < 12 {
		return nil, errors.New("encrypted data too short (missing nonce)")
	}

	block, err := aes.NewCipher(s.config.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce and ciphertext
	nonce := data[:12]
	ciphertext := data[12:]

	// Decrypt and authenticate
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key or tampered data): %w", err)
	}

	return plaintext, nil
}
