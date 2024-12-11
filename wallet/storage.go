// Package wallet implements secure storage functionality for HD wallets.
package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
)

// StorageConfig defines configuration parameters for wallet storage operations.
//
// Fields:
//   - DataDir: Directory path where wallet files will be stored
//   - EncryptionKey: 32-byte key used for AES-256 encryption
//
// Security:
//   - DataDir should have appropriate filesystem permissions
//   - EncryptionKey must be securely generated and stored
type StorageConfig struct {
	DataDir       string
	EncryptionKey []byte // 32-byte key for AES-256
}

// SaveToFile encrypts and saves the wallet to a file.
//
// Parameters:
//   - config: StorageConfig containing storage location and encryption key
//
// Returns:
//   - error: If encryption fails or file operations fail
//
// Security:
//   - Uses AES-256-GCM for encryption
//   - Generates random nonce for each save
//   - Sets restrictive file permissions (0600)
//
// Related: LoadFromFile
func (w *HDWallet) SaveToFile(config StorageConfig) error {
	if len(config.EncryptionKey) != 32 {
		return errors.New("encryption key must be 32 bytes")
	}

	// Prepare wallet data for encryption
	data := make([]byte, len(w.masterKey)+len(w.chainCode)+4)
	copy(data, w.masterKey)
	copy(data[len(w.masterKey):], w.chainCode)
	binary.BigEndian.PutUint32(data[len(w.masterKey)+len(w.chainCode):], w.nextIndex)

	// Create AES cipher
	block, err := aes.NewCipher(config.EncryptionKey)
	if err != nil {
		return err
	}

	// Generate nonce
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	// Create GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Encrypt the data
	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Combine nonce and ciphertext
	finalData := append(nonce, ciphertext...)

	// Ensure directory exists
	if err := os.MkdirAll(config.DataDir, 0o700); err != nil {
		return err
	}

	// Write to file
	filePath := filepath.Join(config.DataDir, "wallet.dat")
	return os.WriteFile(filePath, finalData, 0o600)
}

// LoadFromFile loads and decrypts a wallet from a file.
//
// Parameters:
//   - config: StorageConfig containing storage location and encryption key
//
// Returns:
//   - *HDWallet: Decrypted wallet instance
//   - error: If decryption fails, file is corrupt, or file operations fail
//
// Security:
//   - Validates data integrity using AES-GCM authentication
//   - Verifies minimum data length requirements
//   - Returns errors for any decryption failures
//
// Related: SaveToFile
func LoadFromFile(config StorageConfig) (*HDWallet, error) {
	if len(config.EncryptionKey) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	// Read encrypted data
	filePath := filepath.Join(config.DataDir, "wallet.dat")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if len(data) < 12 {
		return nil, errors.New("invalid wallet file")
	}

	// Extract nonce and ciphertext
	nonce := data[:12]
	ciphertext := data[12:]

	// Create AES cipher
	block, err := aes.NewCipher(config.EncryptionKey)
	if err != nil {
		return nil, err
	}

	// Create GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Decrypt the data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	if len(plaintext) < 68 { // 32 + 32 + 4 bytes
		return nil, errors.New("invalid wallet data")
	}

	// Reconstruct wallet
	w := &HDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams, // Default to mainnet
	}

	copy(w.masterKey, plaintext[:32])
	copy(w.chainCode, plaintext[32:64])
	w.nextIndex = binary.BigEndian.Uint32(plaintext[64:])

	return w, nil
}

// GenerateEncryptionKey creates a cryptographically secure 32-byte key
// suitable for AES-256 encryption.
//
// Returns:
//   - []byte: 32-byte random encryption key
//   - error: If secure random number generation fails
//
// Security:
//   - Uses crypto/rand for secure random number generation
//   - Generates sufficient entropy for AES-256
//
// Usage:
//
//	key, err := GenerateEncryptionKey()
//	config := StorageConfig{
//	    DataDir: "/path/to/storage",
//	    EncryptionKey: key,
//	}
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}
