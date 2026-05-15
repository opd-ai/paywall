// Package wallet implements BIP39 mnemonic functionality for user-friendly seed backup.
package wallet

import (
	"errors"
	"strings"

	"github.com/tyler-smith/go-bip39"
)

// MnemonicStrength represents the entropy strength for mnemonic generation.
type MnemonicStrength int

const (
	// Mnemonic12Words generates a 12-word mnemonic (128 bits of entropy)
	Mnemonic12Words MnemonicStrength = 128

	// Mnemonic24Words generates a 24-word mnemonic (256 bits of entropy, recommended)
	Mnemonic24Words MnemonicStrength = 256
)

// GenerateMnemonic creates a new BIP39 mnemonic phrase with the specified strength.
//
// Parameters:
//   - strength: MnemonicStrength (Mnemonic12Words or Mnemonic24Words)
//
// Returns:
//   - string: Space-separated mnemonic phrase (12 or 24 words)
//   - error: If entropy generation fails or strength is invalid
//
// Security:
//   - Uses crypto/rand for entropy generation
//   - 24-word mnemonics (256 bits) are recommended for maximum security
//   - 12-word mnemonics (128 bits) are acceptable for lower-value wallets
//
// Usage:
//
//	mnemonic, err := GenerateMnemonic(Mnemonic24Words)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Backup this phrase:", mnemonic)
//	// Example output: "abandon ability able about above absent absorb abstract..."
//
// Related: ImportFromMnemonic, ValidateMnemonic
func GenerateMnemonic(strength MnemonicStrength) (string, error) {
	if strength != Mnemonic12Words && strength != Mnemonic24Words {
		return "", errors.New("invalid mnemonic strength: must be 128 (12 words) or 256 (24 words)")
	}

	entropy, err := bip39.NewEntropy(int(strength))
	if err != nil {
		return "", err
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", err
	}

	return mnemonic, nil
}

// ImportFromMnemonic converts a BIP39 mnemonic phrase to a seed suitable for wallet creation.
//
// Parameters:
//   - mnemonic: Space-separated BIP39 mnemonic phrase (12 or 24 words)
//   - passphrase: Optional passphrase for additional security (BIP39 "25th word")
//
// Returns:
//   - []byte: 64-byte seed suitable for NewBTCHDWallet
//   - error: If mnemonic is invalid or checksum fails
//
// Security:
//   - Validates mnemonic checksum before generating seed
//   - Supports optional passphrase for additional protection
//   - Empty passphrase is valid (will use empty string)
//   - Normalizes whitespace to handle user input variations
//
// Usage:
//
//	seed, err := ImportFromMnemonic("abandon ability able...", "")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	wallet, err := NewBTCHDWallet(seed[:32], testnet, 1)
//
// Related: GenerateMnemonic, ValidateMnemonic
func ImportFromMnemonic(mnemonic, passphrase string) ([]byte, error) {
	// Normalize whitespace: trim and collapse multiple spaces
	mnemonic = strings.TrimSpace(mnemonic)
	words := strings.Fields(mnemonic)
	mnemonic = strings.Join(words, " ")

	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid mnemonic: checksum failed or unrecognized word")
	}

	seed := bip39.NewSeed(mnemonic, passphrase)

	return seed, nil
}

// ValidateMnemonic checks if a mnemonic phrase is valid according to BIP39 specification.
//
// Parameters:
//   - mnemonic: Space-separated mnemonic phrase to validate
//
// Returns:
//   - bool: true if mnemonic is valid (checksum passes, all words recognized)
//
// Validation:
//   - Checks word count (12, 15, 18, 21, or 24 words)
//   - Validates all words are in BIP39 English wordlist
//   - Verifies checksum integrity
//   - Normalizes whitespace before validation
//
// Usage:
//
//	if !ValidateMnemonic(userInput) {
//	    fmt.Println("Invalid mnemonic phrase")
//	    return
//	}
//
// Related: GenerateMnemonic, ImportFromMnemonic
func ValidateMnemonic(mnemonic string) bool {
	// Normalize whitespace: trim and collapse multiple spaces
	mnemonic = strings.TrimSpace(mnemonic)
	words := strings.Fields(mnemonic)
	mnemonic = strings.Join(words, " ")
	return bip39.IsMnemonicValid(mnemonic)
}

// MnemonicToSeed converts a validated mnemonic to a seed without passphrase.
// This is a convenience wrapper around ImportFromMnemonic with empty passphrase.
//
// Parameters:
//   - mnemonic: Space-separated BIP39 mnemonic phrase
//
// Returns:
//   - []byte: 64-byte seed suitable for wallet creation
//   - error: If mnemonic is invalid
//
// Usage:
//
//	seed, err := MnemonicToSeed("abandon ability able...")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	wallet, err := NewBTCHDWallet(seed[:32], false, 6)
//
// Related: ImportFromMnemonic
func MnemonicToSeed(mnemonic string) ([]byte, error) {
	return ImportFromMnemonic(mnemonic, "")
}

// NewBTCHDWalletFromMnemonic creates a new HD wallet from a BIP39 mnemonic phrase.
//
// Parameters:
//   - mnemonic: Space-separated BIP39 mnemonic phrase (12 or 24 words)
//   - passphrase: Optional passphrase for additional security (can be empty)
//   - testnet: Boolean flag for testnet/mainnet network selection
//   - minConf: Minimum confirmations required for balance queries
//
// Returns:
//   - *BTCHDWallet: Initialized wallet instance
//   - error: If mnemonic is invalid or wallet creation fails
//
// Security:
//   - Validates mnemonic before creating wallet
//   - Supports BIP39 passphrase (25th word)
//   - Creates deterministic wallet (same mnemonic → same addresses)
//
// Usage:
//
//	wallet, err := NewBTCHDWalletFromMnemonic(
//	    "abandon ability able about above absent...",
//	    "",    // No passphrase
//	    true,  // Testnet
//	    1,     // 1 confirmation
//	)
//
// Related: GenerateMnemonic, ImportFromMnemonic, NewBTCHDWallet
func NewBTCHDWalletFromMnemonic(mnemonic, passphrase string, testnet bool, minConf int) (*BTCHDWallet, error) {
	seed, err := ImportFromMnemonic(mnemonic, passphrase)
	if err != nil {
		return nil, err
	}

	return NewBTCHDWallet(seed[:32], testnet, minConf)
}
