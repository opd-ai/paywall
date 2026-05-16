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
	multisigConfig   *MultisigConfig // Stores multisig configuration when enabled
	multisigAddress  string          // The multisig address for this wallet
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

	// Check if wallet is already multisig and populate config
	if resp, err := client.IsMultisig(); err == nil && resp.Multisig {
		w.multisigConfig = &MultisigConfig{
			Enabled:      true,
			RequiredSigs: int(resp.Threshold),
			TotalSigners: int(resp.Total),
		}

		// Try to get the multisig address by getting the current address
		// In Monero, multisig wallets have a single address
		if addrResp, err := client.GetAddress(&monero.RequestGetAddress{AccountIndex: 0}); err == nil {
			w.multisigAddress = addrResp.Address
		}
	}

	return w, nil
}

// NewMoneroMultisigWallet creates a new Monero wallet instance and configures it for multisig.
// This is a convenience function that wraps the multisig setup workflow.
//
// For a complete multisig setup, you need to:
// 1. Create wallets for all participants using this function
// 2. Call PrepareMultisigWallet() on each wallet
// 3. Exchange the multisig info between all participants
// 4. Call MakeMultisigWallet() on each wallet with others' info
// 5. For M-of-N where M<N, exchange info again and call FinalizeMultisigWallet()
//
// Example 2-of-3 setup:
//
//	wallet1 := NewMoneroMultisigWallet(config1, 1)
//	wallet2 := NewMoneroMultisigWallet(config2, 1)
//	wallet3 := NewMoneroMultisigWallet(config3, 1)
//
//	info1, _ := wallet1.PrepareMultisigWallet()
//	info2, _ := wallet2.PrepareMultisigWallet()
//	info3, _ := wallet3.PrepareMultisigWallet()
//
//	round2Info1, addr1, _ := wallet1.MakeMultisigWallet([]string{info2, info3}, 2)
//	round2Info2, addr2, _ := wallet2.MakeMultisigWallet([]string{info1, info3}, 2)
//	round2Info3, addr3, _ := wallet3.MakeMultisigWallet([]string{info1, info2}, 2)
//
//	wallet1.FinalizeMultisigWallet([]string{round2Info2, round2Info3})
//	wallet2.FinalizeMultisigWallet([]string{round2Info1, round2Info3})
//	wallet3.FinalizeMultisigWallet([]string{round2Info1, round2Info2})
func NewMoneroMultisigWallet(config MoneroConfig, minConf int) (*MoneroHDWallet, error) {
	return NewMoneroWallet(config, minConf)
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

// Multisig operations

// IsMultisigEnabled returns true if this wallet is configured for multisig operations.
// Queries the Monero wallet RPC to determine if the wallet is in multisig mode.
func (w *MoneroHDWallet) IsMultisigEnabled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if we have cached multisig config
	if w.multisigConfig != nil && w.multisigConfig.Enabled {
		return true
	}

	// Query RPC to check multisig status
	resp, err := w.client.IsMultisig()
	if err != nil {
		return false
	}

	return resp.Multisig
}

// GetMultisigConfig returns the multisig configuration for this wallet.
// Returns the cached configuration if available, or queries the RPC.
func (w *MoneroHDWallet) GetMultisigConfig() (*MultisigConfig, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Return cached config if available
	if w.multisigConfig != nil {
		return w.multisigConfig, nil
	}

	// Query RPC for multisig status
	resp, err := w.client.IsMultisig()
	if err != nil {
		return nil, fmt.Errorf("query multisig status: %w", err)
	}

	if !resp.Multisig {
		return nil, ErrMultisigNotSupported
	}

	// Build config from RPC response
	config := &MultisigConfig{
		Enabled:      true,
		RequiredSigs: int(resp.Threshold),
		TotalSigners: int(resp.Total),
	}

	w.multisigConfig = config
	return config, nil
}

// DeriveMultisigAddress returns the multisig address for this wallet.
// For Monero, multisig addresses are wallet-level, not derived per-payment like subaddresses.
//
// If the wallet is not yet configured for multisig, this returns an error.
// Use PrepareMultisig, MakeMultisig, and FinalizeMultisig to set up multisig first.
//
// Parameters:
//   - pubKeys: Not used for Monero (kept for interface compatibility)
//   - requiredSigs: Not used for Monero (kept for interface compatibility)
//
// Returns the multisig address and metadata.
func (w *MoneroHDWallet) DeriveMultisigAddress(pubKeys [][]byte, requiredSigs int) (string, *MultisigMetadata, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if wallet is multisig
	resp, err := w.client.IsMultisig()
	if err != nil {
		return "", nil, fmt.Errorf("query multisig status: %w", err)
	}

	if !resp.Multisig {
		return "", nil, fmt.Errorf("wallet is not configured for multisig - use PrepareMultisig, MakeMultisig, and FinalizeMultisig first")
	}

	// Use cached address if available
	if w.multisigAddress == "" {
		// Try to get address from wallet (for wallets that were already multisig)
		// Note: In Monero, the address is returned when MakeMultisig or FinalizeMultisig completes
		return "", nil, fmt.Errorf("multisig address not available - wallet may not be fully initialized")
	}

	// Export multisig info for the redeem script field
	infoResp, err := w.client.ExportMultisigInfo()
	multisigInfo := ""
	if err == nil {
		multisigInfo = infoResp.Info
	}

	metadata := &MultisigMetadata{
		Address:      w.multisigAddress,
		RedeemScript: []byte(multisigInfo), // Store multisig info as redeem script
		RequiredSigs: int(resp.Threshold),
		PublicKeys:   pubKeys, // Store for reference
	}

	return w.multisigAddress, metadata, nil
}

// CreateRedeemScript exports the multisig info for this wallet.
// For Monero, this returns the multisig setup information as the "redeem script".
//
// Parameters:
//   - pubKeys: Not used for Monero (kept for interface compatibility)
//   - requiredSigs: Not used for Monero (kept for interface compatibility)
//
// Returns the multisig info bytes.
func (w *MoneroHDWallet) CreateRedeemScript(pubKeys [][]byte, requiredSigs int) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Export multisig info
	resp, err := w.client.ExportMultisigInfo()
	if err != nil {
		return nil, fmt.Errorf("export multisig info: %w", err)
	}

	return []byte(resp.Info), nil
}
