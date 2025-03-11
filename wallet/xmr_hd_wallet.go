package wallet

import (
	"fmt"
	"sync"

	monero "github.com/monero-ecosystem/go-monero-rpc-client/wallet"
)

// MoneroHDWallet implements the HDWallet interface for Monero using RPC
type MoneroHDWallet struct {
	client    monero.Client
	mu        sync.Mutex
	nextIndex uint32
}

// MoneroConfig holds Monero wallet RPC connection details
type MoneroConfig struct {
	RPCURL      string
	RPCUser     string
	RPCPassword string
}

// NewMoneroWallet creates a new Monero wallet instance
func NewMoneroWallet(config MoneroConfig) (*MoneroHDWallet, error) {
	client := monero.New(monero.Config{
		Address: config.RPCURL,
	})

	w := &MoneroHDWallet{
		client:    client,
		nextIndex: 0,
	}

	// Test connection by getting balance
	_, err := client.GetBalance(&monero.RequestGetBalance{AccountIndex: 0})
	if err != nil {
		return nil, fmt.Errorf("monero RPC connection failed: %w", err)
	}

	return w, nil
}

// Currency implements HDWallet interface
func (w *MoneroHDWallet) Currency() string {
	return string(Monero)
}

// DeriveNextAddress implements HDWallet interface by creating a new subaddress
func (w *MoneroHDWallet) DeriveNextAddress() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	req := &monero.RequestCreateAddress{
		AccountIndex: 0,
		Label:        fmt.Sprintf("payment-%d", w.nextIndex),
	}

	resp, err := w.client.CreateAddress(req)
	if err != nil {
		return "", fmt.Errorf("create address failed: %w", err)
	}

	w.nextIndex++
	return resp.Address, nil
}

// GetAddress implements HDWallet interface by deriving next address
func (w *MoneroHDWallet) GetAddress() (string, error) {
	address, err := w.DeriveNextAddress()
	if err != nil {
		return "", fmt.Errorf("failed to derive address: %w", err)
	}
	return address, nil
}

// GetAddressBalance implements paywall.CryptoClient by getting balance for specific address
func (w *MoneroHDWallet) GetAddressBalance(address string) (float64, error) {
	resp, err := w.client.GetBalance(&monero.RequestGetBalance{AccountIndex: 0})
	if err != nil {
		return 0, fmt.Errorf("get balance failed: %w", err)
	}

	// Convert atomic units to XMR (1 XMR = 1e12 atomic units)
	balance := float64(resp.Balance) / 1e12
	return balance, nil
}

// GetTransactionConfirmations implements paywall.CryptoClient.
func (w *MoneroHDWallet) GetTransactionConfirmations(txID string) (int, error) {
	resp, err := w.client.GetTransfers(&monero.RequestGetTransfers{
		In:           true,
		AccountIndex: 0,
	})
	if err != nil {
		return 0, fmt.Errorf("get transfers failed: %w", err)
	}

	for _, tx := range resp.In {
		if tx.TxID == txID {
			return int(tx.Confirmations), nil
		}
	}

	return 0, fmt.Errorf("transaction %s not found", txID)
}
