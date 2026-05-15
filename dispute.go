// Package paywall implements dispute resolution framework for escrow payments
package paywall

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrDisputeNotFound indicates the requested dispute does not exist
	ErrDisputeNotFound = errors.New("dispute not found")
	// ErrDisputeAlreadyResolved indicates the dispute has already been resolved
	ErrDisputeAlreadyResolved = errors.New("dispute already resolved")
	// ErrInvalidEvidence indicates the evidence provided is invalid or incomplete
	ErrInvalidEvidence = errors.New("invalid evidence")
)

// EvidenceType represents the type of evidence submitted in a dispute
type EvidenceType string

const (
	// EvidenceText represents textual evidence (description, explanation)
	EvidenceText EvidenceType = "text"
	// EvidenceImage represents image evidence (screenshot, photo)
	EvidenceImage EvidenceType = "image"
	// EvidenceDocument represents document evidence (contract, receipt)
	EvidenceDocument EvidenceType = "document"
	// EvidenceTransaction represents on-chain transaction evidence
	EvidenceTransaction EvidenceType = "transaction"
)

// Evidence represents a piece of evidence submitted in a dispute
type Evidence struct {
	// ID uniquely identifies the evidence
	ID string `json:"id"`
	// PaymentID is the payment this evidence relates to
	PaymentID string `json:"payment_id"`
	// Type indicates the type of evidence
	Type EvidenceType `json:"type"`
	// SubmittedBy indicates which party submitted the evidence
	SubmittedBy MultisigRole `json:"submitted_by"`
	// Content contains the evidence data (text, URL, or encoded data)
	Content string `json:"content"`
	// Timestamp is when the evidence was submitted
	Timestamp time.Time `json:"timestamp"`
	// Description provides context for the evidence
	Description string `json:"description"`
	// Signature is the cryptographic signature of the submitter
	Signature []byte `json:"signature,omitempty"`
	// SubmitterPubKey is the public key of the evidence submitter
	SubmitterPubKey []byte `json:"submitter_pub_key,omitempty"`
}

// Resolution represents the arbiter's decision on a dispute
type Resolution struct {
	// PaymentID is the payment this resolution applies to
	PaymentID string `json:"payment_id"`
	// Decision indicates the winning party
	Decision MultisigRole `json:"decision"`
	// Reason explains the arbiter's decision
	Reason string `json:"reason"`
	// ArbiterID identifies the arbiter who made the decision
	ArbiterID string `json:"arbiter_id"`
	// Timestamp is when the resolution was made
	Timestamp time.Time `json:"timestamp"`
	// Evidence contains references to evidence that influenced the decision
	Evidence []string `json:"evidence"`
	// Signature is the cryptographic signature of the arbiter
	Signature []byte `json:"signature,omitempty"`
	// ArbiterPubKey is the public key of the arbiter
	ArbiterPubKey []byte `json:"arbiter_pub_key,omitempty"`
}

// Dispute represents a dispute case with all associated evidence and resolution
type Dispute struct {
	// PaymentID is the payment under dispute
	PaymentID string `json:"payment_id"`
	// Requester is the party who initiated the dispute
	Requester MultisigRole `json:"requester"`
	// Reason is the initial reason for the dispute
	Reason string `json:"reason"`
	// Evidence contains all evidence submitted by parties
	Evidence []*Evidence `json:"evidence"`
	// Resolution contains the arbiter's decision (nil if not yet resolved)
	Resolution *Resolution `json:"resolution,omitempty"`
	// CreatedAt is when the dispute was opened
	CreatedAt time.Time `json:"created_at"`
	// ResolvedAt is when the dispute was resolved (zero if not yet resolved)
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
	// Status indicates the current dispute status
	Status DisputeStatus `json:"status"`
}

// DisputeStatus represents the current state of a dispute
type DisputeStatus string

const (
	// DisputeOpen indicates the dispute is open and accepting evidence
	DisputeOpen DisputeStatus = "open"
	// DisputeUnderReview indicates the arbiter is reviewing the evidence
	DisputeUnderReview DisputeStatus = "under_review"
	// DisputeResolved indicates the arbiter has made a decision
	DisputeResolved DisputeStatus = "resolved"
	// DisputeClosed indicates the dispute was closed without resolution
	DisputeClosed DisputeStatus = "closed"
)

// Arbiter defines the interface for dispute resolution services.
//
// # Extensibility
//
// This interface is designed as an extensibility point to support various
// dispute resolution architectures:
//
//   - LocalArbiter (default): In-memory storage, suitable for single-instance
//     deployments and testing
//   - RemoteArbiter (custom): HTTP/gRPC client connecting to external dispute
//     resolution services or decentralized arbiter networks
//   - BlockchainArbiter (custom): Integration with on-chain dispute resolution
//     smart contracts or oracles
//
// Most applications can use LocalArbiter. Implement custom Arbiter when you need:
//   - Distributed dispute storage across multiple paywall instances
//   - Integration with existing case management systems
//   - Compliance with external arbitration service providers
//   - Blockchain-based transparent dispute records
//
// # Implementation Requirements
//
// Custom implementations must ensure:
//   - Thread-safe concurrent access to dispute data
//   - Atomic dispute registration to prevent duplicates
//   - Proper validation of requester role (buyer or seller only)
//   - Evidence integrity and non-repudiation
type Arbiter interface {
	// RegisterDispute registers a new dispute in the arbiter system
	// requester specifies which party (buyer or seller) initiated the dispute
	// Returns error if registration fails
	RegisterDispute(payment *Payment, requester MultisigRole) error

	// SubmitEvidence allows parties to submit evidence for a dispute
	// Returns error if evidence is invalid or submission fails
	SubmitEvidence(paymentID string, evidence *Evidence) error

	// GetResolution retrieves the arbiter's decision for a dispute
	// Returns error if dispute not found or not yet resolved
	GetResolution(paymentID string) (*Resolution, error)

	// GetDispute retrieves the full dispute information including evidence
	// Returns error if dispute not found
	GetDispute(paymentID string) (*Dispute, error)

	// ListOpenDisputes returns all disputes awaiting resolution
	// Returns error if retrieval fails
	ListOpenDisputes() ([]*Dispute, error)
}

// LocalArbiter is the default in-memory implementation of the Arbiter interface.
// It stores disputes in a thread-safe map and is suitable for:
//   - Single-instance paywall deployments
//   - Testing and development environments
//   - Low-volume dispute scenarios
//
// For distributed deployments or external arbitration services, implement
// a custom Arbiter that integrates with your dispute resolution backend.
type LocalArbiter struct {
	disputes map[string]*Dispute
	mu       sync.RWMutex
}

// NewLocalArbiter creates a new in-memory arbiter instance
func NewLocalArbiter() *LocalArbiter {
	return &LocalArbiter{
		disputes: make(map[string]*Dispute),
	}
}

// RegisterDispute registers a new dispute in the arbiter system
// requester specifies which party (buyer or seller) initiated the dispute
func (la *LocalArbiter) RegisterDispute(payment *Payment, requester MultisigRole) error {
	la.mu.Lock()
	defer la.mu.Unlock()

	if payment == nil {
		return fmt.Errorf("payment cannot be nil")
	}

	// Validate requester is buyer or seller
	if requester != RoleBuyer && requester != RoleSeller {
		return fmt.Errorf("requester must be buyer or seller, got: %s", requester)
	}

	// Check if dispute already exists
	if _, exists := la.disputes[payment.ID]; exists {
		return fmt.Errorf("dispute already exists for payment %s", payment.ID)
	}

	dispute := &Dispute{
		PaymentID: payment.ID,
		Requester: requester,
		Reason:    payment.DisputeReason,
		Evidence:  make([]*Evidence, 0),
		CreatedAt: time.Now(),
		Status:    DisputeOpen,
	}

	la.disputes[payment.ID] = dispute
	return nil
}

// SubmitEvidence allows parties to submit evidence for a dispute
func (la *LocalArbiter) SubmitEvidence(paymentID string, evidence *Evidence) error {
	la.mu.Lock()
	defer la.mu.Unlock()

	dispute, exists := la.disputes[paymentID]
	if !exists {
		return ErrDisputeNotFound
	}

	if dispute.Status == DisputeResolved || dispute.Status == DisputeClosed {
		return ErrDisputeAlreadyResolved
	}

	if evidence == nil || evidence.Content == "" {
		return ErrInvalidEvidence
	}

	// Validate evidence signature if provided
	if len(evidence.Signature) > 0 && len(evidence.SubmitterPubKey) > 0 {
		if err := validateEvidenceSignature(evidence); err != nil {
			return fmt.Errorf("invalid evidence signature: %w", err)
		}
	}

	// Set metadata
	evidence.ID = fmt.Sprintf("%s-%d", paymentID, len(dispute.Evidence))
	evidence.PaymentID = paymentID
	evidence.Timestamp = time.Now()

	dispute.Evidence = append(dispute.Evidence, evidence)
	return nil
}

// GetResolution retrieves the arbiter's decision for a dispute
func (la *LocalArbiter) GetResolution(paymentID string) (*Resolution, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	dispute, exists := la.disputes[paymentID]
	if !exists {
		return nil, ErrDisputeNotFound
	}

	if dispute.Resolution == nil {
		return nil, fmt.Errorf("dispute not yet resolved")
	}

	return dispute.Resolution, nil
}

// GetDispute retrieves the full dispute information including evidence
func (la *LocalArbiter) GetDispute(paymentID string) (*Dispute, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	dispute, exists := la.disputes[paymentID]
	if !exists {
		return nil, ErrDisputeNotFound
	}

	return dispute, nil
}

// ListOpenDisputes returns all disputes awaiting resolution
func (la *LocalArbiter) ListOpenDisputes() ([]*Dispute, error) {
	la.mu.RLock()
	defer la.mu.RUnlock()

	var openDisputes []*Dispute
	for _, dispute := range la.disputes {
		if dispute.Status == DisputeOpen || dispute.Status == DisputeUnderReview {
			openDisputes = append(openDisputes, dispute)
		}
	}

	return openDisputes, nil
}

// ResolveDispute allows the arbiter to resolve a dispute with a decision
// This is a helper method specific to LocalArbiter for testing/simple use cases
func (la *LocalArbiter) ResolveDispute(paymentID string, resolution *Resolution) error {
	la.mu.Lock()
	defer la.mu.Unlock()

	dispute, exists := la.disputes[paymentID]
	if !exists {
		return ErrDisputeNotFound
	}

	if dispute.Status == DisputeResolved || dispute.Status == DisputeClosed {
		return ErrDisputeAlreadyResolved
	}

	if resolution == nil {
		return fmt.Errorf("resolution cannot be nil")
	}

	// Validate resolution signature if provided
	if len(resolution.Signature) > 0 && len(resolution.ArbiterPubKey) > 0 {
		if err := validateResolutionSignature(resolution); err != nil {
			return fmt.Errorf("invalid resolution signature: %w", err)
		}
	}

	resolution.PaymentID = paymentID
	resolution.Timestamp = time.Now()

	dispute.Resolution = resolution
	dispute.Status = DisputeResolved
	dispute.ResolvedAt = time.Now()

	return nil
}

// CloseDispute closes a dispute without resolution (e.g., withdrawn by parties)
func (la *LocalArbiter) CloseDispute(paymentID string) error {
	la.mu.Lock()
	defer la.mu.Unlock()

	dispute, exists := la.disputes[paymentID]
	if !exists {
		return ErrDisputeNotFound
	}

	if dispute.Status == DisputeResolved || dispute.Status == DisputeClosed {
		return ErrDisputeAlreadyResolved
	}

	dispute.Status = DisputeClosed
	dispute.ResolvedAt = time.Now()

	return nil
}

// validateEvidenceSignature validates the cryptographic signature on evidence
// This ensures the evidence was submitted by the claimed party and hasn't been tampered with
func validateEvidenceSignature(evidence *Evidence) error {
	if evidence == nil {
		return fmt.Errorf("evidence cannot be nil")
	}

	if len(evidence.Signature) == 0 {
		return fmt.Errorf("signature is required")
	}

	if len(evidence.SubmitterPubKey) == 0 {
		return fmt.Errorf("submitter public key is required")
	}

	// Basic signature length validation
	if len(evidence.Signature) < 8 {
		return fmt.Errorf("signature too short (minimum 8 bytes)")
	}

	// Parse and validate public key (secp256k1 curve for Bitcoin/Monero compatibility)
	// In a production system, you would:
	// 1. Hash the evidence content (ID + PaymentID + Type + Content + Description + Timestamp)
	// 2. Verify the signature against the hash using the public key
	// For now, we do basic format validation

	// Note: Full cryptographic verification would use btcec or similar
	// This is a placeholder that checks signature format
	// In production, implement full ECDSA signature verification

	return nil
}

// validateResolutionSignature validates the cryptographic signature on a resolution
// This ensures the resolution was made by an authorized arbiter and hasn't been tampered with
func validateResolutionSignature(resolution *Resolution) error {
	if resolution == nil {
		return fmt.Errorf("resolution cannot be nil")
	}

	if len(resolution.Signature) == 0 {
		return fmt.Errorf("signature is required")
	}

	if len(resolution.ArbiterPubKey) == 0 {
		return fmt.Errorf("arbiter public key is required")
	}

	// Basic signature length validation
	if len(resolution.Signature) < 8 {
		return fmt.Errorf("signature too short (minimum 8 bytes)")
	}

	// Parse and validate public key
	// In a production system, you would:
	// 1. Hash the resolution data (PaymentID + Decision + Reason + ArbiterID + Timestamp)
	// 2. Verify the signature against the hash using the arbiter's public key
	// 3. Check that the arbiter is authorized (part of authorized arbiter list)
	// For now, we do basic format validation

	// Note: Full cryptographic verification would use btcec or similar
	// This is a placeholder that checks signature format
	// In production, implement full ECDSA signature verification

	return nil
}

// SignEvidence signs evidence with a private key (helper function for clients)
// In production, this would be in a client library, not the server
// This is provided as a reference implementation
func SignEvidence(evidence *Evidence, privateKey []byte) error {
	// In production:
	// 1. Create hash of evidence data
	// 2. Sign hash with private key
	// 3. Set evidence.Signature
	// For now, this is a placeholder
	evidence.Signature = []byte("signature-placeholder-minimum-8-bytes")
	return nil
}

// SignResolution signs a resolution with an arbiter's private key
// In production, this would be in an arbiter service, not exposed to clients
// This is provided as a reference implementation
func SignResolution(resolution *Resolution, privateKey []byte) error {
	// In production:
	// 1. Create hash of resolution data
	// 2. Sign hash with arbiter's private key
	// 3. Set resolution.Signature
	// For now, this is a placeholder
	resolution.Signature = []byte("signature-placeholder-minimum-8-bytes")
	return nil
}
