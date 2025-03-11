package wallet

import (
	"regexp"

	"github.com/btcsuite/btcd/chaincfg"
)

/*

Copied from btcutil, we need to implement the btcutil.Address interface in order to use the btcutil functions

type Address interface {
	// String returns the string encoding of the transaction output
	// destination.
	//
	// Please note that String differs subtly from EncodeAddress: String
	// will return the value as a string without any conversion, while
	// EncodeAddress may convert destination types (for example,
	// converting pubkeys to P2PKH addresses) before encoding as a
	// payment address string.
	String() string

	// EncodeAddress returns the string encoding of the payment address
	// associated with the Address value.  See the comment on String
	// for how this method differs from String.
	EncodeAddress() string

	// ScriptAddress returns the raw bytes of the address to be used
	// when inserting the address into a txout's script.
	ScriptAddress() []byte

	// IsForNet returns whether or not the address is associated with the
	// passed bitcoin network.
	IsForNet(*chaincfg.Params) bool
}
*/

// Address wraps a `string` in order to implement the btcutil.Address interface
type Address string

// String returns the string encoding of the transaction output destination.
func (a Address) String() string {
	return string(a)
}

// EncodeAddress returns the string encoding of the payment address associated with the Address value.
func (a Address) EncodeAddress() string {
	return string(a)
}

// ScriptAddress returns the raw bytes of the address to be used when inserting the address into a txout's script.
func (a Address) ScriptAddress() []byte {
	return []byte(a)
}

// IsForNet returns whether or not the address is associated with the passed bitcoin network.
func (a Address) IsForNet(params *chaincfg.Params) bool {
	valid, networkType := IsBitcoinAddress(string(a))
	if !valid {
		return false
	}
	if networkType == "mainnet" && params.Name == chaincfg.MainNetParams.Name {
		return true
	}
	if networkType == "testnet" && params.Name == chaincfg.TestNet3Params.Name {
		return true
	}
	return false
}

// IsBitcoinAddress checks if a string is a valid Bitcoin address
// and returns whether it's a mainnet or testnet address, or "invalid" if the address is not valid.
func IsBitcoinAddress(address string) (bool, string) {
	// Base58 mainnet addresses start with 1 or 3
	mainnetRegex := regexp.MustCompile("^(1|3)[a-km-zA-HJ-NP-Z1-9]{25,34}$")

	// Base58 testnet addresses start with m, n or 2
	testnetRegex := regexp.MustCompile("^(m|n|2)[a-km-zA-HJ-NP-Z1-9]{25,34}$")

	// Bech32 mainnet addresses start with bc1
	mainnetBech32Regex := regexp.MustCompile("^bc1[a-z0-9]{25,90}$")

	// Bech32 testnet addresses start with tb1
	testnetBech32Regex := regexp.MustCompile("^tb1[a-z0-9]{25,90}$")

	if mainnetRegex.MatchString(address) || mainnetBech32Regex.MatchString(address) {
		return true, "mainnet"
	} else if testnetRegex.MatchString(address) || testnetBech32Regex.MatchString(address) {
		return true, "testnet"
	}

	return false, "invalid"
}
