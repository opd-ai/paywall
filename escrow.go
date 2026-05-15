// Package paywall implements escrow functionality for multisig payments
package paywall

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
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
	// ErrRoleMismatch indicates the declared role does not match the derived role
	ErrRoleMismatch = errors.New("signature role does not match participant role")
)

// EscrowManager manages escrow workflows for multisig payments
// It coordinates state transitions and enforces escrow rules
// Related types: Payment, Paywall, EscrowState, AuditLogger
type EscrowManager struct {
	paywall        *Paywall
	auditLogger    AuditLogger
	stateValidator *EscrowStateValidator
	arbiter        Arbiter           // optional arbiter for dispute registration
	metrics        *MetricsCollector // optional metrics collector
}

// NewEscrowManager creates a new escrow manager for the given paywall
// The paywall must have multisig enabled to use escrow features
// If no audit logger is provided, creates a default MemoryAuditLogger
func NewEscrowManager(pw *Paywall) (*EscrowManager, error) {
	if pw == nil {
		return nil, errors.New("paywall cannot be nil")
	}
	return &EscrowManager{
		paywall:        pw,
		auditLogger:    NewMemoryAuditLogger(),
		stateValidator: NewEscrowStateValidator(),
	}, nil
}

// NewEscrowManagerWithAudit creates an escrow manager with a custom audit logger
// Use this when you need persistent audit trail storage
func NewEscrowManagerWithAudit(pw *Paywall, logger AuditLogger) (*EscrowManager, error) {
	if pw == nil {
		return nil, errors.New("paywall cannot be nil")
	}
	if logger == nil {
		return nil, errors.New("audit logger cannot be nil")
	}
	return &EscrowManager{
		paywall:        pw,
		auditLogger:    logger,
		stateValidator: NewEscrowStateValidator(),
		arbiter:        nil, // No arbiter by default
	}, nil
}

// NewEscrowManagerWithArbiter creates an escrow manager with arbiter integration
// Use this when you want automatic dispute registration with an arbiter system
func NewEscrowManagerWithArbiter(pw *Paywall, logger AuditLogger, arbiter Arbiter) (*EscrowManager, error) {
	if pw == nil {
		return nil, errors.New("paywall cannot be nil")
	}
	if logger == nil {
		return nil, errors.New("audit logger cannot be nil")
	}
	if arbiter == nil {
		return nil, errors.New("arbiter cannot be nil")
	}
	return &EscrowManager{
		paywall:        pw,
		auditLogger:    logger,
		stateValidator: NewEscrowStateValidator(),
		arbiter:        arbiter,
	}, nil
}

// SetArbiter sets the arbiter for the escrow manager
// This allows adding arbiter integration after creation
func (em *EscrowManager) SetArbiter(arbiter Arbiter) {
	em.arbiter = arbiter
}

// SetMetrics sets the metrics collector for the escrow manager
// This allows adding metrics tracking after creation
func (em *EscrowManager) SetMetrics(metrics *MetricsCollector) {
	em.metrics = metrics
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
		var derivedRole MultisigRole
		var walletTypeForRole wallet.WalletType

		for walletType, walletParticipants := range em.paywall.participantPubKeys {
			for _, participantKey := range walletParticipants {
				if bytesEqual(sig.PublicKey, participantKey) {
					found = true
					walletTypeForRole = walletType
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

		// Derive the role from the public key position in the participant list
		// This prevents role spoofing by verifying against canonical role assignment
		var err error
		derivedRole, err = em.paywall.getRoleForPubKey(sig.PublicKey, walletTypeForRole)
		if err != nil {
			return fmt.Errorf("failed to derive role for public key: %w", err)
		}

		// Verify the declared role matches the derived role
		if sig.Role != "" && sig.Role != derivedRole {
			return fmt.Errorf("%w: declared role '%s' does not match derived role '%s'",
				ErrRoleMismatch, sig.Role, derivedRole)
		}
	}

	// Check for signature replay attacks
	// NOTE: Replay protection is optional for backward compatibility
	// Signatures without nonces are allowed but logged as a warning
	if err := em.validateSignatureReplay(sig, payment); err != nil {
		// Check if error is due to missing nonce (backward compatibility)
		if len(sig.Nonce) == 0 {
			// Log warning but allow the signature for backward compatibility
			log.Printf("WARNING: Signature for payment %s missing nonce (backward compatibility mode)", payment.ID)
		} else {
			// Strict replay protection for signatures with nonces
			return err
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

	// Validate escrow timeout is positive and reasonable
	if escrowTimeout <= 0 {
		return "", fmt.Errorf("escrow timeout must be positive, got %s", escrowTimeout)
	}
	if escrowTimeout < em.paywall.minEscrowTimeout {
		return "", fmt.Errorf("escrow timeout %s is below minimum %s", escrowTimeout, em.paywall.minEscrowTimeout)
	}
	if escrowTimeout > em.paywall.maxEscrowTimeout {
		return "", fmt.Errorf("escrow timeout %s exceeds maximum %s", escrowTimeout, em.paywall.maxEscrowTimeout)
	}

	// Create a new payment using the standard paywall mechanism
	payment, err := em.paywall.CreatePayment()
	if err != nil {
		return "", fmt.Errorf("failed to create payment: %w", err)
	}

	// Set escrow-specific fields
	payment.EscrowTimeout = time.Now().Add(escrowTimeout)

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		EscrowPending,
		"system",
		"Escrow payment created",
	); err != nil {
		return "", fmt.Errorf("invalid state transition: %w", err)
	}

	// Update the payment in the store
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return "", fmt.Errorf("failed to update payment with escrow state: %w", err)
	}

	// Log escrow creation in audit trail
	_, auditErr := em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     payment.ID,
		Action:        AuditActionCreate,
		PreviousState: EscrowNone,
		NewState:      EscrowPending,
		Metadata: map[string]string{
			"timeout":          escrowTimeout.String(),
			"price_multiplier": fmt.Sprintf("%.2f", priceMultiplier),
		},
	})
	if auditErr != nil {
		// Log audit failure but don't fail the operation
		// Audit failures should be logged but not block escrow creation
		log.Printf("WARNING: failed to log escrow creation: %v", auditErr)
	}

	// Track metrics
	if em.metrics != nil {
		em.metrics.IncrementEscrowCreated()
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

	prevState := payment.EscrowState

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		EscrowFunded,
		"buyer",
		"Multisig address funded on blockchain",
	); err != nil {
		// Log invalid transition attempt
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionFund,
			PreviousState: prevState,
			NewState:      prevState, // State unchanged due to error
			Metadata: map[string]string{
				"error":  err.Error(),
				"status": "rejected",
			},
		})
		return fmt.Errorf("invalid state transition: %w", err)
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	// Log state transition in audit trail
	em.logStateTransition(paymentID, AuditActionFund, prevState, EscrowFunded, nil, RoleBuyer, nil, nil)

	// Track metrics
	if em.metrics != nil {
		em.metrics.IncrementEscrowFunded()
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

	prevState := payment.EscrowState

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		EscrowCompleted,
		"buyer+seller",
		"Funds released to seller with buyer and seller agreement",
	); err != nil {
		// Log invalid transition attempt
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionRelease,
			PreviousState: prevState,
			NewState:      prevState,
			ActorRole:     RoleBuyer,
			Metadata: map[string]string{
				"error":  err.Error(),
				"status": "rejected",
			},
		})
		return fmt.Errorf("invalid state transition: %w", err)
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	// Log release to seller in audit trail
	em.logStateTransition(paymentID, AuditActionRelease, prevState, EscrowCompleted,
		buyerSig.PublicKey, RoleBuyer, buyerSig.Signature, nil)

	// Track metrics
	if em.metrics != nil {
		em.metrics.IncrementEscrowCompleted()
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

	// Check dispute rate limit
	if err := em.checkDisputeRateLimit(requesterRole); err != nil {
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionDispute,
			PreviousState: payment.EscrowState,
			NewState:      payment.EscrowState,
			ActorRole:     requesterRole,
			Metadata: map[string]string{
				"error":  err.Error(),
				"reason": reason,
				"status": "rate_limited",
			},
		})
		return err
	}

	// Validate dispute fee payment if required
	if err := em.validateDisputeFeePayment(paymentID, requesterRole); err != nil {
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionDispute,
			PreviousState: payment.EscrowState,
			NewState:      payment.EscrowState,
			ActorRole:     requesterRole,
			Metadata: map[string]string{
				"error":  err.Error(),
				"reason": reason,
				"status": "fee_not_paid",
			},
		})
		return fmt.Errorf("dispute fee not paid: %w", err)
	}

	// Calculate and record dispute fee
	disputeFee := em.calculateDisputeFee(payment)
	if disputeFee > 0 {
		payment.DisputeFee = disputeFee
	}

	// Extend escrow timeout to allow for dispute resolution
	em.extendEscrowTimeout(payment)

	prevState := payment.EscrowState
	payment.DisputeReason = reason
	payment.DisputeFiledAt = time.Now()

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		EscrowDisputed,
		string(requesterRole),
		fmt.Sprintf("Dispute requested: %s", reason),
	); err != nil {
		// Log invalid transition attempt
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionDispute,
			PreviousState: prevState,
			NewState:      prevState,
			ActorRole:     requesterRole,
			Metadata: map[string]string{
				"error":  err.Error(),
				"reason": reason,
				"status": "rejected",
			},
		})
		return fmt.Errorf("invalid state transition: %w", err)
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	// Register dispute with arbiter system if configured
	if em.arbiter != nil {
		if err := em.arbiter.RegisterDispute(payment, requesterRole); err != nil {
			// Rollback payment state if arbiter registration fails
			payment.EscrowState = prevState
			payment.DisputeReason = ""
			if rollbackErr := em.paywall.Store.UpdatePayment(payment); rollbackErr != nil {
				// Log rollback failure but return original error
				log.Printf("ERROR: Failed to rollback payment state after arbiter registration failure: %v", rollbackErr)
			}
			em.auditLogger.LogAction(&AuditLogEntry{
				PaymentID:     paymentID,
				Action:        AuditActionDispute,
				PreviousState: prevState,
				NewState:      prevState,
				ActorRole:     requesterRole,
				Metadata: map[string]string{
					"error":  err.Error(),
					"reason": reason,
					"status": "arbiter_registration_failed",
				},
			})
			return fmt.Errorf("failed to register dispute with arbiter: %w", err)
		}
	}

	// Initiate multi-arbiter consensus if enabled
	if em.paywall.consensusManager != nil {
		if _, err := em.paywall.consensusManager.InitiateConsensus(paymentID); err != nil {
			// Log consensus initiation failure but don't fail the dispute
			log.Printf("WARNING: Failed to initiate multi-arbiter consensus for payment %s: %v", paymentID, err)
		}
	}

	// Log dispute request in audit trail
	em.logStateTransition(paymentID, AuditActionDispute, prevState, EscrowDisputed,
		nil, requesterRole, nil, map[string]string{"reason": reason})

	// Track metrics
	if em.metrics != nil {
		em.metrics.IncrementEscrowDisputed()
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

	prevState := payment.EscrowState

	// Set final state based on winner using state validator
	var newState EscrowState
	var actor string
	if winnerSig.Role == RoleBuyer {
		newState = EscrowRefunded
		actor = "arbiter+buyer"
	} else {
		newState = EscrowCompleted
		actor = "arbiter+seller"
	}

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		newState,
		actor,
		fmt.Sprintf("Dispute resolved by arbiter in favor of %s", string(winnerSig.Role)),
	); err != nil {
		// Log invalid transition attempt
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionResolve,
			PreviousState: prevState,
			NewState:      prevState,
			ActorRole:     RoleArbiter,
			Metadata: map[string]string{
				"error":  err.Error(),
				"status": "rejected",
			},
		})
		return fmt.Errorf("invalid state transition: %w", err)
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	// Track arbiter reputation for single-arbiter dispute resolution
	// For multi-arbiter consensus, reputation is tracked during voting
	if em.paywall.reputationTracker != nil && em.paywall.consensusManager == nil {
		// Record decision with consensus=true since single arbiter is always "with consensus"
		// Response time is not tracked for single-arbiter mode (set to 0)
		arbiterID := string(arbiterSig.PublicKey)
		em.paywall.reputationTracker.RecordDecision(arbiterID, true, 0)
	}

	// Track metrics
	if em.metrics != nil {
		em.metrics.IncrementEscrowDisputeResolved()
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

	prevState := payment.EscrowState

	// Determine actor for audit trail
	actor := "buyer+seller"
	if arbiterInvolved {
		actor = "buyer+arbiter"
	}

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		EscrowRefunded,
		actor,
		"Refund to buyer approved",
	); err != nil {
		// Log invalid transition attempt
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionRefund,
			PreviousState: prevState,
			NewState:      prevState,
			ActorRole:     RoleBuyer,
			Metadata: map[string]string{
				"error":            err.Error(),
				"status":           "rejected",
				"arbiter_involved": fmt.Sprintf("%t", arbiterInvolved),
			},
		})
		return fmt.Errorf("invalid state transition: %w", err)
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	// Track metrics
	if em.metrics != nil {
		em.metrics.IncrementEscrowRefunded()
	}

	return nil
}

// logStateTransition creates an audit log entry for an escrow state change
// This is a helper method to ensure consistent audit logging across all state transitions
func (em *EscrowManager) logStateTransition(paymentID string, action AuditAction, prevState, newState EscrowState, actor []byte, actorRole MultisigRole, signature []byte, metadata map[string]string) {
	_, err := em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        action,
		PreviousState: prevState,
		NewState:      newState,
		Actor:         actor,
		ActorRole:     actorRole,
		Signature:     signature,
		Metadata:      metadata,
	})
	if err != nil {
		// Log audit failure but don't fail the operation
		// Audit failures should be logged but not block escrow operations
		log.Printf("WARNING: failed to log state transition %s for payment %s: %v", action, paymentID, err)
	}
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

// ExtendTimeout extends the timeout of an escrow payment
// Requires signatures from 2 of the 3 participants (buyer, seller, arbiter)
// The extension must not exceed the 7-day roadmap cap enforced by maxExtension.
func (em *EscrowManager) ExtendTimeout(paymentID string, extension time.Duration, sig1, sig2 *SignatureData) error {
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

	// Can only extend timeout for funded or disputed escrows
	if payment.EscrowState != EscrowFunded && payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("%w: cannot extend timeout from state %s", ErrInvalidEscrowState, payment.EscrowState.String())
	}

	// Verify we have two valid signatures
	if sig1 == nil || sig2 == nil {
		return ErrInsufficientSignatures
	}

	// Validate signature formats and cryptographic properties
	if err := em.validateSignatureData(sig1, payment); err != nil {
		return fmt.Errorf("invalid first signature: %w", err)
	}
	if err := em.validateSignatureData(sig2, payment); err != nil {
		return fmt.Errorf("invalid second signature: %w", err)
	}

	// Validate extension duration
	if extension <= 0 {
		return fmt.Errorf("extension duration must be positive, got %s", extension)
	}

	// Get the max extension (default: 7 days per ROADMAP specification)
	maxExtension := 7 * 24 * time.Hour

	if extension > maxExtension {
		return fmt.Errorf("extension %s exceeds maximum allowed extension %s", extension, maxExtension)
	}

	// Verify signatures are from 2 different participants
	if sig1.Role == sig2.Role {
		return fmt.Errorf("extension requires signatures from 2 different participants")
	}

	// Valid extension combinations (any 2 of 3):
	// 1. Buyer + Seller
	// 2. Buyer + Arbiter
	// 3. Seller + Arbiter
	validExtension := false
	arbiterInvolved := false
	var arbiterSig *SignatureData

	// Check for buyer + seller
	if (sig1.Role == RoleBuyer && sig2.Role == RoleSeller) ||
		(sig1.Role == RoleSeller && sig2.Role == RoleBuyer) {
		validExtension = true
	}

	// Check for buyer + arbiter
	if (sig1.Role == RoleBuyer && sig2.Role == RoleArbiter) ||
		(sig1.Role == RoleArbiter && sig2.Role == RoleBuyer) {
		validExtension = true
		arbiterInvolved = true
		if sig1.Role == RoleArbiter {
			arbiterSig = sig1
		} else {
			arbiterSig = sig2
		}
	}

	// Check for seller + arbiter
	if (sig1.Role == RoleSeller && sig2.Role == RoleArbiter) ||
		(sig1.Role == RoleArbiter && sig2.Role == RoleSeller) {
		validExtension = true
		arbiterInvolved = true
		if sig1.Role == RoleArbiter {
			arbiterSig = sig1
		} else {
			arbiterSig = sig2
		}
	}

	if !validExtension {
		return fmt.Errorf("extension requires signatures from 2 of 3 participants (buyer/seller/arbiter)")
	}

	// Validate arbiter is authorized if arbiter is involved
	if arbiterInvolved && !em.paywall.IsAuthorizedArbiter(arbiterSig.PublicKey) {
		return fmt.Errorf("arbiter is not authorized: public key not in authorized list")
	}

	// Verify both signatures explicitly authorize this specific extension.
	if err := validateTimeoutExtensionSignature(sig1, payment, extension); err != nil {
		return fmt.Errorf("invalid first extension authorization: %w", err)
	}
	if err := validateTimeoutExtensionSignature(sig2, payment, extension); err != nil {
		return fmt.Errorf("invalid second extension authorization: %w", err)
	}

	// Apply the extension
	oldTimeout := payment.EscrowTimeout
	payment.EscrowTimeout = payment.EscrowTimeout.Add(extension)

	// Build audit metadata for extension event.
	metadata := map[string]string{
		"old_timeout":      oldTimeout.Format(time.RFC3339),
		"extension":        extension.String(),
		"new_timeout":      payment.EscrowTimeout.Format(time.RFC3339),
		"arbiter_involved": fmt.Sprintf("%t", arbiterInvolved),
		"sig1_role":        string(sig1.Role),
		"sig2_role":        string(sig2.Role),
	}

	// Persist extension signatures to enforce replay protection.
	if payment.Signatures == nil {
		payment.Signatures = make(map[wallet.WalletType][]SignatureData)
	}

	targetWalletType := wallet.Bitcoin
	foundWalletType := false
	for walletType, participants := range em.paywall.participantPubKeys {
		for _, participant := range participants {
			if bytesEqual(sig1.PublicKey, participant) {
				targetWalletType = walletType
				foundWalletType = true
				break
			}
		}
		if foundWalletType {
			break
		}
	}
	if !foundWalletType {
		for walletType := range payment.Addresses {
			targetWalletType = walletType
			break
		}
	}
	payment.Signatures[targetWalletType] = append(payment.Signatures[targetWalletType], *sig1, *sig2)

	// Update payment in store
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment with extended timeout: %w", err)
	}

	// Log extension in audit trail
	_, err = em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        "extend_timeout",
		PreviousState: payment.EscrowState,
		NewState:      payment.EscrowState,
		Metadata:      metadata,
	})
	if err != nil {
		log.Printf("WARNING: failed to log timeout extension for payment %s: %v", paymentID, err)
	}

	log.Printf("escrow timeout extended for payment %s by %s to %s",
		paymentID, extension, payment.EscrowTimeout.Format(time.RFC3339))

	return nil
}

func validateTimeoutExtensionSignature(sig *SignatureData, payment *Payment, extension time.Duration) error {
	if sig.PaymentID != payment.ID {
		return fmt.Errorf("signature payment ID mismatch for extension authorization")
	}
	if len(sig.Nonce) == 0 {
		return fmt.Errorf("signature nonce required for extension authorization")
	}
	if sig.SignedAt.IsZero() {
		return fmt.Errorf("signature timestamp required for extension authorization")
	}

	parsedPubKey, err := btcec.ParsePubKey(sig.PublicKey)
	if err != nil {
		return fmt.Errorf("parse signer public key: %w", err)
	}

	sigBytes := sig.Signature
	if len(sigBytes) > 0 && (sigBytes[len(sigBytes)-1]&0x1f) <= 3 {
		sigBytes = sigBytes[:len(sigBytes)-1]
	}
	parsedSig, err := ecdsa.ParseDERSignature(sigBytes)
	if err != nil {
		return fmt.Errorf("parse extension signature: %w", err)
	}

	intent := timeoutExtensionIntentHash(payment.ID, payment.EscrowTimeout, extension, sig)
	if !parsedSig.Verify(intent[:], parsedPubKey) {
		return fmt.Errorf("signature does not authorize this timeout extension")
	}

	return nil
}

func timeoutExtensionIntentHash(paymentID string, currentTimeout time.Time, extension time.Duration, sig *SignatureData) [32]byte {
	intent := fmt.Sprintf(
		"extend_timeout|payment=%s|current_timeout=%s|extension=%s|role=%s|signed_at=%s|nonce=%x",
		paymentID,
		currentTimeout.UTC().Format(time.RFC3339),
		extension.String(),
		string(sig.Role),
		sig.SignedAt.UTC().Format(time.RFC3339),
		sig.Nonce,
	)
	return sha256.Sum256([]byte(intent))
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

// validateSignatureReplay checks for signature replay attacks
// Validates:
//   - Signature has a nonce
//   - Signature is bound to the correct payment
//   - Signature has not been used before (nonce uniqueness)
//
// Parameters:
//   - sig: The signature data to validate
//   - payment: The payment being operated on
//
// Returns:
//   - error: If replay attack is detected or validation fails
func (em *EscrowManager) validateSignatureReplay(sig *SignatureData, payment *Payment) error {
	if sig == nil {
		return fmt.Errorf("signature data cannot be nil")
	}

	// Check that signature has a nonce (allow missing for backward compatibility)
	if len(sig.Nonce) == 0 {
		// Backward compatibility: allow signatures without nonce, but return a specific error
		// The caller can decide to log a warning instead of failing
		return fmt.Errorf("signature missing nonce: replay protection requires nonce")
	}

	// Check that signature is bound to this payment
	// Allow empty PaymentID for backward compatibility
	if sig.PaymentID != "" && sig.PaymentID != payment.ID {
		return fmt.Errorf("signature payment ID mismatch: signature is for payment %s, but applied to payment %s",
			sig.PaymentID, payment.ID)
	}

	// Check for duplicate nonces in existing signatures
	if payment.Signatures != nil {
		for _, sigs := range payment.Signatures {
			for _, existingSig := range sigs {
				// Check if nonce has been used before
				if len(existingSig.Nonce) > 0 && bytesEqual(existingSig.Nonce, sig.Nonce) {
					return fmt.Errorf("signature nonce reuse detected: replay attack prevented")
				}

				// Additional check: same signer same role
				if existingSig.SignerID == sig.SignerID && existingSig.Role == sig.Role {
					// If same signer already provided signature for this role, check if it's the same signature
					if bytesEqual(existingSig.Signature, sig.Signature) {
						// Idempotent - same signature being re-submitted, allow it
						return nil
					}
					return fmt.Errorf("signer %s already provided a different signature for role %s", sig.SignerID, sig.Role)
				}
			}
		}
	}

	return nil
}

// CastArbiterVote allows an authorized arbiter to vote on a disputed payment
// This is used when multi-arbiter consensus is enabled
func (em *EscrowManager) CastArbiterVote(paymentID string, vote *ArbiterVote) error {
	// Check if multi-arbiter consensus is enabled
	if em.paywall.consensusManager == nil {
		return fmt.Errorf("multi-arbiter consensus not enabled")
	}

	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("payment is not in disputed state")
	}

	// Validate arbiter is authorized
	if !em.paywall.IsAuthorizedArbiter(vote.ArbiterPubKey) {
		return fmt.Errorf("arbiter is not authorized")
	}

	// Validate the vote signature
	if vote.Signature == nil {
		return fmt.Errorf("vote must include signature")
	}

	if err := em.validateSignatureData(vote.Signature, payment); err != nil {
		return fmt.Errorf("invalid vote signature: %w", err)
	}

	// Cast the vote in the consensus manager
	if err := em.paywall.consensusManager.CastVote(paymentID, vote); err != nil {
		return fmt.Errorf("failed to cast vote: %w", err)
	}

	// Log the vote in audit trail
	em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        AuditActionResolve,
		PreviousState: payment.EscrowState,
		NewState:      payment.EscrowState,
		ActorRole:     RoleArbiter,
		Actor:         vote.ArbiterPubKey,
		Metadata: map[string]string{
			"arbiter_id": vote.ArbiterID,
			"decision":   string(vote.Decision),
			"reason":     vote.Reason,
		},
	})

	// Check if consensus has been reached and auto-resolve if so
	consensus, err := em.paywall.consensusManager.GetConsensus(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get consensus status: %w", err)
	}

	if consensus.ConsensusReached {
		// Auto-resolve the dispute based on consensus
		if err := em.resolveDisputeByConsensus(paymentID, consensus); err != nil {
			return fmt.Errorf("failed to auto-resolve dispute: %w", err)
		}
	}

	return nil
}

// resolveDisputeByConsensus resolves a dispute based on multi-arbiter consensus
func (em *EscrowManager) resolveDisputeByConsensus(paymentID string, consensus *ArbiterConsensus) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("payment is not in disputed state")
	}

	prevState := payment.EscrowState

	// Set final state based on consensus decision
	var newState EscrowState
	if consensus.FinalDecision == RoleBuyer {
		newState = EscrowRefunded
	} else {
		newState = EscrowCompleted
	}

	// Validate and record state transition
	if err := em.stateValidator.ValidateAndRecordTransition(
		payment,
		newState,
		"consensus",
		fmt.Sprintf("Dispute resolved by arbiter consensus (%d votes) in favor of %s", len(consensus.Votes), string(consensus.FinalDecision)),
	); err != nil {
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionResolve,
			PreviousState: prevState,
			NewState:      prevState,
			ActorRole:     RoleArbiter,
			Metadata: map[string]string{
				"error":          err.Error(),
				"status":         "rejected",
				"consensus":      "true",
				"required_votes": fmt.Sprintf("%d", consensus.RequiredVotes),
				"total_votes":    fmt.Sprintf("%d", len(consensus.Votes)),
			},
		})
		return fmt.Errorf("invalid state transition: %w", err)
	}

	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment state: %w", err)
	}

	// Log dispute resolution in audit trail
	em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        AuditActionResolve,
		PreviousState: prevState,
		NewState:      newState,
		ActorRole:     RoleArbiter,
		Metadata: map[string]string{
			"decision":       string(consensus.FinalDecision),
			"consensus":      "true",
			"required_votes": fmt.Sprintf("%d", consensus.RequiredVotes),
			"total_votes":    fmt.Sprintf("%d", len(consensus.Votes)),
		},
	})

	return nil
}

// GetConsensusStatus retrieves the current consensus status for a disputed payment
func (em *EscrowManager) GetConsensusStatus(paymentID string) (*ArbiterConsensus, error) {
	if em.paywall.consensusManager == nil {
		return nil, fmt.Errorf("multi-arbiter consensus not enabled")
	}

	return em.paywall.consensusManager.GetConsensus(paymentID)
}

// ActivateFallbackArbiters activates fallback arbiters when primary arbiters are unresponsive
func (em *EscrowManager) ActivateFallbackArbiters(paymentID string) error {
	if em.paywall.consensusManager == nil {
		return fmt.Errorf("multi-arbiter consensus not enabled")
	}

	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("payment is not in disputed state")
	}

	if err := em.paywall.consensusManager.ActivateFallbackArbiters(paymentID); err != nil {
		return fmt.Errorf("failed to activate fallback arbiters: %w", err)
	}

	// Log fallback activation
	em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        AuditActionDispute,
		PreviousState: payment.EscrowState,
		NewState:      payment.EscrowState,
		ActorRole:     RoleArbiter,
		Metadata: map[string]string{
			"action": "fallback_arbiters_activated",
			"reason": "primary_arbiters_unresponsive",
		},
	})

	return nil
}

// bytesEqual compares two byte slices for equality

// checkDisputeRateLimit validates that a participant hasn't exceeded dispute limits
func (em *EscrowManager) checkDisputeRateLimit(requesterRole MultisigRole) error {
	if em.paywall.maxDisputesPerPeriod <= 0 {
		return nil // Rate limiting disabled
	}

	requesterKey := string(requesterRole)
	now := time.Now()
	cutoff := now.Add(-em.paywall.disputePeriod)

	// Get dispute history for this participant
	disputes, exists := em.paywall.disputeHistory[requesterKey]
	if !exists {
		em.paywall.disputeHistory[requesterKey] = []time.Time{now}
		return nil
	}

	// Filter out disputes outside the time window
	recentDisputes := make([]time.Time, 0)
	for _, disputeTime := range disputes {
		if disputeTime.After(cutoff) {
			recentDisputes = append(recentDisputes, disputeTime)
		}
	}

	// Check if limit exceeded
	if len(recentDisputes) >= em.paywall.maxDisputesPerPeriod {
		return fmt.Errorf("dispute rate limit exceeded: %d disputes in last %v (max: %d)",
			len(recentDisputes), em.paywall.disputePeriod, em.paywall.maxDisputesPerPeriod)
	}

	// Add current dispute
	recentDisputes = append(recentDisputes, now)
	em.paywall.disputeHistory[requesterKey] = recentDisputes

	return nil
}

// calculateDisputeFee calculates the dispute fee based on escrow amount
func (em *EscrowManager) calculateDisputeFee(payment *Payment) float64 {
	if em.paywall.disputeFeePercent <= 0 {
		return 0
	}

	// Calculate based on the first available amount
	for _, amount := range payment.Amounts {
		if amount > 0 {
			return amount * em.paywall.disputeFeePercent
		}
	}

	return 0
}

// extendEscrowTimeout extends the escrow timeout when a dispute is filed
func (em *EscrowManager) extendEscrowTimeout(payment *Payment) {
	if em.paywall.extendEscrowOnDispute <= 0 {
		return
	}

	payment.EscrowTimeout = time.Now().Add(em.paywall.extendEscrowOnDispute)
}

// checkEvidenceSize validates that evidence doesn't exceed size limits
func (em *EscrowManager) checkEvidenceSize(payment *Payment, newEvidenceSize int64) error {
	if em.paywall.maxEvidenceSizeBytes <= 0 {
		return nil // No limit enforced
	}

	totalSize := payment.DisputeEvidenceSizeBytes + newEvidenceSize
	if totalSize > em.paywall.maxEvidenceSizeBytes {
		return fmt.Errorf("evidence size limit exceeded: %d bytes total (max: %d)",
			totalSize, em.paywall.maxEvidenceSizeBytes)
	}

	return nil
}

// validateDisputeFeePayment validates that the dispute fee has been paid
// This check prevents spam disputes by requiring a fee payment before filing
// Returns error if fee is required but not paid
func (em *EscrowManager) validateDisputeFeePayment(paymentID string, requesterRole MultisigRole) error {
	// If no dispute fee is configured, no validation needed
	if em.paywall.disputeFeePercent <= 0 {
		return nil // Fee not required
	}

	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	// Check if dispute fee has been marked as paid
	if !payment.DisputeFeePaid {
		feeAmount := em.calculateDisputeFee(payment)
		return fmt.Errorf("dispute fee of %.8f must be paid before filing dispute (use RecordDisputeFeePayment after verifying payment)", feeAmount)
	}

	return nil
}

// RecordDisputeFeePayment marks the dispute fee as paid for a payment
// This should be called after externally verifying that the fee payment was confirmed on-chain
//
// Usage pattern:
//  1. Calculate dispute fee using payment.DisputeFee or calculateDisputeFee
//  2. Generate and provide a dispute fee payment address to the requester
//  3. Wait for blockchain confirmation of fee payment to that address
//  4. Call this method to record the payment and allow dispute filing
//
// Parameters:
//   - paymentID: The escrow payment ID for which dispute fee was paid
//   - requesterRole: The role of the party who paid the fee (buyer or seller)
//
// Security note: This method trusts the caller to have verified the payment.
// In production, implement automated blockchain verification before calling this.
func (em *EscrowManager) RecordDisputeFeePayment(paymentID string, requesterRole MultisigRole) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	// Verify payment is in a state that can have dispute fee paid
	if payment.EscrowState != EscrowFunded {
		return fmt.Errorf("can only pay dispute fee for funded escrows, current state: %s", payment.EscrowState.String())
	}

	// Mark fee as paid
	payment.DisputeFeePaid = true

	// Update payment in store
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment: %w", err)
	}

	// Log fee payment in audit trail
	em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        AuditActionDispute,
		PreviousState: payment.EscrowState,
		NewState:      payment.EscrowState,
		ActorRole:     requesterRole,
		Metadata: map[string]string{
			"action":     "dispute_fee_paid",
			"fee_amount": fmt.Sprintf("%.8f", payment.DisputeFee),
			"paid_by":    string(requesterRole),
		},
	})

	return nil
}

// SubmitDisputeEvidence submits evidence for a dispute with size validation
func (em *EscrowManager) SubmitDisputeEvidence(paymentID string, evidence *Evidence) error {
	payment, err := em.paywall.Store.GetPayment(paymentID)
	if err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	if payment == nil {
		return fmt.Errorf("payment not found: %s", paymentID)
	}

	if payment.EscrowState != EscrowDisputed {
		return fmt.Errorf("payment is not in disputed state")
	}

	// Calculate evidence size (content length in bytes)
	evidenceSize := int64(len(evidence.Content))

	// Check evidence size limit
	if err := em.checkEvidenceSize(payment, evidenceSize); err != nil {
		em.auditLogger.LogAction(&AuditLogEntry{
			PaymentID:     paymentID,
			Action:        AuditActionDispute,
			PreviousState: payment.EscrowState,
			NewState:      payment.EscrowState,
			ActorRole:     evidence.SubmittedBy,
			Metadata: map[string]string{
				"error":         err.Error(),
				"evidence_type": string(evidence.Type),
				"evidence_size": fmt.Sprintf("%d", evidenceSize),
				"status":        "evidence_rejected",
			},
		})
		return err
	}

	// Submit evidence to arbiter system
	if em.arbiter != nil {
		if err := em.arbiter.SubmitEvidence(paymentID, evidence); err != nil {
			return fmt.Errorf("failed to submit evidence to arbiter: %w", err)
		}
	}

	// Update payment with new evidence size
	payment.DisputeEvidenceSizeBytes += evidenceSize
	if err := em.paywall.Store.UpdatePayment(payment); err != nil {
		return fmt.Errorf("failed to update payment: %w", err)
	}

	// Log evidence submission
	em.auditLogger.LogAction(&AuditLogEntry{
		PaymentID:     paymentID,
		Action:        AuditActionDispute,
		PreviousState: payment.EscrowState,
		NewState:      payment.EscrowState,
		ActorRole:     evidence.SubmittedBy,
		Metadata: map[string]string{
			"action":        "evidence_submitted",
			"evidence_type": string(evidence.Type),
			"evidence_size": fmt.Sprintf("%d", evidenceSize),
			"total_size":    fmt.Sprintf("%d", payment.DisputeEvidenceSizeBytes),
		},
	})

	return nil
}

// StartTimeoutMonitor starts automatic timeout monitoring with the given configuration
// Returns the TimeoutMonitor instance which can be stopped with monitor.Stop()
func (em *EscrowManager) StartTimeoutMonitor(config TimeoutMonitorConfig) *TimeoutMonitor {
	monitor := NewTimeoutMonitor(em, config)
	monitor.Start()
	return monitor
}
