package wallet

import (
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg"
	"golang.org/x/crypto/ripemd160"
)

type BitcoinWallet struct {
	PrivateKey *btcec.PrivateKey
	PublicKey  *btcec.PublicKey
	Address    string
	Network    *chaincfg.Params
}

// Hash160 performs a SHA256 followed by a RIPEMD160
func Hash160(data []byte) []byte {
	h := sha256.Sum256(data)
	r := ripemd160.New()
	r.Write(h[:])
	return r.Sum(nil)
}

// Base58Encode encodes a byte slice to base58 string
func Base58Encode(input []byte) string {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	var result []byte

	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)

	for x.Cmp(zero) > 0 {
		mod := new(big.Int)
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

// NewWallet creates a new Bitcoin wallet
func NewWallet(testnet bool) (*BitcoinWallet, error) {
	// Generate private key
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Get network parameters
	network := &chaincfg.MainNetParams
	if testnet {
		network = &chaincfg.TestNet3Params
	}

	// Get public key
	publicKey := privateKey.PubKey()

	// Generate P2PKH address
	pubKeyHash := Hash160(publicKey.SerializeCompressed())

	// Create address bytes
	version := network.PubKeyHashAddrID
	addressBytes := make([]byte, 0, 1+len(pubKeyHash)+4)
	addressBytes = append(addressBytes, version)
	addressBytes = append(addressBytes, pubKeyHash...)

	// Add checksum
	checksum := sha256.Sum256(addressBytes)
	checksum = sha256.Sum256(checksum[:])
	addressBytes = append(addressBytes, checksum[:4]...)

	// Encode address
	address := Base58Encode(addressBytes)

	return &BitcoinWallet{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Address:    address,
		Network:    network,
	}, nil
}

// GetAddress returns the wallet's Bitcoin address
func (w *BitcoinWallet) GetAddress() string {
	return w.Address
}

// SignMessage signs a message with the wallet's private key
func (w *BitcoinWallet) SignMessage(message []byte) ([]byte, error) {
	// Hash the message first (Bitcoin style)
	hash := sha256.Sum256(message)

	// In btcec/v2, we use ecdsa.Sign
	signature := ecdsa.Sign(w.PrivateKey, hash[:])
	/*if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}*/
	return signature.Serialize(), nil
}

// VerifyMessage verifies a signed message
func (w *BitcoinWallet) VerifyMessage(message, signatureBytes []byte) (bool, error) {
	signature, err := ecdsa.ParseDERSignature(signatureBytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse signature: %w", err)
	}

	hash := sha256.Sum256(message)
	return signature.Verify(hash[:], w.PublicKey), nil
}

// IsTestnet returns whether the wallet is using testnet
func (w *BitcoinWallet) IsTestnet() bool {
	return w.Network == &chaincfg.TestNet3Params
}
