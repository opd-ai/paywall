package wallet

import (
	"crypto/rand"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestBTCHDWallet_GetTransactionConfirmations tests the newly implemented GetTransactionConfirmations method
func TestBTCHDWallet_GetTransactionConfirmations(t *testing.T) {
	// Create test wallet
	seed := make([]byte, 32)
	rand.Read(seed)
	
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
		nextIndex: 0,
		minConf:   6, // Set minimum confirmations for testing
	}
	copy(wallet.masterKey, seed)
	copy(wallet.chainCode, seed)

	t.Run("ValidTransactionID", func(t *testing.T) {
		// Valid 64-character hex transaction ID
		validTxID := "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"
		
		confirmations, err := wallet.GetTransactionConfirmations(validTxID)
		
		// Should return error since no RPC client is available
		if err == nil {
			t.Error("Expected error when no RPC client available")
		}
		
		if confirmations != 0 {
			t.Errorf("Expected 0 confirmations, got %d", confirmations)
		}
		
		expectedErr := "no RPC client available for transaction confirmation"
		if err.Error() != expectedErr {
			t.Errorf("Error message = %q, expected %q", err.Error(), expectedErr)
		}
	})

	t.Run("InvalidTransactionIDLength", func(t *testing.T) {
		testCases := []struct {
			name  string
			txID  string
			valid bool
		}{
			{
				name:  "Too short",
				txID:  "abcd1234",
				valid: false,
			},
			{
				name:  "Too long",
				txID:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				valid: false,
			},
			{
				name:  "Empty",
				txID:  "",
				valid: false,
			},
			{
				name:  "Exactly 64 chars",
				txID:  "abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
				valid: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				confirmations, err := wallet.GetTransactionConfirmations(tc.txID)
				
				if tc.valid {
					// Should fail due to no RPC client, not invalid format
					if err == nil {
						t.Error("Expected error when no RPC client available")
					}
					expectedErr := "no RPC client available for transaction confirmation"
					if err.Error() != expectedErr {
						t.Errorf("Error message = %q, expected %q", err.Error(), expectedErr)
					}
				} else {
					// Should fail due to invalid transaction ID format
					if err == nil {
						t.Error("Expected error for invalid transaction ID")
					}
					if confirmations != 0 {
						t.Errorf("Expected 0 confirmations for invalid ID, got %d", confirmations)
					}
				}
			})
		}
	})

	t.Run("InterfaceCompliance", func(t *testing.T) {
		// Verify that BTCHDWallet still implements HDWallet interface
		var _ HDWallet = wallet
		
		// Test that the method exists and can be called
		_, err := wallet.GetTransactionConfirmations("abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab")
		if err == nil {
			t.Error("Expected error when no RPC client available")
		}
	})
}
