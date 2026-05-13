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

	// Multisig fields (optional - zero values indicate single-signature payment)

	// MultisigEnabled indicates whether this payment uses multisig addresses
	MultisigEnabled bool `json:"multisig_enabled,omitempty"`
	// MultisigMetadata contains multisig-specific data per wallet type (redeem scripts, etc.)
	MultisigMetadata map[wallet.WalletType]*wallet.MultisigMetadata `json:"multisig_metadata,omitempty"`
	// RequiredSignatures specifies the number of signatures needed per wallet type
	RequiredSignatures map[wallet.WalletType]int `json:"required_signatures,omitempty"`
	// Signatures contains collected partial signatures per wallet type
	Signatures map[wallet.WalletType][]SignatureData `json:"signatures,omitempty"`
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

	// Multisig operations (optional - implementations may return empty results)

	// GetPendingMultisigPayments returns all pending payments that have multisig enabled.
	// This is useful for tracking multisig payments that need signature coordination.
	// Returns error if retrieval fails. Returns empty slice if no multisig payments pending.
	GetPendingMultisigPayments() ([]*Payment, error)

	// GetPaymentsByMultisigAddress finds payments by multisig address.
	// Useful for identifying payments associated with a specific multisig address.
	// Returns error if retrieval fails. Returns empty slice if no matching payments.
	GetPaymentsByMultisigAddress(address string) ([]*Payment, error)
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

// MultisigRole identifies the role of a participant in a multisig transaction
// Used for escrow and dispute resolution workflows
type MultisigRole string

const (
	// RoleBuyer represents the party paying for goods/services
	RoleBuyer MultisigRole = "buyer"
	// RoleSeller represents the party providing goods/services
	RoleSeller MultisigRole = "seller"
	// RoleArbiter represents the neutral third party for dispute resolution
	RoleArbiter MultisigRole = "arbiter"
)

// SignatureData contains a signature and signer identity for multisig transactions
// Used to track partial signatures in m-of-n multisig payments
type SignatureData struct {
	// SignerID uniquely identifies the signer (public key hash or other identifier)
	SignerID string `json:"signer_id"`
	// Role indicates the signer's role in the transaction
	Role MultisigRole `json:"role"`
	// Signature contains the cryptographic signature bytes
	Signature []byte `json:"signature"`
	// PublicKey is the signer's public key used for verification
	PublicKey []byte `json:"public_key"`
	// SignedAt is the timestamp when the signature was created
	SignedAt time.Time `json:"signed_at"`
}
