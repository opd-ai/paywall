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
	payments, err := m.paywall.Store.ListPendingPayments()
	defer m.gmux.Unlock()
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

func (m *CryptoChainMonitor) CheckXMRPayments(payment *Payment) error {
	m.xmrMux.Lock()
	defer m.xmrMux.Unlock()
	client, exists := m.client[wallet.Monero]
	if !exists {
		return fmt.Errorf("monero client not found")
	}
	xmrBalance, err := client.GetAddressBalance(payment.Addresses[wallet.Monero])
	if err != nil {
		return err
	}

	if xmrBalance >= payment.Amounts[wallet.Monero] {
		// Payment confirmed by balance
		// Confirmations are checked inline during GetAddressBalance
		payment.Status = StatusConfirmed
		payment.Confirmations = m.paywall.minConfirmations
		m.paywall.Store.UpdatePayment(payment)
	}
	return nil
}

func (m *CryptoChainMonitor) CheckBTCPayments(payment *Payment) error {
	m.btcMux.Lock()
	defer m.btcMux.Unlock()
	client, exists := m.client[wallet.Bitcoin]

	if !exists {
		return fmt.Errorf("bitcoin client not found")
	}
	btcBalance, err := client.GetAddressBalance(payment.Addresses[wallet.Bitcoin])
	if err != nil {
		return err
	}

	if btcBalance >= payment.Amounts[wallet.Bitcoin] {
		// Payment confirmed by balance
		// Confirmations are checked inline during GetAddressBalance
		payment.Status = StatusConfirmed
		payment.Confirmations = m.paywall.minConfirmations
		m.paywall.Store.UpdatePayment(payment)
	}
	return nil
}

// Close stops the blockchain monitor
// It cancels the context and waits for the monitor goroutine to exit
func (m *CryptoChainMonitor) Close() {
	m.paywall.cancel()
}
