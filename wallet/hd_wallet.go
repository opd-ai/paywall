// wallet/wallet.go
package wallet

// HDWallet defines the interface for cryptocurrency wallets
type HDWallet interface {
	DeriveNextAddress() (string, error)
	GetAddress() (string, error)
	Currency() string
}

// WalletType identifies the cryptocurrency wallet implementation
type WalletType string

const (
	Bitcoin WalletType = "BTC"
	Monero  WalletType = "XMR"
)
