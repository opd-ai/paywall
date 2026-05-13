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

// Arbiter defines the interface for dispute resolution services
// Integrators can provide their own implementations of this interface
type Arbiter interface {
	// RegisterDispute registers a new dispute in the arbiter system
	// Returns error if registration fails
	RegisterDispute(payment *Payment) error

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

// LocalArbiter is an in-memory implementation of the Arbiter interface
// Suitable for testing or single-instance deployments
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
func (la *LocalArbiter) RegisterDispute(payment *Payment) error {
	la.mu.Lock()
	defer la.mu.Unlock()

	if payment == nil {
		return fmt.Errorf("payment cannot be nil")
	}

	// Check if dispute already exists
	if _, exists := la.disputes[payment.ID]; exists {
		return fmt.Errorf("dispute already exists for payment %s", payment.ID)
	}

	dispute := &Dispute{
		PaymentID: payment.ID,
		Requester: RoleBuyer, // Default, should be set based on who requested
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
