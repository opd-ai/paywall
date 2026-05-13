package paywall

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opd-ai/paywall/wallet"
)

// EncryptedFileStore extends FileStore with encryption capabilities
type EncryptedFileStore struct {
	*FileStore // embed the FileStore
	keyPath    string
	key        []byte
	gcm        cipher.AEAD
}

// NewEncryptedFileStore creates a new encrypted filesystem-based payment store
func NewEncryptedFileStore(keyPath, base string) (*EncryptedFileStore, error) {
	if keyPath == "" {
		keyPath = "./keys/store.key"
	}

	// Ensure key directory exists
	keyDir := filepath.Dir(keyPath)
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}

	// Load or generate key
	key, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("key setup: %w", err)
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	return &EncryptedFileStore{
		FileStore: NewFileStore(base), // use existing FileStore implementation
		keyPath:   keyPath,
		key:       key,
		gcm:       gcm,
	}, nil
}

func loadOrGenerateKey(keyPath string) ([]byte, error) {
	// Try to load existing key
	key, err := os.ReadFile(keyPath)
	if err == nil && len(key) >= 32 {
		return key[:32], nil
	}

	// Generate new key
	key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	// Save key
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, fmt.Errorf("save key: %w", err)
	}

	return key, nil
}

func (m *EncryptedFileStore) encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, m.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return m.gcm.Seal(nonce, nonce, data, nil), nil
}

func (m *EncryptedFileStore) decrypt(data []byte) ([]byte, error) {
	nonceSize := m.gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return m.gcm.Open(nil, nonce, ciphertext, nil)
}

// writeEncryptedPayment is a helper that marshals, encrypts, and writes a payment to disk.
// Must be called with the mutex held.
func (m *EncryptedFileStore) writeEncryptedPayment(p *Payment) error {
	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payment: %w", err)
	}

	encrypted, err := m.encrypt(data)
	if err != nil {
		return fmt.Errorf("encrypt payment: %w", err)
	}

	filename := filepath.Join(m.baseDir, p.ID+".enc")
	return os.WriteFile(filename, encrypted, 0o600)
}

// CreatePayment stores an encrypted payment record
func (m *EncryptedFileStore) CreatePayment(p *Payment) error {
	// Use the embedded FileStore's mutex
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeEncryptedPayment(p)
}

// GetPayment retrieves and decrypts a payment record
func (m *EncryptedFileStore) GetPayment(id string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filename := filepath.Join(m.baseDir, id+".enc")
	encrypted, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	data, err := m.decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt payment: %w", err)
	}

	var payment Payment
	if err := json.Unmarshal(data, &payment); err != nil {
		return nil, fmt.Errorf("unmarshal payment: %w", err)
	}

	return &payment, nil
}

// UpdatePayment updates an encrypted payment record
func (m *EncryptedFileStore) UpdatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeEncryptedPayment(p)
}

// readAndDecryptPayment is a helper that reads, decrypts, and unmarshals a payment file.
// Returns (nil, nil) if the file has the wrong extension.
// Returns (nil, error) for read errors, decryption errors, or unmarshal errors.
// Must be called with the mutex held.
func (m *EncryptedFileStore) readAndDecryptPayment(filename string) (*Payment, error) {
	if filepath.Ext(filename) != ".enc" {
		return nil, nil
	}

	encrypted, err := os.ReadFile(filepath.Join(m.baseDir, filename))
	if err != nil {
		return nil, err
	}

	data, err := m.decrypt(encrypted)
	if err != nil {
		return nil, err
	}

	var payment Payment
	if err := json.Unmarshal(data, &payment); err != nil {
		return nil, err
	}

	return &payment, nil
}

// ListPendingPayments returns all encrypted payment records with less than 1 confirmation
func (m *EncryptedFileStore) ListPendingPayments() ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	var payments []*Payment
	for _, file := range files {
		payment, err := m.readAndDecryptPayment(file.Name())
		if err != nil || payment == nil {
			continue
		}

		if payment.Confirmations < 1 {
			payments = append(payments, payment)
		}
	}

	return payments, nil
}

// GetPaymentByAddress retrieves an encrypted payment record by Bitcoin address
func (m *EncryptedFileStore) GetPaymentByAddress(addr string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		payment, err := m.readAndDecryptPayment(file.Name())
		if err != nil || payment == nil {
			continue
		}

		if addr != "" {
			if payment.Addresses[wallet.Bitcoin] == addr {
				return payment, nil
			}
			if payment.Addresses[wallet.Monero] == addr {
				return payment, nil
			}
		}
	}

	return nil, nil
}

// GetPendingMultisigPayments returns all pending payments that have multisig enabled.
//
// Returns:
//   - []*Payment: Slice of pending multisig payments
//   - error: Directory read or decryption errors
func (m *EncryptedFileStore) GetPendingMultisigPayments() ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	var payments []*Payment
	for _, file := range files {
		payment, err := m.readAndDecryptPayment(file.Name())
		if err != nil || payment == nil {
			continue
		}

		if payment.MultisigEnabled && payment.Status == StatusPending {
			payments = append(payments, payment)
		}
	}

	return payments, nil
}

// GetPaymentsByMultisigAddress finds payments by multisig address.
//
// Parameters:
//   - address: The multisig address to search for
//
// Returns:
//   - []*Payment: Slice of payments associated with the address
//   - error: Directory read or decryption errors
func (m *EncryptedFileStore) GetPaymentsByMultisigAddress(address string) ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	var payments []*Payment
	for _, file := range files {
		payment, err := m.readAndDecryptPayment(file.Name())
		if err != nil || payment == nil {
			continue
		}

		if !payment.MultisigEnabled {
			continue
		}

		// Check if any wallet address matches
		for _, addr := range payment.Addresses {
			if addr == address {
				payments = append(payments, payment)
				break
			}
		}
	}

	return payments, nil
}
