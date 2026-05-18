// Package paywall implements file-based audit logging for persistent audit trails
package paywall

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileAuditLogger is a file-based implementation of AuditLogger
// that persists audit entries to disk in JSONL (JSON Lines) format.
// Each audit entry is written as a single line of JSON for easy parsing.
// This implementation provides persistent audit trails that survive restarts.
type FileAuditLogger struct {
	// filePath is the path to the audit log file
	filePath string
	// file is the open file handle
	file *os.File
	// writer is the buffered writer for efficient writes
	writer *bufio.Writer
	// mu protects concurrent writes
	mu sync.Mutex
	// entries is an in-memory cache of all entries (for GetAuditTrail query performance)
	entries map[string][]*AuditLogEntry
	// allEntries is a slice of all entries in chronological order
	allEntries []*AuditLogEntry
	// readMu protects concurrent reads
	readMu sync.RWMutex
}

// NewFileAuditLogger creates a new file-based audit logger
// The log file is created if it doesn't exist, or appended to if it exists.
// Existing entries are loaded into memory for query performance.
//
// Parameters:
//   - filePath: Path to the audit log file (directory will be created if needed)
//
// Returns:
//   - *FileAuditLogger: Initialized file audit logger
//   - error: If file cannot be created/opened or existing entries cannot be loaded
//
// Related types: AuditLogger, AuditLogEntry
func NewFileAuditLogger(filePath string) (*FileAuditLogger, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create audit log directory: %w", err)
	}

	// Open file in append mode, create if doesn't exist
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	logger := &FileAuditLogger{
		filePath:   filePath,
		file:       file,
		writer:     bufio.NewWriter(file),
		entries:    make(map[string][]*AuditLogEntry),
		allEntries: make([]*AuditLogEntry, 0),
	}

	// Load existing entries from disk
	if err := logger.loadExistingEntries(); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to load existing audit entries: %w", err)
	}

	return logger, nil
}

// loadExistingEntries reads all existing entries from the log file into memory
func (f *FileAuditLogger) loadExistingEntries() error {
	// Open file for reading
	readFile, err := os.Open(f.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // New file, no entries to load
		}
		return err
	}
	defer readFile.Close()

	scanner := bufio.NewScanner(readFile)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue // Skip empty lines
		}

		var entry AuditLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Log warning but continue - don't fail on corrupted lines
			fmt.Printf("Warning: failed to parse audit log line %d: %v\n", lineNum, err)
			continue
		}

		// Add to in-memory cache
		f.allEntries = append(f.allEntries, &entry)
		if entry.PaymentID != "" {
			f.entries[entry.PaymentID] = append(f.entries[entry.PaymentID], &entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading audit log file: %w", err)
	}

	return nil
}

// LogAction records an action in the audit trail and persists it to disk
// Implements AuditLogger.LogAction
func (f *FileAuditLogger) LogAction(entry *AuditLogEntry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("audit entry cannot be nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Generate entry ID if not provided
	if entry.ID == "" {
		idBytes := make([]byte, 16)
		if _, err := rand.Read(idBytes); err != nil {
			return "", fmt.Errorf("failed to generate entry ID: %w", err)
		}
		entry.ID = fmt.Sprintf("audit-%x", idBytes)
	}

	// Set timestamp if not provided
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	// Write to file (JSONL format: one JSON object per line)
	if _, err := f.writer.Write(jsonBytes); err != nil {
		return "", fmt.Errorf("failed to write audit entry: %w", err)
	}
	if err := f.writer.WriteByte('\n'); err != nil {
		return "", fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush to ensure persistence
	if err := f.writer.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush audit log: %w", err)
	}
	if err := f.file.Sync(); err != nil {
		return "", fmt.Errorf("failed to sync audit log: %w", err)
	}

	// Update in-memory cache
	f.readMu.Lock()
	f.allEntries = append(f.allEntries, entry)
	if entry.PaymentID != "" {
		f.entries[entry.PaymentID] = append(f.entries[entry.PaymentID], entry)
	}
	f.readMu.Unlock()

	return entry.ID, nil
}

// GetAuditTrail retrieves all audit entries for a specific payment
// Implements AuditLogger.GetAuditTrail
func (f *FileAuditLogger) GetAuditTrail(paymentID string) ([]*AuditLogEntry, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("payment ID cannot be empty")
	}

	f.readMu.RLock()
	defer f.readMu.RUnlock()

	entries, exists := f.entries[paymentID]
	if !exists {
		return []*AuditLogEntry{}, nil
	}

	// Return a copy to prevent external modification
	result := make([]*AuditLogEntry, len(entries))
	copy(result, entries)
	return result, nil
}

// GetAllEntries retrieves all audit log entries in chronological order
// Implements AuditLogger.GetAllEntries
func (f *FileAuditLogger) GetAllEntries() ([]*AuditLogEntry, error) {
	f.readMu.RLock()
	defer f.readMu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*AuditLogEntry, len(f.allEntries))
	copy(result, f.allEntries)
	return result, nil
}

// Close closes the audit log file and flushes any pending writes
// Should be called when shutting down to ensure all entries are persisted
func (f *FileAuditLogger) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.writer != nil {
		if err := f.writer.Flush(); err != nil {
			return fmt.Errorf("failed to flush audit log on close: %w", err)
		}
	}

	if f.file != nil {
		if err := f.file.Close(); err != nil {
			return fmt.Errorf("failed to close audit log file: %w", err)
		}
	}

	return nil
}
