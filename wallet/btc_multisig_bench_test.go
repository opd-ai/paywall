package wallet

import (
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
)

// BenchmarkMultisig_AddressGeneration_P2SH compares multisig P2SH address generation to single-sig
func BenchmarkMultisig_AddressGeneration_P2SH(b *testing.B) {
	// Generate test public keys
	pubKeys := make([][]byte, 3)
	for i := range pubKeys {
		privKey, _ := btcec.NewPrivateKey()
		pubKeys[i] = privKey.PubKey().SerializeCompressed()
	}

	network := &chaincfg.MainNetParams

	b.Run("SingleSig", func(b *testing.B) {
		wallet := &BTCHDWallet{
			masterKey: make([]byte, 32),
			chainCode: make([]byte, 32),
			network:   network,
			nextIndex: 0,
		}
		copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
		copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := wallet.DeriveNextAddress()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Multisig_2of3", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := CreateMultisigAddress(pubKeys, 2, P2SH, network)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMultisig_AddressGeneration_P2WSH compares multisig P2WSH (SegWit) address generation
func BenchmarkMultisig_AddressGeneration_P2WSH(b *testing.B) {
	// Generate test public keys
	pubKeys := make([][]byte, 3)
	for i := range pubKeys {
		privKey, _ := btcec.NewPrivateKey()
		pubKeys[i] = privKey.PubKey().SerializeCompressed()
	}

	network := &chaincfg.MainNetParams

	b.Run("SingleSig", func(b *testing.B) {
		wallet := &BTCHDWallet{
			masterKey: make([]byte, 32),
			chainCode: make([]byte, 32),
			network:   network,
			nextIndex: 0,
		}
		copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
		copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := wallet.DeriveNextAddress()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Multisig_2of3_SegWit", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := CreateMultisigAddress(pubKeys, 2, P2WSH, network)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMultisig_RedeemScriptGeneration benchmarks redeem script creation with various m-of-n configs
func BenchmarkMultisig_RedeemScriptGeneration(b *testing.B) {
	// Generate test public keys for various configurations
	pubKeys := make([][]byte, 15) // Max Bitcoin allows
	for i := range pubKeys {
		privKey, _ := btcec.NewPrivateKey()
		pubKeys[i] = privKey.PubKey().SerializeCompressed()
	}

	configs := []struct {
		name string
		m    int
		n    int
	}{
		{"1of1", 1, 1},
		{"2of2", 2, 2},
		{"2of3", 2, 3},
		{"3of5", 3, 5},
		{"5of7", 5, 7},
		{"7of10", 7, 10},
		{"10of15", 10, 15},
	}

	for _, config := range configs {
		b.Run(config.name, func(b *testing.B) {
			keys := pubKeys[:config.n]
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := BuildRedeemScript(keys, config.m)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMultisig_SignatureCreation benchmarks creating signatures for multisig transactions
func BenchmarkMultisig_SignatureCreation(b *testing.B) {
	// Setup: Create a test multisig transaction
	privKeys := make([]*btcec.PrivateKey, 3)
	pubKeys := make([][]byte, 3)
	for i := range privKeys {
		privKeys[i], _ = btcec.NewPrivateKey()
		pubKeys[i] = privKeys[i].PubKey().SerializeCompressed()
	}

	network := &chaincfg.TestNet3Params
	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	// Create a simple test UTXO
	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000000",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	// Create test transaction
	outputs := map[string]int64{
		"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx": 90000, // Valid SegWit testnet address
	}

	tx, err := CreateMultisigPaymentTx([]UTXO{utxo}, outputs, network)
	if err != nil {
		b.Fatalf("Failed to create transaction: %v", err)
	}

	b.Run("SignSingleInput", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := tx.SignMultisigTx(0, privKeys[0], txscript.SigHashAll)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("SignMultipleInputs_3Inputs", func(b *testing.B) {
		// Create transaction with 3 inputs
		tx3, err := CreateMultisigPaymentTx([]UTXO{utxo, utxo, utxo}, outputs, network)
		if err != nil {
			b.Fatalf("Failed to create transaction: %v", err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for inputIdx := 0; inputIdx < 3; inputIdx++ {
				err := tx3.SignMultisigTx(inputIdx, privKeys[0], txscript.SigHashAll)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkMultisig_SignatureVerification benchmarks signature verification
func BenchmarkMultisig_SignatureVerification(b *testing.B) {
	// Setup: Create a test multisig transaction with signature
	privKeys := make([]*btcec.PrivateKey, 3)
	pubKeys := make([][]byte, 3)
	for i := range privKeys {
		privKeys[i], _ = btcec.NewPrivateKey()
		pubKeys[i] = privKeys[i].PubKey().SerializeCompressed()
	}

	network := &chaincfg.TestNet3Params
	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000000",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	outputs := map[string]int64{
		"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx": 90000,
	}

	tx, err := CreateMultisigPaymentTx([]UTXO{utxo}, outputs, network)
	if err != nil {
		b.Fatalf("Failed to create transaction: %v", err)
	}
	err = tx.SignMultisigTx(0, privKeys[0], txscript.SigHashAll)
	if err != nil {
		b.Fatalf("Failed to sign transaction: %v", err)
	}

	// Get the signature we just created
	sig := tx.Signatures[0][0]

	b.Run("VerifySingleSignature", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := tx.VerifySignature(0, sig.PublicKey, sig.Signature)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("VerifyMultipleSignatures_3Sigs", func(b *testing.B) {
		// Sign with all 3 keys
		tx.SignMultisigTx(0, privKeys[1], txscript.SigHashAll)
		tx.SignMultisigTx(0, privKeys[2], txscript.SigHashAll)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, sig := range tx.Signatures[0] {
				_, err := tx.VerifySignature(0, sig.PublicKey, sig.Signature)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkMultisig_KeyDerivation benchmarks public key derivation for multisig participants
func BenchmarkMultisig_KeyDerivation(b *testing.B) {
	privKey, _ := btcec.NewPrivateKey()
	masterKey := privKey.Serialize()
	chainCode := make([]byte, 32)
	copy(chainCode, []byte("test_chain_code_32_bytes_long___"))

	b.Run("DeriveIndex1", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := DeriveParticipantKey(masterKey, chainCode, 1)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DeriveIndex1000", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := DeriveParticipantKey(masterKey, chainCode, 1000)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DeriveSequential100", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for idx := uint32(0); idx < 100; idx++ {
				_, err := DeriveParticipantKey(masterKey, chainCode, idx)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// BenchmarkMultisig_TransactionSerialization benchmarks transaction serialization
func BenchmarkMultisig_TransactionSerialization(b *testing.B) {
	privKeys := make([]*btcec.PrivateKey, 3)
	pubKeys := make([][]byte, 3)
	for i := range privKeys {
		privKeys[i], _ = btcec.NewPrivateKey()
		pubKeys[i] = privKeys[i].PubKey().SerializeCompressed()
	}

	network := &chaincfg.TestNet3Params
	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000000",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	outputs := map[string]int64{
		"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx": 90000,
	}

	tx, err := CreateMultisigPaymentTx([]UTXO{utxo}, outputs, network)
	if err != nil {
		b.Fatalf("Failed to create transaction: %v", err)
	}

	b.Run("SerializeUnsigned", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := tx.Serialize()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Sign transaction
	tx.SignMultisigTx(0, privKeys[0], txscript.SigHashAll)
	tx.SignMultisigTx(0, privKeys[1], txscript.SigHashAll)

	b.Run("SerializeSigned", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := tx.Serialize()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("SerializeHex", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := tx.SerializeHex()
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkMultisig_CombineSignatures benchmarks combining partial signatures
func BenchmarkMultisig_CombineSignatures(b *testing.B) {
	privKeys := make([]*btcec.PrivateKey, 5)
	pubKeys := make([][]byte, 5)
	for i := range privKeys {
		privKeys[i], _ = btcec.NewPrivateKey()
		pubKeys[i] = privKeys[i].PubKey().SerializeCompressed()
	}

	network := &chaincfg.TestNet3Params
	redeemScript, _ := BuildRedeemScript(pubKeys, 3)

	utxo := UTXO{
		TxID:         "0000000000000000000000000000000000000000000000000000000000000000",
		Vout:         0,
		Amount:       100000,
		RedeemScript: redeemScript,
	}

	outputs := map[string]int64{
		"tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx": 90000,
	}

	b.Run("Combine3of5Signatures", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Create fresh transaction for each iteration
			tx, err := CreateMultisigPaymentTx([]UTXO{utxo}, outputs, network)
			if err != nil {
				b.Fatalf("Failed to create transaction: %v", err)
			}

			// Collect signatures from 3 participants
			tx.SignMultisigTx(0, privKeys[0], txscript.SigHashAll)
			tx.SignMultisigTx(0, privKeys[2], txscript.SigHashAll)
			tx.SignMultisigTx(0, privKeys[4], txscript.SigHashAll)

			// Verify we have required signatures
			required, collected, _ := tx.GetRequiredSignatures(0)
			if collected < required {
				b.Fatal("insufficient signatures")
			}
		}
	})
}

// BenchmarkMultisig_ValidateRedeemScript benchmarks redeem script validation
func BenchmarkMultisig_ValidateRedeemScript(b *testing.B) {
	pubKeys := make([][]byte, 3)
	for i := range pubKeys {
		privKey, _ := btcec.NewPrivateKey()
		pubKeys[i] = privKey.PubKey().SerializeCompressed()
	}

	redeemScript, _ := BuildRedeemScript(pubKeys, 2)

	b.Run("Validate2of3", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := ValidateRedeemScript(redeemScript)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Larger configuration
	pubKeys10 := make([][]byte, 10)
	for i := range pubKeys10 {
		privKey, _ := btcec.NewPrivateKey()
		pubKeys10[i] = privKey.PubKey().SerializeCompressed()
	}
	redeemScript10, _ := BuildRedeemScript(pubKeys10, 7)

	b.Run("Validate7of10", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := ValidateRedeemScript(redeemScript10)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
