package wallet

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
)

func TestAddress_String(t *testing.T) {
	tests := []struct {
		name    string
		address Address
		want    string
	}{
		{
			name:    "mainnet P2PKH address",
			address: Address("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
			want:    "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
		},
		{
			name:    "mainnet P2SH address",
			address: Address("3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy"),
			want:    "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy",
		},
		{
			name:    "testnet address",
			address: Address("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"),
			want:    "mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn",
		},
		{
			name:    "bech32 mainnet address",
			address: Address("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"),
			want:    "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
		},
		{
			name:    "empty address",
			address: Address(""),
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.address.String(); got != tt.want {
				t.Errorf("Address.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddress_EncodeAddress(t *testing.T) {
	tests := []struct {
		name    string
		address Address
		want    string
	}{
		{
			name:    "mainnet address encoding",
			address: Address("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
			want:    "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
		},
		{
			name:    "testnet address encoding",
			address: Address("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"),
			want:    "mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn",
		},
		{
			name:    "bech32 address encoding",
			address: Address("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"),
			want:    "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
		},
		{
			name:    "empty address encoding",
			address: Address(""),
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.address.EncodeAddress(); got != tt.want {
				t.Errorf("Address.EncodeAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddress_ScriptAddress(t *testing.T) {
	tests := []struct {
		name    string
		address Address
		want    []byte
	}{
		{
			name:    "convert address to script bytes",
			address: Address("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
			want:    []byte("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
		},
		{
			name:    "convert testnet address to script bytes",
			address: Address("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"),
			want:    []byte("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"),
		},
		{
			name:    "empty address to empty bytes",
			address: Address(""),
			want:    []byte(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.address.ScriptAddress()
			if string(got) != string(tt.want) {
				t.Errorf("Address.ScriptAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddress_IsForNet(t *testing.T) {
	tests := []struct {
		name    string
		address Address
		params  *chaincfg.Params
		want    bool
	}{
		{
			name:    "mainnet P2PKH address with mainnet params",
			address: Address("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
			params:  &chaincfg.MainNetParams,
			want:    true,
		},
		{
			name:    "mainnet P2SH address with mainnet params",
			address: Address("3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy"),
			params:  &chaincfg.MainNetParams,
			want:    true,
		},
		{
			name:    "mainnet bech32 address with mainnet params",
			address: Address("bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"),
			params:  &chaincfg.MainNetParams,
			want:    true,
		},
		{
			name:    "testnet address with testnet params",
			address: Address("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"),
			params:  &chaincfg.TestNet3Params,
			want:    true,
		},
		{
			name:    "testnet P2SH address with testnet params",
			address: Address("2MzQwSSnBHWHqSAqtTVQ6v47XtaisrJa1Vc"),
			params:  &chaincfg.TestNet3Params,
			want:    true,
		},
		{
			name:    "testnet bech32 address with testnet params",
			address: Address("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"),
			params:  &chaincfg.TestNet3Params,
			want:    true,
		},
		{
			name:    "mainnet address with testnet params should fail",
			address: Address("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"),
			params:  &chaincfg.TestNet3Params,
			want:    false,
		},
		{
			name:    "testnet address with mainnet params should fail",
			address: Address("mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"),
			params:  &chaincfg.MainNetParams,
			want:    false,
		},
		{
			name:    "invalid address should fail",
			address: Address("invalid-address"),
			params:  &chaincfg.MainNetParams,
			want:    false,
		},
		{
			name:    "empty address should fail",
			address: Address(""),
			params:  &chaincfg.MainNetParams,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.address.IsForNet(tt.params); got != tt.want {
				t.Errorf("Address.IsForNet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBitcoinAddress(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		wantValid   bool
		wantNetwork string
	}{
		// Mainnet P2PKH addresses (start with 1)
		{
			name:        "valid mainnet P2PKH address",
			address:     "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		{
			name:        "valid mainnet P2PKH short address",
			address:     "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		// Mainnet P2SH addresses (start with 3)
		{
			name:        "valid mainnet P2SH address",
			address:     "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy",
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		{
			name:        "valid mainnet P2SH address alternative",
			address:     "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC",
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		// Mainnet Bech32 addresses (start with bc1)
		{
			name:        "valid mainnet bech32 address",
			address:     "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		{
			name:        "valid mainnet bech32 long address",
			address:     "bc1qrp33g0q4c70qt8d6u56c8f6x8sa9dnhxpx8dqt6",
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		// Testnet addresses (start with m, n)
		{
			name:        "valid testnet address starting with m",
			address:     "mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn",
			wantValid:   true,
			wantNetwork: "testnet",
		},
		{
			name:        "valid testnet address starting with n",
			address:     "n2eMqTT929pb1RDNuqEnxdaLau1rxy3efi",
			wantValid:   true,
			wantNetwork: "testnet",
		},
		// Testnet P2SH addresses (start with 2)
		{
			name:        "valid testnet P2SH address",
			address:     "2MzQwSSnBHWHqSAqtTVQ6v47XtaisrJa1Vc",
			wantValid:   true,
			wantNetwork: "testnet",
		},
		// Testnet Bech32 addresses (start with tb1)
		{
			name:        "valid testnet bech32 address",
			address:     "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx",
			wantValid:   true,
			wantNetwork: "testnet",
		},
		{
			name:        "valid testnet bech32 long address",
			address:     "tb1qrp33g0q4c70qt8d6u56c8f6x8sa9dnhxpx8dqt6",
			wantValid:   true,
			wantNetwork: "testnet",
		},
		// Invalid addresses
		{
			name:        "empty address",
			address:     "",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "too short address",
			address:     "1A1zP",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "too long address",
			address:     "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNaTooLongAddress",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "invalid starting character",
			address:     "4A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "contains invalid characters (0)",
			address:     "1A0zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "contains invalid characters (O)",
			address:     "1AOzP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "contains invalid characters (I)",
			address:     "1AIzP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "contains invalid characters (l)",
			address:     "1AlzP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "invalid bech32 prefix",
			address:     "xyz1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "malformed address",
			address:     "not-a-bitcoin-address",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "numeric only",
			address:     "123456789",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "special characters",
			address:     "1A1zP1eP5QGefi2DMPTfTL5SLmv7Divf@#",
			wantValid:   false,
			wantNetwork: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValid, gotNetwork := IsBitcoinAddress(tt.address)
			if gotValid != tt.wantValid {
				t.Errorf("IsBitcoinAddress() gotValid = %v, want %v", gotValid, tt.wantValid)
			}
			if gotNetwork != tt.wantNetwork {
				t.Errorf("IsBitcoinAddress() gotNetwork = %v, want %v", gotNetwork, tt.wantNetwork)
			}
		})
	}
}

func TestIsBitcoinAddress_EdgeCases(t *testing.T) {
	// Test edge cases separately for better organization
	edgeCases := []struct {
		name        string
		address     string
		wantValid   bool
		wantNetwork string
	}{
		{
			name:        "minimum valid mainnet length (26 chars total)",
			address:     "1" + strings.Repeat("1", 25),
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		{
			name:        "maximum valid mainnet length (35 chars total)",
			address:     "1" + strings.Repeat("1", 34),
			wantValid:   true,
			wantNetwork: "mainnet",
		},
		{
			name:        "minimum valid testnet length (26 chars total)",
			address:     "m" + strings.Repeat("m", 25),
			wantValid:   true,
			wantNetwork: "testnet",
		},
		{
			name:        "bech32 with uppercase (should fail)",
			address:     "BC1QW508D6QEJXTDG4Y5R3ZARVARY0C5XW7KV8F3T4",
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "minimum valid bech32 testnet length (28 chars total)",
			address:     "tb1" + strings.Repeat("q", 25),
			wantValid:   true,
			wantNetwork: "testnet",
		},
		{
			name:        "too short base58 mainnet address",
			address:     "1" + strings.Repeat("1", 24),
			wantValid:   false,
			wantNetwork: "invalid",
		},
		{
			name:        "too long base58 mainnet address",
			address:     "1" + strings.Repeat("1", 35),
			wantValid:   false,
			wantNetwork: "invalid",
		},
	}

	for _, tt := range edgeCases {
		t.Run(tt.name, func(t *testing.T) {
			gotValid, gotNetwork := IsBitcoinAddress(tt.address)
			if gotValid != tt.wantValid {
				t.Errorf("IsBitcoinAddress() gotValid = %v, want %v", gotValid, tt.wantValid)
			}
			if gotNetwork != tt.wantNetwork {
				t.Errorf("IsBitcoinAddress() gotNetwork = %v, want %v", gotNetwork, tt.wantNetwork)
			}
		})
	}
}
