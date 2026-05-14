// Package paywall implements a Bitcoin payment verification system for protecting web content
package paywall

import (
	"errors"
	"html/template"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

var (
	// ErrVersionConflict indicates a payment was modified by another operation
	// This error is returned when optimistic locking detects concurrent modifications
	ErrVersionConflict = errors.New("payment version conflict: payment was modified by another operation")
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
	// Version is used for optimistic locking to prevent concurrent modifications
	// This field is incremented on each update to detect race conditions
	Version int `json:"version"`

	// Multisig fields (optional - zero values indicate single-signature payment)

	// MultisigEnabled indicates whether this payment uses multisig addresses
	MultisigEnabled bool `json:"multisig_enabled,omitempty"`
	// MultisigMetadata contains multisig-specific data per wallet type (redeem scripts, etc.)
	MultisigMetadata map[wallet.WalletType]*wallet.MultisigMetadata `json:"multisig_metadata,omitempty"`
	// RequiredSignatures specifies the number of signatures needed per wallet type
	RequiredSignatures map[wallet.WalletType]int `json:"required_signatures,omitempty"`
	// Signatures contains collected partial signatures per wallet type
	Signatures map[wallet.WalletType][]SignatureData `json:"signatures,omitempty"`

	// Escrow fields (optional - only used when escrow is enabled)

	// EscrowState indicates the current state of the escrow (if this is an escrow payment)
	EscrowState EscrowState `json:"escrow_state,omitempty"`
	// EscrowTimeout is when the escrow will automatically refund if not resolved
	EscrowTimeout time.Time `json:"escrow_timeout,omitempty"`
	// DisputeReason contains the reason provided when a dispute is requested
	DisputeReason string `json:"dispute_reason,omitempty"`
}

// EscrowState represents the current state of an escrow transaction
type EscrowState int

const (
	// EscrowNone indicates this payment is not using escrow
	EscrowNone EscrowState = iota
	// EscrowPending indicates escrow has been created but not yet funded
	EscrowPending
	// EscrowFunded indicates the buyer has funded the escrow
	EscrowFunded
	// EscrowCompleted indicates funds have been released to the seller
	EscrowCompleted
	// EscrowDisputed indicates a dispute has been raised
	EscrowDisputed
	// EscrowRefunded indicates funds have been returned to the buyer
	EscrowRefunded
)

// String returns the string representation of the escrow state
func (e EscrowState) String() string {
	switch e {
	case EscrowNone:
		return "none"
	case EscrowPending:
		return "pending"
	case EscrowFunded:
		return "funded"
	case EscrowCompleted:
		return "completed"
	case EscrowDisputed:
		return "disputed"
	case EscrowRefunded:
		return "refunded"
	default:
		return "unknown"
	}
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

	// Multisig-specific fields (optional)

	// IsMultisig indicates whether this is a multisig payment
	IsMultisig bool `json:"is_multisig,omitempty"`
	// MultisigType describes the multisig configuration (e.g., "2-of-3", "3-of-5")
	MultisigType string `json:"multisig_type,omitempty"`
	// MultisigRole describes the role of this participant (buyer/seller/arbiter)
	MultisigRole MultisigRole `json:"multisig_role,omitempty"`
	// MultisigInstructions provides guidance for multisig payments
	MultisigInstructions string `json:"multisig_instructions,omitempty"`
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

// AuditAction represents a type of action in the audit log
type AuditAction string

const (
	// AuditActionCreate indicates an escrow payment was created
	AuditActionCreate AuditAction = "create"
	// AuditActionFund indicates an escrow was funded by the buyer
	AuditActionFund AuditAction = "fund"
	// AuditActionRelease indicates funds were released to the seller
	AuditActionRelease AuditAction = "release"
	// AuditActionRefund indicates funds were refunded to the buyer
	AuditActionRefund AuditAction = "refund"
	// AuditActionDispute indicates a dispute was raised
	AuditActionDispute AuditAction = "dispute"
	// AuditActionResolve indicates an arbiter resolved a dispute
	AuditActionResolve AuditAction = "resolve"
)

// AuditLogEntry represents a single immutable record in the audit trail
// Used for forensics, compliance, and non-repudiation
// Related types: Payment, EscrowState, MultisigRole
type AuditLogEntry struct {
	// ID uniquely identifies this audit log entry
	ID string `json:"id"`
	// PaymentID links this entry to a specific payment
	PaymentID string `json:"payment_id"`
	// Timestamp records when this action occurred
	Timestamp time.Time `json:"timestamp"`
	// Action describes what operation was performed
	Action AuditAction `json:"action"`
	// Actor is the public key of the entity performing the action
	Actor []byte `json:"actor,omitempty"`
	// ActorRole indicates the role of the actor (buyer, seller, arbiter)
	ActorRole MultisigRole `json:"actor_role,omitempty"`
	// PreviousState is the escrow state before this action
	PreviousState EscrowState `json:"previous_state"`
	// NewState is the escrow state after this action
	NewState EscrowState `json:"new_state"`
	// Signature is the cryptographic signature authorizing this action
	Signature []byte `json:"signature,omitempty"`
	// Metadata contains additional context (dispute reason, IP address, etc.)
	Metadata map[string]string `json:"metadata,omitempty"`
}
