package wallet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewMultisigStorage(t *testing.T) {
	tests := []struct {
		name    string
		config  MultisigStorageConfig
		wantErr bool
	}{
		{
			name: "valid config without encryption",
			config: MultisigStorageConfig{
				DataDir:    "/tmp/test",
				WalletType: Bitcoin,
			},
			wantErr: false,
		},
		{
			name: "valid config with encryption",
			config: MultisigStorageConfig{
				DataDir:       "/tmp/test",
				EncryptionKey: make([]byte, 32),
				WalletType:    Bitcoin,
			},
			wantErr: false,
		},
		{
			name: "missing data directory",
			config: MultisigStorageConfig{
				WalletType: Bitcoin,
			},
			wantErr: true,
		},
		{
			name: "missing wallet type",
			config: MultisigStorageConfig{
				DataDir: "/tmp/test",
			},
			wantErr: true,
		},
		{
			name: "invalid encryption key length",
			config: MultisigStorageConfig{
				DataDir:       "/tmp/test",
				EncryptionKey: make([]byte, 16), // Wrong size
				WalletType:    Bitcoin,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewMultisigStorage(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMultisigStorage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && storage == nil {
				t.Error("expected non-nil storage when no error")
			}
		})
	}
}

func TestMultisigStorage_SaveAndLoad(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		encryptionKey []byte
		data          *MultisigWalletData
		wantErr       bool
	}{
		{
			name:          "save and load unencrypted",
			encryptionKey: nil,
			data: &MultisigWalletData{
				WalletType: Bitcoin,
				Config: &MultisigConfig{
					Enabled:      true,
					RequiredSigs: 2,
					TotalSigners: 3,
					PublicKeys: [][]byte{
						[]byte("pubkey1"),
						[]byte("pubkey2"),
						[]byte("pubkey3"),
					},
				},
				Addresses: map[string]*MultisigMetadata{
					"3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC": {
						Address:      "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC",
						RedeemScript: []byte("redeem-script"),
						RequiredSigs: 2,
					},
				},
				Version: 1,
			},
			wantErr: false,
		},
		{
			name:          "save and load encrypted",
			encryptionKey: make([]byte, 32), // Will be filled with zeros, but valid
			data: &MultisigWalletData{
				WalletType: Bitcoin,
				Config: &MultisigConfig{
					Enabled:      true,
					RequiredSigs: 2,
					TotalSigners: 3,
				},
				Addresses: map[string]*MultisigMetadata{},
				Version:   1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create storage
			storage, err := NewMultisigStorage(MultisigStorageConfig{
				DataDir:       tempDir,
				EncryptionKey: tt.encryptionKey,
				WalletType:    Bitcoin,
			})
			if err != nil {
				t.Fatalf("failed to create storage: %v", err)
			}

			// Check exists before save
			exists, err := storage.MultisigWalletExists()
			if err != nil {
				t.Errorf("MultisigWalletExists() before save error = %v", err)
			}
			if exists {
				t.Error("wallet should not exist before save")
			}

			// Save
			err = storage.SaveMultisigWallet(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveMultisigWallet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Check exists after save
			exists, err = storage.MultisigWalletExists()
			if err != nil {
				t.Errorf("MultisigWalletExists() after save error = %v", err)
			}
			if !exists {
				t.Error("wallet should exist after save")
			}

			// Load
			loadedData, err := storage.LoadMultisigWallet()
			if err != nil {
				t.Errorf("LoadMultisigWallet() error = %v", err)
				return
			}

			// Verify data
			if loadedData.WalletType != tt.data.WalletType {
				t.Errorf("WalletType = %v, want %v", loadedData.WalletType, tt.data.WalletType)
			}
			if loadedData.Config.RequiredSigs != tt.data.Config.RequiredSigs {
				t.Errorf("RequiredSigs = %v, want %v", loadedData.Config.RequiredSigs, tt.data.Config.RequiredSigs)
			}
			if len(loadedData.Addresses) != len(tt.data.Addresses) {
				t.Errorf("Addresses count = %v, want %v", len(loadedData.Addresses), len(tt.data.Addresses))
			}

			// Clean up
			err = storage.DeleteMultisigWallet()
			if err != nil {
				t.Errorf("DeleteMultisigWallet() error = %v", err)
			}

			// Verify deleted
			exists, err = storage.MultisigWalletExists()
			if err != nil {
				t.Errorf("MultisigWalletExists() after delete error = %v", err)
			}
			if exists {
				t.Error("wallet should not exist after delete")
			}
		})
	}
}

func TestMultisigStorage_SaveNilData(t *testing.T) {
	tempDir := t.TempDir()

	storage, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:    tempDir,
		WalletType: Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	err = storage.SaveMultisigWallet(nil)
	if err == nil {
		t.Error("SaveMultisigWallet(nil) should return error")
	}
}

func TestMultisigStorage_LoadNonExistent(t *testing.T) {
	tempDir := t.TempDir()

	storage, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:    tempDir,
		WalletType: Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	_, err = storage.LoadMultisigWallet()
	if err == nil {
		t.Error("LoadMultisigWallet() should return error for non-existent file")
	}
}

func TestMultisigStorage_EncryptionKeyMismatch(t *testing.T) {
	tempDir := t.TempDir()

	// Save with one key
	key1 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i)
	}

	storage1, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:       tempDir,
		EncryptionKey: key1,
		WalletType:    Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	data := &MultisigWalletData{
		WalletType: Bitcoin,
		Config:     &MultisigConfig{Enabled: true},
		Addresses:  map[string]*MultisigMetadata{},
		Version:    1,
	}

	err = storage1.SaveMultisigWallet(data)
	if err != nil {
		t.Fatalf("SaveMultisigWallet() error = %v", err)
	}

	// Try to load with different key
	key2 := make([]byte, 32)
	for i := range key2 {
		key2[i] = byte(i + 1)
	}

	storage2, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:       tempDir,
		EncryptionKey: key2,
		WalletType:    Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	_, err = storage2.LoadMultisigWallet()
	if err == nil {
		t.Error("LoadMultisigWallet() should fail with wrong encryption key")
	}
}

func TestMultisigStorage_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()

	storage, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:    tempDir,
		WalletType: Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	data := &MultisigWalletData{
		WalletType: Bitcoin,
		Config:     &MultisigConfig{Enabled: true},
		Addresses:  map[string]*MultisigMetadata{},
		Version:    1,
	}

	// Save wallet
	err = storage.SaveMultisigWallet(data)
	if err != nil {
		t.Fatalf("SaveMultisigWallet() error = %v", err)
	}

	// Verify temp file is cleaned up
	tempFile := filepath.Join(tempDir, "multisig_BTC.dat.tmp")
	if _, err := os.Stat(tempFile); err == nil {
		t.Error("temporary file should be cleaned up after save")
	}

	// Verify actual file exists
	actualFile := filepath.Join(tempDir, "multisig_BTC.dat")
	if _, err := os.Stat(actualFile); err != nil {
		t.Errorf("wallet file should exist: %v", err)
	}
}

func TestMultisigStorage_DeleteNonExistent(t *testing.T) {
	tempDir := t.TempDir()

	storage, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:    tempDir,
		WalletType: Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Deleting non-existent file should not error
	err = storage.DeleteMultisigWallet()
	if err != nil {
		t.Errorf("DeleteMultisigWallet() on non-existent file should not error: %v", err)
	}
}

func TestMultisigStorage_VersionValidation(t *testing.T) {
	tempDir := t.TempDir()

	storage, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:    tempDir,
		WalletType: Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Create data with future version
	data := &MultisigWalletData{
		WalletType: Bitcoin,
		Config:     &MultisigConfig{Enabled: true},
		Addresses:  map[string]*MultisigMetadata{},
		Version:    999, // Future version
	}

	// Save should succeed (we save whatever version is set)
	err = storage.SaveMultisigWallet(data)
	if err != nil {
		t.Fatalf("SaveMultisigWallet() should save any version: %v", err)
	}

	// Load should fail due to unsupported version
	_, err = storage.LoadMultisigWallet()
	if err == nil {
		t.Error("LoadMultisigWallet() should fail with future version")
	}
}

func TestMultisigStorage_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()

	storage, err := NewMultisigStorage(MultisigStorageConfig{
		DataDir:    tempDir,
		WalletType: Bitcoin,
	})
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	data := &MultisigWalletData{
		WalletType: Bitcoin,
		Config:     &MultisigConfig{Enabled: true},
		Addresses:  map[string]*MultisigMetadata{},
		Version:    1,
	}

	// Save initial data
	err = storage.SaveMultisigWallet(data)
	if err != nil {
		t.Fatalf("SaveMultisigWallet() error = %v", err)
	}

	// Run concurrent reads and writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			// Try to load
			_, err := storage.LoadMultisigWallet()
			if err != nil {
				t.Errorf("concurrent LoadMultisigWallet() error = %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
