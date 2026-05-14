// Package paywall implements Monero transaction broadcasting for multisig payments
package paywall

import (
	"fmt"

	monero "github.com/monero-ecosystem/go-monero-rpc-client/wallet"
	"github.com/opd-ai/paywall/wallet"
)

// XMRBroadcaster handles Monero transaction broadcasting to the network
// It wraps Monero wallet RPC client for multisig transaction submission
type XMRBroadcaster struct {
	client monero.Client
}

// NewXMRBroadcaster creates a new Monero broadcaster with RPC client
// Parameters:
//   - rpcURL: Monero wallet RPC server address (e.g., "http://localhost:18082")
//   - rpcUser: RPC username for authentication
//   - rpcPass: RPC password for authentication
//
// Returns:
//   - *XMRBroadcaster: Initialized broadcaster instance
//   - error: If RPC connection fails
func NewXMRBroadcaster(rpcURL, rpcUser, rpcPass string) (*XMRBroadcaster, error) {
	if rpcURL == "" {
		return nil, fmt.Errorf("monero rpc url is required")
	}

	client := monero.New(monero.Config{
		Address: rpcURL,
	})

	// Test connection
	_, err := client.GetBalance(&monero.RequestGetBalance{AccountIndex: 0})
	if err != nil {
		return nil, fmt.Errorf("monero rpc connection failed: %w", err)
	}

	return &XMRBroadcaster{
		client: client,
	}, nil
}

// Broadcast sends a Monero multisig transaction to the network
// Parameters:
//   - txHex: Fully-signed multisig transaction hex string
//
// Returns:
//   - string: Transaction hash/ID if successful (first txID if multiple)
//   - error: If broadcast fails or validation fails
func (b *XMRBroadcaster) Broadcast(txHex string) (string, error) {
	if txHex == "" {
		return "", fmt.Errorf("transaction hex cannot be empty")
	}

	// Submit multisig transaction to Monero network
	resp, err := b.client.SubmitMultisig(&monero.RequestSubmitMultisig{
		TxDataHex: txHex,
	})
	if err != nil {
		return "", fmt.Errorf("broadcast monero transaction: %w", err)
	}

	if len(resp.TxHashList) == 0 {
		return "", fmt.Errorf("no transaction hash returned from submission")
	}

	// Return first transaction hash (Monero can split transactions)
	return resp.TxHashList[0], nil
}

// ValidateTransaction validates a Monero transaction against payment requirements
// Note: Monero transaction validation is limited compared to Bitcoin due to privacy features
// We can only verify basic requirements like non-empty transaction and payment exists
// Parameters:
//   - txHex: Transaction hex string to validate
//   - payment: Payment record containing expected details
//
// Returns:
//   - error: nil if basic validation passes, error otherwise
func (b *XMRBroadcaster) ValidateTransaction(txHex string, payment *Payment) error {
	if txHex == "" {
		return fmt.Errorf("transaction hex cannot be empty")
	}
	if payment == nil {
		return fmt.Errorf("payment cannot be nil")
	}

	// Verify payment has Monero address configured
	xmrAddress, ok := payment.Addresses[wallet.Monero]
	if !ok || xmrAddress == "" {
		return fmt.Errorf("payment has no monero address")
	}

	// Verify payment has Monero amount configured
	expectedAmount, ok := payment.Amounts[wallet.Monero]
	if !ok || expectedAmount <= 0 {
		return fmt.Errorf("payment has no valid monero amount")
	}

	// Note: Due to Monero's privacy features (RingCT, stealth addresses),
	// we cannot validate transaction outputs without the view key.
	// Full validation happens when the wallet detects incoming transfers.
	// This method performs only basic sanity checks.

	return nil
}

// BroadcastAll sends a Monero multisig transaction and returns all transaction IDs
// Monero can split large transactions into multiple parts
// Parameters:
//   - txHex: Fully-signed multisig transaction hex string
//
// Returns:
//   - []string: All transaction hashes from the submission
//   - error: If broadcast fails
func (b *XMRBroadcaster) BroadcastAll(txHex string) ([]string, error) {
	if txHex == "" {
		return nil, fmt.Errorf("transaction hex cannot be empty")
	}

	// Submit multisig transaction to Monero network
	resp, err := b.client.SubmitMultisig(&monero.RequestSubmitMultisig{
		TxDataHex: txHex,
	})
	if err != nil {
		return nil, fmt.Errorf("broadcast monero transaction: %w", err)
	}

	if len(resp.TxHashList) == 0 {
		return nil, fmt.Errorf("no transaction hashes returned from submission")
	}

	return resp.TxHashList, nil
}
