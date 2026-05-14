// Package paywall implements multi-arbiter consensus for dispute resolution
package paywall

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrInsufficientArbiterVotes indicates not enough arbiters have voted
	ErrInsufficientArbiterVotes = errors.New("insufficient arbiter votes for consensus")
	// ErrArbiterNotFound indicates the arbiter ID does not exist
	ErrArbiterNotFound = errors.New("arbiter not found")
	// ErrDuplicateVote indicates an arbiter has already voted on this dispute
	ErrDuplicateVote = errors.New("arbiter has already voted on this dispute")
	// ErrVotingClosed indicates voting has ended for this dispute
	ErrVotingClosed = errors.New("voting is closed for this dispute")
)

// ArbiterVote represents a single arbiter's vote on a dispute
type ArbiterVote struct {
	// ArbiterPubKey is the arbiter's public key
	ArbiterPubKey []byte `json:"arbiter_pub_key"`
	// ArbiterID is a unique identifier for the arbiter
	ArbiterID string `json:"arbiter_id"`
	// Decision is who the arbiter votes to win (buyer or seller)
	Decision MultisigRole `json:"decision"`
	// Reason is the arbiter's explanation for their decision
	Reason string `json:"reason"`
	// Signature is the arbiter's signature over the dispute resolution
	Signature *SignatureData `json:"signature"`
	// VotedAt is when the vote was cast
	VotedAt time.Time `json:"voted_at"`
}

// ArbiterConsensus manages multi-arbiter voting on disputes
type ArbiterConsensus struct {
	// PaymentID is the payment under dispute
	PaymentID string `json:"payment_id"`
	// RequiredVotes is how many arbiter votes are needed (e.g., 3 in 3-of-5)
	RequiredVotes int `json:"required_votes"`
	// TotalArbiters is the total number of arbiters (e.g., 5 in 3-of-5)
	TotalArbiters int `json:"total_arbiters"`
	// Votes contains all votes cast by arbiters
	Votes []*ArbiterVote `json:"votes"`
	// VotingDeadline is when voting closes
	VotingDeadline time.Time `json:"voting_deadline"`
	// ConsensusReached indicates if consensus has been achieved
	ConsensusReached bool `json:"consensus_reached"`
	// FinalDecision is the consensus decision (empty if not reached)
	FinalDecision MultisigRole `json:"final_decision,omitempty"`
	// Status indicates the current voting status
	Status ConsensusStatus `json:"status"`
}

// ConsensusStatus represents the state of the consensus process
type ConsensusStatus string

const (
	// ConsensusOpen indicates voting is open
	ConsensusOpen ConsensusStatus = "open"
	// ConsensusReached indicates consensus has been achieved
	ConsensusReached ConsensusStatus = "reached"
	// ConsensusExpired indicates voting deadline passed without consensus
	ConsensusExpired ConsensusStatus = "expired"
	// ConsensusFallback indicates fallback arbiters activated
	ConsensusFallback ConsensusStatus = "fallback"
)

// ArbiterConfig extends Config with multi-arbiter settings
type ArbiterConfig struct {
	// RequiredArbiterVotes is how many arbiters must agree (e.g., 3 in 3-of-5)
	RequiredArbiterVotes int
	// TotalArbiters is the total number of arbiters (e.g., 5 in 3-of-5)
	TotalArbiters int
	// PrimaryArbiters are the primary arbiters' public keys
	PrimaryArbiters [][]byte
	// FallbackArbiters are backup arbiters used if primary arbiters are unavailable
	FallbackArbiters [][]byte
	// VotingTimeout is how long arbiters have to vote
	VotingTimeout time.Duration
}

// ArbiterConsensusManager handles multi-arbiter dispute resolution
type ArbiterConsensusManager struct {
	// consensuses tracks active consensus processes by payment ID
	consensuses map[string]*ArbiterConsensus
	// config contains multi-arbiter configuration
	config *ArbiterConfig
	// mu protects concurrent access to consensuses
	mu sync.RWMutex
	// reputationTracker tracks arbiter performance
	reputationTracker *ArbiterReputationTracker
}

// NewArbiterConsensusManager creates a new multi-arbiter consensus manager
func NewArbiterConsensusManager(config *ArbiterConfig, reputationTracker *ArbiterReputationTracker) (*ArbiterConsensusManager, error) {
	if config == nil {
		return nil, fmt.Errorf("arbiter config cannot be nil")
	}
	if config.RequiredArbiterVotes < 2 {
		return nil, fmt.Errorf("RequiredArbiterVotes must be at least 2, got: %d", config.RequiredArbiterVotes)
	}
	if config.TotalArbiters < config.RequiredArbiterVotes {
		return nil, fmt.Errorf("TotalArbiters (%d) must be >= RequiredArbiterVotes (%d)", config.TotalArbiters, config.RequiredArbiterVotes)
	}
	if len(config.PrimaryArbiters) < config.TotalArbiters {
		return nil, fmt.Errorf("must provide at least %d primary arbiters, got %d", config.TotalArbiters, len(config.PrimaryArbiters))
	}
	if config.VotingTimeout <= 0 {
		config.VotingTimeout = 48 * time.Hour // Default: 48 hours
	}

	if reputationTracker == nil {
		reputationTracker = NewArbiterReputationTracker()
	}

	return &ArbiterConsensusManager{
		consensuses:       make(map[string]*ArbiterConsensus),
		config:            config,
		reputationTracker: reputationTracker,
	}, nil
}

// InitiateConsensus starts a new consensus process for a disputed payment
func (acm *ArbiterConsensusManager) InitiateConsensus(paymentID string) (*ArbiterConsensus, error) {
	acm.mu.Lock()
	defer acm.mu.Unlock()

	// Check if consensus already exists
	if _, exists := acm.consensuses[paymentID]; exists {
		return nil, fmt.Errorf("consensus already initiated for payment %s", paymentID)
	}

	consensus := &ArbiterConsensus{
		PaymentID:        paymentID,
		RequiredVotes:    acm.config.RequiredArbiterVotes,
		TotalArbiters:    acm.config.TotalArbiters,
		Votes:            make([]*ArbiterVote, 0),
		VotingDeadline:   time.Now().Add(acm.config.VotingTimeout),
		ConsensusReached: false,
		Status:           ConsensusOpen,
	}

	acm.consensuses[paymentID] = consensus
	return consensus, nil
}

// CastVote records an arbiter's vote on a dispute
func (acm *ArbiterConsensusManager) CastVote(paymentID string, vote *ArbiterVote) error {
	acm.mu.Lock()
	defer acm.mu.Unlock()

	consensus, exists := acm.consensuses[paymentID]
	if !exists {
		return fmt.Errorf("no consensus process for payment %s", paymentID)
	}

	// Check if voting is still open (Open or Fallback statuses allow voting)
	if consensus.Status != ConsensusOpen && consensus.Status != ConsensusFallback {
		return ErrVotingClosed
	}

	// Check if voting deadline has passed
	if time.Now().After(consensus.VotingDeadline) {
		consensus.Status = ConsensusExpired
		return ErrVotingClosed
	}

	// Check if arbiter has already voted
	for _, existingVote := range consensus.Votes {
		if bytesEqual(existingVote.ArbiterPubKey, vote.ArbiterPubKey) {
			return ErrDuplicateVote
		}
	}

	// Validate arbiter is authorized (primary or fallback)
	validArbiter := false
	for _, arbiterKey := range acm.config.PrimaryArbiters {
		if bytesEqual(arbiterKey, vote.ArbiterPubKey) {
			validArbiter = true
			break
		}
	}
	if !validArbiter && len(acm.config.FallbackArbiters) > 0 {
		for _, arbiterKey := range acm.config.FallbackArbiters {
			if bytesEqual(arbiterKey, vote.ArbiterPubKey) {
				validArbiter = true
				consensus.Status = ConsensusFallback
				break
			}
		}
	}
	if !validArbiter {
		return fmt.Errorf("arbiter is not authorized")
	}

	// Record the vote
	vote.VotedAt = time.Now()
	consensus.Votes = append(consensus.Votes, vote)

	// Check if consensus is reached
	if len(consensus.Votes) >= consensus.RequiredVotes {
		acm.evaluateConsensus(consensus)
	}

	return nil
}

// evaluateConsensus determines if consensus has been reached
// Must be called with lock held
func (acm *ArbiterConsensusManager) evaluateConsensus(consensus *ArbiterConsensus) {
	// Count votes for each decision
	buyerVotes := 0
	sellerVotes := 0

	for _, vote := range consensus.Votes {
		switch vote.Decision {
		case RoleBuyer:
			buyerVotes++
		case RoleSeller:
			sellerVotes++
		}
	}

	// Check if required votes reached for either party
	if buyerVotes >= consensus.RequiredVotes {
		consensus.ConsensusReached = true
		consensus.FinalDecision = RoleBuyer
		consensus.Status = ConsensusReached
		// Update arbiter reputation
		acm.updateArbiterReputation(consensus, RoleBuyer)
	} else if sellerVotes >= consensus.RequiredVotes {
		consensus.ConsensusReached = true
		consensus.FinalDecision = RoleSeller
		consensus.Status = ConsensusReached
		// Update arbiter reputation
		acm.updateArbiterReputation(consensus, RoleSeller)
	}
}

// updateArbiterReputation updates reputation for arbiters based on consensus
func (acm *ArbiterConsensusManager) updateArbiterReputation(consensus *ArbiterConsensus, decision MultisigRole) {
	for _, vote := range consensus.Votes {
		// Arbiters who voted with consensus get positive reputation
		if vote.Decision == decision {
			acm.reputationTracker.RecordDecision(vote.ArbiterID, true, time.Since(vote.VotedAt))
		} else {
			// Arbiters who voted against consensus get negative reputation
			acm.reputationTracker.RecordDecision(vote.ArbiterID, false, time.Since(vote.VotedAt))
		}
	}

	// Penalize arbiters who didn't vote
	allArbiters := make(map[string]bool)
	for i := range acm.config.PrimaryArbiters {
		arbiterID := fmt.Sprintf("arbiter-%d", i)
		allArbiters[arbiterID] = true
	}
	for _, vote := range consensus.Votes {
		delete(allArbiters, vote.ArbiterID)
	}
	for arbiterID := range allArbiters {
		acm.reputationTracker.RecordNonParticipation(arbiterID)
	}
}

// GetConsensus retrieves the consensus status for a payment
func (acm *ArbiterConsensusManager) GetConsensus(paymentID string) (*ArbiterConsensus, error) {
	acm.mu.RLock()
	defer acm.mu.RUnlock()

	consensus, exists := acm.consensuses[paymentID]
	if !exists {
		return nil, fmt.Errorf("no consensus process for payment %s", paymentID)
	}

	return consensus, nil
}

// CheckExpiredVoting checks for expired voting periods and updates status
func (acm *ArbiterConsensusManager) CheckExpiredVoting() {
	acm.mu.Lock()
	defer acm.mu.Unlock()

	now := time.Now()
	for _, consensus := range acm.consensuses {
		if consensus.Status == ConsensusOpen && now.After(consensus.VotingDeadline) {
			consensus.Status = ConsensusExpired
		}
	}
}

// ActivateFallbackArbiters activates fallback arbiters when primary arbiters are unavailable
func (acm *ArbiterConsensusManager) ActivateFallbackArbiters(paymentID string) error {
	acm.mu.Lock()
	defer acm.mu.Unlock()

	consensus, exists := acm.consensuses[paymentID]
	if !exists {
		return fmt.Errorf("no consensus process for payment %s", paymentID)
	}

	if consensus.Status != ConsensusOpen {
		return fmt.Errorf("cannot activate fallback arbiters: voting is %s", consensus.Status)
	}

	// Check if we're past the midpoint of voting deadline
	midpoint := consensus.VotingDeadline.Add(-acm.config.VotingTimeout / 2)
	if time.Now().Before(midpoint) {
		return fmt.Errorf("fallback arbiters can only be activated after voting midpoint")
	}

	// Check if we have insufficient primary arbiter votes
	if len(consensus.Votes) >= consensus.RequiredVotes {
		return fmt.Errorf("sufficient votes already cast, fallback not needed")
	}

	// Extend deadline and allow fallback arbiters
	consensus.VotingDeadline = time.Now().Add(24 * time.Hour)
	consensus.Status = ConsensusFallback

	return nil
}
