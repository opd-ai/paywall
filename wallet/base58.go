// Package wallet provides Bitcoin wallet functionality including address generation and encoding
package wallet

import (
	"errors"
	"math/big"
	"strings"
)

// base58Alphabet defines the characters used in Bitcoin's base58 encoding scheme,
// excluding similar-looking characters (0OIl) to prevent visual ambiguity
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// Base58Encode converts a byte slice into a base58-encoded string using Bitcoin's alphabet.
//
// Parameters:
//   - input: Raw bytes to encode
//
// Returns:
//   - string: Base58-encoded representation of the input
//
// Features:
//   - Preserves leading zeros in the input
//   - Uses Bitcoin's specific base58 alphabet
//   - Handles arbitrary-length inputs via big.Int
//
// Example:
//
//	bytes := []byte{0, 60, 23, 110}
//	encoded := Base58Encode(bytes) // "12f9b"
//
// Related: Base58Decode for reverse operation
func Base58Encode(input []byte) string {
	x := new(big.Int)
	x.SetBytes(input)

	// Initialize
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var result []byte

	// Perform base58 encoding
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// Add leading zeros
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, base58Alphabet[0])
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// Base58Decode converts a base58-encoded string back into bytes.
//
// Parameters:
//   - input: Base58-encoded string to decode
//
// Returns:
//   - []byte: Decoded bytes
//   - error: Invalid character error if input contains characters outside base58 alphabet
//
// Error cases:
//   - Returns error if input contains invalid base58 characters
//   - Never returns error for empty input (returns empty byte slice)
//
// Features:
//   - Preserves leading zeros (encoded as '1' characters)
//   - Validates all input characters
//   - Handles arbitrary-length inputs via big.Int
//
// Example:
//
//	decoded, err := Base58Decode("12f9b")
//	// decoded = []byte{0, 60, 23, 110}
//
// Related: Base58Encode for reverse operation
func Base58Decode(input string) ([]byte, error) {
	result := big.NewInt(0)
	for _, r := range input {
		pos := strings.IndexRune(base58Alphabet, r)
		if pos == -1 {
			return nil, errors.New("invalid base58 character")
		}
		result.Mul(result, big.NewInt(58))
		result.Add(result, big.NewInt(int64(pos)))
	}

	decoded := result.Bytes()

	// Add leading zeros
	for i := 0; i < len(input); i++ {
		if input[i] != '1' {
			break
		}
		decoded = append([]byte{0}, decoded...)
	}

	return decoded, nil
}
