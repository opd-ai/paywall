// Package paywall implements transaction broadcasting for Bitcoin multisig payments
package paywall

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/opd-ai/paywall/wallet"
)

// BTCBroadcaster handles Bitcoin transaction broadcasting to the network
// It wraps btcd RPC client and provides validation before broadcasting
type BTCBroadcaster struct {
	client  *rpcclient.Client
	network *chaincfg.Params
}

// NewBTCBroadcaster creates a new Bitcoin broadcaster with RPC client
// Parameters:
//   - host: Bitcoin RPC server address (e.g., "localhost:18332")
//   - user: RPC username for authentication
//   - pass: RPC password for authentication
//   - useTLS: Whether to use TLS for RPC connection
//   - network: Bitcoin network parameters (mainnet or testnet)
//
// Returns:
//   - *BTCBroadcaster: Initialized broadcaster instance
//   - error: If RPC connection fails
func NewBTCBroadcaster(host, user, pass string, useTLS bool, network *chaincfg.Params) (*BTCBroadcaster, error) {
	if host == "" {
		return nil, fmt.Errorf("btc rpc host is required")
	}
	if user == "" {
		return nil, fmt.Errorf("btc rpc user is required")
	}
	if pass == "" {
		return nil, fmt.Errorf("btc rpc password is required")
	}

	connCfg := &rpcclient.ConnConfig{
		Host:         host,
		User:         user,
		Pass:         pass,
		HTTPPostMode: true,
		DisableTLS:   !useTLS,
	}

	client, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("create rpc client: %w", err)
	}

	return &BTCBroadcaster{
		client:  client,
		network: network,
	}, nil
}

// Broadcast sends a Bitcoin transaction to the network
// Parameters:
//   - txBytes: Raw transaction bytes to broadcast
//
// Returns:
//   - string: Transaction hash/ID if successful
//   - error: If broadcast fails or validation fails
func (b *BTCBroadcaster) Broadcast(txBytes []byte) (string, error) {
	if len(txBytes) == 0 {
		return "", fmt.Errorf("transaction bytes cannot be empty")
	}

	// Parse transaction to validate it
	tx := wire.NewMsgTx(wire.TxVersion)
	if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return "", fmt.Errorf("invalid transaction format: %w", err)
	}

	// Basic validation
	if len(tx.TxIn) == 0 {
		return "", fmt.Errorf("transaction has no inputs")
	}
	if len(tx.TxOut) == 0 {
		return "", fmt.Errorf("transaction has no outputs")
	}

	// Send raw transaction to network
	txHash, err := b.client.SendRawTransaction(tx, false)
	if err != nil {
		return "", fmt.Errorf("broadcast transaction: %w", err)
	}

	return txHash.String(), nil
}

// ValidateTransaction validates a transaction against payment requirements
// This checks that the transaction matches the expected payment details
// Parameters:
//   - txBytes: Raw transaction bytes to validate
//   - payment: Payment record containing expected details
//
// Returns:
//   - error: nil if valid, error describing validation failure otherwise
func (b *BTCBroadcaster) ValidateTransaction(txBytes []byte, payment *Payment) error {
	if len(txBytes) == 0 {
		return fmt.Errorf("transaction bytes cannot be empty")
	}
	if payment == nil {
		return fmt.Errorf("payment cannot be nil")
	}

	// Parse transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	if err := tx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return fmt.Errorf("invalid transaction format: %w", err)
	}

	// Validate transaction has inputs and outputs
	if len(tx.TxIn) == 0 {
		return fmt.Errorf("transaction has no inputs")
	}
	if len(tx.TxOut) == 0 {
		return fmt.Errorf("transaction has no outputs")
	}

	// Validate outputs match expected payment
	btcAddress, ok := payment.Addresses[wallet.Bitcoin]
	if !ok || btcAddress == "" {
		return fmt.Errorf("payment has no bitcoin address")
	}

	expectedAmount, ok := payment.Amounts[wallet.Bitcoin]
	if !ok {
		return fmt.Errorf("payment has no bitcoin amount")
	}

	// Convert expected amount to satoshis
	expectedSatoshis := int64(expectedAmount * 1e8)

	// Parse expected address
	addr, err := btcutil.DecodeAddress(btcAddress, b.network)
	if err != nil {
		return fmt.Errorf("invalid payment address: %w", err)
	}

	// Check if any output sends to the expected address with correct amount
	foundCorrectOutput := false
	for _, txOut := range tx.TxOut {
		// Extract address from output script
		_, addrs, _, err := txscript.ExtractPkScriptAddrs(txOut.PkScript, b.network)
		if err != nil {
			continue
		}

		// Check if this output matches our expected address
		for _, outputAddr := range addrs {
			if outputAddr.String() == addr.String() {
				// Check amount matches (allow small fee differences)
				if txOut.Value >= expectedSatoshis {
					foundCorrectOutput = true
					break
				}
			}
		}
		if foundCorrectOutput {
			break
		}
	}

	if !foundCorrectOutput {
		return fmt.Errorf("transaction does not pay correct amount to expected address")
	}

	// Calculate total fee (input value - output value)
	// Note: Full validation would require fetching input UTXOs to calculate fee
	// For now, we validate structure and outputs only

	totalOutput := int64(0)
	for _, txOut := range tx.TxOut {
		totalOutput += txOut.Value
	}

	// Sanity check: outputs shouldn't be zero
	if totalOutput == 0 {
		return fmt.Errorf("transaction has zero output value")
	}

	return nil
}

// Close closes the RPC client connection
func (b *BTCBroadcaster) Close() {
	if b.client != nil {
		b.client.Shutdown()
	}
}
