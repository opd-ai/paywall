// Package paywall implements tests for Monero transaction broadcasting
package paywall

import (
	"testing"

	"github.com/opd-ai/paywall/wallet"
)

// TestXMRBroadcaster_NewXMRBroadcaster tests Monero broadcaster creation
func TestXMRBroadcaster_NewXMRBroadcaster(t *testing.T) {
	tests := []struct {
		name    string
		rpcURL  string
		rpcUser string
		rpcPass string
		wantErr bool
	}{
		{
			name:    "missing rpc url",
			rpcURL:  "",
			rpcUser: "user",
			rpcPass: "pass",
			wantErr: true,
		},
		{
			name:    "valid config but unreachable",
			rpcURL:  "http://localhost:18082",
			rpcUser: "user",
			rpcPass: "pass",
			wantErr: true, // Will fail because no actual RPC server running
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if testing.Short() && tt.rpcURL != "" {
				t.Skip("skipping Monero RPC connectivity test in short mode")
			}

			_, err := NewXMRBroadcaster(tt.rpcURL, tt.rpcUser, tt.rpcPass)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewXMRBroadcaster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestXMRBroadcaster_ValidateTransaction tests Monero transaction validation
func TestXMRBroadcaster_ValidateTransaction(t *testing.T) {
	// Create a mock broadcaster (won't actually connect to RPC for validation tests)
	broadcaster := &XMRBroadcaster{
		client: nil, // We won't use the client for validation
	}

	// Create test payment with Monero configuration
	payment := &Payment{
		ID: "test-payment",
		Addresses: map[wallet.WalletType]string{
			wallet.Monero: "4AdUndXHHZ6cfufTMvppY6JwXNouMBzSkbLYfpAV5Usx3skxNgYeYTRj5UzqtReoS44qo9mtmXCqY45DJ852K5Jv2684Rge",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Monero: 0.1,
		},
		MultisigEnabled: true,
	}

	tests := []struct {
		name    string
		txHex   string
		payment *Payment
		wantErr bool
	}{
		{
			name:    "valid transaction hex",
			txHex:   "0123456789abcdef",
			payment: payment,
			wantErr: false,
		},
		{
			name:    "empty transaction hex",
			txHex:   "",
			payment: payment,
			wantErr: true,
		},
		{
			name:    "nil payment",
			txHex:   "0123456789abcdef",
			payment: nil,
			wantErr: true,
		},
		{
			name:  "payment without monero address",
			txHex: "0123456789abcdef",
			payment: &Payment{
				ID: "test",
				Addresses: map[wallet.WalletType]string{
					wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
				},
				Amounts: map[wallet.WalletType]float64{
					wallet.Monero: 0.1,
				},
			},
			wantErr: true,
		},
		{
			name:  "payment without monero amount",
			txHex: "0123456789abcdef",
			payment: &Payment{
				ID: "test",
				Addresses: map[wallet.WalletType]string{
					wallet.Monero: "4AdUndXHHZ6cfufTMvppY6JwXNouMBzSkbLYfpAV5Usx3skxNgYeYTRj5UzqtReoS44qo9mtmXCqY45DJ852K5Jv2684Rge",
				},
				Amounts: map[wallet.WalletType]float64{
					wallet.Bitcoin: 0.001,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := broadcaster.ValidateTransaction(tt.txHex, tt.payment)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransaction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestXMRBroadcaster_Broadcast_EmptyTx tests broadcasting with empty transaction
func TestXMRBroadcaster_Broadcast_EmptyTx(t *testing.T) {
	broadcaster := &XMRBroadcaster{
		client: nil,
	}

	_, err := broadcaster.Broadcast("")
	if err == nil {
		t.Error("Expected error for empty transaction hex, got nil")
	}
}

// TestXMRBroadcaster_BroadcastAll_EmptyTx tests BroadcastAll with empty transaction
func TestXMRBroadcaster_BroadcastAll_EmptyTx(t *testing.T) {
	broadcaster := &XMRBroadcaster{
		client: nil,
	}

	_, err := broadcaster.BroadcastAll("")
	if err == nil {
		t.Error("Expected error for empty transaction hex, got nil")
	}
}
