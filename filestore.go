package paywall

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/opd-ai/paywall/wallet"
)

// FileStore implements the Store interface for filesystem-based payment tracking.
// It stores each payment as a separate JSON file in a designated directory.
// Thread-safety is ensured through a read-write mutex.
//
// Fields:
//   - baseDir: Directory path where payment files are stored
//   - mu: Mutex for thread-safe file operations
//
// Related: Store interface
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStore creates a new filesystem-based payment store instance.
// It initializes a "./payments" directory if it doesn't exist.
//
// Returns:
//   - *FileStore: New payment store configured to use "./payments" directory
//
// Error handling:
//   - Creates payments directory with 0755 permissions
//   - Silently continues if directory already exists
func NewFileStore(base string) *FileStore {
	// Create payments directory if it doesn't exist
	baseDir := base
	if baseDir == "" {
		baseDir = "./payments"
	}
	os.MkdirAll(baseDir, 0o755)
	return &FileStore{baseDir: baseDir}
}

// CreatePayment stores a new payment record as a JSON file.
// The payment ID is used as the filename.
//
// Parameters:
//   - p: Payment record to store (must not be nil and must have valid ID)
//
// Returns:
//   - error: File creation/write errors or JSON marshaling errors
//
// Thread-safety: Protected by write lock
func (m *FileStore) CreatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payment: %w", err)
	}

	filename := filepath.Join(m.baseDir, p.ID+".json")
	return os.WriteFile(filename, data, 0o600)
}

// GetPayment retrieves a payment record by ID from its JSON file.
//
// Parameters:
//   - id: Payment identifier used as filename (without .json extension)
//
// Returns:
//   - *Payment: Payment record if found, nil if not found
//   - error: File read errors or JSON unmarshaling errors
//
// Thread-safety: Protected by read lock
func (m *FileStore) GetPayment(id string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filename := filepath.Join(m.baseDir, id+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var payment Payment
	if err := json.Unmarshal(data, &payment); err != nil {
		return nil, fmt.Errorf("unmarshal payment: %w", err)
	}

	return &payment, nil
}

// UpdatePayment updates an existing payment record file.
// Creates the file if it doesn't exist.
//
// Parameters:
//   - p: Payment record with updated fields (must not be nil and must have valid ID)
//
// Returns:
//   - error: File write errors or JSON marshaling errors
//
// Thread-safety: Protected by write lock
func (m *FileStore) UpdatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payment: %w", err)
	}

	filename := filepath.Join(m.baseDir, p.ID+".json")
	return os.WriteFile(filename, data, 0o600)
}

// ListPendingPayments returns all payment records with less than 1 confirmation.
// Scans all JSON files in the storage directory.
//
// Returns:
//   - []*Payment: Slice of pending payments, empty slice if none found
//   - error: Directory read errors
//
// Notes:
//   - Silently skips non-JSON files
//   - Silently skips files with read or parse errors
//   - Thread-safety: Protected by read lock
func (m *FileStore) ListPendingPayments() ([]*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	var payments []*Payment
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.baseDir, file.Name()))
		if err != nil {
			log.Printf("Error reading file %s: %v", file.Name(), err)
			continue
		}

		var payment Payment
		if err := json.Unmarshal(data, &payment); err != nil {
			log.Printf("Error parsing file %s: %v", file.Name(), err)
			continue
		}

		if payment.Confirmations <= 1 {
			payments = append(payments, &payment)
		}
	}

	return payments, nil
}

// GetPaymentByAddress retrieves a payment record by Bitcoin address.
// Scans all payment files sequentially until a match is found.
//
// Parameters:
//   - addr: Bitcoin address to search for (case-sensitive)
//
// Returns:
//   - *Payment: Matching payment record, nil if not found
//   - error: Directory read errors
//
// Notes:
//   - Silently skips non-JSON files
//   - Silently skips files with read or parse errors
//   - Thread-safety: Protected by read lock
func (m *FileStore) GetPaymentByAddress(addr string) (*Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, err := os.ReadDir(m.baseDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.baseDir, file.Name()))
		if err != nil {
			continue
		}

		var payment Payment
		if err := json.Unmarshal(data, &payment); err != nil {
			continue
		}

		if addr != "" && payment.Addresses[wallet.Bitcoin] == addr {
			return &payment, nil
		}
		if addr != "" && payment.Addresses[wallet.Monero] == addr {
			return &payment, nil
		}
	}

	return nil, nil
}

// FileStoreConfig defines configuration parameters for file-based payment storage
//
// Fields:
//   - DataDir: Directory path where payment files will be stored
//   - EncryptionKey: Optional 32-byte key for AES-256 encryption (if nil, no encryption)
//
// Security:
//   - DataDir should have appropriate filesystem permissions (0755)
//   - EncryptionKey must be securely generated and stored if provided
//   - When EncryptionKey is provided, files are stored with AES-256-GCM encryption
type FileStoreConfig struct {
	DataDir       string
	EncryptionKey []byte // Optional: 32-byte key for AES-256 encryption
}

// NewFileStoreWithConfig creates a new filesystem-based payment store with configuration.
// If encryption key is provided, returns an EncryptedFileStore, otherwise returns a standard FileStore.
//
// Parameters:
//   - config: FileStoreConfig containing storage location and optional encryption key
//
// Returns:
//   - PaymentStore: Either *FileStore or *EncryptedFileStore depending on encryption key
//   - error: If directory creation fails or encryption setup fails
//
// Security:
//   - Creates directory with 0755 permissions
//   - Validates encryption key length (must be 32 bytes if provided)
//   - Uses AES-256-GCM encryption when key is provided
//
// Related: FileStore, EncryptedFileStore, PaymentStore interface
func NewFileStoreWithConfig(config FileStoreConfig) (PaymentStore, error) {
	// Create directory if it doesn't exist
	if config.DataDir == "" {
		config.DataDir = "./payments"
	}
	
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}

	// If encryption key provided, use encrypted store
	if config.EncryptionKey != nil {
		if len(config.EncryptionKey) != 32 {
			return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(config.EncryptionKey))
		}
		
		// For encrypted store, we need to save the key to a file
		keyPath := filepath.Join(config.DataDir, "store.key")
		return NewEncryptedFileStore(keyPath, config.DataDir)
	}

	// Use standard file store without encryption
	return NewFileStore(config.DataDir), nil
}
