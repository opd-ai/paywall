// wallet/wallet.go
package wallet

import "errors"

// ErrMultisigNotSupported is returned when multisig operations are attempted on wallets that don't support them
var ErrMultisigNotSupported = errors.New("multisig not supported by this wallet implementation")

// HDWallet defines the interface for cryptocurrency wallets
type HDWallet interface {
	DeriveNextAddress() (string, error)
	GetAddress() (string, error)
	Currency() string
	GetAddressBalance(address string) (float64, error)
	GetTransactionConfirmations(txID string) (int, error)

	// Multisig operations (optional - implementations may return ErrMultisigNotSupported)

	// IsMultisigEnabled returns true if this wallet instance supports multisig operations.
	// Default implementations return false for backward compatibility.
	IsMultisigEnabled() bool

	// GetMultisigConfig returns the multisig configuration for this wallet.
	// Returns nil if multisig is not enabled or ErrMultisigNotSupported if not supported.
	GetMultisigConfig() (*MultisigConfig, error)

	// DeriveMultisigAddress generates a new multisig address from multiple public keys.
	// Parameters:
	//   - pubKeys: Array of public keys from all participants (including this wallet)
	//   - requiredSigs: Number of signatures required (m in m-of-n)
	// Returns:
	//   - address: The generated multisig address
	//   - metadata: Multisig-specific data needed for signing (redeem scripts, etc.)
	//   - error: ErrMultisigNotSupported if not supported, or other errors
	DeriveMultisigAddress(pubKeys [][]byte, requiredSigs int) (address string, metadata *MultisigMetadata, err error)

	// CreateRedeemScript generates the redeem script for Bitcoin P2SH multisig addresses.
	// For Monero, this may return the multisig setup info instead.
	// Parameters:
	//   - pubKeys: Array of public keys from all participants
	//   - requiredSigs: Number of signatures required (m in m-of-n)
	// Returns:
	//   - script: The redeem script bytes (Bitcoin) or multisig info (Monero)
	//   - error: ErrMultisigNotSupported if not supported, or other errors
	CreateRedeemScript(pubKeys [][]byte, requiredSigs int) ([]byte, error)
}

// WalletType identifies the cryptocurrency wallet implementation
type WalletType string

const (
	Bitcoin WalletType = "BTC"
	Monero  WalletType = "XMR"
)
