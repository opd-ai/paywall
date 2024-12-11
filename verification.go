// blockchain.go
package paywall

import (
	"context"
	"time"
)

type BlockchainMonitor struct {
	paywall *Paywall
	client  BitcoinClient
}

type BitcoinClient interface {
	GetAddressBalance(address string) (float64, error)
	GetTransactionConfirmations(txID string) (int, error)
}

func (m *BlockchainMonitor) Start(ctx context.Context) {
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

func (m *BlockchainMonitor) checkPendingPayments() {
	payments, err := m.paywall.store.ListPendingPayments()
	if err != nil {
		// Handle error
		return
	}

	for _, payment := range payments {
		balance, err := m.client.GetAddressBalance(payment.Address)
		if err != nil {
			continue
		}

		if balance >= payment.AmountBTC {
			confirmations, err := m.client.GetTransactionConfirmations(payment.TransactionID)
			if err != nil {
				continue
			}

			if confirmations >= m.paywall.minConfirmations {
				payment.Status = StatusConfirmed
				payment.Confirmations = confirmations
				m.paywall.store.UpdatePayment(payment)
			}
		}
	}
}
