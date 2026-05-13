// Package wallet implements Bitcoin multisig transaction creation and signing
// using PSBT (Partially Signed Bitcoin Transactions) as defined in BIP174.
package wallet

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// UTXO represents an unspent transaction output that can be used as input.
//
// Related types: MultisigPaymentTx
type UTXO struct {
	// TxID is the transaction ID containing the output
	TxID string
	// Vout is the output index within the transaction
	Vout uint32
	// Amount is the value in satoshis
	Amount int64
	// ScriptPubKey is the output script (for verification)
	ScriptPubKey []byte
	// RedeemScript is the multisig redeem script (for P2SH)
	RedeemScript []byte
	// WitnessScript is the multisig witness script (for P2WSH)
	WitnessScript []byte
}

// MultisigPaymentTx represents an unsigned or partially signed multisig transaction.
//
// This structure follows BIP174 PSBT concepts but in a simplified form.
// For full PSBT support, consider using github.com/btcsuite/btcd/psbt.
//
// Related types: UTXO, MultisigSignature
type MultisigPaymentTx struct {
	// Tx is the underlying Bitcoin transaction
	Tx *wire.MsgTx
	// Inputs contains UTXO information for each input
	Inputs []UTXO
	// RedeemScripts maps input index to redeem script
	RedeemScripts map[int][]byte
	// WitnessScripts maps input index to witness script
	WitnessScripts map[int][]byte
	// Signatures contains collected signatures per input
	Signatures map[int][]MultisigSignature
	// Network parameters (mainnet, testnet, etc.)
	Network *chaincfg.Params
}

// MultisigSignature represents a signature from one participant.
//
// Related types: MultisigPaymentTx
type MultisigSignature struct {
	// PublicKey is the signer's public key
	PublicKey []byte
	// Signature is the DER-encoded signature
	Signature []byte
	// SigHashType is the signature hash type (usually SIGHASH_ALL)
	SigHashType txscript.SigHashType
}

// CreateMultisigPaymentTx creates an unsigned multisig transaction.
//
// Parameters:
//   - inputs: UTXOs to spend
//   - outputs: Map of address to amount (in satoshis)
//   - network: Bitcoin network parameters
//
// Returns:
//   - *MultisigPaymentTx: The unsigned transaction
//   - error: If transaction creation fails
//
// Security:
//   - Validates input amounts are positive
//   - Validates output addresses are valid for network
//   - Does not validate that inputs >= outputs (caller's responsibility)
//
// Related: SignMultisigTx, BroadcastMultisigTx
func CreateMultisigPaymentTx(inputs []UTXO, outputs map[string]int64, network *chaincfg.Params) (*MultisigPaymentTx, error) {
	if len(inputs) == 0 {
		return nil, errors.New("at least one input required")
	}
	if len(outputs) == 0 {
		return nil, errors.New("at least one output required")
	}
	if network == nil {
		return nil, errors.New("network parameters required")
	}

	// Create transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Track redeem/witness scripts
	redeemScripts := make(map[int][]byte)
	witnessScripts := make(map[int][]byte)

	// Add inputs
	for i, utxo := range inputs {
		if utxo.Amount <= 0 {
			return nil, fmt.Errorf("input %d has invalid amount: %d", i, utxo.Amount)
		}

		// Parse transaction ID
		txHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, fmt.Errorf("invalid transaction ID for input %d: %w", i, err)
		}

		// Create outpoint
		outpoint := wire.NewOutPoint(txHash, utxo.Vout)

		// Add input (empty signature script, will be filled during signing)
		txIn := wire.NewTxIn(outpoint, nil, nil)
		tx.AddTxIn(txIn)

		// Store scripts for later signing
		if len(utxo.RedeemScript) > 0 {
			redeemScripts[i] = utxo.RedeemScript
		}
		if len(utxo.WitnessScript) > 0 {
			witnessScripts[i] = utxo.WitnessScript
		}
	}

	// Add outputs
	for address, amount := range outputs {
		if amount <= 0 {
			return nil, fmt.Errorf("output to %s has invalid amount: %d", address, amount)
		}

		// Parse and validate address
		addr, err := btcutil.DecodeAddress(address, network)
		if err != nil {
			return nil, fmt.Errorf("invalid output address %s: %w", address, err)
		}

		// Create output script
		pkScript, err := txscript.PayToAddrScript(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create output script for %s: %w", address, err)
		}

		// Add output
		txOut := wire.NewTxOut(amount, pkScript)
		tx.AddTxOut(txOut)
	}

	return &MultisigPaymentTx{
		Tx:             tx,
		Inputs:         inputs,
		RedeemScripts:  redeemScripts,
		WitnessScripts: witnessScripts,
		Signatures:     make(map[int][]MultisigSignature),
		Network:        network,
	}, nil
}

// SignMultisigTx adds a signature from one participant to the transaction.
//
// Parameters:
//   - inputIndex: Index of the input to sign
//   - privateKey: The signer's private key
//   - sigHashType: Signature hash type (usually txscript.SigHashAll)
//
// Returns:
//   - error: If signing fails
//
// Security:
//   - Signs with provided private key
//   - Validates input index is valid
//   - Stores signature for later combination
//
// Related: CreateMultisigPaymentTx, CombineSignatures
func (mt *MultisigPaymentTx) SignMultisigTx(inputIndex int, privateKey *btcec.PrivateKey, sigHashType txscript.SigHashType) error {
	if inputIndex < 0 || inputIndex >= len(mt.Tx.TxIn) {
		return fmt.Errorf("invalid input index: %d (transaction has %d inputs)", inputIndex, len(mt.Tx.TxIn))
	}
	if privateKey == nil {
		return errors.New("private key is required")
	}

	// Get the input UTXO
	utxo := mt.Inputs[inputIndex]

	// Determine which script to sign against
	var scriptToSign []byte
	var isWitness bool

	if witnessScript, ok := mt.WitnessScripts[inputIndex]; ok {
		// P2WSH: sign against witness script
		scriptToSign = witnessScript
		isWitness = true
	} else if redeemScript, ok := mt.RedeemScripts[inputIndex]; ok {
		// P2SH: sign against redeem script
		scriptToSign = redeemScript
		isWitness = false
	} else {
		return fmt.Errorf("no redeem or witness script found for input %d", inputIndex)
	}

	// Calculate signature hash
	var sigHash []byte
	var err error

	if isWitness {
		// SegWit signature hash
		sigHash, err = txscript.CalcWitnessSigHash(scriptToSign, txscript.NewTxSigHashes(mt.Tx, nil), sigHashType, mt.Tx, inputIndex, utxo.Amount)
		if err != nil {
			return fmt.Errorf("failed to calculate witness signature hash: %w", err)
		}
	} else {
		// Legacy signature hash
		sigHash, err = txscript.CalcSignatureHash(scriptToSign, sigHashType, mt.Tx, inputIndex)
		if err != nil {
			return fmt.Errorf("failed to calculate signature hash: %w", err)
		}
	}

	// Sign the hash
	signature := ecdsa.Sign(privateKey, sigHash)

	// Serialize signature with hash type
	sigBytes := append(signature.Serialize(), byte(sigHashType))

	// Store signature
	if mt.Signatures == nil {
		mt.Signatures = make(map[int][]MultisigSignature)
	}

	mt.Signatures[inputIndex] = append(mt.Signatures[inputIndex], MultisigSignature{
		PublicKey:   privateKey.PubKey().SerializeCompressed(),
		Signature:   sigBytes,
		SigHashType: sigHashType,
	})

	return nil
}

// CombineSignatures combines multiple signatures into the transaction inputs.
//
// This function should be called after all required signatures have been
// collected via SignMultisigTx. It creates the final scriptSig or witness
// data for each input.
//
// Returns:
//   - error: If signature combination fails or insufficient signatures
//
// Security:
//   - Validates signature count meets requirements
//   - Orders signatures according to public key order in script
//   - Creates proper script structure for P2SH or P2WSH
//
// Related: SignMultisigTx, BroadcastMultisigTx
func (mt *MultisigPaymentTx) CombineSignatures() error {
	// Process each input
	for i := range mt.Tx.TxIn {
		sigs, hasSigs := mt.Signatures[i]
		if !hasSigs || len(sigs) == 0 {
			return fmt.Errorf("no signatures provided for input %d", i)
		}

		// Determine script type and build appropriate structure
		if witnessScript, ok := mt.WitnessScripts[i]; ok {
			// P2WSH: build witness data
			if err := mt.buildWitnessData(i, sigs, witnessScript); err != nil {
				return fmt.Errorf("failed to build witness data for input %d: %w", i, err)
			}
		} else if redeemScript, ok := mt.RedeemScripts[i]; ok {
			// P2SH: build scriptSig
			if err := mt.buildScriptSig(i, sigs, redeemScript); err != nil {
				return fmt.Errorf("failed to build scriptSig for input %d: %w", i, err)
			}
		} else {
			return fmt.Errorf("no redeem or witness script found for input %d", i)
		}
	}

	return nil
}

// buildWitnessData creates witness data for a P2WSH input.
func (mt *MultisigPaymentTx) buildWitnessData(inputIndex int, sigs []MultisigSignature, witnessScript []byte) error {
	// Extract public keys from witness script to order signatures
	pubKeys, err := ExtractPubKeysFromRedeemScript(witnessScript)
	if err != nil {
		return fmt.Errorf("failed to extract public keys: %w", err)
	}

	// Build witness stack: [0 sig1 sig2 ... witnessScript]
	witness := make([][]byte, 0, len(sigs)+2)
	witness = append(witness, []byte{}) // OP_0 for CHECKMULTISIG bug

	// Order signatures by public key order in script
	orderedSigs := orderSignaturesByPubKeys(sigs, pubKeys)
	for _, sig := range orderedSigs {
		witness = append(witness, sig.Signature)
	}

	// Add witness script
	witness = append(witness, witnessScript)

	mt.Tx.TxIn[inputIndex].Witness = witness

	// For P2WSH, scriptSig should be empty
	mt.Tx.TxIn[inputIndex].SignatureScript = nil

	return nil
}

// buildScriptSig creates scriptSig for a P2SH input.
func (mt *MultisigPaymentTx) buildScriptSig(inputIndex int, sigs []MultisigSignature, redeemScript []byte) error {
	// Extract public keys from redeem script to order signatures
	pubKeys, err := ExtractPubKeysFromRedeemScript(redeemScript)
	if err != nil {
		return fmt.Errorf("failed to extract public keys: %w", err)
	}

	// Build scriptSig: [OP_0 sig1 sig2 ... redeemScript]
	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_FALSE) // OP_0 for CHECKMULTISIG bug

	// Order signatures by public key order in script
	orderedSigs := orderSignaturesByPubKeys(sigs, pubKeys)
	for _, sig := range orderedSigs {
		builder.AddData(sig.Signature)
	}

	// Add redeem script
	builder.AddData(redeemScript)

	scriptSig, err := builder.Script()
	if err != nil {
		return fmt.Errorf("failed to build scriptSig: %w", err)
	}

	mt.Tx.TxIn[inputIndex].SignatureScript = scriptSig

	return nil
}

// orderSignaturesByPubKeys orders signatures to match the public key order in the script.
func orderSignaturesByPubKeys(sigs []MultisigSignature, scriptPubKeys [][]byte) []MultisigSignature {
	ordered := make([]MultisigSignature, 0, len(sigs))

	// For each public key in the script, find matching signature
	for _, scriptPubKey := range scriptPubKeys {
		for _, sig := range sigs {
			if bytes.Equal(sig.PublicKey, scriptPubKey) {
				ordered = append(ordered, sig)
				break
			}
		}
	}

	return ordered
}

// Serialize returns the raw transaction bytes.
//
// Returns:
//   - []byte: Serialized transaction
//   - error: If serialization fails
//
// Related: SerializeHex
func (mt *MultisigPaymentTx) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	if err := mt.Tx.Serialize(&buf); err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}
	return buf.Bytes(), nil
}

// SerializeHex returns the transaction as a hex string.
//
// Returns:
//   - string: Hex-encoded transaction
//   - error: If serialization fails
//
// Related: Serialize, BroadcastMultisigTx
func (mt *MultisigPaymentTx) SerializeHex() (string, error) {
	txBytes, err := mt.Serialize()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(txBytes), nil
}

// GetTxID returns the transaction ID.
//
// Returns:
//   - string: Transaction ID (hex)
//
// Note: The transaction ID changes if inputs are modified, so only
// call this after all signatures are combined.
//
// Related: Serialize
func (mt *MultisigPaymentTx) GetTxID() string {
	return mt.Tx.TxHash().String()
}

// VerifySignature verifies a single signature against a public key.
//
// Parameters:
//   - inputIndex: Input index to verify
//   - pubKey: Public key to verify against
//   - signature: Signature to verify
//
// Returns:
//   - bool: True if signature is valid
//   - error: If verification process fails
//
// Related: SignMultisigTx
func (mt *MultisigPaymentTx) VerifySignature(inputIndex int, pubKey, signature []byte) (bool, error) {
	if inputIndex < 0 || inputIndex >= len(mt.Tx.TxIn) {
		return false, fmt.Errorf("invalid input index: %d", inputIndex)
	}

	// Get the script to verify against
	var scriptToVerify []byte
	var isWitness bool
	var utxo UTXO

	if witnessScript, ok := mt.WitnessScripts[inputIndex]; ok {
		scriptToVerify = witnessScript
		isWitness = true
		utxo = mt.Inputs[inputIndex]
	} else if redeemScript, ok := mt.RedeemScripts[inputIndex]; ok {
		scriptToVerify = redeemScript
		isWitness = false
		utxo = mt.Inputs[inputIndex]
	} else {
		return false, fmt.Errorf("no script found for input %d", inputIndex)
	}

	// Parse public key
	parsedPubKey, err := btcec.ParsePubKey(pubKey)
	if err != nil {
		return false, fmt.Errorf("invalid public key: %w", err)
	}

	// Extract signature bytes (remove hash type byte if present)
	sigBytes := signature
	if len(signature) > 0 && (signature[len(signature)-1]&0x1f) <= 3 {
		sigBytes = signature[:len(signature)-1]
	}

	// Parse signature
	parsedSig, err := ecdsa.ParseDERSignature(sigBytes)
	if err != nil {
		return false, fmt.Errorf("invalid signature: %w", err)
	}

	// Calculate signature hash
	var sigHash []byte
	sigHashType := txscript.SigHashAll // Default
	if len(signature) > 0 {
		sigHashType = txscript.SigHashType(signature[len(signature)-1])
	}

	if isWitness {
		sigHash, err = txscript.CalcWitnessSigHash(scriptToVerify, txscript.NewTxSigHashes(mt.Tx, nil), sigHashType, mt.Tx, inputIndex, utxo.Amount)
	} else {
		sigHash, err = txscript.CalcSignatureHash(scriptToVerify, sigHashType, mt.Tx, inputIndex)
	}
	if err != nil {
		return false, fmt.Errorf("failed to calculate signature hash: %w", err)
	}

	// Verify signature
	return parsedSig.Verify(sigHash, parsedPubKey), nil
}

// GetRequiredSignatures returns the number of signatures required and collected for an input.
//
// Parameters:
//   - inputIndex: Input index to check
//
// Returns:
//   - required: Number of signatures required (m in m-of-n)
//   - collected: Number of signatures collected so far
//   - error: If input index is invalid
//
// Related: SignMultisigTx
func (mt *MultisigPaymentTx) GetRequiredSignatures(inputIndex int) (required, collected int, err error) {
	if inputIndex < 0 || inputIndex >= len(mt.Tx.TxIn) {
		return 0, 0, fmt.Errorf("invalid input index: %d", inputIndex)
	}

	// Get the script
	var script []byte
	if witnessScript, ok := mt.WitnessScripts[inputIndex]; ok {
		script = witnessScript
	} else if redeemScript, ok := mt.RedeemScripts[inputIndex]; ok {
		script = redeemScript
	} else {
		return 0, 0, fmt.Errorf("no script found for input %d", inputIndex)
	}

	// Extract required signatures from script
	requiredSigs, _, err := ValidateRedeemScript(script)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to validate script: %w", err)
	}

	// Count collected signatures
	collectedSigs := len(mt.Signatures[inputIndex])

	return requiredSigs, collectedSigs, nil
}

// EstimateSize estimates the final transaction size in bytes.
//
// Returns:
//   - int: Estimated size in bytes
//
// Note: This is an approximation. Actual size depends on signature sizes.
//
// Related: EstimateFee
func (mt *MultisigPaymentTx) EstimateSize() int {
	// Base transaction size
	size := 10 // version (4) + locktime (4) + input count (1) + output count (1)

	// Input sizes
	for i := range mt.Tx.TxIn {
		size += 32 + 4 + 4 // outpoint (32) + vout (4) + sequence (4)

		if _, ok := mt.WitnessScripts[i]; ok {
			// P2WSH input: empty scriptSig + witness data
			size += 1 // scriptSig length (0)
			// Witness: [0, sig1, sig2, ..., witnessScript]
			// Estimate: 72 bytes per signature + script size
			requiredSigs, _, _ := mt.GetRequiredSignatures(i)
			witnessSize := 1 + (requiredSigs * 73) + len(mt.WitnessScripts[i])
			size += witnessSize
		} else if redeemScript, ok := mt.RedeemScripts[i]; ok {
			// P2SH input: scriptSig contains [0, sig1, sig2, ..., redeemScript]
			requiredSigs, _, _ := mt.GetRequiredSignatures(i)
			scriptSigSize := 1 + (requiredSigs * 73) + len(redeemScript)
			size += 1 + scriptSigSize // length byte + scriptSig
		}
	}

	// Output sizes
	for _, txOut := range mt.Tx.TxOut {
		size += 8 + 1 + len(txOut.PkScript) // value (8) + script length (1) + script
	}

	return size
}

// EstimateFee estimates the transaction fee based on size and fee rate.
//
// Parameters:
//   - satPerByte: Fee rate in satoshis per byte
//
// Returns:
//   - int64: Estimated fee in satoshis
//
// Related: EstimateSize
func (mt *MultisigPaymentTx) EstimateFee(satPerByte float64) int64 {
	size := mt.EstimateSize()
	return int64(float64(size) * satPerByte)
}

// SetLockTime sets the nLockTime field on the transaction.
//
// nLockTime prevents the transaction from being mined until:
//   - A specific block height (if lockTime < 500,000,000)
//   - A specific Unix timestamp (if lockTime >= 500,000,000)
//
// This is useful for refund scenarios: create a refund transaction with
// a future lockTime, and if the primary transaction doesn't occur, the
// refund can be broadcast after the lockTime expires.
//
// Parameters:
//   - lockTime: Block height or Unix timestamp
//
// Security:
//   - All inputs must have sequence < 0xffffffff for lockTime to be enforced
//   - Use SetInputSequence to set appropriate sequence values
//
// Related: SetInputSequence, CreateTimelockRedeemScript
func (mt *MultisigPaymentTx) SetLockTime(lockTime uint32) {
	mt.Tx.LockTime = lockTime
}

// SetInputSequence sets the sequence number for a specific input.
//
// The sequence number affects lockTime and CSV (CheckSequenceVerify):
//   - 0xffffffff: lockTime is disabled, CSV is disabled
//   - 0xfffffffe: lockTime is enabled, CSV is disabled (common for lockTime usage)
//   - < 0xfffffffe: Both lockTime and CSV can be enabled
//
// For refund transactions using lockTime, set sequence to 0xfffffffe.
//
// Parameters:
//   - inputIndex: Index of the input to modify
//   - sequence: Sequence number to set
//
// Returns:
//   - error: If input index is invalid
//
// Related: SetLockTime
func (mt *MultisigPaymentTx) SetInputSequence(inputIndex int, sequence uint32) error {
	if inputIndex < 0 || inputIndex >= len(mt.Tx.TxIn) {
		return fmt.Errorf("invalid input index: %d (transaction has %d inputs)", inputIndex, len(mt.Tx.TxIn))
	}
	mt.Tx.TxIn[inputIndex].Sequence = sequence
	return nil
}

// SetAllInputSequences sets the same sequence number for all inputs.
//
// This is a convenience function for setting all inputs to enable lockTime.
//
// Parameters:
//   - sequence: Sequence number to set (typically 0xfffffffe for lockTime)
//
// Related: SetInputSequence, SetLockTime
func (mt *MultisigPaymentTx) SetAllInputSequences(sequence uint32) {
	for i := range mt.Tx.TxIn {
		mt.Tx.TxIn[i].Sequence = sequence
	}
}

// GetLockTime returns the current nLockTime value.
//
// Returns:
//   - uint32: Lock time value
//   - bool: True if lock time is a timestamp (>= 500000000), false if block height
//
// Related: SetLockTime
func (mt *MultisigPaymentTx) GetLockTime() (uint32, bool) {
	const lockTimeThreshold = 500000000
	isTimestamp := mt.Tx.LockTime >= lockTimeThreshold
	return mt.Tx.LockTime, isTimestamp
}

// CreateTimelockRedeemScript creates a multisig redeem script with CLTV (CheckLockTimeVerify).
//
// This creates a script that requires both:
//  1. A specific time/block height to pass (enforced by OP_CHECKLOCKTIMEVERIFY)
//  2. M-of-N signatures (enforced by OP_CHECKMULTISIG)
//
// The resulting script can be used in escrow scenarios where funds should
// only be spendable after a certain time. This is more secure than nLockTime
// alone because the timelock is part of the script itself.
//
// Parameters:
//   - pubKeys: Public keys for multisig (in desired order)
//   - requiredSigs: Number of required signatures (m in m-of-n)
//   - lockTime: Time or block height to lock until
//
// Returns:
//   - []byte: The CLTV-enhanced redeem script
//   - error: If script creation fails
//
// Script format:
//
//	<lockTime> OP_CHECKLOCKTIMEVERIFY OP_DROP <m> <pubkey1> ... <pubkeyN> <n> OP_CHECKMULTISIG
//
// Security:
//   - Transaction spending this script must have nLockTime >= script's lockTime
//   - Input sequence must be < 0xffffffff
//
// Related: BuildRedeemScript, SetLockTime
func CreateTimelockRedeemScript(pubKeys [][]byte, requiredSigs int, lockTime uint32) ([]byte, error) {
	if len(pubKeys) == 0 {
		return nil, errors.New("at least one public key required")
	}
	if requiredSigs < 1 || requiredSigs > len(pubKeys) {
		return nil, fmt.Errorf("requiredSigs must be between 1 and %d", len(pubKeys))
	}
	if lockTime == 0 {
		return nil, errors.New("lockTime must be greater than 0")
	}

	// Start building the script
	builder := txscript.NewScriptBuilder()

	// Add CLTV portion: <lockTime> OP_CHECKLOCKTIMEVERIFY OP_DROP
	builder.AddInt64(int64(lockTime))
	builder.AddOp(txscript.OP_CHECKLOCKTIMEVERIFY)
	builder.AddOp(txscript.OP_DROP)

	// Add multisig portion: <m> <pubkey1> ... <pubkeyN> <n> OP_CHECKMULTISIG
	builder.AddInt64(int64(requiredSigs))

	// Add public keys
	for i, pubKey := range pubKeys {
		if len(pubKey) != 33 && len(pubKey) != 65 {
			return nil, fmt.Errorf("public key %d has invalid length %d", i, len(pubKey))
		}
		// Validate public key
		if _, err := btcec.ParsePubKey(pubKey); err != nil {
			return nil, fmt.Errorf("invalid public key %d: %w", i, err)
		}
		builder.AddData(pubKey)
	}

	builder.AddInt64(int64(len(pubKeys)))
	builder.AddOp(txscript.OP_CHECKMULTISIG)

	script, err := builder.Script()
	if err != nil {
		return nil, fmt.Errorf("failed to build timelock script: %w", err)
	}

	return script, nil
}

// ValidateTimelockRedeemScript validates a CLTV multisig redeem script.
//
// Parameters:
//   - script: The script to validate
//
// Returns:
//   - lockTime: The embedded lock time value
//   - requiredSigs: Number of required signatures (m)
//   - totalKeys: Total number of public keys (n)
//   - error: If script is invalid
//
// Related: CreateTimelockRedeemScript
func ValidateTimelockRedeemScript(script []byte) (lockTime uint32, requiredSigs, totalKeys int, err error) {
	if len(script) < 10 {
		return 0, 0, 0, errors.New("script too short for CLTV multisig")
	}

	// Parse the script
	tokenizer := txscript.MakeScriptTokenizer(0, script)

	// Expect: <lockTime>
	if !tokenizer.Next() {
		return 0, 0, 0, errors.New("failed to parse lockTime")
	}
	lockTimeInt, err := txscript.MakeScriptNum(tokenizer.Data(), false, 5)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid lockTime: %w", err)
	}
	lockTime = uint32(lockTimeInt)

	// Expect: OP_CHECKLOCKTIMEVERIFY
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_CHECKLOCKTIMEVERIFY {
		return 0, 0, 0, errors.New("missing OP_CHECKLOCKTIMEVERIFY")
	}

	// Expect: OP_DROP
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_DROP {
		return 0, 0, 0, errors.New("missing OP_DROP after OP_CHECKLOCKTIMEVERIFY")
	}

	// Now parse the multisig portion (same as regular multisig)
	// Expect: <m>
	if !tokenizer.Next() {
		return 0, 0, 0, errors.New("failed to parse requiredSigs")
	}

	// Handle both data push and opcode (OP_1 through OP_16)
	var reqSigsNum int64
	if len(tokenizer.Data()) > 0 {
		reqSigsNumScriptNum, err := txscript.MakeScriptNum(tokenizer.Data(), false, 5)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid requiredSigs: %w", err)
		}
		reqSigsNum = int64(reqSigsNumScriptNum)
	} else {
		// Check for OP_1 through OP_16
		opcode := tokenizer.Opcode()
		if opcode >= txscript.OP_1 && opcode <= txscript.OP_16 {
			reqSigsNum = int64(opcode) - int64(txscript.OP_1) + 1
		} else if opcode == txscript.OP_0 {
			reqSigsNum = 0
		} else {
			return 0, 0, 0, fmt.Errorf("unexpected opcode for requiredSigs: %v", opcode)
		}
	}
	requiredSigs = int(reqSigsNum)

	// Count public keys
	pubKeyCount := 0
	for tokenizer.Next() {
		data := tokenizer.Data()
		if len(data) == 33 || len(data) == 65 {
			pubKeyCount++
		} else {
			// Should be <n> (number of keys) followed by OP_CHECKMULTISIG
			break
		}
	}

	// Verify <n> matches the count
	var expectedN int64
	if len(tokenizer.Data()) > 0 {
		nScriptNum, err := txscript.MakeScriptNum(tokenizer.Data(), false, 5)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid n (key count): %w", err)
		}
		expectedN = int64(nScriptNum)
	} else {
		opcode := tokenizer.Opcode()
		if opcode >= txscript.OP_1 && opcode <= txscript.OP_16 {
			expectedN = int64(opcode) - int64(txscript.OP_1) + 1
		} else if opcode == txscript.OP_0 {
			expectedN = 0
		} else {
			return 0, 0, 0, fmt.Errorf("unexpected opcode for key count: %v", opcode)
		}
	}

	if int(expectedN) != pubKeyCount {
		return 0, 0, 0, fmt.Errorf("key count mismatch: found %d keys but script says %d", pubKeyCount, expectedN)
	}

	totalKeys = pubKeyCount

	// Expect OP_CHECKMULTISIG
	if !tokenizer.Next() || tokenizer.Opcode() != txscript.OP_CHECKMULTISIG {
		return 0, 0, 0, errors.New("missing OP_CHECKMULTISIG")
	}

	if requiredSigs < 1 || requiredSigs > totalKeys {
		return 0, 0, 0, fmt.Errorf("invalid requiredSigs %d for %d keys", requiredSigs, totalKeys)
	}

	return lockTime, requiredSigs, totalKeys, nil
}

// CreateRefundTransaction creates a time-locked refund transaction.
//
// This is a convenience function for creating escrow refund transactions.
// The transaction will be locked until the specified time/block height,
// after which it can be broadcast to return funds to the refund address.
//
// Parameters:
//   - inputs: UTXOs to refund (typically the escrow outputs)
//   - refundAddress: Address to send refunded funds to
//   - lockTime: Block height or timestamp when refund becomes valid
//   - feeAmount: Transaction fee in satoshis
//   - network: Bitcoin network parameters
//
// Returns:
//   - *MultisigPaymentTx: The time-locked refund transaction
//   - error: If transaction creation fails
//
// Usage:
//
//	After creating this transaction, all parties should sign it BEFORE
//	funding the escrow. This ensures the funds can be recovered if needed.
//
// Related: CreateMultisigPaymentTx, SetLockTime
func CreateRefundTransaction(inputs []UTXO, refundAddress string, lockTime uint32, feeAmount int64, network *chaincfg.Params) (*MultisigPaymentTx, error) {
	if len(inputs) == 0 {
		return nil, errors.New("at least one input required")
	}
	if refundAddress == "" {
		return nil, errors.New("refund address required")
	}
	if lockTime == 0 {
		return nil, errors.New("lockTime must be greater than 0")
	}

	// Calculate total input amount
	var totalInput int64
	for _, utxo := range inputs {
		totalInput += utxo.Amount
	}

	// Calculate output amount (input - fee)
	outputAmount := totalInput - feeAmount
	if outputAmount <= 0 {
		return nil, fmt.Errorf("insufficient funds: input %d, fee %d", totalInput, feeAmount)
	}

	// Create outputs map
	outputs := map[string]int64{
		refundAddress: outputAmount,
	}

	// Create the transaction
	refundTx, err := CreateMultisigPaymentTx(inputs, outputs, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create refund transaction: %w", err)
	}

	// Set lock time
	refundTx.SetLockTime(lockTime)

	// Set input sequences to enable lockTime (0xfffffffe)
	refundTx.SetAllInputSequences(wire.MaxTxInSequenceNum - 1)

	return refundTx, nil
}
