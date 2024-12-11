// wallet/hd_wallet.go
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
	// HDWallet constants
	hardenedKeyStart = 0x80000000
	purposeBIP44     = 44
	coinTypeBTC      = 0
	accountDefault   = 0
	changeExternal   = 0
)

type HDWallet struct {
	masterKey []byte
	chainCode []byte
	network   *chaincfg.Params
	nextIndex uint32
}

func NewHDWallet(seed []byte, testnet bool) (*HDWallet, error) {
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

	return &HDWallet{
		masterKey: masterKey,
		chainCode: chainCode,
		network:   network,
		nextIndex: 0,
	}, nil
}

func (w *HDWallet) DeriveNextAddress() (string, error) {
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

func (w *HDWallet) deriveKey(key, chainCode []byte, index uint32) ([]byte, []byte, error) {
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

func (w *HDWallet) pubKeyToAddress(pubKey []byte) (string, error) {
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
	address := base58Encode(fullPayload)

	return address, nil
}

// base58Encode encodes a byte slice to base58
func base58Encode(input []byte) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

	var result []byte
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append(result, alphabet[mod.Int64()])
	}

	// Add leading zeros
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, alphabet[0])
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// GetAddress returns the next available Bitcoin address
func (w *HDWallet) GetAddress() (string, error) {
	address, err := w.DeriveNextAddress()
	if err != nil {
		return "", fmt.Errorf("failed to derive address: %w", err)
	}
	return address, nil
}
