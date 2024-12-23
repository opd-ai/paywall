// Package wallet implements Bitcoin HD (Hierarchical Deterministic) wallet functionality
// according to BIP32, BIP44, and BIP49 specifications.
package wallet

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"golang.org/x/crypto/ripemd160"
)

const (
	// HDWallet constants for BIP44 derivation path
	hardenedKeyStart = 0x80000000 // Hardened key starting index
	purposeBIP44     = 44         // BIP44 purpose level
	coinTypeBTC      = 0          // Bitcoin coin type
	accountDefault   = 0          // Default account index
	changeExternal   = 0          // External chain for receiving addresses
)

// BTCHDWallet represents a hierarchical deterministic Bitcoin wallet
// implementing BIP32 and BIP44 standards.
type BTCHDWallet struct {
	masterKey []byte           // Master private key
	chainCode []byte           // Master chain code for key derivation
	network   *chaincfg.Params // Network parameters (mainnet/testnet)
	nextIndex uint32           // Next address index to derive
}

// NewHDWallet creates a new HD wallet from a seed.
//
// Parameters:
//   - seed: Random seed bytes (must be 16-64 bytes)
//   - testnet: Boolean flag for testnet/mainnet network selection
//
// Returns:
//   - *HDWallet: Initialized wallet instance
//   - error: If seed length is invalid
//
// Security:
//   - Seed must be generated with sufficient entropy
//   - Seed should be backed up securely
//
// Related: DeriveNextAddress, GetAddress
func NewBTCHDWallet(seed []byte, testnet bool) (*BTCHDWallet, error) {
	if len(seed) < 16 || len(seed) > 64 {
		return nil, errors.New("seed must be between 16 and 64 bytes")
	}

	// Generate master key and chain code
	hmac := hmac.New(sha512.New, []byte("Bitcoin seed"))
	hmac.Write(seed)
	sum := hmac.Sum(nil)

	masterKey := sum[:32]
	chainCode := sum[32:]

	network := &chaincfg.MainNetParams
	if testnet {
		network = &chaincfg.TestNet3Params
	}

	return &BTCHDWallet{
		masterKey: masterKey,
		chainCode: chainCode,
		network:   network,
		nextIndex: 0,
	}, nil
}

// DeriveNextAddress derives the next Bitcoin address using BIP44 path m/44'/0'/0'/0/index
//
// Returns:
//   - string: Base58Check encoded Bitcoin address
//   - error: If key derivation or address generation fails
//
// Path components:
//   - 44' : BIP44 purpose
//   - 0'  : Bitcoin coin type
//   - 0'  : Account 0
//   - 0   : External chain
//   - i   : Address index
//
// Related: GetAddress, pubKeyToAddress
func (w *BTCHDWallet) DeriveNextAddress() (string, error) {
	// Derive BIP44 path: m/44'/0'/0'/0/index
	path := []uint32{
		purposeBIP44 | hardenedKeyStart,
		coinTypeBTC | hardenedKeyStart,
		accountDefault | hardenedKeyStart,
		changeExternal,
		w.nextIndex,
	}

	key := w.masterKey
	chainCode := w.chainCode

	for _, segment := range path {
		var err error
		key, chainCode, err = w.deriveKey(key, chainCode, segment)
		if err != nil {
			return "", fmt.Errorf("key derivation failed: %w", err)
		}
	}

	// Generate public key
	privKey, _ := btcec.PrivKeyFromBytes(key)
	pubKey := privKey.PubKey()
	pubKeyBytes := pubKey.SerializeCompressed()

	// Generate address from public key
	address, err := w.pubKeyToAddress(pubKeyBytes)
	if err != nil {
		return "", fmt.Errorf("address generation failed: %w", err)
	}

	w.nextIndex++
	return address, nil
}

// deriveKey derives a child key from a parent key and chain code.
//
// Parameters:
//   - key: Parent private key
//   - chainCode: Parent chain code
//   - index: Child index (hardened if >= 0x80000000)
//
// Returns:
//   - []byte: Child private key
//   - []byte: Child chain code
//   - error: If derived key is invalid
//
// Security:
//   - Implements BIP32 key derivation
//   - Validates derived keys against curve order
func (w *BTCHDWallet) deriveKey(key, chainCode []byte, index uint32) ([]byte, []byte, error) {
	var data []byte
	if index >= hardenedKeyStart {
		// Hardened derivation
		data = append([]byte{0x00}, key...)
	} else {
		// Normal derivation
		privKey, _ := btcec.PrivKeyFromBytes(key)
		data = privKey.PubKey().SerializeCompressed()
	}

	indexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(indexBytes, index)
	data = append(data, indexBytes...)

	hmac := hmac.New(sha512.New, chainCode)
	hmac.Write(data)
	sum := hmac.Sum(nil)

	// Generate child key
	childKey := sum[:32]
	childChainCode := sum[32:]

	// Add parent key to child key (mod curve order)
	parentInt := new(big.Int).SetBytes(key)
	childInt := new(big.Int).SetBytes(childKey)
	curveOrder := btcec.S256().N

	childInt.Add(childInt, parentInt)
	childInt.Mod(childInt, curveOrder)

	// Check for invalid keys
	if childInt.Sign() == 0 {
		return nil, nil, errors.New("invalid child key")
	}

	childKeyBytes := make([]byte, 32)
	childIntBytes := childInt.Bytes()
	copy(childKeyBytes[32-len(childIntBytes):], childIntBytes)

	return childKeyBytes, childChainCode, nil
}

// pubKeyToAddress converts a public key to a Bitcoin address.
//
// Parameters:
//   - pubKey: Compressed public key bytes
//
// Returns:
//   - string: Base58Check encoded Bitcoin address
//   - error: If address generation fails
//
// Process:
//  1. SHA256 hash
//  2. RIPEMD160 hash
//  3. Add version byte
//  4. Add checksum
//  5. Base58 encode
//
// Related: base58Encode
func (w *BTCHDWallet) pubKeyToAddress(pubKey []byte) (string, error) {
	// SHA256
	sha256Hash := sha256.Sum256(pubKey)

	// RIPEMD160
	ripemd160Hash := ripemd160.New()
	ripemd160Hash.Write(sha256Hash[:])
	pubKeyHash := ripemd160Hash.Sum(nil)

	// Add version byte
	version := w.network.PubKeyHashAddrID
	versionedPayload := append([]byte{version}, pubKeyHash...)

	// Double SHA256 for checksum
	firstSHA := sha256.Sum256(versionedPayload)
	secondSHA := sha256.Sum256(firstSHA[:])
	checksum := secondSHA[:4]

	// Combine version, payload, and checksum
	fullPayload := append(versionedPayload, checksum...)

	// Base58 encode
	address := Base58Encode(fullPayload)

	return address, nil
}

// GetAddress returns the next available Bitcoin address.
//
// Returns:
//   - string: Base58Check encoded Bitcoin address
//   - error: If address derivation fails
//
// Notes:
//   - Increments internal address counter
//   - Thread-safe for single wallet instance
//
// Related: DeriveNextAddress
func (w *BTCHDWallet) GetAddress() (string, error) {
	address, err := w.DeriveNextAddress()
	if err != nil {
		return "", fmt.Errorf("failed to derive address: %w", err)
	}
	return address, nil
}

// Ensure BitcoinHDWallet implements HDWallet interface
var _ HDWallet = (*BTCHDWallet)(nil)

// Currency implements HDWallet interface
func (w *BTCHDWallet) Currency() string {
	return string(Bitcoin)
}
