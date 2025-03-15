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
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
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

var testnetAPIEndpoints = []string{
	// BlockCypher endpoints
	"api.blockcypher.com/v1/btc/test3",
	"tbtc.blockdozer.com",

	// Bitcoin Core compatible endpoints
	"testnet.bitcoin.criptolayer.net",
	"testnet3.blockchain.info",
	"testnet-btc.cointools.io",
	"btc-testnet.greyh.at",
	"testnet.bitcoinrpc.io",
	"testnet.blockchain.info",
	"testnet-api.smartbit.com.au/v1/blockchain",
	"tchain.api.btc.com/v3",
	"test-insight.bitpay.com/api",
	"testnet.blockexplorer.com/api",
	"tbtc1.trezor.io",
	"tbtc2.trezor.io",
	"bitcoin-testnet-api.blockcypher.com",
	"testnet-api.blockchain.info",
	"testnet.blockstream.info/api",
	"testnet.bitcoinexplorer.org",
	"testnet.bitaps.com",
	"testnet-api.smartbit.com.au",
	"testnet.chain.so/api/v2",
	"testnet.blockchair.com/bitcoin/testnet",
}

var mainnetAPIEndpoints = []string{
	// BlockCypher endpoints
	"api.blockcypher.com/v1/btc/main",

	// Bitcoin Core compatible endpoints
	"mainnet.bitcoin.criptolayer.net",
	"blockchain.info",
	"btc.cointools.io",
	"main.btc.greyh.at",
	"bitcoin.bitcoinrpc.io",
	"api.blockchain.info",
	"api.smartbit.com.au/v1/blockchain",
	"chain.api.btc.com/v3",
	"insight.bitpay.com/api",
	"blockexplorer.com/api",
	"btc1.trezor.io",
	"btc2.trezor.io",
	"bitcoin-mainnet-api.blockcypher.com",
	"api.blockchain.info",
	"blockstream.info/api",
	"bitcoinexplorer.org",
	"api.bitaps.com",
	"api.smartbit.com.au",
	"chain.so/api/v2",
	"api.blockchair.com/bitcoin",
	"mempool.space/api",
	"blockbook.bitcoin.org",
}

// Usage considerations:
// 1. Many of these endpoints may implement rate limiting
// 2. Some endpoints might require HTTPS
// 3. Consider implementing fallback logic between endpoints
// 4. Monitor endpoint health in production
// 5. Some endpoints might have slightly different API structures

// Example helper function to validate endpoints
func validateEndpoint(endpoint string) bool {
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	// Add https:// if not present
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "https://" + endpoint
	}

	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

func randomInt(min, max int) int {
	return min + rand.Intn(max-min)
}

func randomElement(list []string) string {
	min := 0
	max := len(list)
	index := randomInt(min, max)
	return list[index]
}

func randomEndpoint(testnet bool) string {
	if testnet {
		endpoint := randomElement(testnetAPIEndpoints)
		for !validateEndpoint(endpoint) {
			endpoint = randomElement(testnetAPIEndpoints)
		}
		return endpoint
	}
	endpoint := randomElement(mainnetAPIEndpoints)
	for !validateEndpoint(endpoint) {
		endpoint = randomElement(mainnetAPIEndpoints)
	}
	return endpoint
}

// BTCHDWallet represents a hierarchical deterministic Bitcoin wallet
// implementing BIP32 and BIP44 standards.
type BTCHDWallet struct {
	masterKey []byte            // Master private key
	chainCode []byte            // Master chain code for key derivation
	network   *chaincfg.Params  // Network parameters (mainnet/testnet)
	nextIndex uint32            // Next address index to derive
	client    *rpcclient.Client // RPC client for blockchain queries
	mu        sync.RWMutex      // Mutex for thread safety
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

	// Try local node first
	port := "8332"
	if testnet {
		port = "18332"
	}

	localConfig := &rpcclient.ConnConfig{
		Host:         "localhost:" + port,
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := rpcclient.New(localConfig, nil)
	if err != nil {
		// Fall back to public node if local fails
		publicHost := randomEndpoint(testnet)

		publicConfig := &rpcclient.ConnConfig{
			Host:         publicHost,
			HTTPPostMode: true,
			DisableTLS:   false,
		}

		client, err = rpcclient.New(publicConfig, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to both local and public nodes: %v", err)
		}
	}

	return &BTCHDWallet{
		masterKey: masterKey,
		chainCode: chainCode,
		network:   network,
		nextIndex: 0,
		client:    client,
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
	w.mu.Lock()
	defer w.mu.Unlock()
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

// GetAddressBalance implements paywall.CryptoClient
// Returns the balance for a specific Bitcoin address.
//
// Parameters:
//   - address: Bitcoin address to check
//
// Returns:
//   - float64: Current balance in BTC
//   - error: If address is invalid or query fails
//
// Related: GetTransactionConfirmations
func (w *BTCHDWallet) GetAddressBalance(address string) (float64, error) {
	// Validate address format
	_, err := Base58Decode(address)
	if err != nil {
		return 0, fmt.Errorf("invalid bitcoin address: %w", err)
	}

	// Use RPC client to get address balance
	balance, err := w.client.GetReceivedByAddress(Address(address))
	if err != nil {
		return 0, fmt.Errorf("failed to get address balance: %w", err)
	}

	// Convert from satoshis to BTC
	btcBalance := float64(balance) / 1e8

	return btcBalance, nil
}

// GetTransactionConfirmations implements paywall.CryptoClient.
// Returns the number of confirmations for a specific transaction.
//
// Parameters:
//   - txID: Bitcoin transaction ID
//
// Returns:
//   - int: Number of confirmations
//   - error: If transaction is not found or query fails
//
// Related: GetAddressBalance
func (h *BTCHDWallet) GetTransactionConfirmations(txID string) (int, error) {
	return 0, fmt.Errorf("GetTransactionConfirmations not implemented")
}

func (w *BTCHDWallet) RecoverNextIndex() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Query the blockchain for the highest used index
	// This is a placeholder implementation
	highestIndex := uint32(0)
	for i := uint32(0); i < 1000; i++ {
		path := []uint32{
			purposeBIP44 | hardenedKeyStart,
			coinTypeBTC | hardenedKeyStart,
			accountDefault | hardenedKeyStart,
			changeExternal,
			i,
		}
		// Derive address and check if it has been used
		// If used, update highestIndex
		key := w.masterKey
		chainCode := w.chainCode

		// Derive address at index i
		for _, segment := range path {
			var err error
			key, chainCode, err = w.deriveKey(key, chainCode, segment)
			if err != nil {
				return fmt.Errorf("key derivation failed: %w", err)
			}
		}

		// Generate public key and address
		privKey, _ := btcec.PrivKeyFromBytes(key)
		pubKey := privKey.PubKey()
		pubKeyBytes := pubKey.SerializeCompressed()

		address, err := w.pubKeyToAddress(pubKeyBytes)
		if err != nil {
			return fmt.Errorf("address generation failed: %w", err)
		}

		// Check if address has been used by querying balance
		balance, err := w.GetAddressBalance(address)
		if err != nil {
			return fmt.Errorf("failed to check address balance: %w", err)
		}

		// If balance > 0 or transactions exist, update highestIndex
		if balance > 0 {
			highestIndex = i
		}
	}

	w.nextIndex = highestIndex + 1
	return nil
}
