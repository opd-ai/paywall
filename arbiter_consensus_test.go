// Package paywall tests multi-arbiter consensus functionality
package paywall

import (
	"testing"
	"time"
)

func TestNewArbiterConsensusManager(t *testing.T) {
	tests := []struct {
		name        string
		config      *ArbiterConfig
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "arbiter config cannot be nil",
		},
		{
			name: "too few required votes",
			config: &ArbiterConfig{
				RequiredArbiterVotes: 1,
				TotalArbiters:        5,
				PrimaryArbiters:      make([][]byte, 5),
			},
			wantErr:     true,
			errContains: "RequiredArbiterVotes must be at least 2",
		},
		{
			name: "required votes exceeds total",
			config: &ArbiterConfig{
				RequiredArbiterVotes: 5,
				TotalArbiters:        3,
				PrimaryArbiters:      make([][]byte, 3),
			},
			wantErr:     true,
			errContains: "TotalArbiters (3) must be >= RequiredArbiterVotes (5)",
		},
		{
			name: "insufficient primary arbiters",
			config: &ArbiterConfig{
				RequiredArbiterVotes: 3,
				TotalArbiters:        5,
				PrimaryArbiters:      make([][]byte, 2),
			},
			wantErr:     true,
			errContains: "must provide at least 5 primary arbiters",
		},
		{
			name: "valid 3-of-5 config",
			config: &ArbiterConfig{
				RequiredArbiterVotes: 3,
				TotalArbiters:        5,
				PrimaryArbiters:      make([][]byte, 5),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewArbiterConsensusManager(tt.config, nil)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewArbiterConsensusManager() expected error, got nil")
				} else if tt.errContains != "" {
					// Simple substring check without helper function
					errStr := err.Error()
					found := false
					for i := 0; i <= len(errStr)-len(tt.errContains); i++ {
						if errStr[i:i+len(tt.errContains)] == tt.errContains {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("NewArbiterConsensusManager() error = %v, want error containing %q", err, tt.errContains)
					}
				}
			} else {
				if err != nil {
					t.Errorf("NewArbiterConsensusManager() unexpected error = %v", err)
				}
				if manager == nil {
					t.Errorf("NewArbiterConsensusManager() returned nil manager")
				}
			}
		})
	}
}

func TestInitiateConsensus(t *testing.T) {
	config := &ArbiterConfig{
		RequiredArbiterVotes: 3,
		TotalArbiters:        5,
		PrimaryArbiters:      make([][]byte, 5),
		VotingTimeout:        24 * time.Hour,
	}

	manager, err := NewArbiterConsensusManager(config, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test initiating consensus
	consensus, err := manager.InitiateConsensus("payment-123")
	if err != nil {
		t.Fatalf("InitiateConsensus() unexpected error = %v", err)
	}

	if consensus.PaymentID != "payment-123" {
		t.Errorf("InitiateConsensus() PaymentID = %v, want %v", consensus.PaymentID, "payment-123")
	}
	if consensus.RequiredVotes != 3 {
		t.Errorf("InitiateConsensus() RequiredVotes = %v, want %v", consensus.RequiredVotes, 3)
	}
	if consensus.TotalArbiters != 5 {
		t.Errorf("InitiateConsensus() TotalArbiters = %v, want %v", consensus.TotalArbiters, 5)
	}
	if consensus.Status != ConsensusOpen {
		t.Errorf("InitiateConsensus() Status = %v, want %v", consensus.Status, ConsensusOpen)
	}

	// Test duplicate initiation
	_, err = manager.InitiateConsensus("payment-123")
	if err == nil {
		t.Errorf("InitiateConsensus() expected error for duplicate, got nil")
	}
}

func TestCastVote(t *testing.T) {
	arbiter1Key := []byte("arbiter1-key-123")
	arbiter2Key := []byte("arbiter2-key-456")
	arbiter3Key := []byte("arbiter3-key-789")
	unauthorizedKey := []byte("unauthorized-key")

	config := &ArbiterConfig{
		RequiredArbiterVotes: 3,
		TotalArbiters:        5,
		PrimaryArbiters:      [][]byte{arbiter1Key, arbiter2Key, arbiter3Key, []byte("arbiter4"), []byte("arbiter5")},
		VotingTimeout:        24 * time.Hour,
	}

	manager, err := NewArbiterConsensusManager(config, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	_, err = manager.InitiateConsensus("payment-123")
	if err != nil {
		t.Fatalf("InitiateConsensus() unexpected error = %v", err)
	}

	// Test casting valid vote
	vote1 := &ArbiterVote{
		ArbiterPubKey: arbiter1Key,
		ArbiterID:     "arbiter-1",
		Decision:      RoleBuyer,
		Reason:        "Buyer provided evidence",
		Signature:     &SignatureData{PublicKey: arbiter1Key, Role: RoleArbiter},
	}

	err = manager.CastVote("payment-123", vote1)
	if err != nil {
		t.Errorf("CastVote() unexpected error = %v", err)
	}

	// Verify vote was recorded
	updatedConsensus, _ := manager.GetConsensus("payment-123")
	if len(updatedConsensus.Votes) != 1 {
		t.Errorf("CastVote() votes count = %d, want 1", len(updatedConsensus.Votes))
	}

	// Test duplicate vote
	err = manager.CastVote("payment-123", vote1)
	if err != ErrDuplicateVote {
		t.Errorf("CastVote() duplicate error = %v, want %v", err, ErrDuplicateVote)
	}

	// Test unauthorized arbiter
	unauthorizedVote := &ArbiterVote{
		ArbiterPubKey: unauthorizedKey,
		ArbiterID:     "unauthorized",
		Decision:      RoleSeller,
		Reason:        "Seller delivered",
		Signature:     &SignatureData{PublicKey: unauthorizedKey, Role: RoleArbiter},
	}

	err = manager.CastVote("payment-123", unauthorizedVote)
	if err == nil {
		t.Errorf("CastVote() expected error for unauthorized arbiter, got nil")
	}

	// Test consensus reached with 3 votes
	vote2 := &ArbiterVote{
		ArbiterPubKey: arbiter2Key,
		ArbiterID:     "arbiter-2",
		Decision:      RoleBuyer,
		Reason:        "Agree with arbiter 1",
		Signature:     &SignatureData{PublicKey: arbiter2Key, Role: RoleArbiter},
	}
	vote3 := &ArbiterVote{
		ArbiterPubKey: arbiter3Key,
		ArbiterID:     "arbiter-3",
		Decision:      RoleBuyer,
		Reason:        "Consensus for buyer",
		Signature:     &SignatureData{PublicKey: arbiter3Key, Role: RoleArbiter},
	}

	manager.CastVote("payment-123", vote2)
	manager.CastVote("payment-123", vote3)

	finalConsensus, _ := manager.GetConsensus("payment-123")
	if !finalConsensus.ConsensusReached {
		t.Errorf("CastVote() ConsensusReached = false, want true")
	}
	if finalConsensus.FinalDecision != RoleBuyer {
		t.Errorf("CastVote() FinalDecision = %v, want %v", finalConsensus.FinalDecision, RoleBuyer)
	}
	if finalConsensus.Status != ConsensusReached {
		t.Errorf("CastVote() Status = %v, want %v", finalConsensus.Status, ConsensusReached)
	}

	// Test voting after consensus reached
	vote4 := &ArbiterVote{
		ArbiterPubKey: []byte("arbiter4"),
		ArbiterID:     "arbiter-4",
		Decision:      RoleSeller,
		Reason:        "Late vote",
		Signature:     &SignatureData{PublicKey: []byte("arbiter4"), Role: RoleArbiter},
	}
	err = manager.CastVote("payment-123", vote4)
	if err != ErrVotingClosed {
		t.Errorf("CastVote() after consensus error = %v, want %v", err, ErrVotingClosed)
	}
}

func TestActivateFallbackArbiters(t *testing.T) {
	primaryKeys := [][]byte{
		[]byte("primary1"),
		[]byte("primary2"),
		[]byte("primary3"),
	}
	fallbackKeys := [][]byte{
		[]byte("fallback1"),
		[]byte("fallback2"),
	}

	config := &ArbiterConfig{
		RequiredArbiterVotes: 2,
		TotalArbiters:        3,
		PrimaryArbiters:      primaryKeys,
		FallbackArbiters:     fallbackKeys,
		VotingTimeout:        24 * time.Hour,
	}

	manager, err := NewArbiterConsensusManager(config, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	_, err = manager.InitiateConsensus("payment-fallback")
	if err != nil {
		t.Fatalf("InitiateConsensus() unexpected error = %v", err)
	}

	// Test activating fallback too early
	err = manager.ActivateFallbackArbiters("payment-fallback")
	if err == nil {
		t.Errorf("ActivateFallbackArbiters() expected error for early activation, got nil")
	}

	// Manually adjust deadline to simulate time passing
	manager.mu.Lock()
	cons, _ := manager.consensuses["payment-fallback"]
	cons.VotingDeadline = time.Now().Add(time.Hour)
	manager.mu.Unlock()

	// Now fallback activation should work
	err = manager.ActivateFallbackArbiters("payment-fallback")
	if err != nil {
		t.Errorf("ActivateFallbackArbiters() unexpected error = %v", err)
	}

	updatedConsensus, _ := manager.GetConsensus("payment-fallback")
	if updatedConsensus.Status != ConsensusFallback {
		t.Errorf("ActivateFallbackArbiters() Status = %v, want %v", updatedConsensus.Status, ConsensusFallback)
	}

	// Test voting with fallback arbiter
	fallbackVote := &ArbiterVote{
		ArbiterPubKey: fallbackKeys[0],
		ArbiterID:     "fallback-1",
		Decision:      RoleSeller,
		Reason:        "Fallback decision",
		Signature:     &SignatureData{PublicKey: fallbackKeys[0], Role: RoleArbiter},
	}

	err = manager.CastVote("payment-fallback", fallbackVote)
	if err != nil {
		t.Errorf("CastVote() with fallback arbiter unexpected error = %v", err)
	}
}

func TestCheckExpiredVoting(t *testing.T) {
	config := &ArbiterConfig{
		RequiredArbiterVotes: 3,
		TotalArbiters:        5,
		PrimaryArbiters:      make([][]byte, 5),
		VotingTimeout:        1 * time.Millisecond,
	}

	manager, err := NewArbiterConsensusManager(config, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	_, err = manager.InitiateConsensus("payment-expired")
	if err != nil {
		t.Fatalf("InitiateConsensus() unexpected error = %v", err)
	}

	// Wait for voting to expire
	time.Sleep(10 * time.Millisecond)

	manager.CheckExpiredVoting()

	consensus, _ := manager.GetConsensus("payment-expired")
	if consensus.Status != ConsensusExpired {
		t.Errorf("CheckExpiredVoting() Status = %v, want %v", consensus.Status, ConsensusExpired)
	}
}

func TestGetConsensus(t *testing.T) {
	config := &ArbiterConfig{
		RequiredArbiterVotes: 3,
		TotalArbiters:        5,
		PrimaryArbiters:      make([][]byte, 5),
	}

	manager, err := NewArbiterConsensusManager(config, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test getting non-existent consensus
	_, err = manager.GetConsensus("non-existent")
	if err == nil {
		t.Errorf("GetConsensus() expected error for non-existent payment, got nil")
	}

	// Create and retrieve consensus
	manager.InitiateConsensus("payment-123")
	consensus, err := manager.GetConsensus("payment-123")
	if err != nil {
		t.Errorf("GetConsensus() unexpected error = %v", err)
	}
	if consensus.PaymentID != "payment-123" {
		t.Errorf("GetConsensus() PaymentID = %v, want %v", consensus.PaymentID, "payment-123")
	}
}
