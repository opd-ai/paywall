// Package paywall implements arbiter reputation tracking
package paywall

import (
	"fmt"
	"sync"
	"time"
)

// ArbiterReputation tracks an arbiter's performance metrics
type ArbiterReputation struct {
	// ArbiterID uniquely identifies the arbiter
	ArbiterID string `json:"arbiter_id"`
	// PublicKey is the arbiter's public key
	PublicKey []byte `json:"public_key"`
	// TotalDecisions is the number of disputes the arbiter has voted on
	TotalDecisions int `json:"total_decisions"`
	// ConsensusDecisions is the number of times the arbiter voted with consensus
	ConsensusDecisions int `json:"consensus_decisions"`
	// DissentingDecisions is the number of times the arbiter voted against consensus
	DissentingDecisions int `json:"dissenting_decisions"`
	// NonParticipations is the number of times the arbiter didn't vote
	NonParticipations int `json:"non_participations"`
	// AverageResponseTime is the average time to vote
	AverageResponseTime time.Duration `json:"average_response_time"`
	// ReputationScore is a computed score (0-100)
	ReputationScore float64 `json:"reputation_score"`
	// FirstDecisionAt is when the arbiter made their first decision
	FirstDecisionAt time.Time `json:"first_decision_at,omitempty"`
	// LastDecisionAt is when the arbiter made their last decision
	LastDecisionAt time.Time `json:"last_decision_at,omitempty"`
	// LastUpdated is when the reputation was last updated
	LastUpdated time.Time `json:"last_updated"`
}

// ArbiterReputationTracker manages reputation for all arbiters
type ArbiterReputationTracker struct {
	// reputations maps arbiter ID to reputation
	reputations map[string]*ArbiterReputation
	// mu protects concurrent access
	mu sync.RWMutex
}

// NewArbiterReputationTracker creates a new reputation tracker
func NewArbiterReputationTracker() *ArbiterReputationTracker {
	return &ArbiterReputationTracker{
		reputations: make(map[string]*ArbiterReputation),
	}
}

// RecordDecision records an arbiter's decision and updates their reputation
// withConsensus indicates if the arbiter voted with the final consensus
// responseTime is how long it took the arbiter to vote
func (art *ArbiterReputationTracker) RecordDecision(arbiterID string, withConsensus bool, responseTime time.Duration) {
	art.mu.Lock()
	defer art.mu.Unlock()

	rep, exists := art.reputations[arbiterID]
	if !exists {
		rep = &ArbiterReputation{
			ArbiterID:       arbiterID,
			FirstDecisionAt: time.Now(),
		}
		art.reputations[arbiterID] = rep
	}

	rep.TotalDecisions++
	if withConsensus {
		rep.ConsensusDecisions++
	} else {
		rep.DissentingDecisions++
	}

	// Update average response time
	if rep.TotalDecisions == 1 {
		rep.AverageResponseTime = responseTime
	} else {
		totalTime := rep.AverageResponseTime * time.Duration(rep.TotalDecisions-1)
		rep.AverageResponseTime = (totalTime + responseTime) / time.Duration(rep.TotalDecisions)
	}

	rep.LastDecisionAt = time.Now()
	rep.LastUpdated = time.Now()

	// Recalculate reputation score
	art.calculateReputationScore(rep)
}

// RecordNonParticipation records when an arbiter failed to vote
func (art *ArbiterReputationTracker) RecordNonParticipation(arbiterID string) {
	art.mu.Lock()
	defer art.mu.Unlock()

	rep, exists := art.reputations[arbiterID]
	if !exists {
		rep = &ArbiterReputation{
			ArbiterID: arbiterID,
		}
		art.reputations[arbiterID] = rep
	}

	rep.NonParticipations++
	rep.LastUpdated = time.Now()

	// Recalculate reputation score
	art.calculateReputationScore(rep)
}

// calculateReputationScore computes the reputation score (0-100)
// Must be called with lock held
func (art *ArbiterReputationTracker) calculateReputationScore(rep *ArbiterReputation) {
	// Base score from consensus rate
	var consensusRate float64
	if rep.TotalDecisions > 0 {
		consensusRate = float64(rep.ConsensusDecisions) / float64(rep.TotalDecisions)
	} else {
		consensusRate = 0.5 // Neutral starting point
	}

	// Penalty for non-participation
	totalOpportunities := rep.TotalDecisions + rep.NonParticipations
	participationRate := 1.0
	if totalOpportunities > 0 {
		participationRate = float64(rep.TotalDecisions) / float64(totalOpportunities)
	}

	// Bonus for quick response times (up to 10 points)
	responseBonus := 0.0
	if rep.AverageResponseTime > 0 {
		// Reward responses under 24 hours
		hoursToRespond := rep.AverageResponseTime.Hours()
		if hoursToRespond <= 24 {
			responseBonus = 10.0 * (1.0 - hoursToRespond/24.0)
		}
	}

	// Calculate final score (0-100)
	// 70% weight on consensus rate
	// 20% weight on participation rate
	// 10% weight on response time
	score := (consensusRate * 70.0) + (participationRate * 20.0) + responseBonus

	// Ensure score is in valid range
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	rep.ReputationScore = score
}

// GetReputation retrieves an arbiter's reputation
func (art *ArbiterReputationTracker) GetReputation(arbiterID string) (*ArbiterReputation, error) {
	art.mu.RLock()
	defer art.mu.RUnlock()

	rep, exists := art.reputations[arbiterID]
	if !exists {
		return nil, ErrArbiterNotFound
	}

	// Return a copy to prevent external modification
	repCopy := *rep
	return &repCopy, nil
}

// ListReputations returns all arbiter reputations, sorted by score
func (art *ArbiterReputationTracker) ListReputations() []*ArbiterReputation {
	art.mu.RLock()
	defer art.mu.RUnlock()

	reps := make([]*ArbiterReputation, 0, len(art.reputations))
	for _, rep := range art.reputations {
		repCopy := *rep
		reps = append(reps, &repCopy)
	}

	// Sort by reputation score (highest first)
	for i := 0; i < len(reps); i++ {
		for j := i + 1; j < len(reps); j++ {
			if reps[j].ReputationScore > reps[i].ReputationScore {
				reps[i], reps[j] = reps[j], reps[i]
			}
		}
	}

	return reps
}

// GetTopArbiters returns the N arbiters with the highest reputation scores
func (art *ArbiterReputationTracker) GetTopArbiters(n int) []*ArbiterReputation {
	all := art.ListReputations()
	if len(all) <= n {
		return all
	}
	return all[:n]
}

// RegisterArbiter registers a new arbiter in the reputation system
func (art *ArbiterReputationTracker) RegisterArbiter(arbiterID string, publicKey []byte) error {
	art.mu.Lock()
	defer art.mu.Unlock()

	if _, exists := art.reputations[arbiterID]; exists {
		return fmt.Errorf("arbiter %s already registered", arbiterID)
	}

	art.reputations[arbiterID] = &ArbiterReputation{
		ArbiterID:       arbiterID,
		PublicKey:       publicKey,
		ReputationScore: 50.0, // Start with neutral score
		LastUpdated:     time.Now(),
	}

	return nil
}

// RemoveArbiter removes an arbiter from the reputation system
// This should be used with caution as it removes all historical data
func (art *ArbiterReputationTracker) RemoveArbiter(arbiterID string) error {
	art.mu.Lock()
	defer art.mu.Unlock()

	if _, exists := art.reputations[arbiterID]; !exists {
		return ErrArbiterNotFound
	}

	delete(art.reputations, arbiterID)
	return nil
}

// GetStatistics returns aggregate statistics across all arbiters
func (art *ArbiterReputationTracker) GetStatistics() *ArbiterStatistics {
	art.mu.RLock()
	defer art.mu.RUnlock()

	stats := &ArbiterStatistics{
		TotalArbiters: len(art.reputations),
	}

	if stats.TotalArbiters == 0 {
		return stats
	}

	var totalScore float64
	var totalDecisions int
	var totalConsensus int
	var totalNonParticipations int
	var totalResponseTime time.Duration

	for _, rep := range art.reputations {
		totalScore += rep.ReputationScore
		totalDecisions += rep.TotalDecisions
		totalConsensus += rep.ConsensusDecisions
		totalNonParticipations += rep.NonParticipations
		if rep.TotalDecisions > 0 {
			totalResponseTime += rep.AverageResponseTime
		}
	}

	stats.AverageScore = totalScore / float64(stats.TotalArbiters)
	stats.TotalDecisions = totalDecisions
	stats.TotalConsensusDecisions = totalConsensus
	stats.TotalNonParticipations = totalNonParticipations

	if totalDecisions > 0 {
		stats.AverageConsensusRate = float64(totalConsensus) / float64(totalDecisions)
	}

	arbitersWithDecisions := 0
	for _, rep := range art.reputations {
		if rep.TotalDecisions > 0 {
			arbitersWithDecisions++
		}
	}
	if arbitersWithDecisions > 0 {
		stats.AverageResponseTime = totalResponseTime / time.Duration(arbitersWithDecisions)
	}

	return stats
}

// ArbiterStatistics contains aggregate statistics for all arbiters
type ArbiterStatistics struct {
	// TotalArbiters is the number of registered arbiters
	TotalArbiters int `json:"total_arbiters"`
	// AverageScore is the average reputation score
	AverageScore float64 `json:"average_score"`
	// TotalDecisions is the total number of decisions across all arbiters
	TotalDecisions int `json:"total_decisions"`
	// TotalConsensusDecisions is decisions that matched consensus
	TotalConsensusDecisions int `json:"total_consensus_decisions"`
	// TotalNonParticipations is the total times arbiters didn't vote
	TotalNonParticipations int `json:"total_non_participations"`
	// AverageConsensusRate is the average rate of consensus agreement
	AverageConsensusRate float64 `json:"average_consensus_rate"`
	// AverageResponseTime is the average time to vote
	AverageResponseTime time.Duration `json:"average_response_time"`
}
