// Package paywall implements Bitcoin payment verification for protected web content
package paywall

import (
	"context"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// BlockchainMonitor manages periodic verification of Bitcoin payments
// It polls the blockchain for payment confirmations and updates payment status
// Related types: Paywall, BitcoinClient, Payment
type CryptoChainMonitor struct {
	paywall *Paywall
	client  map[wallet.WalletType]CryptoClient
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

	// GetTransactionConfirmations returns the number of confirmations for a transaction
	// Parameters:
	//   - txID: Bitcoin transaction ID (string)
	// Returns:
	//   - number of confirmations (int)
	//   - error if transaction not found or request fails
	GetTransactionConfirmations(txID string) (int, error)
}

// Start begins monitoring the blockchain for payment confirmations
// It runs in a separate goroutine and checks pending payments every minute
// Parameters:
//   - ctx: Context for cancellation control
//
// The monitor will run until the context is cancelled
// Related methods: checkPendingPayments
func (m *CryptoChainMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				m.checkPendingPayments()
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
//   - Failed database queries are skipped
//   - Failed blockchain queries for individual payments are skipped
//   - Invalid transactions are left in pending state
//
// Related types: Payment, PaymentStore
func (m *CryptoChainMonitor) checkPendingPayments() {
	payments, err := m.paywall.Store.ListPendingPayments()
	if err != nil {
		// Handle error
		return
	}

	for _, payment := range payments {
		btcBalance, err := m.client[wallet.Bitcoin].GetAddressBalance(payment.Addresses[wallet.Monero])
		if err != nil {
			continue
		}

		if btcBalance >= payment.Amounts[wallet.Bitcoin] {
			confirmations, err := m.client[wallet.Bitcoin].GetTransactionConfirmations(payment.TransactionID)
			if err != nil {
				continue
			}

			if confirmations >= m.paywall.minConfirmations {
				payment.Status = StatusConfirmed
				payment.Confirmations = confirmations
				m.paywall.Store.UpdatePayment(payment)
			}
		}
		xmrBalance, err := m.client[wallet.Monero].GetAddressBalance(payment.Addresses[wallet.Monero])
		if err != nil {
			continue
		}

		if xmrBalance >= payment.Amounts[wallet.Monero] {
			confirmations, err := m.client[wallet.Monero].GetTransactionConfirmations(payment.TransactionID)
			if err != nil {
				continue
			}

			if confirmations >= m.paywall.minConfirmations {
				payment.Status = StatusConfirmed
				payment.Confirmations = confirmations
				m.paywall.Store.UpdatePayment(payment)
			}
		}
	}
}
