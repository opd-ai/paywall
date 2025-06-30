// Package paywall implements a Bitcoin payment verification system for protecting web content
package paywall

import (
	"html/template"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// PaymentStatus represents the current state of a payment in the system
type PaymentStatus string

const (
	// StatusPending indicates a payment has been created but not yet confirmed
	StatusPending PaymentStatus = "pending"
	// StatusConfirmed indicates a payment has been verified on the blockchain
	StatusConfirmed PaymentStatus = "confirmed"
	// StatusExpired indicates the payment window has elapsed without confirmation
	StatusExpired PaymentStatus = "expired"
)

// Payment represents a Bitcoin payment transaction and its current state
// Related types: PaymentStatus, PaymentStore
type Payment struct {
	// ID uniquely identifies the payment
	ID string `json:"id"`
	// Addresses holds the BTC and XMR wallet addresses
	Addresses map[wallet.WalletType]string `json:"addresses"`
	// Amounts holds the BTC and XMR payment amounts
	Amounts map[wallet.WalletType]float64 `json:"amounts"`
	// CreatedAt is the timestamp when the payment was initiated
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt is the timestamp when the payment will expire if not confirmed
	ExpiresAt time.Time `json:"expires_at"`
	// Status indicates the current state of the payment
	Status PaymentStatus `json:"status"`
	// Confirmations is the number of blockchain confirmations received
	Confirmations int `json:"confirmations"`
}

// PaymentStore defines the interface for payment persistence operations
// Implementations should handle concurrent access safely
// Related type: Payment
type PaymentStore interface {
	// CreatePayment stores a new payment record
	// Returns error if storage fails or payment already exists
	CreatePayment(payment *Payment) error
	// GetPayment retrieves a payment by its ID
	// Returns error if payment not found or retrieval fails
	GetPayment(id string) (*Payment, error)
	// GetPaymentByAddress finds a payment by its Bitcoin address
	// Returns error if payment not found or retrieval fails
	GetPaymentByAddress(address string) (*Payment, error)
	// UpdatePayment modifies an existing payment record
	// Returns error if payment doesn't exist or update fails
	UpdatePayment(payment *Payment) error
	// ListPendingPayments returns all payments in pending status
	// Returns error if retrieval fails
	ListPendingPayments() ([]*Payment, error)
}

// PaymentPageData contains the data needed to render the payment page template
// Related types: Payment
type PaymentPageData struct {
	// BTCAddress is the Bitcoin address where payment should be sent
	BTCAddress string `json:"btc_address"`
	// AmountBTC is the required payment amount in Bitcoin
	AmountBTC float64 `json:"amount_btc"`
	// XMRAddress is the Bitcoin address where payment should be sent
	XMRAddress string `json:"xmr_address"`
	// AmountXMR is the required payment amount in Monero
	AmountXMR float64 `json:"amount_xmr"`
	// ExpiresAt is the human-readable expiration time
	ExpiresAt string `json:"expires_at"`
	// PaymentID uniquely identifies the payment
	PaymentID string `json:"payment_id"`
	// QrcodeJs contains the JS code for generating the QR cde
	QrcodeJs template.JS
}
