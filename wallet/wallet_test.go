package wallet

import (
	"testing"
)

func TestNewWallet(t *testing.T) {
	// Test mainnet wallet creation
	mainnetWallet, err := NewWallet(false)
	if err != nil {
		t.Fatalf("Failed to create mainnet wallet: %v", err)
	}
	if len(mainnetWallet.Address) == 0 {
		t.Error("Mainnet wallet address is empty")
	}
	if mainnetWallet.Address[0] != '1' {
		t.Error("Mainnet address should start with '1'")
	}

	// Test testnet wallet creation
	testnetWallet, err := NewWallet(true)
	if err != nil {
		t.Fatalf("Failed to create testnet wallet: %v", err)
	}
	if len(testnetWallet.Address) == 0 {
		t.Error("Testnet wallet address is empty")
	}
	if testnetWallet.Address[0] != 'm' && testnetWallet.Address[0] != 'n' {
		t.Error("Testnet address should start with 'm' or 'n'")
	}

	// Test message signing and verification
	message := []byte("Hello, Bitcoin!")
	signature, err := mainnetWallet.SignMessage(message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	valid, err := mainnetWallet.VerifyMessage(message, signature)
	if err != nil {
		t.Fatalf("Failed to verify message: %v", err)
	}
	if !valid {
		t.Error("Signature verification failed")
	}
}
