package wallet

import (
	"fmt"
	"sync"

	monero "github.com/monero-ecosystem/go-monero-rpc-client/wallet"
)

// MoneroHDWallet implements the HDWallet interface for Monero using RPC
type MoneroHDWallet struct {
	client           monero.Client
	mu               sync.Mutex
	nextIndex        uint32
	minConfirmations int
}

// MoneroConfig holds Monero wallet RPC connection details
type MoneroConfig struct {
	RPCURL      string
	RPCUser     string
	RPCPassword string
}

// NewMoneroWallet creates a new Monero wallet instance
func NewMoneroWallet(config MoneroConfig, minConf int) (*MoneroHDWallet, error) {
	client := monero.New(monero.Config{
		Address: config.RPCURL,
	})

	w := &MoneroHDWallet{
		client:           client,
		nextIndex:        0,
		minConfirmations: minConf,
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
	//get the TxID for the address
	// Note: Monero does not use addresses in the same way as Bitcoin, so we
	// will assume the balance is for the account index 0 which is the default account.
	balance := float64(resp.Balance) / 1e12
	txId, err := w.GetTransactionIDByAmount(float64(balance)) // Convert atomic units to XMR
	if err != nil {
		return 0, fmt.Errorf("get transaction ID failed: %w", err)
	}
	// Log the transaction ID for debugging
	fmt.Printf("Transaction ID for address %s: %s\n", address, txId)
	// Get the confirmations for the TxID
	conf, err := w.GetTransactionConfirmations(txId)
	if err != nil {
		return 0, fmt.Errorf("Get confirmations failed: %w", err)
	}
	if conf < w.minConfirmations {
		return 0, fmt.Errorf("Unconfirmed, balance considered 0(this is temporary): %w", err)
	}
	// Convert atomic units to XMR (1 XMR = 1e12 atomic units)
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

// GetTransactionIDByAmount finds the transaction ID for an incoming transfer of the specified amount
// Returns the transaction ID of the first incoming transfer that meets or exceeds the specified amount
func (w *MoneroHDWallet) GetTransactionIDByAmount(amount float64) (string, error) {
	resp, err := w.client.GetTransfers(&monero.RequestGetTransfers{
		In:           true,
		AccountIndex: 0,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get transfers: %w", err)
	}

	// Find matching transaction by amount
	for _, tx := range resp.In {
		txAmount := float64(tx.Amount) / 1e12 // Convert atomic units to XMR
		if txAmount >= amount {
			return tx.TxID, nil
		}
	}

	return "", fmt.Errorf("no transaction found with amount >= %f XMR", amount)
}
