package migrations

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/opd-ai/paywall"
)

// EncryptExisting handles migration of unencrypted payment files to encrypted format.
// It preserves the original files and creates encrypted versions alongside them.
func EncryptExisting(keyPath, base string) error {
	// Create encrypted store
	encStore, err := paywall.NewEncryptedFileStore(keyPath, base)
	if err != nil {
		return fmt.Errorf("create encrypted store: %w", err)
	}

	// Create unencrypted store
	plainStore := paywall.NewFileStore(base)

	// Get list of JSON files
	files, err := os.ReadDir(base)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	var processed, errors int

	// Process each file
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// Extract payment ID from filename
		id := file.Name()[:len(file.Name())-5] // remove .json

		// Skip if encrypted version already exists
		encPath := filepath.Join(base, id+".enc")
		if _, err := os.Stat(encPath); err == nil {
			log.Printf("Skipping already encrypted payment: %s", id)
			continue
		}

		// Read unencrypted payment
		payment, err := plainStore.GetPayment(id)
		if err != nil {
			log.Printf("Error reading payment %s: %v", id, err)
			errors++
			continue
		}

		// Create encrypted version
		if err := encStore.CreatePayment(payment); err != nil {
			log.Printf("Error encrypting payment %s: %v", id, err)
			errors++
			continue
		}

		processed++
		log.Printf("Encrypted payment %s", id)
	}

	log.Printf("Migration complete. Processed: %d, Errors: %d", processed, errors)
	return nil
}
