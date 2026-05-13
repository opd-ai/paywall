package wallet

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
)

// Test vectors for well-known Bitcoin multisig addresses
func TestBuildRedeemScript(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	pubKey1 := key1.PubKey().SerializeCompressed()
	pubKey2 := key2.PubKey().SerializeCompressed()
	pubKey3 := key3.PubKey().SerializeCompressed()

	tests := []struct {
		name         string
		pubKeys      [][]byte
		requiredSigs int
		wantErr      bool
	}{
		{
			name:         "valid 2-of-3 multisig",
			pubKeys:      [][]byte{pubKey1, pubKey2, pubKey3},
			requiredSigs: 2,
			wantErr:      false,
		},
		{
			name:         "valid 1-of-1 multisig",
			pubKeys:      [][]byte{pubKey1},
			requiredSigs: 1,
			wantErr:      false,
		},
		{
			name:         "valid 3-of-3 multisig",
			pubKeys:      [][]byte{pubKey1, pubKey2, pubKey3},
			requiredSigs: 3,
			wantErr:      false,
		},
		{
			name:         "empty public keys",
			pubKeys:      [][]byte{},
			requiredSigs: 1,
			wantErr:      true,
		},
		{
			name:         "requiredSigs = 0",
			pubKeys:      [][]byte{pubKey1},
			requiredSigs: 0,
			wantErr:      true,
		},
		{
			name:         "requiredSigs > totalKeys",
			pubKeys:      [][]byte{pubKey1, pubKey2},
			requiredSigs: 3,
			wantErr:      true,
		},
		{
			name:         "too many keys (>15)",
			pubKeys:      make([][]byte, 16),
			requiredSigs: 10,
			wantErr:      true,
		},
		{
			name:         "invalid public key length",
			pubKeys:      [][]byte{{0x01, 0x02}},
			requiredSigs: 1,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fill dummy keys for too-many-keys test
			if len(tt.pubKeys) == 16 {
				for i := 0; i < 16; i++ {
					key, _ := btcec.NewPrivateKey()
					tt.pubKeys[i] = key.PubKey().SerializeCompressed()
				}
			}

			redeemScript, err := BuildRedeemScript(tt.pubKeys, tt.requiredSigs)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildRedeemScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(redeemScript) == 0 {
					t.Error("expected non-empty redeem script")
				}

				// Validate the script
				m, n, err := ValidateRedeemScript(redeemScript)
				if err != nil {
					t.Errorf("ValidateRedeemScript() failed: %v", err)
				}
				if m != tt.requiredSigs {
					t.Errorf("got requiredSigs %d, want %d", m, tt.requiredSigs)
				}
				if n != len(tt.pubKeys) {
					t.Errorf("got totalKeys %d, want %d", n, len(tt.pubKeys))
				}
			}
		})
	}
}

func TestCreateP2SHAddress(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()

	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
	}

	redeemScript, err := BuildRedeemScript(pubKeys, 2)
	if err != nil {
		t.Fatalf("failed to build redeem script: %v", err)
	}

	tests := []struct {
		name         string
		redeemScript []byte
		network      *chaincfg.Params
		wantErr      bool
		wantPrefix   string
	}{
		{
			name:         "mainnet P2SH",
			redeemScript: redeemScript,
			network:      &chaincfg.MainNetParams,
			wantErr:      false,
			wantPrefix:   "3",
		},
		{
			name:         "testnet P2SH",
			redeemScript: redeemScript,
			network:      &chaincfg.TestNet3Params,
			wantErr:      false,
			wantPrefix:   "2",
		},
		{
			name:         "empty redeem script",
			redeemScript: []byte{},
			network:      &chaincfg.MainNetParams,
			wantErr:      true,
		},
		{
			name:         "nil network",
			redeemScript: redeemScript,
			network:      nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address, err := CreateP2SHAddress(tt.redeemScript, tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateP2SHAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(address) == 0 {
					t.Error("expected non-empty address")
				}
				if address[0:1] != tt.wantPrefix {
					t.Errorf("address should start with %s, got %s", tt.wantPrefix, address[0:1])
				}
			}
		})
	}
}

func TestCreateP2WSHAddress(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()

	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
	}

	redeemScript, err := BuildRedeemScript(pubKeys, 2)
	if err != nil {
		t.Fatalf("failed to build redeem script: %v", err)
	}

	tests := []struct {
		name         string
		redeemScript []byte
		network      *chaincfg.Params
		wantErr      bool
		wantPrefix   string
	}{
		{
			name:         "mainnet P2WSH",
			redeemScript: redeemScript,
			network:      &chaincfg.MainNetParams,
			wantErr:      false,
			wantPrefix:   "bc1",
		},
		{
			name:         "testnet P2WSH",
			redeemScript: redeemScript,
			network:      &chaincfg.TestNet3Params,
			wantErr:      false,
			wantPrefix:   "tb1",
		},
		{
			name:         "empty redeem script",
			redeemScript: []byte{},
			network:      &chaincfg.MainNetParams,
			wantErr:      true,
		},
		{
			name:         "nil network",
			redeemScript: redeemScript,
			network:      nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address, err := CreateP2WSHAddress(tt.redeemScript, tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateP2WSHAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(address) == 0 {
					t.Error("expected non-empty address")
				}
				if address[0:3] != tt.wantPrefix {
					t.Errorf("address should start with %s, got %s", tt.wantPrefix, address[0:3])
				}
			}
		})
	}
}

func TestCreateMultisigAddress(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	pubKeys := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
		key3.PubKey().SerializeCompressed(),
	}

	tests := []struct {
		name         string
		pubKeys      [][]byte
		requiredSigs int
		addressType  MultisigAddressType
		network      *chaincfg.Params
		wantErr      bool
	}{
		{
			name:         "P2SH 2-of-3",
			pubKeys:      pubKeys,
			requiredSigs: 2,
			addressType:  P2SH,
			network:      &chaincfg.MainNetParams,
			wantErr:      false,
		},
		{
			name:         "P2WSH 2-of-3",
			pubKeys:      pubKeys,
			requiredSigs: 2,
			addressType:  P2WSH,
			network:      &chaincfg.MainNetParams,
			wantErr:      false,
		},
		{
			name:         "testnet P2SH",
			pubKeys:      pubKeys,
			requiredSigs: 2,
			addressType:  P2SH,
			network:      &chaincfg.TestNet3Params,
			wantErr:      false,
		},
		{
			name:         "invalid public keys",
			pubKeys:      [][]byte{},
			requiredSigs: 1,
			addressType:  P2SH,
			network:      &chaincfg.MainNetParams,
			wantErr:      true,
		},
		{
			name:         "invalid address type",
			pubKeys:      pubKeys,
			requiredSigs: 2,
			addressType:  MultisigAddressType(999),
			network:      &chaincfg.MainNetParams,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address, redeemScript, err := CreateMultisigAddress(tt.pubKeys, tt.requiredSigs, tt.addressType, tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateMultisigAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(address) == 0 {
					t.Error("expected non-empty address")
				}
				if len(redeemScript) == 0 {
					t.Error("expected non-empty redeem script")
				}

				// Verify we can validate the redeem script
				m, n, err := ValidateRedeemScript(redeemScript)
				if err != nil {
					t.Errorf("ValidateRedeemScript() failed: %v", err)
				}
				if m != tt.requiredSigs {
					t.Errorf("got requiredSigs %d, want %d", m, tt.requiredSigs)
				}
				if n != len(tt.pubKeys) {
					t.Errorf("got totalKeys %d, want %d", n, len(tt.pubKeys))
				}
			}
		})
	}
}

func TestDeriveParticipantKey(t *testing.T) {
	// Test with a known seed
	masterKey := make([]byte, 32)
	chainCode := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
		chainCode[i] = byte(i + 1)
	}

	tests := []struct {
		name      string
		masterKey []byte
		chainCode []byte
		index     uint32
		wantErr   bool
	}{
		{
			name:      "valid derivation index 0",
			masterKey: masterKey,
			chainCode: chainCode,
			index:     0,
			wantErr:   false,
		},
		{
			name:      "valid derivation index 1",
			masterKey: masterKey,
			chainCode: chainCode,
			index:     1,
			wantErr:   false,
		},
		{
			name:      "invalid master key length",
			masterKey: make([]byte, 16),
			chainCode: chainCode,
			index:     0,
			wantErr:   true,
		},
		{
			name:      "invalid chain code length",
			masterKey: masterKey,
			chainCode: make([]byte, 16),
			index:     0,
			wantErr:   true,
		},
		{
			name:      "hardened index not supported",
			masterKey: masterKey,
			chainCode: chainCode,
			index:     hardenedKeyStart,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pubKey, err := DeriveParticipantKey(tt.masterKey, tt.chainCode, tt.index)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveParticipantKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if pubKey == nil {
					t.Error("expected non-nil public key")
				}
				// Verify serialization works
				serialized := pubKey.SerializeCompressed()
				if len(serialized) != 33 {
					t.Errorf("compressed public key should be 33 bytes, got %d", len(serialized))
				}
			}
		})
	}
}

func TestExtractPubKeysFromRedeemScript(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

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
		name         string
		redeemScript []byte
		wantCount    int
		wantErr      bool
	}{
		{
			name:         "extract from valid 2-of-3 script",
			redeemScript: redeemScript,
			wantCount:    3,
			wantErr:      false,
		},
		{
			name:         "empty redeem script",
			redeemScript: []byte{},
			wantCount:    0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractedKeys, err := ExtractPubKeysFromRedeemScript(tt.redeemScript)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractPubKeysFromRedeemScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(extractedKeys) != tt.wantCount {
					t.Errorf("got %d keys, want %d", len(extractedKeys), tt.wantCount)
				}

				// Verify extracted keys match original
				for i, key := range extractedKeys {
					if !bytes.Equal(key, pubKeys[i]) {
						t.Errorf("key %d doesn't match: got %x, want %x", i, key, pubKeys[i])
					}
				}
			}
		})
	}
}

func TestCompareRedeemScripts(t *testing.T) {
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()

	pubKeys1 := [][]byte{
		key1.PubKey().SerializeCompressed(),
		key2.PubKey().SerializeCompressed(),
	}

	script1, _ := BuildRedeemScript(pubKeys1, 2)
	script2, _ := BuildRedeemScript(pubKeys1, 2)
	script3, _ := BuildRedeemScript(pubKeys1, 1) // Different requiredSigs

	tests := []struct {
		name    string
		script1 []byte
		script2 []byte
		want    bool
	}{
		{
			name:    "identical scripts",
			script1: script1,
			script2: script2,
			want:    true,
		},
		{
			name:    "different scripts",
			script1: script1,
			script2: script3,
			want:    false,
		},
		{
			name:    "both nil",
			script1: nil,
			script2: nil,
			want:    true,
		},
		{
			name:    "one nil",
			script1: script1,
			script2: nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareRedeemScripts(tt.script1, tt.script2); got != tt.want {
				t.Errorf("CompareRedeemScripts() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestKnownMultisigAddress tests multisig address generation with real keys
func TestKnownMultisigAddress(t *testing.T) {
	// Generate real keys (secure for testing)
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	pubKey1 := key1.PubKey().SerializeCompressed()
	pubKey2 := key2.PubKey().SerializeCompressed()
	pubKey3 := key3.PubKey().SerializeCompressed()

	pubKeys := [][]byte{pubKey1, pubKey2, pubKey3}

	// Create 2-of-3 multisig
	redeemScript, err := BuildRedeemScript(pubKeys, 2)
	if err != nil {
		t.Fatalf("BuildRedeemScript failed: %v", err)
	}

	// Create P2SH address
	p2shAddress, err := CreateP2SHAddress(redeemScript, &chaincfg.TestNet3Params)
	if err != nil {
		t.Fatalf("CreateP2SHAddress failed: %v", err)
	}

	// Verify address starts with '2' (testnet P2SH)
	if p2shAddress[0] != '2' {
		t.Errorf("expected testnet P2SH address to start with '2', got %c", p2shAddress[0])
	}

	// Create P2WSH address
	p2wshAddress, err := CreateP2WSHAddress(redeemScript, &chaincfg.TestNet3Params)
	if err != nil {
		t.Fatalf("CreateP2WSHAddress failed: %v", err)
	}

	// Verify address starts with 'tb1' (testnet P2WSH)
	if p2wshAddress[0:3] != "tb1" {
		t.Errorf("expected testnet P2WSH address to start with 'tb1', got %s", p2wshAddress[0:3])
	}

	t.Logf("P2SH address: %s", p2shAddress)
	t.Logf("P2WSH address: %s", p2wshAddress)
}

// TestBuildRedeemScript_EdgeCases tests additional edge cases for multisig script creation
func TestBuildRedeemScript_EdgeCases(t *testing.T) {
	// Generate test keys
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	pubKey1 := key1.PubKey().SerializeCompressed()
	pubKey2 := key2.PubKey().SerializeCompressed()
	pubKey3 := key3.PubKey().SerializeCompressed()
	pubKey1Uncompressed := key1.PubKey().SerializeUncompressed()

	tests := []struct {
		name         string
		pubKeys      [][]byte
		requiredSigs int
		wantErr      bool
		errorMsg     string
	}{
		{
			name:         "nil public key in array",
			pubKeys:      [][]byte{pubKey1, nil, pubKey2},
			requiredSigs: 2,
			wantErr:      true,
			errorMsg:     "invalid length",
		},
		{
			name:         "duplicate public keys",
			pubKeys:      [][]byte{pubKey1, pubKey1, pubKey2},
			requiredSigs: 2,
			wantErr:      false, // Bitcoin allows duplicate keys technically
		},
		{
			name:         "mix of compressed and uncompressed keys",
			pubKeys:      [][]byte{pubKey1, pubKey1Uncompressed, pubKey2},
			requiredSigs: 2,
			wantErr:      false, // Should work with both formats
		},
		{
			name: "exactly 15 keys (boundary)",
			pubKeys: func() [][]byte {
				keys := make([][]byte, 15)
				for i := 0; i < 15; i++ {
					k, _ := btcec.NewPrivateKey()
					keys[i] = k.PubKey().SerializeCompressed()
				}
				return keys
			}(),
			requiredSigs: 10,
			wantErr:      false,
		},
		{
			name:         "negative requiredSigs",
			pubKeys:      [][]byte{pubKey1, pubKey2},
			requiredSigs: -1,
			wantErr:      true,
			errorMsg:     "at least 1",
		},
		{
			name:         "all nil public keys",
			pubKeys:      [][]byte{nil, nil},
			requiredSigs: 1,
			wantErr:      true,
			errorMsg:     "invalid length",
		},
		{
			name:         "invalid key - all zeros",
			pubKeys:      [][]byte{make([]byte, 33)},
			requiredSigs: 1,
			wantErr:      true,
			errorMsg:     "invalid public key",
		},
		{
			name:         "single uncompressed key",
			pubKeys:      [][]byte{pubKey1Uncompressed},
			requiredSigs: 1,
			wantErr:      false,
		},
		{
			name:         "requiredSigs equals totalKeys (n-of-n)",
			pubKeys:      [][]byte{pubKey1, pubKey2, pubKey3},
			requiredSigs: 3,
			wantErr:      false,
		},
		{
			name:         "malformed key - wrong format marker",
			pubKeys:      [][]byte{{0x05, 0x02, 0x03}}, // Invalid first byte
			requiredSigs: 1,
			wantErr:      true,
			errorMsg:     "invalid length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redeemScript, err := BuildRedeemScript(tt.pubKeys, tt.requiredSigs)

			if tt.wantErr {
				if err == nil {
					t.Errorf("BuildRedeemScript() expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("BuildRedeemScript() error = %v, want error containing %q", err, tt.errorMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("BuildRedeemScript() unexpected error = %v", err)
				return
			}

			if len(redeemScript) == 0 {
				t.Error("expected non-empty redeem script")
			}

			// Validate the script structure
			m, n, err := ValidateRedeemScript(redeemScript)
			if err != nil {
				t.Errorf("ValidateRedeemScript() failed: %v", err)
			}
			if m != tt.requiredSigs {
				t.Errorf("got requiredSigs %d, want %d", m, tt.requiredSigs)
			}
			// Count non-nil keys for validation
			nonNilKeys := 0
			for _, key := range tt.pubKeys {
				if key != nil {
					nonNilKeys++
				}
			}
			if n != nonNilKeys {
				t.Errorf("got totalKeys %d, want %d", n, nonNilKeys)
			}
		})
	}
}

// contains is a helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && contains(s[1:], substr)) ||
		(len(s) >= len(substr) && s[:len(substr)] == substr))
}

// TestValidateRedeemScript_EdgeCases tests edge cases for redeem script validation
func TestValidateRedeemScript_EdgeCases(t *testing.T) {
	// Generate valid test keys for creating valid scripts
	key1, _ := btcec.NewPrivateKey()
	key2, _ := btcec.NewPrivateKey()
	key3, _ := btcec.NewPrivateKey()

	pubKey1 := key1.PubKey().SerializeCompressed()
	pubKey2 := key2.PubKey().SerializeCompressed()
	pubKey3 := key3.PubKey().SerializeCompressed()

	tests := []struct {
		name    string
		script  func() []byte
		wantM   int
		wantN   int
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid 2-of-3 script",
			script: func() []byte {
				script, _ := BuildRedeemScript([][]byte{pubKey1, pubKey2, pubKey3}, 2)
				return script
			},
			wantM:   2,
			wantN:   3,
			wantErr: false,
		},
		{
			name: "empty script",
			script: func() []byte {
				return []byte{}
			},
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name: "nil script",
			script: func() []byte {
				return nil
			},
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name: "script too short",
			script: func() []byte {
				return []byte{0x52, 0xae} // Just 2 bytes
			},
			wantErr: true,
			errMsg:  "too short",
		},
		{
			name: "script without OP_CHECKMULTISIG",
			script: func() []byte {
				return []byte{0x52, 0x21, 0x03, 0x00, 0x52, 0xff} // Doesn't end with 0xae
			},
			wantErr: true,
			errMsg:  "OP_CHECKMULTISIG",
		},
		{
			name: "script with invalid requiredSigs (0)",
			script: func() []byte {
				return []byte{0x50, 0x21, 0x03, 0x00, 0x51, 0xae} // OP_0 for required sigs
			},
			wantErr: true,
			errMsg:  "invalid requiredSigs",
		},
		{
			name: "script with invalid requiredSigs (>15)",
			script: func() []byte {
				return []byte{0x60, 0x21, 0x03, 0x00, 0x51, 0xae} // OP_16 = 0x60
			},
			wantErr: true,
			errMsg:  "invalid requiredSigs",
		},
		{
			name: "script with invalid totalKeys (0)",
			script: func() []byte {
				return []byte{0x51, 0x21, 0x03, 0x00, 0x50, 0xae} // OP_0 for total keys
			},
			wantErr: true,
			errMsg:  "invalid totalKeys",
		},
		{
			name: "script with requiredSigs > totalKeys",
			script: func() []byte {
				return []byte{0x53, 0x21, 0x03, 0x00, 0x52, 0xae} // OP_3 required, OP_2 total
			},
			wantErr: true,
			errMsg:  "requiredSigs",
		},
		{
			name: "valid 1-of-1 script",
			script: func() []byte {
				script, _ := BuildRedeemScript([][]byte{pubKey1}, 1)
				return script
			},
			wantM:   1,
			wantN:   1,
			wantErr: false,
		},
		{
			name: "valid 15-of-15 script (boundary)",
			script: func() []byte {
				keys := make([][]byte, 15)
				for i := 0; i < 15; i++ {
					k, _ := btcec.NewPrivateKey()
					keys[i] = k.PubKey().SerializeCompressed()
				}
				script, _ := BuildRedeemScript(keys, 15)
				return script
			},
			wantM:   15,
			wantN:   15,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := tt.script()
			m, n, err := ValidateRedeemScript(script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateRedeemScript() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateRedeemScript() error = %v, want error containing %q", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateRedeemScript() unexpected error = %v", err)
				return
			}

			if m != tt.wantM {
				t.Errorf("requiredSigs = %d, want %d", m, tt.wantM)
			}
			if n != tt.wantN {
				t.Errorf("totalKeys = %d, want %d", n, tt.wantN)
			}
		})
	}
}
