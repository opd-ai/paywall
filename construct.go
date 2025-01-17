package paywall

import (
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
func ConstructPaywall() (*Paywall, error) {
	key, err := wallet.GenerateEncryptionKey()
	if err != nil {
		return nil, err
	}

	storageConfig := wallet.StorageConfig{
		DataDir:       "./paywallet",
		EncryptionKey: key,
	}

	fileStore := NewFileStore()

	// Initialize paywall with minimal config
	pw, err := NewPaywall(Config{
		PriceInBTC:       0.0001, // 0.0001 BTC
		PriceInXMR:       .005,
		TestNet:          false,     // Use testnet
		Store:            fileStore, // Required for payment tracking
		PaymentTimeout:   time.Hour * 2,
		MinConfirmations: 1,
	})
	if err != nil {
		return nil, err
	}
	// Attempt to load wallet from disk, if it fails store the new one
	if HDWallet, err := wallet.LoadFromFile(storageConfig); err != nil {
		// Save newly generated wallet
		if err := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet).SaveToFile(storageConfig); err != nil {
			return nil, err
		}
	} else {
		// Load stored wallet from disk
		pw.HDWallets[wallet.Bitcoin] = HDWallet
	}
	return pw, nil
}
