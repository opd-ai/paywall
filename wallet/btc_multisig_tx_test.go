package wallet

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

func TestCreateMultisigPaymentTx(t *testing.T) {
	// Create test UTXOs
	utxo1 := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		ScriptPubKey: []byte{0x00, 0x14}, // Mock script
		RedeemScript: []byte{0x52, 0x21}, // Mock redeem script
	}

	utxo2 := UTXO{
		TxID:          "0000000000000000000000000000000000000000000000000000000000000002",
		Vout:          1,
		Amount:        200000,
		ScriptPubKey:  []byte{0x00, 0x20}, // Mock script
		WitnessScript: []byte{0x52, 0x21}, // Mock witness script
	}

	tests := []struct {
		name    string
		inputs  []UTXO
		outputs map[string]int64
		network *chaincfg.Params
		wantErr bool
	}{
		{
			name:   "valid transaction with one input",
			inputs: []UTXO{utxo1},
			outputs: map[string]int64{
				"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000,
			},
			network: &chaincfg.TestNet3Params,
			wantErr: false,
		},
		{
			name:   "valid transaction with multiple inputs",
			inputs: []UTXO{utxo1, utxo2},
			outputs: map[string]int64{
				"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF":        100000,
				"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx": 150000,
			},
			network: &chaincfg.TestNet3Params,
			wantErr: false,
		},
		{
			name:    "no inputs",
			inputs:  []UTXO{},
			outputs: map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:    "no outputs",
			inputs:  []UTXO{utxo1},
			outputs: map[string]int64{},
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:   "nil network",
			inputs: []UTXO{utxo1},
			outputs: map[string]int64{
				"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000,
			},
			network: nil,
			wantErr: true,
		},
		{
			name: "invalid input amount",
			inputs: []UTXO{
				{
					TxID:   "0000000000000000000000000000000000000000000000000000000000000001",
					Vout:   0,
					Amount: -100,
				},
			},
			outputs: map[string]int64{
				"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000,
			},
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:   "invalid output amount",
			inputs: []UTXO{utxo1},
			outputs: map[string]int64{
				"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": -50000,
			},
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:   "invalid output address",
			inputs: []UTXO{utxo1},
			outputs: map[string]int64{
				"invalid-address": 50000,
			},
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
		{
			name:   "invalid transaction ID",
			inputs: []UTXO{{TxID: "invalid", Vout: 0, Amount: 100000}},
			outputs: map[string]int64{
				"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000,
			},
			network: &chaincfg.TestNet3Params,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := CreateMultisigPaymentTx(tt.inputs, tt.outputs, tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateMultisigPaymentTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if tx == nil {
					t.Error("expected non-nil transaction")
					return
				}
				if len(tx.Tx.TxIn) != len(tt.inputs) {
					t.Errorf("expected %d inputs, got %d", len(tt.inputs), len(tx.Tx.TxIn))
				}
				if len(tx.Tx.TxOut) != len(tt.outputs) {
					t.Errorf("expected %d outputs, got %d", len(tt.outputs), len(tx.Tx.TxOut))
				}
			}
		})
	}
}

func TestSignMultisigTx(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	// Create redeem script
	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
		key3.PubKey().SerializeCompressed(),
	}
	redeemScript, err := BuildRedeemScript(pubKeys, 2)
	if err != nil {
		t.Fatalf("failed to build redeem script: %v", err)
	}

	// Create test UTXO with redeem script
	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	// Create transaction
	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	tests := []struct {
		name        string
		inputIndex  int
		privateKey  *btcec.PrivateKey
		sigHashType txscript.SigHashType
		wantErr     bool
	}{
		{
			name:        "valid signature with key1",
			inputIndex:  0,
			privateKey:  key1,
			sigHashType: txscript.SigHashAll,
			wantErr:     false,
		},
		{
			name:        "valid signature with key2",
			inputIndex:  0,
			privateKey:  key2,
			sigHashType: txscript.SigHashAll,
			wantErr:     false,
		},
		{
			name:        "invalid input index",
			inputIndex:  999,
			privateKey:  key1,
			sigHashType: txscript.SigHashAll,
			wantErr:     true,
		},
		{
			name:        "nil private key",
			inputIndex:  0,
			privateKey:  nil,
			sigHashType: txscript.SigHashAll,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tx.SignMultisigTx(tt.inputIndex, tt.privateKey, tt.sigHashType)
			if (err != nil) != tt.wantErr {
				t.Errorf("SignMultisigTx() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify signature was stored
				if len(tx.Signatures[tt.inputIndex]) == 0 {
					t.Error("expected signature to be stored")
				}
			}
		})
	}
}

func TestCombineSignatures(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	// Create 2-of-3 redeem script
	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
		key3.PubKey().SerializeCompressed(),
	}
	redeemScript, err := BuildRedeemScript(pubKeys, 2)
	if err != nil {
		t.Fatalf("failed to build redeem script: %v", err)
	}

	tests := []struct {
		name     string
		setupTx  func() *MultisigPaymentTx
		signKeys []*btcec.PrivateKey
		wantErr  bool
	}{
		{
			name: "successful combination with 2 signatures",
			setupTx: func() *MultisigPaymentTx {
				utxo := UTXO{
					TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
					Vout:         0,
					Amount:       100000,
					RedeemScript: redeemScript,
				}
				tx, _ := CreateMultisigPaymentTx(
					[]UTXO{utxo},
					map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
					&chaincfg.TestNet3Params,
				)
				return tx
			},
			signKeys: []*btcec.PrivateKey{key1, key2},
			wantErr:  false,
		},
		{
			name: "fail with no signatures",
			setupTx: func() *MultisigPaymentTx {
				utxo := UTXO{
					TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
					Vout:         0,
					Amount:       100000,
					RedeemScript: redeemScript,
				}
				tx, _ := CreateMultisigPaymentTx(
					[]UTXO{utxo},
					map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
					&chaincfg.TestNet3Params,
				)
				return tx
			},
			signKeys: []*btcec.PrivateKey{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := tt.setupTx()

			// Sign with provided keys
			for _, key := range tt.signKeys {
				if err := tx.SignMultisigTx(0, key, txscript.SigHashAll); err != nil {
					t.Fatalf("failed to sign: %v", err)
				}
			}

			// Combine signatures
			err := tx.CombineSignatures()
			if (err != nil) != tt.wantErr {
				t.Errorf("CombineSignatures() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify scriptSig was set
				if len(tx.Tx.TxIn[0].SignatureScript) == 0 {
					t.Error("expected non-empty scriptSig")
				}
			}
		})
	}
}

func TestSerialize(t *testing.T) {
	// Create a simple transaction
	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: []byte{0x52, 0x21}, // Mock
	}

	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "serialize transaction",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txBytes, err := tx.Serialize()
			if (err != nil) != tt.wantErr {
				t.Errorf("Serialize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(txBytes) == 0 {
					t.Error("expected non-empty transaction bytes")
				}

				// Test hex serialization
				hexStr, err := tx.SerializeHex()
				if err != nil {
					t.Errorf("SerializeHex() error = %v", err)
				}
				if len(hexStr) == 0 {
					t.Error("expected non-empty hex string")
				}
			}
		})
	}
}

func TestGetTxID(t *testing.T) {
	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: []byte{0x52, 0x21},
	}

	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	txID := tx.GetTxID()
	if len(txID) != 64 {
		t.Errorf("expected 64-character transaction ID, got %d characters", len(txID))
	}
}

func TestGetRequiredSignatures(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	// Create 2-of-3 redeem script
	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
		key3.PubKey().SerializeCompressed(),
	}
	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Initially, no signatures
	required, collected, err := tx.GetRequiredSignatures(0)
	if err != nil {
		t.Errorf("GetRequiredSignatures() error = %v", err)
	}
	if required != 2 {
		t.Errorf("expected 2 required signatures, got %d", required)
	}
	if collected != 0 {
		t.Errorf("expected 0 collected signatures, got %d", collected)
	}

	// Sign once
	tx.SignMultisigTx(0, key1, txscript.SigHashAll)

	required, collected, err = tx.GetRequiredSignatures(0)
	if err != nil {
		t.Errorf("GetRequiredSignatures() error = %v", err)
	}
	if collected != 1 {
		t.Errorf("expected 1 collected signature, got %d", collected)
	}

	// Sign twice
	tx.SignMultisigTx(0, key2, txscript.SigHashAll)

	required, collected, err = tx.GetRequiredSignatures(0)
	if err != nil {
		t.Errorf("GetRequiredSignatures() error = %v", err)
	}
	if collected != 2 {
		t.Errorf("expected 2 collected signatures, got %d", collected)
	}
}

func TestEstimateSize(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()

	// Create 2-of-2 redeem script
	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
	}
	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	size := tx.EstimateSize()
	if size <= 0 {
		t.Errorf("expected positive size estimate, got %d", size)
	}

	// Estimate fee
	fee := tx.EstimateFee(10.0) // 10 sat/byte
	if fee <= 0 {
		t.Errorf("expected positive fee estimate, got %d", fee)
	}
}

func TestVerifySignature(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	// Create 2-of-3 redeem script
	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
		key3.PubKey().SerializeCompressed(),
	}
	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Sign with key1
	err = tx.SignMultisigTx(0, key1, txscript.SigHashAll)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	// Get the signature
	sig := tx.Signatures[0][0]

	// Verify correct signature
	valid, err := tx.VerifySignature(0, sig.PublicKey, sig.Signature)
	if err != nil {
		t.Errorf("VerifySignature() error = %v", err)
	}
	if !valid {
		t.Error("expected valid signature")
	}

	// Verify with wrong public key
	valid, err = tx.VerifySignature(0, key2.PubKey().SerializeCompressed(), sig.Signature)
	if err != nil {
		t.Errorf("VerifySignature() error = %v", err)
	}
	if valid {
		t.Error("expected invalid signature with wrong public key")
	}
}

func TestEndToEndMultisigTransaction(t *testing.T) {
	// Generate three keys for 2-of-3 multisig
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	// Create 2-of-3 multisig address
	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
		key3.PubKey().SerializeCompressed(),
	}
	redeemScript, err := BuildRedeemScript(pubKeys, 2)
	if err != nil {
		t.Fatalf("failed to build redeem script: %v", err)
	}

	// Create UTXO (simulating funds sent to multisig address)
	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000001",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	// Create spending transaction
	tx, err := CreateMultisigPaymentTx(
		[]UTXO{utxo},
		map[string]int64{"2N8hwP1WmJrFF5QWABn38y63uYLhnJYJYTF": 50000},
		&chaincfg.TestNet3Params,
	)
	if err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	// Sign with first key
	err = tx.SignMultisigTx(0, key1, txscript.SigHashAll)
	if err != nil {
		t.Fatalf("failed to sign with key1: %v", err)
	}

	// Sign with second key
	err = tx.SignMultisigTx(0, key2, txscript.SigHashAll)
	if err != nil {
		t.Fatalf("failed to sign with key2: %v", err)
	}

	// Verify we have enough signatures
	required, collected, err := tx.GetRequiredSignatures(0)
	if err != nil {
		t.Fatalf("failed to get required signatures: %v", err)
	}
	if collected < required {
		t.Errorf("insufficient signatures: have %d, need %d", collected, required)
	}

	// Combine signatures
	err = tx.CombineSignatures()
	if err != nil {
		t.Fatalf("failed to combine signatures: %v", err)
	}

	// Verify transaction can be serialized
	txBytes, err := tx.Serialize()
	if err != nil {
		t.Fatalf("failed to serialize transaction: %v", err)
	}
	if len(txBytes) == 0 {
		t.Error("expected non-empty serialized transaction")
	}

	// Get transaction ID
	txID := tx.GetTxID()
	if len(txID) != 64 {
		t.Errorf("expected 64-character transaction ID, got %d", len(txID))
	}

	t.Logf("Successfully created and signed 2-of-3 multisig transaction: %s", txID)
}
