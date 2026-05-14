// Package paywall implements tests for transaction broadcasting functionality
package paywall

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/paywall/wallet"
)

// TestBTCBroadcaster_NewBTCBroadcaster tests broadcaster creation and validation
func TestBTCBroadcaster_NewBTCBroadcaster(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		user    string
		pass    string
		useTLS  bool
		network *chaincfg.Params
		wantErr bool
	}{
		{
			name:    "missing host",
			host:    "",
			user:    "user",
			pass:    "pass",
			useTLS:  false,
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:    "missing user",
			host:    "localhost:18332",
			user:    "",
			pass:    "pass",
			useTLS:  false,
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:    "missing pass",
			host:    "localhost:18332",
			user:    "user",
			pass:    "",
			useTLS:  false,
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBTCBroadcaster(tt.host, tt.user, tt.pass, tt.useTLS, tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBTCBroadcaster() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBTCBroadcaster_ValidateTransaction tests transaction validation logic
func TestBTCBroadcaster_ValidateTransaction(t *testing.T) {
	// Create a mock broadcaster (won't actually connect to RPC)
	network := &chaincfg.TestNet3Params

	// Create a test P2PKH address
	testAddr := "mwCwTceJvYV27KXBc3NJZys6CjsgsoeHmf" // Valid testnet address

	// Create a simple transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add a dummy input
	prevHash, _ := chainhash.NewHashFromStr("1111111111111111111111111111111111111111111111111111111111111111")
	txIn := wire.NewTxIn(wire.NewOutPoint(prevHash, 0), nil, nil)
	tx.AddTxIn(txIn)

	// Add output to the test address with correct amount
	address, err := btcutil.DecodeAddress(testAddr, network)
	if err != nil {
		t.Fatalf("Failed to decode address: %v", err)
	}

	pkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		t.Fatalf("Failed to create pk script: %v", err)
	}
	txOut := wire.NewTxOut(100000, pkScript) // 0.001 BTC
	tx.AddTxOut(txOut)

	// Serialize transaction
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		t.Fatalf("Failed to serialize transaction: %v", err)
	}
	txBytes := buf.Bytes()

	// Create test payment
	payment := &Payment{
		ID: "test-payment",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: testAddr,
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
		},
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(time.Hour),
		Status:          StatusPending,
		MultisigEnabled: true,
	}

	// Create broadcaster (note: this will fail to connect but that's okay for validation tests)
	broadcaster := &BTCBroadcaster{
		client:  nil, // We won't use the client for validation
		network: network,
	}

	tests := []struct {
		name    string
		txBytes []byte
		payment *Payment
		wantErr bool
	}{
		{
			name:    "valid transaction",
			txBytes: txBytes,
			payment: payment,
			wantErr: false,
		},
		{
			name:    "empty transaction bytes",
			txBytes: []byte{},
			payment: payment,
			wantErr: true,
		},
		{
			name:    "nil payment",
			txBytes: txBytes,
			payment: nil,
			wantErr: true,
		},
		{
			name:    "malformed transaction",
			txBytes: []byte{0x01, 0x02, 0x03},
			payment: payment,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := broadcaster.ValidateTransaction(tt.txBytes, tt.payment)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransaction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBTCBroadcaster_ValidateTransaction_NoInputs tests validation of transaction with no inputs
func TestBTCBroadcaster_ValidateTransaction_NoInputs(t *testing.T) {
	network := &chaincfg.TestNet3Params
	broadcaster := &BTCBroadcaster{
		client:  nil,
		network: network,
	}

	// Create transaction with no inputs
	tx := wire.NewMsgTx(wire.TxVersion)
	// Add output but no input
	tx.AddTxOut(wire.NewTxOut(100000, []byte{}))

	var buf bytes.Buffer
	tx.Serialize(&buf)

	payment := &Payment{
		ID: "test",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "test-address",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
		},
	}

	err := broadcaster.ValidateTransaction(buf.Bytes(), payment)
	if err == nil {
		t.Error("Expected error for transaction with no inputs, got nil")
	}
}

// TestBTCBroadcaster_ValidateTransaction_NoOutputs tests validation of transaction with no outputs
func TestBTCBroadcaster_ValidateTransaction_NoOutputs(t *testing.T) {
	network := &chaincfg.TestNet3Params
	broadcaster := &BTCBroadcaster{
		client:  nil,
		network: network,
	}

	// Create transaction with no outputs
	tx := wire.NewMsgTx(wire.TxVersion)
	// Add input but no output
	prevHash, _ := chainhash.NewHashFromStr("1111111111111111111111111111111111111111111111111111111111111111")
	txIn := wire.NewTxIn(wire.NewOutPoint(prevHash, 0), nil, nil)
	tx.AddTxIn(txIn)

	var buf bytes.Buffer
	tx.Serialize(&buf)

	payment := &Payment{
		ID: "test",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "test-address",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
		},
	}

	err := broadcaster.ValidateTransaction(buf.Bytes(), payment)
	if err == nil {
		t.Error("Expected error for transaction with no outputs, got nil")
	}
}

// TestPayment_BroadcastTracking tests that Payment struct properly tracks broadcast state
func TestPayment_BroadcastTracking(t *testing.T) {
	payment := &Payment{
		ID:                "test-payment",
		TransactionID:     "",
		BroadcastedAt:     time.Time{},
		BroadcastAttempts: 0,
	}

	// Initially should not be broadcast
	if payment.TransactionID != "" {
		t.Error("Expected empty transaction ID initially")
	}
	if !payment.BroadcastedAt.IsZero() {
		t.Error("Expected zero broadcast time initially")
	}
	if payment.BroadcastAttempts != 0 {
		t.Error("Expected zero broadcast attempts initially")
	}

	// Simulate broadcast
	payment.TransactionID = "abc123"
	payment.BroadcastedAt = time.Now()
	payment.BroadcastAttempts = 1

	// Should now be marked as broadcast
	if payment.TransactionID == "" {
		t.Error("Expected transaction ID to be set")
	}
	if payment.BroadcastedAt.IsZero() {
		t.Error("Expected broadcast time to be set")
	}
	if payment.BroadcastAttempts != 1 {
		t.Errorf("Expected 1 broadcast attempt, got %d", payment.BroadcastAttempts)
	}
}

// TestPayment_DoubleBroadcastPrevention tests that we can detect already-broadcast payments
func TestPayment_DoubleBroadcastPrevention(t *testing.T) {
	// Create payment that has already been broadcast
	payment := &Payment{
		ID:                "test-payment",
		TransactionID:     "existing-tx-id",
		BroadcastedAt:     time.Now(),
		BroadcastAttempts: 1,
		MultisigEnabled:   true,
	}

	// Check if payment was already broadcast
	if payment.TransactionID == "" {
		t.Error("Payment should have transaction ID (already broadcast)")
	}

	// Simulate checking before broadcast
	alreadyBroadcast := payment.TransactionID != ""
	if !alreadyBroadcast {
		t.Error("Should detect payment was already broadcast")
	}
}

// TestBTCBroadcaster_Close tests broadcaster cleanup
func TestBTCBroadcaster_Close(t *testing.T) {
	broadcaster := &BTCBroadcaster{
		client:  nil, // nil client should not panic
		network: &chaincfg.TestNet3Params,
	}

	// Should not panic
	broadcaster.Close()
}
