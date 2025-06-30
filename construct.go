package paywall

import (
	"crypto/rand"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// ConstructPaywall creates and initializes a new Paywall instance.
// Unlike "NewPaywall" ConstructPaywall automatically configures a
// persistent wallet with a file backed store.
// Parameters:
//   - config: Configuration options for the paywall
//
// Returns:
//   - *Paywall: Initialized paywall instance
//   - error: If initialization fails
//
// Errors:
//   - If random seed generation fails
//   - If HD wallet creation fails
//   - If template parsing fails
//
// Related types: Config, Paywall
func ConstructPaywall(base string) (*Paywall, error) {
	key, err := wallet.GenerateEncryptionKey()
	if err != nil {
		return nil, err
	}
	if base == "" {
		base = "./paywallet"
	}

	storageConfig := wallet.StorageConfig{
		DataDir:       base,
		EncryptionKey: key,
	}

	fileStore := NewFileStore(storageConfig.DataDir)

	// Initialize paywall with minimal config
	pw, err := NewPaywall(Config{
		PriceInBTC:       0.0001, // 0.0001 BTC
		PriceInXMR:       .001,
		TestNet:          false,     // don't use testnet
		Store:            fileStore, // Required for payment tracking
		PaymentTimeout:   time.Hour * 2,
		MinConfirmations: 1,
	})
	if err != nil {
		return nil, err
	}

	// Initialize wallet map
	pw.HDWallets = make(map[wallet.WalletType]wallet.HDWallet)

	// Try to load existing wallet or create new one
	var btcWallet wallet.HDWallet
	if loadedWallet, err := wallet.LoadFromFile(storageConfig); err != nil {
		// Create new wallet if loading fails
		// securely generate a random 64-byte seed using crypto/rand
		seed, err := secureRandomSeed()
		if err != nil {
			return nil, err
		}
		btcWallet, err = wallet.NewBTCHDWallet(seed, false, pw.minConfirmations)
		if err != nil {
			return nil, err
		}
		// Save the newly generated wallet
		if err := btcWallet.(*wallet.BTCHDWallet).SaveToFile(storageConfig); err != nil {
			return nil, err
		}
	} else {
		btcWallet = loadedWallet
	}

	// Assign wallet to paywall
	pw.HDWallets[wallet.Bitcoin] = btcWallet

	return pw, nil
}

func secureRandomSeed() ([]byte, error) {
	seed := make([]byte, 64) // 64 bytes for a secure seed
	_, err := rand.Read(seed)
	if err != nil {
		return nil, err
	}
	return seed, nil
}
