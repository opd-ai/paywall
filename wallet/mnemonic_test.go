package wallet

import (
	"strings"
	"testing"
)

func TestGenerateMnemonic_12Words(t *testing.T) {
	mnemonic, err := GenerateMnemonic(Mnemonic12Words)
	if err != nil {
		t.Fatalf("GenerateMnemonic(12 words) failed: %v", err)
	}

	words := strings.Fields(mnemonic)
	if len(words) != 12 {
		t.Errorf("Expected 12 words, got %d", len(words))
	}

	if !ValidateMnemonic(mnemonic) {
		t.Errorf("Generated mnemonic is invalid: %s", mnemonic)
	}
}

func TestGenerateMnemonic_24Words(t *testing.T) {
	mnemonic, err := GenerateMnemonic(Mnemonic24Words)
	if err != nil {
		t.Fatalf("GenerateMnemonic(24 words) failed: %v", err)
	}

	words := strings.Fields(mnemonic)
	if len(words) != 24 {
		t.Errorf("Expected 24 words, got %d", len(words))
	}

	if !ValidateMnemonic(mnemonic) {
		t.Errorf("Generated mnemonic is invalid: %s", mnemonic)
	}
}

func TestGenerateMnemonic_InvalidStrength(t *testing.T) {
	_, err := GenerateMnemonic(MnemonicStrength(999))
	if err == nil {
		t.Error("Expected error for invalid strength, got nil")
	}
}

func TestGenerateMnemonic_Uniqueness(t *testing.T) {
	mnemonic1, err := GenerateMnemonic(Mnemonic24Words)
	if err != nil {
		t.Fatalf("First generation failed: %v", err)
	}

	mnemonic2, err := GenerateMnemonic(Mnemonic24Words)
	if err != nil {
		t.Fatalf("Second generation failed: %v", err)
	}

	if mnemonic1 == mnemonic2 {
		t.Error("Generated mnemonics are not unique (extremely unlikely, possible security issue)")
	}
}

func TestImportFromMnemonic_ValidMnemonic(t *testing.T) {
	// Test vector from BIP39 specification
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	seed, err := ImportFromMnemonic(mnemonic, "")
	if err != nil {
		t.Fatalf("ImportFromMnemonic failed: %v", err)
	}

	if len(seed) != 64 {
		t.Errorf("Expected 64-byte seed, got %d bytes", len(seed))
	}

	// Verify seed is deterministic (same mnemonic → same seed)
	seed2, err := ImportFromMnemonic(mnemonic, "")
	if err != nil {
		t.Fatalf("Second import failed: %v", err)
	}

	if string(seed) != string(seed2) {
		t.Error("Same mnemonic produced different seeds (not deterministic)")
	}
}

func TestImportFromMnemonic_WithPassphrase(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	seedNoPass, err := ImportFromMnemonic(mnemonic, "")
	if err != nil {
		t.Fatalf("Import without passphrase failed: %v", err)
	}

	seedWithPass, err := ImportFromMnemonic(mnemonic, "mypassphrase")
	if err != nil {
		t.Fatalf("Import with passphrase failed: %v", err)
	}

	if string(seedNoPass) == string(seedWithPass) {
		t.Error("Passphrase did not affect seed generation")
	}
}

func TestImportFromMnemonic_InvalidMnemonic(t *testing.T) {
	testCases := []struct {
		name     string
		mnemonic string
	}{
		{
			name:     "Invalid word",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon invalid",
		},
		{
			name:     "Wrong checksum",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon",
		},
		{
			name:     "Too few words",
			mnemonic: "abandon abandon abandon",
		},
		{
			name:     "Empty mnemonic",
			mnemonic: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportFromMnemonic(tc.mnemonic, "")
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestValidateMnemonic(t *testing.T) {
	testCases := []struct {
		name     string
		mnemonic string
		valid    bool
	}{
		{
			name:     "Valid 12-word mnemonic",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
			valid:    true,
		},
		{
			name:     "Valid 24-word mnemonic",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art",
			valid:    true,
		},
		{
			name:     "Invalid word",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon xyz",
			valid:    false,
		},
		{
			name:     "Wrong checksum",
			mnemonic: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon",
			valid:    false,
		},
		{
			name:     "Empty string",
			mnemonic: "",
			valid:    false,
		},
		{
			name:     "Too few words",
			mnemonic: "abandon abandon abandon",
			valid:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			valid := ValidateMnemonic(tc.mnemonic)
			if valid != tc.valid {
				t.Errorf("ValidateMnemonic(%s) = %v, want %v", tc.name, valid, tc.valid)
			}
		})
	}
}

func TestMnemonicToSeed(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	seed1, err := MnemonicToSeed(mnemonic)
	if err != nil {
		t.Fatalf("MnemonicToSeed failed: %v", err)
	}

	seed2, err := ImportFromMnemonic(mnemonic, "")
	if err != nil {
		t.Fatalf("ImportFromMnemonic failed: %v", err)
	}

	if string(seed1) != string(seed2) {
		t.Error("MnemonicToSeed and ImportFromMnemonic produced different results")
	}
}

func TestNewBTCHDWalletFromMnemonic(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	wallet, err := NewBTCHDWalletFromMnemonic(mnemonic, "", true, 1)
	if err != nil {
		t.Fatalf("NewBTCHDWalletFromMnemonic failed: %v", err)
	}

	if wallet == nil {
		t.Fatal("Wallet is nil")
	}

	// Verify wallet can derive addresses
	addr1, err := wallet.DeriveNextAddress()
	if err != nil {
		t.Fatalf("DeriveNextAddress failed: %v", err)
	}

	if addr1 == "" {
		t.Error("Derived address is empty")
	}

	// Verify determinism: same mnemonic should produce same addresses
	wallet2, err := NewBTCHDWalletFromMnemonic(mnemonic, "", true, 1)
	if err != nil {
		t.Fatalf("Second wallet creation failed: %v", err)
	}

	addr2, err := wallet2.DeriveNextAddress()
	if err != nil {
		t.Fatalf("Second address derivation failed: %v", err)
	}

	if addr1 != addr2 {
		t.Errorf("Same mnemonic produced different addresses: %s vs %s", addr1, addr2)
	}
}

func TestNewBTCHDWalletFromMnemonic_InvalidMnemonic(t *testing.T) {
	_, err := NewBTCHDWalletFromMnemonic("invalid mnemonic phrase", "", true, 1)
	if err == nil {
		t.Error("Expected error for invalid mnemonic, got nil")
	}
}

func TestNewBTCHDWalletFromMnemonic_WithPassphrase(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	wallet1, err := NewBTCHDWalletFromMnemonic(mnemonic, "", true, 1)
	if err != nil {
		t.Fatalf("Wallet without passphrase failed: %v", err)
	}

	wallet2, err := NewBTCHDWalletFromMnemonic(mnemonic, "mypassphrase", true, 1)
	if err != nil {
		t.Fatalf("Wallet with passphrase failed: %v", err)
	}

	addr1, _ := wallet1.DeriveNextAddress()
	addr2, _ := wallet2.DeriveNextAddress()

	if addr1 == addr2 {
		t.Error("Passphrase did not affect wallet address derivation")
	}
}

func TestMnemonic_RoundTrip(t *testing.T) {
	// Generate → Import → Derive should work
	mnemonic, err := GenerateMnemonic(Mnemonic24Words)
	if err != nil {
		t.Fatalf("GenerateMnemonic failed: %v", err)
	}

	wallet, err := NewBTCHDWalletFromMnemonic(mnemonic, "", true, 1)
	if err != nil {
		t.Fatalf("NewBTCHDWalletFromMnemonic failed: %v", err)
	}

	addr, err := wallet.DeriveNextAddress()
	if err != nil {
		t.Fatalf("DeriveNextAddress failed: %v", err)
	}

	if addr == "" {
		t.Error("Derived address is empty")
	}

	// Verify we can recreate same wallet
	wallet2, err := NewBTCHDWalletFromMnemonic(mnemonic, "", true, 1)
	if err != nil {
		t.Fatalf("Wallet recreation failed: %v", err)
	}

	addr2, err := wallet2.DeriveNextAddress()
	if err != nil {
		t.Fatalf("Second derivation failed: %v", err)
	}

	if addr != addr2 {
		t.Errorf("Recreated wallet produced different address: %s vs %s", addr, addr2)
	}
}

func TestMnemonic_WhitespaceHandling(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Test with extra whitespace
	mnemonicWithSpaces := "  abandon   abandon  abandon abandon abandon abandon abandon abandon abandon abandon abandon about  "

	seed1, err := ImportFromMnemonic(mnemonic, "")
	if err != nil {
		t.Fatalf("Import of clean mnemonic failed: %v", err)
	}

	seed2, err := ImportFromMnemonic(mnemonicWithSpaces, "")
	if err != nil {
		t.Fatalf("Import of mnemonic with spaces failed: %v", err)
	}

	if string(seed1) != string(seed2) {
		t.Error("Whitespace handling produced different seeds")
	}
}
