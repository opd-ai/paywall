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
