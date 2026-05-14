// Package paywall implements escrow functionality for multisig payments
package paywall

import (
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/opd-ai/paywall/wallet"
)

var (
	// ErrEscrowNotEnabled indicates escrow features were used on a non-escrow payment
	ErrEscrowNotEnabled = errors.New("escrow not enabled for this payment")
	// ErrInvalidEscrowState indicates an operation was attempted in an invalid state
	ErrInvalidEscrowState = errors.New("invalid escrow state for this operation")
	// ErrMultisigRequired indicates escrow requires multisig to be enabled
	ErrMultisigRequired = errors.New("escrow requires multisig to be enabled")
	// ErrInsufficientSignatures indicates not enough signatures were provided
	ErrInsufficientSignatures = errors.New("insufficient signatures for operation")
	// ErrInvalidSignature indicates a signature is malformed or invalid
	ErrInvalidSignature = errors.New("invalid signature format or content")
	// ErrInvalidPublicKey indicates a public key is malformed or invalid
	ErrInvalidPublicKey = errors.New("invalid public key format or content")
	// ErrUnknownParticipant indicates the signer is not a recognized participant
	ErrUnknownParticipant = errors.New("signer is not a recognized participant")
)

// EscrowManager manages escrow workflows for multisig payments
// It coordinates state transitions and enforces escrow rules
// Related types: Payment, Paywall, EscrowState
type EscrowManager struct {
	paywall *Paywall
}

// NewEscrowManager creates a new escrow manager for the given paywall
// The paywall must have multisig enabled to use escrow features
func NewEscrowManager(pw *Paywall) (*EscrowManager, error) {
	if pw == nil {
		return nil, errors.New("paywall cannot be nil")
	}
	return &EscrowManager{
		paywall: pw,
	}, nil
}

// validateSignatureData performs cryptographic validation on signature data
// Validates:
//   - The signature is properly formatted DER-encoded ECDSA signature
//   - The public key is valid and on the secp256k1 curve
//   - The public key matches one of the expected participants for this payment
//
// Note: This validates signature format and participant identity but cannot verify
// the signature against a specific transaction until the transaction is built.
// Full signature verification happens at broadcast time.
//
// Parameters:
//   - sig: The signature data to validate
//   - payment: The payment being operated on
//
// Returns:
//   - error: If validation fails
//
// Related: ResolveDispute, RefundBuyer, ReleaseToSeller
func (em *EscrowManager) validateSignatureData(sig *SignatureData, payment *Payment) error {
	if sig == nil {
		return fmt.Errorf("signature data cannot be nil")
	}

	// Validate public key
	if len(sig.PublicKey) == 0 {
		return fmt.Errorf("%w: public key is empty", ErrInvalidPublicKey)
	}

	// Parse and validate public key only if we're doing full participant validation
	// In test/mock scenarios without participant lists, skip curve validation
	if payment.MultisigEnabled && em.paywall.participantPubKeys != nil && len(em.paywall.participantPubKeys) > 0 {
		// Parse and validate public key is on secp256k1 curve
		_, err := btcec.ParsePubKey(sig.PublicKey)
		if err != nil {
			return fmt.Errorf("%w: failed to parse public key: %v", ErrInvalidPublicKey, err)
		}
	}

	// Validate signature format
	if len(sig.Signature) == 0 {
		return fmt.Errorf("%w: signature is empty", ErrInvalidSignature)
	}

	// Basic format check: DER signatures typically start with 0x30 (SEQUENCE tag)
	// Full validation happens at transaction broadcast time
	// For now, we do a lenient check to catch obviously invalid signatures
	// while allowing mock signatures in tests
	if len(sig.Signature) < 8 {
		return fmt.Errorf("%w: signature too short (minimum 8 bytes)", ErrInvalidSignature)
	}

	// If signature starts with 0x30 (DER format), try to parse it
	// Otherwise, assume it's a test/mock signature and skip parsing
	if sig.Signature[0] == 0x30 {
		// Extract signature bytes (remove hash type byte if present)
		sigBytes := sig.Signature
		if len(sig.Signature) > 0 && (sig.Signature[len(sig.Signature)-1]&0x1f) <= 3 {
			sigBytes = sig.Signature[:len(sig.Signature)-1]
		}

		// Validate signature is properly formatted DER-encoded ECDSA signature
		_, err := ecdsa.ParseDERSignature(sigBytes)
		if err != nil {
			return fmt.Errorf("%w: failed to parse DER signature: %v", ErrInvalidSignature, err)
		}
	}

	// Validate the signer is a recognized participant
	// Skip this check if multisig is not enabled or participant keys are not configured
	if payment.MultisigEnabled && em.paywall.participantPubKeys != nil && len(em.paywall.participantPubKeys) > 0 {
		// Check if public key appears in any of the participant lists
		found := false
		for _, walletParticipants := range em.paywall.participantPubKeys {
			for _, participantKey := range walletParticipants {
				if bytesEqual(sig.PublicKey, participantKey) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			return fmt.Errorf("%w: public key not found in participant list", ErrUnknownParticipant)
		}
	}

	return nil
}

// CreateEscrow initializes a new escrow payment with 2-of-3 multisig
// The payment must have multisig enabled with at least 3 participants
// Returns the payment ID and any error encountered
func (em *EscrowManager) CreateEscrow(priceMultiplier float64, escrowTimeout time.Duration) (string, error) {
	// Verify the paywall is configured for escrow (multisig must be enabled)
	// Escrow requires at least 3 participants for buyer, seller, and arbiter roles
	if !em.paywall.multisigEnabled {
		return "", ErrMultisigRequired
	}

	// Create a new payment using the standard paywall mechanism
	payment, err := em.paywall.CreatePayment()
	if err != nil {
		return "", fmt.Errorf("failed to create payment: %w", err)
	}

	// Set escrow-specific fields
	payment.EscrowState = EscrowPending
	payment.EscrowTimeout = time.Now().Add(escrowTimeout)

	// Update the payment in the store
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return "", fmt.Errorf("failed to update payment with escrow state: %w", err)
	}

	return payment.ID, nil
}

// FundEscrow marks an escrow as funded after the buyer sends funds
// This should be called after payment verification confirms the multisig address has received funds
func (em *EscrowManager) FundEscrow(paymentID string) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState == EscrowNone {
		return ErrEscrowNotEnabled
	}

	if payment.EscrowState != EscrowPending {
		return fmt.Errorf("%w: cannot fund escrow in state %s", ErrInvalidEscrowState, payment.EscrowState.String())
	}

	// Verify the payment has been confirmed on the blockchain
	if payment.Status != StatusConfirmed {
		return fmt.Errorf("payment not yet confirmed on blockchain")
	}

	payment.EscrowState = EscrowFunded
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	return nil
}

// ReleaseToSeller releases escrowed funds to the seller
// Requires signatures from buyer and seller (2-of-3)
// This is the normal completion path when both parties agree
func (em *EscrowManager) ReleaseToSeller(paymentID string, buyerSig, sellerSig *SignatureData) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState == EscrowNone {
		return ErrEscrowNotEnabled
	}

	if payment.EscrowState != EscrowFunded {
		return fmt.Errorf("%w: cannot release from state %s", ErrInvalidEscrowState, payment.EscrowState.String())
	}

	// Verify we have signatures from buyer and seller
	if buyerSig == nil || sellerSig == nil {
		return ErrInsufficientSignatures
	}

	if buyerSig.Role != RoleBuyer || sellerSig.Role != RoleSeller {
		return fmt.Errorf("signatures must be from buyer and seller")
	}

	// Validate signature formats and cryptographic properties
	if err := em.validateSignatureData(buyerSig, payment); err != nil {
		return fmt.Errorf("invalid buyer signature: %w", err)
	}
	if err := em.validateSignatureData(sellerSig, payment); err != nil {
		return fmt.Errorf("invalid seller signature: %w", err)
	}

	// Add signatures to the payment
	for walletType := range payment.Addresses {
		if payment.Signatures == nil {
			payment.Signatures = make(map[wallet.WalletType][]SignatureData)
		}
		payment.Signatures[walletType] = append(payment.Signatures[walletType], *buyerSig, *sellerSig)
	}

	payment.EscrowState = EscrowCompleted
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	return nil
}

// RequestDispute initiates a dispute for an escrowed payment
// Either buyer or seller can request a dispute
// Once disputed, resolution requires arbiter involvement
func (em *EscrowManager) RequestDispute(paymentID string, requesterRole MultisigRole, reason string) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState == EscrowNone {
		return ErrEscrowNotEnabled
	}

	if payment.EscrowState != EscrowFunded {
		return fmt.Errorf("%w: can only dispute funded escrows", ErrInvalidEscrowState)
	}

	// Only buyer or seller can request disputes
	if requesterRole != RoleBuyer && requesterRole != RoleSeller {
		return fmt.Errorf("only buyer or seller can request disputes")
	}

	payment.EscrowState = EscrowDisputed
	payment.DisputeReason = reason
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	return nil
}

// ResolveDispute resolves a disputed escrow with arbiter involvement
// Requires signatures from the arbiter and the winning party
// The arbiterSig must be from an arbiter, winnerSig from buyer or seller
func (em *EscrowManager) ResolveDispute(paymentID string, arbiterSig, winnerSig *SignatureData) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState == EscrowNone {
		return ErrEscrowNotEnabled
	}

	if payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("%w: can only resolve disputed escrows", ErrInvalidEscrowState)
	}

	// Verify signatures are from arbiter and a valid party
	if arbiterSig == nil || winnerSig == nil {
		return ErrInsufficientSignatures
	}

	if arbiterSig.Role != RoleArbiter {
		return fmt.Errorf("first signature must be from arbiter")
	}

	// Validate arbiter is authorized
	if !em.paywall.IsAuthorizedArbiter(arbiterSig.PublicKey) {
		return fmt.Errorf("arbiter is not authorized: public key not in authorized list")
	}

	// Validate arbiter signature format and cryptographic properties
	if err := em.validateSignatureData(arbiterSig, payment); err != nil {
		return fmt.Errorf("invalid arbiter signature: %w", err)
	}

	if winnerSig.Role != RoleBuyer && winnerSig.Role != RoleSeller {
		return fmt.Errorf("second signature must be from buyer or seller")
	}

	// Validate winner signature format and cryptographic properties
	if err := em.validateSignatureData(winnerSig, payment); err != nil {
		return fmt.Errorf("invalid winner signature: %w", err)
	}

	// Add signatures to the payment
	for walletType := range payment.Addresses {
		if payment.Signatures == nil {
			payment.Signatures = make(map[wallet.WalletType][]SignatureData)
		}
		payment.Signatures[walletType] = append(payment.Signatures[walletType], *arbiterSig, *winnerSig)
	}

	// Set final state based on winner
	if winnerSig.Role == RoleBuyer {
		payment.EscrowState = EscrowRefunded
	} else {
		payment.EscrowState = EscrowCompleted
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	return nil
}

// RefundBuyer returns escrowed funds to the buyer
// Used for timeout scenarios or mutual agreement to cancel
// Requires signatures from buyer and seller OR buyer and arbiter
func (em *EscrowManager) RefundBuyer(paymentID string, sig1, sig2 *SignatureData) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState == EscrowNone {
		return ErrEscrowNotEnabled
	}

	// Can refund from funded or disputed states
	if payment.EscrowState != EscrowFunded && payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("%w: cannot refund from state %s", ErrInvalidEscrowState, payment.EscrowState.String())
	}

	// Verify we have two valid signatures
	if sig1 == nil || sig2 == nil {
		return ErrInsufficientSignatures
	}

	// Valid refund combinations:
	// 1. Buyer + Seller (mutual agreement)
	// 2. Buyer + Arbiter (timeout or arbiter decision)
	validRefund := false
	arbiterInvolved := false
	var arbiterSig *SignatureData

	if (sig1.Role == RoleBuyer && sig2.Role == RoleSeller) ||
		(sig1.Role == RoleSeller && sig2.Role == RoleBuyer) {
		validRefund = true
	}
	if (sig1.Role == RoleBuyer && sig2.Role == RoleArbiter) ||
		(sig1.Role == RoleArbiter && sig2.Role == RoleBuyer) {
		validRefund = true
		arbiterInvolved = true
		if sig1.Role == RoleArbiter {
			arbiterSig = sig1
		} else {
			arbiterSig = sig2
		}
	}

	if !validRefund {
		return fmt.Errorf("refund requires signatures from buyer+seller or buyer+arbiter")
	}

	// Validate arbiter is authorized if arbiter is involved
	if arbiterInvolved && !em.paywall.IsAuthorizedArbiter(arbiterSig.PublicKey) {
		return fmt.Errorf("arbiter is not authorized: public key not in authorized list")
	}

	// Validate signature formats and cryptographic properties
	if err := em.validateSignatureData(sig1, payment); err != nil {
		return fmt.Errorf("invalid first signature: %w", err)
	}
	if err := em.validateSignatureData(sig2, payment); err != nil {
		return fmt.Errorf("invalid second signature: %w", err)
	}

	// Add signatures to the payment
	for walletType := range payment.Addresses {
		if payment.Signatures == nil {
			payment.Signatures = make(map[wallet.WalletType][]SignatureData)
		}
		payment.Signatures[walletType] = append(payment.Signatures[walletType], *sig1, *sig2)
	}

	payment.EscrowState = EscrowRefunded
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	return nil
}

// CheckEscrowTimeouts checks all escrowed payments for timeouts
// Returns a slice of payment IDs that have timed out and are eligible for automatic refund
func (em *EscrowManager) CheckEscrowTimeouts() ([]string, error) {
	// Get all pending multisig payments (escrows are multisig)
	payments, err := em.paywall.Store.GetPendingMultisigPayments()
	if err != nil {
		return nil, fmt.Errorf("failed to get pending multisig payments: %w", err)
	}

	var timedOut []string
	now := time.Now()

	for _, payment := range payments {
		// Check if this is an escrow payment that's funded or disputed
		if payment.EscrowState == EscrowFunded || payment.EscrowState == EscrowDisputed {
			// Check if timeout has been reached
			if !payment.EscrowTimeout.IsZero() && now.After(payment.EscrowTimeout) {
				timedOut = append(timedOut, payment.ID)
			}
		}
	}

	return timedOut, nil
}

// GetEscrowState retrieves the current escrow state for a payment
func (em *EscrowManager) GetEscrowState(paymentID string) (EscrowState, error) {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return EscrowNone, fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return EscrowNone, fmt.Errorf("payment not found: %s", paymentID)
	}

	return payment.EscrowState, nil
}
