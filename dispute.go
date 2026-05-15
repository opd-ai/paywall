// Package paywall implements dispute resolution framework for escrow payments
package paywall

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
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
	// Signature is the cryptographic signature over the evidence data
	// This ensures non-repudiation and tamper-evidence
	Signature []byte `json:"signature,omitempty"`
	// PublicKey is the submitter's public key for signature verification
	PublicKey []byte `json:"public_key,omitempty"`
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
	// Signature is the arbiter's cryptographic signature over the resolution
	// This ensures non-repudiation and authenticity of the decision
	Signature []byte `json:"signature,omitempty"`
	// PublicKey is the arbiter's public key for signature verification
	PublicKey []byte `json:"public_key,omitempty"`
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

// computeEvidenceHash generates a cryptographic hash of evidence data for signing
// The hash includes all critical fields to prevent tampering
func computeEvidenceHash(e *Evidence) [32]byte {
	h := sha256.New()
	writeHashString(h, e.PaymentID)
	writeHashString(h, string(e.Type))
	writeHashString(h, string(e.SubmittedBy))
	writeHashString(h, e.Content)
	writeHashString(h, e.Description)
	writeHashTimestamp(h, e.Timestamp)

	var hash [32]byte
	copy(hash[:], h.Sum(nil))
	return hash
}

// SignEvidence signs evidence data with the submitter's private key
// Returns error if signing fails
func SignEvidence(evidence *Evidence, privateKey *btcec.PrivateKey) error {
	if evidence == nil {
		return errors.New("evidence cannot be nil")
	}
	if privateKey == nil {
		return errors.New("private key cannot be nil")
	}

	// Compute hash of evidence data
	hash := computeEvidenceHash(evidence)

	// Sign the hash
	signature := ecdsa.Sign(privateKey, hash[:])
	evidence.Signature = signature.Serialize()
	evidence.PublicKey = privateKey.PubKey().SerializeCompressed()

	return nil
}

// VerifyEvidenceSignature verifies the signature on evidence data
// Returns true if signature is valid, false otherwise
func VerifyEvidenceSignature(evidence *Evidence) (bool, error) {
	if evidence == nil {
		return false, errors.New("evidence cannot be nil")
	}
	if len(evidence.Signature) == 0 {
		return false, errors.New("evidence has no signature")
	}
	if len(evidence.PublicKey) == 0 {
		return false, errors.New("evidence has no public key")
	}

	// Parse public key
	pubKey, err := btcec.ParsePubKey(evidence.PublicKey)
	if err != nil {
		return false, fmt.Errorf("invalid public key: %w", err)
	}

	// Parse signature
	sig, err := ecdsa.ParseSignature(evidence.Signature)
	if err != nil {
		return false, fmt.Errorf("invalid signature: %w", err)
	}

	// Compute hash of evidence data
	hash := computeEvidenceHash(evidence)

	// Verify signature
	return sig.Verify(hash[:], pubKey), nil
}

// computeResolutionHash generates a cryptographic hash of resolution data for signing
// The hash includes all critical fields to prevent tampering
func computeResolutionHash(r *Resolution) [32]byte {
	h := sha256.New()
	writeHashString(h, r.PaymentID)
	writeHashString(h, string(r.Decision))
	writeHashString(h, r.Reason)
	writeHashString(h, r.ArbiterID)
	writeHashTimestamp(h, r.Timestamp)
	writeHashStringSlice(h, r.Evidence)

	var hash [32]byte
	copy(hash[:], h.Sum(nil))
	return hash
}

// writeHashString writes a length-prefixed string into the hash stream.
// Length prefixing prevents ambiguous field boundaries in signed payloads.
func writeHashString(h hash.Hash, value string) {
	var lengthBytes [8]byte
	binary.BigEndian.PutUint64(lengthBytes[:], uint64(len(value)))
	h.Write(lengthBytes[:])
	h.Write([]byte(value))
}

// writeHashStringSlice writes a length-prefixed string slice into the hash stream.
// It includes element count and each element's length for deterministic encoding.
func writeHashStringSlice(h hash.Hash, values []string) {
	var countBytes [8]byte
	binary.BigEndian.PutUint64(countBytes[:], uint64(len(values)))
	h.Write(countBytes[:])
	for _, value := range values {
		writeHashString(h, value)
	}
}

// writeHashTimestamp writes timestamp seconds and nanoseconds into the hash stream.
// Including both values preserves sub-second precision for signature verification.
func writeHashTimestamp(h hash.Hash, ts time.Time) {
	var secondsBytes [8]byte
	binary.BigEndian.PutUint64(secondsBytes[:], uint64(ts.Unix()))
	h.Write(secondsBytes[:])

	var nanosBytes [4]byte
	binary.BigEndian.PutUint32(nanosBytes[:], uint32(ts.Nanosecond()))
	h.Write(nanosBytes[:])
}

// SignResolution signs resolution data with the arbiter's private key
// Returns error if signing fails
func SignResolution(resolution *Resolution, privateKey *btcec.PrivateKey) error {
	if resolution == nil {
		return errors.New("resolution cannot be nil")
	}
	if privateKey == nil {
		return errors.New("private key cannot be nil")
	}

	// Compute hash of resolution data
	hash := computeResolutionHash(resolution)

	// Sign the hash
	signature := ecdsa.Sign(privateKey, hash[:])
	resolution.Signature = signature.Serialize()
	resolution.PublicKey = privateKey.PubKey().SerializeCompressed()

	return nil
}

// VerifyResolutionSignature verifies the signature on resolution data
// Returns true if signature is valid, false otherwise
func VerifyResolutionSignature(resolution *Resolution) (bool, error) {
	if resolution == nil {
		return false, errors.New("resolution cannot be nil")
	}
	if len(resolution.Signature) == 0 {
		return false, errors.New("resolution has no signature")
	}
	if len(resolution.PublicKey) == 0 {
		return false, errors.New("resolution has no public key")
	}

	// Parse public key
	pubKey, err := btcec.ParsePubKey(resolution.PublicKey)
	if err != nil {
		return false, fmt.Errorf("invalid public key: %w", err)
	}

	// Parse signature
	sig, err := ecdsa.ParseSignature(resolution.Signature)
	if err != nil {
		return false, fmt.Errorf("invalid signature: %w", err)
	}

	// Compute hash of resolution data
	hash := computeResolutionHash(resolution)

	// Verify signature
	return sig.Verify(hash[:], pubKey), nil
}

// Arbiter defines the interface for dispute resolution services
// Integrators can provide their own implementations of this interface
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

	// Validate signature if provided
	if len(evidence.Signature) > 0 {
		valid, err := VerifyEvidenceSignature(evidence)
		if err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
		if !valid {
			return fmt.Errorf("invalid evidence signature")
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

	if resolution.PaymentID == "" {
		resolution.PaymentID = paymentID
	} else if resolution.PaymentID != paymentID {
		if len(resolution.Signature) > 0 {
			return fmt.Errorf("resolution payment ID mismatch: got %s, expected %s", resolution.PaymentID, paymentID)
		}
		resolution.PaymentID = paymentID
	}

	if resolution.Timestamp.IsZero() {
		if len(resolution.Signature) > 0 {
			return fmt.Errorf("signed resolution must include timestamp")
		}
		resolution.Timestamp = time.Now()
	}

	// Validate signature if provided after all signed fields are finalized
	if len(resolution.Signature) > 0 {
		valid, err := VerifyResolutionSignature(resolution)
		if err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
		if !valid {
			return fmt.Errorf("invalid resolution signature")
		}
	}

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
