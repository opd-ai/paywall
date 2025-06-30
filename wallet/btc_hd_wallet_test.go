// Package wallet implements Bitcoin HD (Hierarchical Deterministic) wallet functionality
// according to BIP32, BIP44, and BIP49 specifications.
package wallet

import (
	"bytes"
	"crypto/rand"
	"strings"
	"sync"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

// TestNewBTCHDWallet tests the creation of new HD wallets
func TestNewBTCHDWallet_Success(t *testing.T) {
	tests := []struct {
		name    string
		seed    []byte
		testnet bool
	}{
		{
			name:    "Valid mainnet wallet with 16-byte seed",
			seed:    make([]byte, 16),
			testnet: false,
		},
		{
			name:    "Valid testnet wallet with 32-byte seed",
			seed:    make([]byte, 32),
			testnet: true,
		},
		{
			name:    "Valid mainnet wallet with 64-byte seed",
			seed:    make([]byte, 64),
			testnet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate random seed
			_, err := rand.Read(tt.seed)
			if err != nil {
				t.Fatalf("Failed to generate random seed: %v", err)
			}

			// Note: This will fail due to RPC connection, but we can test the basic validation
			wallet, err := NewBTCHDWallet(tt.seed, tt.testnet, 1)

			// We expect RPC connection to fail in test environment
			if err != nil && !strings.Contains(err.Error(), "failed to connect") {
				t.Errorf("NewBTCHDWallet() unexpected error = %v", err)
			}

			// If wallet creation succeeded (unlikely in test env), verify basic properties
			if wallet != nil {
				if tt.testnet && wallet.network.Name != chaincfg.TestNet3Params.Name {
					t.Error("Expected testnet parameters")
				}
				if !tt.testnet && wallet.network.Name != chaincfg.MainNetParams.Name {
					t.Error("Expected mainnet parameters")
				}
			}
		})
	}
}

// TestNewBTCHDWallet_InvalidSeed tests wallet creation with invalid seeds
func TestNewBTCHDWallet_InvalidSeed(t *testing.T) {
	tests := []struct {
		name        string
		seed        []byte
		expectError string
	}{
		{
			name:        "Seed too short",
			seed:        make([]byte, 15),
			expectError: "seed must be between 16 and 64 bytes",
		},
		{
			name:        "Seed too long",
			seed:        make([]byte, 65),
			expectError: "seed must be between 16 and 64 bytes",
		},
		{
			name:        "Empty seed",
			seed:        []byte{},
			expectError: "seed must be between 16 and 64 bytes",
		},
		{
			name:        "Nil seed",
			seed:        nil,
			expectError: "seed must be between 16 and 64 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet, err := NewBTCHDWallet(tt.seed, false, 1)

			if err == nil {
				t.Error("Expected error for invalid seed")
			}

			if wallet != nil {
				t.Error("Expected nil wallet for invalid seed")
			}

			if err != nil && !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("Expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

// TestBTCHDWallet_Currency tests the Currency method
func TestBTCHDWallet_Currency(t *testing.T) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
	}

	currency := wallet.Currency()
	expected := "BTC"

	if currency != expected {
		t.Errorf("Currency() = %q, want %q", currency, expected)
	}
}

// TestBTCHDWallet_deriveKey tests the key derivation functionality
func TestBTCHDWallet_deriveKey(t *testing.T) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
	}

	// Fill with test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	tests := []struct {
		name        string
		key         []byte
		chainCode   []byte
		index       uint32
		expectError bool
	}{
		{
			name:        "Valid hardened derivation",
			key:         wallet.masterKey,
			chainCode:   wallet.chainCode,
			index:       hardenedKeyStart + 44,
			expectError: false,
		},
		{
			name:        "Valid normal derivation",
			key:         wallet.masterKey,
			chainCode:   wallet.chainCode,
			index:       0,
			expectError: false,
		},
		{
			name:        "Valid high index",
			key:         wallet.masterKey,
			chainCode:   wallet.chainCode,
			index:       1000,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			childKey, childChainCode, err := wallet.deriveKey(tt.key, tt.chainCode, tt.index)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectError {
				if len(childKey) != 32 {
					t.Errorf("Expected child key length 32, got %d", len(childKey))
				}

				if len(childChainCode) != 32 {
					t.Errorf("Expected child chain code length 32, got %d", len(childChainCode))
				}

				// Child key should be different from parent
				if bytes.Equal(childKey, tt.key) {
					t.Error("Child key should be different from parent key")
				}

				// Child chain code should be different from parent
				if bytes.Equal(childChainCode, tt.chainCode) {
					t.Error("Child chain code should be different from parent chain code")
				}
			}
		})
	}
}

// TestBTCHDWallet_pubKeyToAddress tests address generation from public keys
func TestBTCHDWallet_pubKeyToAddress(t *testing.T) {
	tests := []struct {
		name     string
		network  *chaincfg.Params
		pubKey   []byte
		testAddr bool // Whether to test the address format
	}{
		{
			name:     "Mainnet address generation",
			network:  &chaincfg.MainNetParams,
			pubKey:   []byte{0x02, 0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac, 0x55, 0xa0, 0x62, 0x95, 0xce, 0x87, 0x0b, 0x07, 0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28, 0xd9, 0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98},
			testAddr: true,
		},
		{
			name:     "Testnet address generation",
			network:  &chaincfg.TestNet3Params,
			pubKey:   []byte{0x02, 0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac, 0x55, 0xa0, 0x62, 0x95, 0xce, 0x87, 0x0b, 0x07, 0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28, 0xd9, 0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98},
			testAddr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet := &BTCHDWallet{
				network: tt.network,
			}

			address, err := wallet.pubKeyToAddress(tt.pubKey)

			if err != nil {
				t.Errorf("pubKeyToAddress() error = %v", err)
			}

			if address == "" {
				t.Error("pubKeyToAddress() returned empty address")
			}

			if tt.testAddr {
				// Verify address format
				valid, networkType := IsBitcoinAddress(address)
				if !valid {
					t.Errorf("Generated invalid Bitcoin address: %s", address)
				}

				expectedNetwork := "mainnet"
				if tt.network.Name == chaincfg.TestNet3Params.Name {
					expectedNetwork = "testnet"
				}

				if networkType != expectedNetwork {
					t.Errorf("Expected %s address, got %s", expectedNetwork, networkType)
				}
			}
		})
	}
}

// TestBTCHDWallet_DeriveNextAddress tests address derivation
func TestBTCHDWallet_DeriveNextAddress(t *testing.T) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
		nextIndex: 0,
	}

	// Fill with deterministic test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	// Test multiple address derivations
	addresses := make([]string, 3)
	for i := 0; i < 3; i++ {
		address, err := wallet.DeriveNextAddress()
		if err != nil {
			t.Fatalf("DeriveNextAddress() error = %v", err)
		}

		if address == "" {
			t.Error("DeriveNextAddress() returned empty address")
		}

		// Verify address format
		valid, _ := IsBitcoinAddress(address)
		if !valid {
			t.Errorf("Generated invalid Bitcoin address: %s", address)
		}

		addresses[i] = address
	}

	// Verify addresses are different
	for i := 0; i < len(addresses); i++ {
		for j := i + 1; j < len(addresses); j++ {
			if addresses[i] == addresses[j] {
				t.Errorf("Addresses should be unique: %s == %s", addresses[i], addresses[j])
			}
		}
	}

	// Verify nextIndex incremented
	if wallet.nextIndex != 3 {
		t.Errorf("Expected nextIndex = 3, got %d", wallet.nextIndex)
	}
}

// TestBTCHDWallet_GetAddress tests the GetAddress method
func TestBTCHDWallet_GetAddress(t *testing.T) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.TestNet3Params,
		nextIndex: 0,
	}

	// Fill with test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	address, err := wallet.GetAddress()
	if err != nil {
		t.Errorf("GetAddress() error = %v", err)
	}

	if address == "" {
		t.Error("GetAddress() returned empty address")
	}

	// Verify it's a valid testnet address
	valid, networkType := IsBitcoinAddress(address)
	if !valid {
		t.Errorf("Generated invalid Bitcoin address: %s", address)
	}

	if networkType != "testnet" {
		t.Errorf("Expected testnet address, got %s", networkType)
	}
}

// TestBTCHDWallet_GetAddressBalance tests balance retrieval (will fail without RPC)
func TestBTCHDWallet_GetAddressBalance(t *testing.T) {
	tests := []struct {
		name        string
		wallet      *BTCHDWallet
		address     string
		expectError bool
		errorMsg    string
	}{
		{
			name: "Invalid address format",
			wallet: &BTCHDWallet{
				masterKey: make([]byte, 32),
				chainCode: make([]byte, 32),
				network:   &chaincfg.MainNetParams,
				rpcClient: nil,
			},
			address:     "invalid-address",
			expectError: true,
			errorMsg:    "invalid bitcoin address",
		},
		{
			name: "Empty address",
			wallet: &BTCHDWallet{
				masterKey: make([]byte, 32),
				chainCode: make([]byte, 32),
				network:   &chaincfg.MainNetParams,
				rpcClient: nil,
			},
			address:     "",
			expectError: true,
			errorMsg:    "invalid bitcoin address",
		},
		{
			name: "No RPC client with valid address",
			wallet: &BTCHDWallet{
				masterKey: make([]byte, 32),
				chainCode: make([]byte, 32),
				network:   &chaincfg.MainNetParams,
				rpcClient: nil,
			},
			address:     "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			expectError: true,
			errorMsg:    "RPC client not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balance, err := tt.wallet.GetAddressBalance(tt.address)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				if balance != 0 {
					t.Errorf("Expected zero balance on error, got %f", balance)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestBTCHDWallet_ConcurrentAccess tests thread safety
func TestBTCHDWallet_ConcurrentAccess(t *testing.T) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
		nextIndex: 0,
	}

	// Fill with test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	const numGoroutines = 10
	const addressesPerGoroutine = 5

	var wg sync.WaitGroup
	addresses := make(chan string, numGoroutines*addressesPerGoroutine)
	errors := make(chan error, numGoroutines*addressesPerGoroutine)

	// Start multiple goroutines deriving addresses concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < addressesPerGoroutine; j++ {
				addr, err := wallet.DeriveNextAddress()
				if err != nil {
					errors <- err
					return
				}
				addresses <- addr
			}
		}()
	}

	wg.Wait()
	close(addresses)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	// Collect all addresses
	var addrList []string
	for addr := range addresses {
		addrList = append(addrList, addr)
	}

	// Verify we got the expected number of addresses
	expectedCount := numGoroutines * addressesPerGoroutine
	if len(addrList) != expectedCount {
		t.Errorf("Expected %d addresses, got %d", expectedCount, len(addrList))
	}

	// Verify all addresses are unique
	addrMap := make(map[string]bool)
	for _, addr := range addrList {
		if addrMap[addr] {
			t.Errorf("Duplicate address found: %s", addr)
		}
		addrMap[addr] = true

		// Verify address format
		valid, _ := IsBitcoinAddress(addr)
		if !valid {
			t.Errorf("Invalid address generated: %s", addr)
		}
	}

	// Verify final nextIndex
	if wallet.nextIndex != uint32(expectedCount) {
		t.Errorf("Expected nextIndex = %d, got %d", expectedCount, wallet.nextIndex)
	}
}

// TestBTCHDWallet_HDWalletInterface tests interface compliance
func TestBTCHDWallet_HDWalletInterface(t *testing.T) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
		nextIndex: 0,
	}

	// Fill with test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	// Test that BTCHDWallet implements HDWallet interface
	var _ HDWallet = wallet

	// Test interface methods
	currency := wallet.Currency()
	if currency != "BTC" {
		t.Errorf("Currency() = %q, want %q", currency, "BTC")
	}

	address, err := wallet.GetAddress()
	if err != nil {
		t.Errorf("GetAddress() error = %v", err)
	}
	if address == "" {
		t.Error("GetAddress() returned empty address")
	}

	// Test balance method (will fail without RPC)
	_, err = wallet.GetAddressBalance("invalid")
	if err == nil {
		t.Error("Expected error for invalid address")
	}

}

// TestUtilityFunctions tests the utility functions
func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected bool
	}{
		{
			name:     "Valid HTTPS endpoint",
			endpoint: "https://httpbin.org/status/200",
			expected: true,
		},
		{
			name:     "Valid HTTP endpoint",
			endpoint: "http://httpbin.org/status/200",
			expected: true,
		},
		{
			name:     "Endpoint without protocol",
			endpoint: "httpbin.org/status/200",
			expected: true,
		},
		{
			name:     "Invalid endpoint",
			endpoint: "https://invalid-endpoint-that-does-not-exist.com",
			expected: false,
		},
		{
			name:     "Empty endpoint",
			endpoint: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateEndpoint(tt.endpoint)
			if result != tt.expected {
				t.Errorf("validateEndpoint(%q) = %v, want %v", tt.endpoint, result, tt.expected)
			}
		})
	}
}

// TestRandomFunctions tests the random utility functions
func TestRandomFunctions(t *testing.T) {
	t.Run("randomInt", func(t *testing.T) {
		min, max := 1, 10
		for i := 0; i < 100; i++ {
			result := randomInt(min, max)
			if result < min || result >= max {
				t.Errorf("randomInt(%d, %d) = %d, want value in range [%d, %d)", min, max, result, min, max)
			}
		}
	})

	t.Run("randomElement", func(t *testing.T) {
		list := []string{"a", "b", "c", "d", "e"}
		for i := 0; i < 50; i++ {
			result := randomElement(list)
			found := false
			for _, item := range list {
				if item == result {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("randomElement() returned %q which is not in the list", result)
			}
		}
	})

	t.Run("randomElement empty list", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for empty list")
			}
		}()
		randomElement([]string{})
	})
}

// Benchmark tests for performance validation
func BenchmarkBTCHDWallet_DeriveNextAddress(b *testing.B) {
	wallet := &BTCHDWallet{
		masterKey: make([]byte, 32),
		chainCode: make([]byte, 32),
		network:   &chaincfg.MainNetParams,
		nextIndex: 0,
	}

	// Fill with test data
	copy(wallet.masterKey, []byte("test_master_key_32_bytes_long___"))
	copy(wallet.chainCode, []byte("test_chain_code_32_bytes_long___"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wallet.DeriveNextAddress()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBTCHDWallet_pubKeyToAddress(b *testing.B) {
	wallet := &BTCHDWallet{
		network: &chaincfg.MainNetParams,
	}

	pubKey := []byte{0x02, 0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac, 0x55, 0xa0, 0x62, 0x95, 0xce, 0x87, 0x0b, 0x07, 0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28, 0xd9, 0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := wallet.pubKeyToAddress(pubKey)
		if err != nil {
			b.Fatal(err)
		}
	}
}
