Project Path: filestore.go

I'd like you to add documentation comments to all public functions, methods, classes and modules in this codebase.

For each one, the comment should include:
1. A brief description of what it does
2. Explanations of all parameters including types/constraints 
3. Description of the return value (if applicable)
4. Any notable error or edge cases handled
5. Links to any related code entities

Do not rename or remove any code. Document the existing code only.

If possible, output a complete, documented version of the file you have been given.

Source Tree: 
```
filestore.go

```

`/home/user/go/src/github.com/opd-ai/paywall/filestore.go`:

```go
package paywall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FileStore implements Store interface for filesystem-based payment tracking.
type FileStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileStore creates a new filesystem-based payment store instance.
//
// Returns:
//   - *FileStore: New payment store initialized to use "./payments" directory
func NewFileStore() *FileStore {
	// Create payments directory if it doesn't exist
	baseDir := "./payments"
	os.MkdirAll(baseDir, 0o755)
	return &FileStore{baseDir: baseDir}
}

// CreatePayment stores a new payment record as a JSON file.
func (m *FileStore) CreatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payment: %w", err)
	}

	filename := filepath.Join(m.baseDir, p.ID+".json")
	return os.WriteFile(filename, data, 0o644)
}

// GetPayment retrieves a payment record by ID from its JSON file.
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
func (m *FileStore) UpdatePayment(p *Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal payment: %w", err)
	}

	filename := filepath.Join(m.baseDir, p.ID+".json")
	return os.WriteFile(filename, data, 0o644)
}

// ListPendingPayments returns all pending payment records.
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
			continue
		}

		var payment Payment
		if err := json.Unmarshal(data, &payment); err != nil {
			continue
		}

		if payment.Confirmations > 1 {
			payments = append(payments, &payment)
		}
	}

	return payments, nil
}

// GetPaymentByAddress retrieves a payment record by Bitcoin address.
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

		if payment.Address == addr {
			return &payment, nil
		}
	}

	return nil, nil
}

```  


I'd like you to add documentation comments to all public functions, methods, classes and modules in this codebase.

Try to keep comments concise but informative. Use the function/parameter names as clues to infer their purpose. Analyze the implementation carefully to determine behavior.

Comments should use the idiomatic style for the language, e.g. /// for Rust, """ for Python, /** */ for TypeScript, etc. Place them directly above the function/class/module definition.

Do not rename or remove any code. Document the existing code only.

If possible, output a complete, documented version of the file you have been given.

Let me know if you have any questions! And be sure to review your work for accuracy before submitting.