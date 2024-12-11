// wallet/base58.go
package wallet

import (
	"errors"
	"math/big"
	"strings"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

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
