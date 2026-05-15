package wallet

import (
	"fmt"
	"log"
	"sync"
	"time"

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

// GetAddressBalance implements paywall.CryptoClient by getting balance for specific address.
//
// Unlike Bitcoin which queries address-level balance directly from blockchain explorers,
// Monero uses account-level transfer queries and filters by subaddress. This method:
//  1. Calls GetTransfers() to retrieve all incoming transfers for account 0
//  2. Filters transfers by the specific address parameter
//  3. Sums the amounts for matching transfers
//
// This approach is necessary because Monero's RPC provides account-level balance data,
// not per-subaddress balances. Each payment receives a unique subaddress, and this method
// ensures payment-to-address binding security by filtering transfers to verify that
// the specific payment address received funds, preventing false positive confirmations
// where payment A (address X, unpaid) could incorrectly confirm if payment B (address Y, paid)
// exists in the same account.
//
// Returns 0 balance if no transfers found for the specified address.
func (w *MoneroHDWallet) GetAddressBalance(address string) (float64, error) {
	// Get all incoming transfers for the account
	resp, err := w.client.GetTransfers(&monero.RequestGetTransfers{
		In:           true,
		AccountIndex: 0,
	})
	if err != nil {
		return 0, fmt.Errorf("get transfers failed: %w", err)
	}

	// Filter transfers to the specific address and sum their balance
	var addressBalance uint64
	var confirmations uint64
	found := false

	for _, tx := range resp.In {
		// Check if transfer is to the requested address
		// The Address field in monero Transfer indicates the destination subaddress
		if tx.Address == address {
			addressBalance += tx.Amount
			// Store the confirmations for confirmation checking
			// Use the first matching transaction's data
			if !found {
				confirmations = tx.Confirmations
				found = true
			}
		}
	}

	// If no transfer found for this address, return 0 balance
	if !found {
		return 0, nil
	}

	// Check confirmations meet minimum requirement, but still return balance
	if int(confirmations) < w.minConfirmations {
		// Return actual balance but log insufficient confirmations
		// This allows payment detection while noting confirmation status
		log.Printf("Monero payment to address %s received but insufficient confirmations: %d/%d", address, confirmations, w.minConfirmations)
		balance := float64(addressBalance) / 1e12 // Convert atomic units to XMR
		return balance, nil
	}

	balance := float64(addressBalance) / 1e12 // Convert atomic units to XMR
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

// RollbackLastAddress decrements the next index counter
// This is used for atomic payment operations - when payment storage fails
// after address generation, we need to rollback the address index
func (w *MoneroHDWallet) RollbackLastAddress() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nextIndex > 0 {
		w.nextIndex--
	}
}

// GetLatestBlockTime retrieves the timestamp of the latest Monero block
// by querying the wallet's current block height
func (w *MoneroHDWallet) GetLatestBlockTime() (time.Time, error) {
	// Get current block height from wallet
	heightResp, err := w.client.GetHeight()
	if err != nil {
		return time.Time{}, fmt.Errorf("get wallet height: %w", err)
	}

	// Monero wallet RPC doesn't directly provide block timestamps
	// Block time is approximately 2 minutes per block
	// We calculate an approximate timestamp based on genesis time
	// Genesis block timestamp: 2014-04-18 (Monero launch)
	genesisTime := time.Date(2014, 4, 18, 0, 0, 0, 0, time.UTC)

	// Average 2 minutes per block
	blockDuration := 2 * time.Minute
	estimatedTime := genesisTime.Add(time.Duration(heightResp.Height) * blockDuration)

	return estimatedTime, nil
}

// Multisig operations (default implementations for backward compatibility)

// IsMultisigEnabled returns false as multisig is not yet implemented for Monero wallets.
// This default implementation maintains backward compatibility with existing code.
func (w *MoneroHDWallet) IsMultisigEnabled() bool {
	return false
}

// GetMultisigConfig returns ErrMultisigNotSupported as multisig is not yet implemented.
// This default implementation maintains backward compatibility with existing code.
func (w *MoneroHDWallet) GetMultisigConfig() (*MultisigConfig, error) {
	return nil, ErrMultisigNotSupported
}

// DeriveMultisigAddress returns ErrMultisigNotSupported as multisig is not yet implemented.
// This default implementation maintains backward compatibility with existing code.
//
// Future implementation will support Monero's native multisig via RPC commands:
// prepare_multisig, make_multisig, export_multisig_info, import_multisig_info, finalize_multisig.
func (w *MoneroHDWallet) DeriveMultisigAddress(pubKeys [][]byte, requiredSigs int) (string, *MultisigMetadata, error) {
	return "", nil, ErrMultisigNotSupported
}

// CreateRedeemScript returns ErrMultisigNotSupported as multisig is not yet implemented.
// This default implementation maintains backward compatibility with existing code.
//
// Future implementation will use Monero RPC's multisig workflow to generate and export
// multisig setup information for coordination between participants.
func (w *MoneroHDWallet) CreateRedeemScript(pubKeys [][]byte, requiredSigs int) ([]byte, error) {
	return nil, ErrMultisigNotSupported
}
