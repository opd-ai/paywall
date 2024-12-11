// wallet/storage.go
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

// StorageConfig holds configuration for wallet storage
type StorageConfig struct {
	DataDir       string
	EncryptionKey []byte // 32-byte key for AES-256
}

// SaveToFile securely saves the wallet to a file
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
	if err := os.MkdirAll(config.DataDir, 0700); err != nil {
		return err
	}

	// Write to file
	filePath := filepath.Join(config.DataDir, "wallet.dat")
	return os.WriteFile(filePath, finalData, 0600)
}

// LoadFromFile loads a wallet from an encrypted file
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

// Helper function to generate a secure encryption key
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}
