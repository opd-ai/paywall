// types.go
package paywall

import (
	"time"
)

type PaymentStatus string

const (
	StatusPending   PaymentStatus = "pending"
	StatusConfirmed PaymentStatus = "confirmed"
	StatusExpired   PaymentStatus = "expired"
)

type Payment struct {
	ID            string        `json:"id"`
	Address       string        `json:"address"`
	AmountBTC     float64       `json:"amount_btc"`
	CreatedAt     time.Time     `json:"created_at"`
	ExpiresAt     time.Time     `json:"expires_at"`
	Status        PaymentStatus `json:"status"`
	Confirmations int           `json:"confirmations"`
	TransactionID string        `json:"transaction_id,omitempty"`
}

type PaymentStore interface {
	CreatePayment(payment *Payment) error
	GetPayment(id string) (*Payment, error)
	GetPaymentByAddress(address string) (*Payment, error)
	UpdatePayment(payment *Payment) error
	ListPendingPayments() ([]*Payment, error)
}

type PaymentPageData struct {
	Address   string  `json:"address"`
	AmountBTC float64 `json:"amount_btc"`
	ExpiresAt string  `json:"expires_at"`
	PaymentID string  `json:"payment_id"`
	QrcodeJs  string
}
