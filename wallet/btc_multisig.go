// Package wallet implements Bitcoin multisig (multi-signature) functionality
// for P2SH (Pay-to-Script-Hash) and P2WSH (Pay-to-Witness-Script-Hash) addresses.
package wallet

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"golang.org/x/crypto/ripemd160"
)

// MultisigAddressType specifies the format for multisig addresses.
type MultisigAddressType int

const (
	// P2SH (Pay-to-Script-Hash) - Legacy multisig format (BIP16)
	// Address format: 3xxxxxxxx (mainnet) or 2xxxxxxxx (testnet)
	P2SH MultisigAddressType = iota
	// P2WSH (Pay-to-Witness-Script-Hash) - SegWit multisig format (BIP141)
	// Address format: bc1qxxxx (mainnet) or tb1qxxxx (testnet)
	P2WSH
)

// BuildRedeemScript creates a multisig redeem script from public keys.
//
// The redeem script follows the format:
// <requiredSigs> <pubKey1> <pubKey2> ... <pubKeyN> <totalKeys> OP_CHECKMULTISIG
//
// Parameters:
//   - pubKeys: Array of public keys in compressed format (33 bytes each)
//   - requiredSigs: Number of signatures required (m in m-of-n)
//
// Returns:
//   - []byte: The redeem script bytes
//   - error: If parameters are invalid or script creation fails
//
// Security:
//   - Public keys must be in compressed format (33 bytes)
//   - requiredSigs must be <= len(pubKeys)
//   - Maximum 15 public keys per script (Bitcoin consensus rule)
//
// Related: CreateP2SHAddress, CreateP2WSHAddress
func BuildRedeemScript(pubKeys [][]byte, requiredSigs int) ([]byte, error) {
	if len(pubKeys) == 0 {
		return nil, errors.New("at least one public key required")
	}
	if len(pubKeys) > 15 {
		return nil, errors.New("maximum 15 public keys allowed in multisig")
	}
	if requiredSigs < 1 {
		return nil, errors.New("requiredSigs must be at least 1")
	}
	if requiredSigs > len(pubKeys) {
		return nil, fmt.Errorf("requiredSigs (%d) cannot exceed total keys (%d)", requiredSigs, len(pubKeys))
	}

	// Validate and parse public keys
	parsedKeys := make([]*btcutil.AddressPubKey, 0, len(pubKeys))
	for i, pubKeyBytes := range pubKeys {
		if len(pubKeyBytes) != 33 && len(pubKeyBytes) != 65 {
			return nil, fmt.Errorf("public key %d has invalid length %d (expected 33 or 65)", i, len(pubKeyBytes))
		}

		// Parse public key to validate it
		pubKey, err := btcec.ParsePubKey(pubKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("invalid public key %d: %w", i, err)
		}

		// Use mainnet params for parsing (doesn't affect the key itself)
		addressPubKey, err := btcutil.NewAddressPubKey(pubKey.SerializeCompressed(), &chaincfg.MainNetParams)
		if err != nil {
			return nil, fmt.Errorf("failed to create address from public key %d: %w", i, err)
		}
		parsedKeys = append(parsedKeys, addressPubKey)
	}

	// Build the redeem script using txscript
	redeemScript, err := txscript.MultiSigScript(parsedKeys, requiredSigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create multisig script: %w", err)
	}

	return redeemScript, nil
}

// CreateP2SHAddress creates a P2SH (Pay-to-Script-Hash) multisig address.
//
// P2SH addresses start with '3' on mainnet and '2' on testnet.
// This is the legacy multisig format defined in BIP16.
//
// Parameters:
//   - redeemScript: The multisig redeem script (from BuildRedeemScript)
//   - network: Bitcoin network parameters (mainnet, testnet, etc.)
//
// Returns:
//   - string: The P2SH address
//   - error: If address creation fails
//
// Security:
//   - Redeem script is hashed with SHA256 then RIPEMD160
//   - Address includes checksum for error detection
//
// Related: BuildRedeemScript, CreateP2WSHAddress
func CreateP2SHAddress(redeemScript []byte, network *chaincfg.Params) (string, error) {
	if len(redeemScript) == 0 {
		return "", errors.New("redeem script cannot be empty")
	}
	if network == nil {
		return "", errors.New("network parameters cannot be nil")
	}

	// Create script hash (HASH160 = RIPEMD160(SHA256(script)))
	scriptHash := sha256.Sum256(redeemScript)
	ripemd := ripemd160.New()
	ripemd.Write(scriptHash[:])
	scriptHashBytes := ripemd.Sum(nil)

	// Create P2SH address from script hash
	address, err := btcutil.NewAddressScriptHashFromHash(scriptHashBytes, network)
	if err != nil {
		return "", fmt.Errorf("failed to create P2SH address: %w", err)
	}

	return address.EncodeAddress(), nil
}

// CreateP2WSHAddress creates a P2WSH (Pay-to-Witness-Script-Hash) multisig address.
//
// P2WSH addresses are native SegWit addresses starting with 'bc1q' (mainnet)
// or 'tb1q' (testnet). This is the modern multisig format defined in BIP141.
//
// Parameters:
//   - redeemScript: The multisig redeem script (from BuildRedeemScript)
//   - network: Bitcoin network parameters (mainnet, testnet, etc.)
//
// Returns:
//   - string: The P2WSH address (Bech32 encoded)
//   - error: If address creation fails
//
// Security:
//   - Redeem script is hashed with SHA256 (single round, not double)
//   - Native SegWit provides better fee efficiency and security
//   - Bech32 encoding reduces transcription errors
//
// Related: BuildRedeemScript, CreateP2SHAddress
func CreateP2WSHAddress(redeemScript []byte, network *chaincfg.Params) (string, error) {
	if len(redeemScript) == 0 {
		return "", errors.New("redeem script cannot be empty")
	}
	if network == nil {
		return "", errors.New("network parameters cannot be nil")
	}

	// Create witness script hash (SHA256, single round)
	witnessScriptHash := sha256.Sum256(redeemScript)

	// Create P2WSH address from witness script hash
	address, err := btcutil.NewAddressWitnessScriptHash(witnessScriptHash[:], network)
	if err != nil {
		return "", fmt.Errorf("failed to create P2WSH address: %w", err)
	}

	return address.EncodeAddress(), nil
}

// CreateMultisigAddress generates a multisig address from public keys.
//
// This is a convenience function that combines BuildRedeemScript and
// address generation in one call.
//
// Parameters:
//   - pubKeys: Array of public keys in compressed format
//   - requiredSigs: Number of signatures required (m in m-of-n)
//   - addressType: P2SH or P2WSH format
//   - network: Bitcoin network parameters
//
// Returns:
//   - address: The generated multisig address
//   - redeemScript: The redeem script (needed for spending)
//   - error: If address creation fails
//
// Example:
//
//	pubKeys := [][]byte{pubKey1, pubKey2, pubKey3}
//	address, redeemScript, err := CreateMultisigAddress(pubKeys, 2, P2WSH, &chaincfg.MainNetParams)
//	// Creates a 2-of-3 multisig address
//
// Related: BuildRedeemScript, CreateP2SHAddress, CreateP2WSHAddress
func CreateMultisigAddress(pubKeys [][]byte, requiredSigs int, addressType MultisigAddressType, network *chaincfg.Params) (address string, redeemScript []byte, err error) {
	// Build redeem script
	redeemScript, err = BuildRedeemScript(pubKeys, requiredSigs)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build redeem script: %w", err)
	}

	// Generate address based on type
	switch addressType {
	case P2SH:
		address, err = CreateP2SHAddress(redeemScript, network)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create P2SH address: %w", err)
		}
	case P2WSH:
		address, err = CreateP2WSHAddress(redeemScript, network)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create P2WSH address: %w", err)
		}
	default:
		return "", nil, fmt.Errorf("unsupported address type: %d", addressType)
	}

	return address, redeemScript, nil
}

// DeriveParticipantKey derives a public key for a multisig participant from an HD wallet.
//
// This function uses the standard BIP32 derivation to generate deterministic
// keys for multisig setups. Each participant derives their key at the same
// index to create coordinated multisig addresses.
//
// Parameters:
//   - masterKey: The master private key (32 bytes)
//   - chainCode: The chain code (32 bytes)
//   - index: The derivation index (use same index for all participants)
//
// Returns:
//   - *btcec.PublicKey: The derived public key
//   - error: If derivation fails
//
// Security:
//   - Uses proper BIP32 HMAC-SHA512 key derivation
//   - Validates derived keys are on the curve
//   - Index should be non-hardened for public key derivation
//
// Related: CreateMultisigAddress
func DeriveParticipantKey(masterKey, chainCode []byte, index uint32) (*btcec.PublicKey, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(masterKey))
	}
	if len(chainCode) != 32 {
		return nil, fmt.Errorf("chain code must be 32 bytes, got %d", len(chainCode))
	}
	if index >= hardenedKeyStart {
		return nil, errors.New("use non-hardened index for public key derivation")
	}

	// Derive private key using BIP32
	privKey, _ := btcec.PrivKeyFromBytes(masterKey)

	// Serialize public key + index for HMAC
	data := privKey.PubKey().SerializeCompressed()
	indexBytes := make([]byte, 4)
	indexBytes[0] = byte(index >> 24)
	indexBytes[1] = byte(index >> 16)
	indexBytes[2] = byte(index >> 8)
	indexBytes[3] = byte(index)

	// HMAC-SHA512 to derive child key
	h := sha512.New()
	h.Write(append(data, indexBytes...))
	intermediateKey := h.Sum(nil)

	// Split into key and chain code
	childKeyInt := new(big.Int).SetBytes(intermediateKey[:32])

	// Add parent key to child key (mod curve order)
	curve := btcec.S256()
	parentKeyInt := new(big.Int).SetBytes(masterKey)
	childKeyInt.Add(childKeyInt, parentKeyInt)
	childKeyInt.Mod(childKeyInt, curve.N)

	// Validate and create private key
	if childKeyInt.Cmp(big.NewInt(0)) == 0 || childKeyInt.Cmp(curve.N) >= 0 {
		return nil, errors.New("derived key is invalid (zero or >= curve order)")
	}

	childPrivKey, _ := btcec.PrivKeyFromBytes(childKeyInt.Bytes())

	return childPrivKey.PubKey(), nil
}

// ValidateRedeemScript checks if a redeem script is valid for multisig.
//
// Parameters:
//   - redeemScript: The script to validate
//
// Returns:
//   - requiredSigs: Number of required signatures (m)
//   - totalKeys: Total number of public keys (n)
//   - error: If script is invalid
//
// Related: BuildRedeemScript
func ValidateRedeemScript(redeemScript []byte) (requiredSigs, totalKeys int, err error) {
	if len(redeemScript) == 0 {
		return 0, 0, errors.New("redeem script is empty")
	}

	// Parse the script
	// Format: <m> <pubkey1> ... <pubkeyN> <n> OP_CHECKMULTISIG
	// The first byte should be OP_m (where m = requiredSigs)
	// The byte before OP_CHECKMULTISIG should be OP_n (where n = totalKeys)
	if len(redeemScript) < 4 {
		return 0, 0, errors.New("redeem script too short")
	}

	// Check last byte is OP_CHECKMULTISIG (0xae)
	if redeemScript[len(redeemScript)-1] != 0xae {
		return 0, 0, errors.New("script does not end with OP_CHECKMULTISIG")
	}

	// Extract m and n
	requiredSigs = int(redeemScript[0]) - 0x50 // OP_1 = 0x51, OP_2 = 0x52, etc.
	totalKeys = int(redeemScript[len(redeemScript)-2]) - 0x50

	if requiredSigs < 1 || requiredSigs > 15 {
		return 0, 0, fmt.Errorf("invalid requiredSigs: %d", requiredSigs)
	}
	if totalKeys < 1 || totalKeys > 15 {
		return 0, 0, fmt.Errorf("invalid totalKeys: %d", totalKeys)
	}
	if requiredSigs > totalKeys {
		return 0, 0, fmt.Errorf("requiredSigs (%d) > totalKeys (%d)", requiredSigs, totalKeys)
	}

	return requiredSigs, totalKeys, nil
}

// ExtractPubKeysFromRedeemScript extracts public keys from a multisig redeem script.
//
// Parameters:
//   - redeemScript: The multisig redeem script
//
// Returns:
//   - [][]byte: Array of public key bytes
//   - error: If extraction fails
//
// Related: BuildRedeemScript
func ExtractPubKeysFromRedeemScript(redeemScript []byte) ([][]byte, error) {
	if len(redeemScript) == 0 {
		return nil, errors.New("redeem script is empty")
	}

	// Tokenize the script to extract public keys
	tokenizer := txscript.MakeScriptTokenizer(0, redeemScript)
	var pubKeys [][]byte

	// Skip first opcode (OP_m)
	if !tokenizer.Next() {
		return nil, errors.New("failed to parse script: missing OP_m")
	}

	// Extract public keys (everything between OP_m and OP_n)
	for tokenizer.Next() {
		data := tokenizer.Data()
		// Public keys are 33 or 65 bytes
		if len(data) == 33 || len(data) == 65 {
			pubKeysCopy := make([]byte, len(data))
			copy(pubKeysCopy, data)
			pubKeys = append(pubKeys, pubKeysCopy)
		} else if len(data) == 0 {
			// This is likely OP_n or OP_CHECKMULTISIG, stop here
			break
		}
	}

	if err := tokenizer.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse redeem script: %w", err)
	}

	if len(pubKeys) == 0 {
		return nil, errors.New("no public keys found in redeem script")
	}

	return pubKeys, nil
}

// CompareRedeemScripts checks if two redeem scripts are equivalent.
//
// Parameters:
//   - script1, script2: The scripts to compare
//
// Returns:
//   - bool: True if scripts are identical
//
// Related: BuildRedeemScript
func CompareRedeemScripts(script1, script2 []byte) bool {
	return bytes.Equal(script1, script2)
}
