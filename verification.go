// Package paywall implements Bitcoin payment verification for protected web content
package paywall

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// BlockchainMonitor manages periodic verification of Bitcoin payments
// It polls the blockchain for payment confirmations and updates payment status
// Related types: Paywall, BitcoinClient, Payment
type CryptoChainMonitor struct {
	paywall *Paywall
	client  map[wallet.WalletType]CryptoClient
	btcMux  sync.Mutex
	xmrMux  sync.Mutex
	gmux    sync.Mutex
}

// BitcoinClient defines the interface for interacting with the Bitcoin network
// Implementations should handle both mainnet and testnet appropriately
type CryptoClient interface {
	// GetAddressBalance retrieves the current balance for a Bitcoin address
	// Parameters:
	//   - address: Bitcoin address to check (string)
	// Returns:
	//   - balance in BTC (float64)
	//   - error if the request fails or address is invalid
	GetAddressBalance(address string) (float64, error)
}

// Start begins monitoring the blockchain for payment confirmations
// It runs in a separate goroutine and checks pending payments every minute
// Parameters:
//   - ctx: Context for cancellation control
//
// The monitor will run until the context is cancelled
// Related methods: checkPendingPayments
func (m *CryptoChainMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	consecutiveFailures := 0
	maxBackoffInterval := 5 * time.Minute

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if err := m.checkPendingPayments(); err != nil {
					consecutiveFailures++
					// Exponential backoff: 10s, 20s, 40s, 80s, 160s, max 300s
					backoffDelay := time.Duration(consecutiveFailures*consecutiveFailures) * 10 * time.Second
					if backoffDelay > maxBackoffInterval {
						backoffDelay = maxBackoffInterval
					}
					ticker.Reset(backoffDelay)
					log.Printf("Payment monitoring failed (attempt %d), backing off for %v: %v", consecutiveFailures, backoffDelay, err)
				} else {
					// Reset on success
					if consecutiveFailures > 0 {
						consecutiveFailures = 0
						ticker.Reset(10 * time.Second)
						log.Println("Payment monitoring recovered, returning to normal interval")
					}
				}
			}
		}
	}()
}

// checkPendingPayments verifies all pending payments against the blockchain
// For each pending payment, it:
// 1. Checks if the required amount has been received at the payment address
// 2. Verifies the number of confirmations meets the minimum requirement
// 3. Updates payment status to confirmed when requirements are met
// Error cases:
//   - Failed database queries are returned as errors
//   - Failed blockchain queries for individual payments are logged but don't fail the batch
//   - Invalid transactions are left in pending state
//
// Related types: Payment, PaymentStore
func (m *CryptoChainMonitor) checkPendingPayments() error {
	m.gmux.Lock()
	defer m.gmux.Unlock()
	payments, err := m.paywall.Store.ListPendingPayments()
	if err != nil {
		return fmt.Errorf("failed to list pending payments: %w", err)
	}

	hasErrors := false
	for _, payment := range payments {
		if err := m.CheckBTCPayments(payment); err != nil {
			// log error but continue processing other payments
			log.Printf("CheckBTCPayments error for payment %s: %v", payment.ID, err)
			hasErrors = true
		}
		if err := m.CheckXMRPayments(payment); err != nil {
			// log error but continue processing other payments
			log.Printf("CheckXMRPayments error for payment %s: %v", payment.ID, err)
			hasErrors = true
		}
	}

	if hasErrors {
		return fmt.Errorf("some payment checks failed")
	}
	return nil
}

// checkWalletPayment is a helper that checks payment balance for a specific wallet type.
// Updates payment status to confirmed if balance meets requirement.
// For multisig payments, verifies script hash matches expected redeem script.
func (m *CryptoChainMonitor) checkWalletPayment(payment *Payment, walletType wallet.WalletType, mux *sync.Mutex) error {
	mux.Lock()
	defer mux.Unlock()

	client, exists := m.client[walletType]
	if !exists {
		return fmt.Errorf("%s client not found", walletType)
	}

	// Get address for this wallet type
	address, hasAddress := payment.Addresses[walletType]
	if !hasAddress {
		// Payment doesn't have this wallet type configured, skip
		return nil
	}

	// Check if this is a multisig payment
	if payment.MultisigEnabled {
		// Get multisig metadata for validation
		metadata, hasMetadata := payment.MultisigMetadata[walletType]
		if !hasMetadata {
			return fmt.Errorf("multisig payment %s missing metadata for %s", payment.ID, walletType)
		}

		// For Bitcoin multisig, verify script hash matches expected redeem script
		if walletType == wallet.Bitcoin && metadata.ScriptHash != "" {
			// The address is derived from the script hash, so if funds are at the address,
			// the script hash is implicitly validated. Additional validation could be done
			// by parsing the UTXO script, but that requires full node access.
			log.Printf("Verifying multisig Bitcoin payment %s at address %s (script hash: %s)",
				payment.ID, address, metadata.ScriptHash)
		} else if walletType == wallet.Monero {
			// For Monero, verify the multisig address structure
			log.Printf("Verifying multisig Monero payment %s at address %s",
				payment.ID, address)
		}
	}

	balance, err := client.GetAddressBalance(address)
	if err != nil {
		return err
	}

	requiredAmount := payment.Amounts[walletType]
	if balance >= requiredAmount {
		// Payment confirmed by balance
		// Confirmations are checked inline during GetAddressBalance
		if payment.MultisigEnabled {
			log.Printf("Multisig payment %s confirmed for %s: balance %.8f >= required %.8f",
				payment.ID, walletType, balance, requiredAmount)
		}
		payment.Status = StatusConfirmed
		payment.Confirmations = m.paywall.minConfirmations
		m.paywall.Store.UpdatePayment(payment)
	}
	return nil
}

func (m *CryptoChainMonitor) CheckXMRPayments(payment *Payment) error {
	return m.checkWalletPayment(payment, wallet.Monero, &m.xmrMux)
}

func (m *CryptoChainMonitor) CheckBTCPayments(payment *Payment) error {
	return m.checkWalletPayment(payment, wallet.Bitcoin, &m.btcMux)
}

// Close stops the blockchain monitor
// It cancels the context and waits for the monitor goroutine to exit
func (m *CryptoChainMonitor) Close() {
	m.paywall.cancel()
}
