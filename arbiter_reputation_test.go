// Package paywall tests arbiter reputation tracking
package paywall

import (
	"testing"
	"time"
)

func TestNewArbiterReputationTracker(t *testing.T) {
	tracker := NewArbiterReputationTracker()
	if tracker == nil {
		t.Fatal("NewArbiterReputationTracker() returned nil")
	}
	if tracker.reputations == nil {
		t.Error("NewArbiterReputationTracker() reputations map is nil")
	}
}

func TestRegisterArbiter(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	pubKey := []byte("arbiter-public-key")
	err := tracker.RegisterArbiter("arbiter-1", pubKey)
	if err != nil {
		t.Errorf("RegisterArbiter() unexpected error = %v", err)
	}

	// Test duplicate registration
	err = tracker.RegisterArbiter("arbiter-1", pubKey)
	if err == nil {
		t.Errorf("RegisterArbiter() expected error for duplicate, got nil")
	}

	// Verify arbiter was registered
	rep, err := tracker.GetReputation("arbiter-1")
	if err != nil {
		t.Errorf("GetReputation() unexpected error = %v", err)
	}
	if rep.ArbiterID != "arbiter-1" {
		t.Errorf("GetReputation() ArbiterID = %v, want arbiter-1", rep.ArbiterID)
	}
	if rep.ReputationScore != 50.0 {
		t.Errorf("GetReputation() initial ReputationScore = %v, want 50.0", rep.ReputationScore)
	}
}

func TestRecordDecision(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	// Record decisions for arbiter
	tracker.RecordDecision("arbiter-1", true, 2*time.Hour)
	tracker.RecordDecision("arbiter-1", true, 3*time.Hour)
	tracker.RecordDecision("arbiter-1", false, 1*time.Hour)

	rep, err := tracker.GetReputation("arbiter-1")
	if err != nil {
		t.Fatalf("GetReputation() unexpected error = %v", err)
	}

	// Verify decision counts
	if rep.TotalDecisions != 3 {
		t.Errorf("RecordDecision() TotalDecisions = %d, want 3", rep.TotalDecisions)
	}
	if rep.ConsensusDecisions != 2 {
		t.Errorf("RecordDecision() ConsensusDecisions = %d, want 2", rep.ConsensusDecisions)
	}
	if rep.DissentingDecisions != 1 {
		t.Errorf("RecordDecision() DissentingDecisions = %d, want 1", rep.DissentingDecisions)
	}

	// Verify average response time (2+3+1)/3 = 2 hours
	expectedAvg := 2 * time.Hour
	if rep.AverageResponseTime != expectedAvg {
		t.Errorf("RecordDecision() AverageResponseTime = %v, want %v", rep.AverageResponseTime, expectedAvg)
	}

	// Verify timestamps
	if rep.FirstDecisionAt.IsZero() {
		t.Error("RecordDecision() FirstDecisionAt is zero")
	}
	if rep.LastDecisionAt.IsZero() {
		t.Error("RecordDecision() LastDecisionAt is zero")
	}
	if rep.LastDecisionAt.Before(rep.FirstDecisionAt) {
		t.Error("RecordDecision() LastDecisionAt is before FirstDecisionAt")
	}
}

func TestRecordNonParticipation(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	// Record some decisions and non-participations
	tracker.RecordDecision("arbiter-1", true, 1*time.Hour)
	tracker.RecordNonParticipation("arbiter-1")
	tracker.RecordNonParticipation("arbiter-1")

	rep, err := tracker.GetReputation("arbiter-1")
	if err != nil {
		t.Fatalf("GetReputation() unexpected error = %v", err)
	}

	if rep.NonParticipations != 2 {
		t.Errorf("RecordNonParticipation() NonParticipations = %d, want 2", rep.NonParticipations)
	}

	// Non-participation should lower the score
	// With 1 decision and 2 non-participations, participation rate = 1/3 = 33.3%
	// This should negatively impact the score, but the actual score depends on consensus rate too
	if rep.ReputationScore > 90.0 {
		t.Errorf("RecordNonParticipation() ReputationScore = %v, expected lower score due to non-participation", rep.ReputationScore)
	}
}

func TestCalculateReputationScore(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	tests := []struct {
		name                string
		consensusDecisions  int
		dissentingDecisions int
		nonParticipations   int
		avgResponseTime     time.Duration
		expectScoreRange    [2]float64 // min, max
	}{
		{
			name:                "perfect arbiter",
			consensusDecisions:  10,
			dissentingDecisions: 0,
			nonParticipations:   0,
			avgResponseTime:     1 * time.Hour,
			expectScoreRange:    [2]float64{90.0, 100.0},
		},
		{
			name:                "good arbiter",
			consensusDecisions:  8,
			dissentingDecisions: 2,
			nonParticipations:   0,
			avgResponseTime:     12 * time.Hour,
			expectScoreRange:    [2]float64{70.0, 85.0},
		},
		{
			name:                "average arbiter",
			consensusDecisions:  5,
			dissentingDecisions: 5,
			nonParticipations:   2,
			avgResponseTime:     24 * time.Hour,
			expectScoreRange:    [2]float64{45.0, 60.0},
		},
		{
			name:                "poor arbiter",
			consensusDecisions:  2,
			dissentingDecisions: 8,
			nonParticipations:   5,
			avgResponseTime:     48 * time.Hour,
			expectScoreRange:    [2]float64{10.0, 30.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arbiterID := "arbiter-" + tt.name

			// Record consensus decisions
			for i := 0; i < tt.consensusDecisions; i++ {
				tracker.RecordDecision(arbiterID, true, tt.avgResponseTime)
			}

			// Record dissenting decisions
			for i := 0; i < tt.dissentingDecisions; i++ {
				tracker.RecordDecision(arbiterID, false, tt.avgResponseTime)
			}

			// Record non-participations
			for i := 0; i < tt.nonParticipations; i++ {
				tracker.RecordNonParticipation(arbiterID)
			}

			rep, err := tracker.GetReputation(arbiterID)
			if err != nil {
				t.Fatalf("GetReputation() unexpected error = %v", err)
			}

			if rep.ReputationScore < tt.expectScoreRange[0] || rep.ReputationScore > tt.expectScoreRange[1] {
				t.Errorf("ReputationScore = %v, want in range [%v, %v]",
					rep.ReputationScore, tt.expectScoreRange[0], tt.expectScoreRange[1])
			}
		})
	}
}

func TestListReputations(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	// Create arbiters with different scores
	tracker.RecordDecision("arbiter-1", true, 1*time.Hour)
	tracker.RecordDecision("arbiter-1", true, 1*time.Hour)
	tracker.RecordDecision("arbiter-1", true, 1*time.Hour)

	tracker.RecordDecision("arbiter-2", true, 2*time.Hour)
	tracker.RecordDecision("arbiter-2", false, 2*time.Hour)

	tracker.RecordDecision("arbiter-3", false, 3*time.Hour)
	tracker.RecordDecision("arbiter-3", false, 3*time.Hour)
	tracker.RecordDecision("arbiter-3", false, 3*time.Hour)

	reps := tracker.ListReputations()

	if len(reps) != 3 {
		t.Fatalf("ListReputations() returned %d reputations, want 3", len(reps))
	}

	// Verify sorting (highest score first)
	for i := 0; i < len(reps)-1; i++ {
		if reps[i].ReputationScore < reps[i+1].ReputationScore {
			t.Errorf("ListReputations() not sorted: reps[%d].Score=%v < reps[%d].Score=%v",
				i, reps[i].ReputationScore, i+1, reps[i+1].ReputationScore)
		}
	}

	// arbiter-1 should be first (all consensus)
	if reps[0].ArbiterID != "arbiter-1" {
		t.Errorf("ListReputations() first arbiter = %v, want arbiter-1", reps[0].ArbiterID)
	}

	// arbiter-3 should be last (all dissenting)
	if reps[2].ArbiterID != "arbiter-3" {
		t.Errorf("ListReputations() last arbiter = %v, want arbiter-3", reps[2].ArbiterID)
	}
}

func TestGetTopArbiters(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	// Create 5 arbiters
	for i := 1; i <= 5; i++ {
		arbiterID := "arbiter-" + string(rune('0'+i))
		// Give them different scores based on number of consensus decisions
		for j := 0; j < i*2; j++ {
			tracker.RecordDecision(arbiterID, true, 1*time.Hour)
		}
	}

	// Get top 3
	topThree := tracker.GetTopArbiters(3)
	if len(topThree) != 3 {
		t.Errorf("GetTopArbiters(3) returned %d arbiters, want 3", len(topThree))
	}

	// Get more than available
	topTen := tracker.GetTopArbiters(10)
	if len(topTen) != 5 {
		t.Errorf("GetTopArbiters(10) returned %d arbiters, want 5 (all available)", len(topTen))
	}
}

func TestRemoveArbiter(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	tracker.RegisterArbiter("arbiter-1", []byte("key"))
	tracker.RecordDecision("arbiter-1", true, 1*time.Hour)

	// Remove arbiter
	err := tracker.RemoveArbiter("arbiter-1")
	if err != nil {
		t.Errorf("RemoveArbiter() unexpected error = %v", err)
	}

	// Verify arbiter is gone
	_, err = tracker.GetReputation("arbiter-1")
	if err != ErrArbiterNotFound {
		t.Errorf("GetReputation() after removal error = %v, want %v", err, ErrArbiterNotFound)
	}

	// Try to remove non-existent arbiter
	err = tracker.RemoveArbiter("non-existent")
	if err != ErrArbiterNotFound {
		t.Errorf("RemoveArbiter() non-existent error = %v, want %v", err, ErrArbiterNotFound)
	}
}

func TestGetStatistics(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	// Create multiple arbiters with various records
	tracker.RecordDecision("arbiter-1", true, 1*time.Hour)
	tracker.RecordDecision("arbiter-1", true, 2*time.Hour)
	tracker.RecordNonParticipation("arbiter-1")

	tracker.RecordDecision("arbiter-2", false, 3*time.Hour)
	tracker.RecordDecision("arbiter-2", true, 4*time.Hour)

	tracker.RecordDecision("arbiter-3", true, 5*time.Hour)
	tracker.RecordNonParticipation("arbiter-3")
	tracker.RecordNonParticipation("arbiter-3")

	stats := tracker.GetStatistics()

	if stats.TotalArbiters != 3 {
		t.Errorf("GetStatistics() TotalArbiters = %d, want 3", stats.TotalArbiters)
	}

	expectedTotalDecisions := 5 // 2+2+1
	if stats.TotalDecisions != expectedTotalDecisions {
		t.Errorf("GetStatistics() TotalDecisions = %d, want %d", stats.TotalDecisions, expectedTotalDecisions)
	}

	expectedConsensus := 4 // arbiter-1: 2, arbiter-2: 1, arbiter-3: 1
	if stats.TotalConsensusDecisions != expectedConsensus {
		t.Errorf("GetStatistics() TotalConsensusDecisions = %d, want %d", stats.TotalConsensusDecisions, expectedConsensus)
	}

	expectedNonParticipations := 3 // arbiter-1: 1, arbiter-3: 2
	if stats.TotalNonParticipations != expectedNonParticipations {
		t.Errorf("GetStatistics() TotalNonParticipations = %d, want %d", stats.TotalNonParticipations, expectedNonParticipations)
	}

	// Consensus rate should be 4/5 = 0.8
	expectedConsensusRate := 0.8
	if stats.AverageConsensusRate < expectedConsensusRate-0.01 || stats.AverageConsensusRate > expectedConsensusRate+0.01 {
		t.Errorf("GetStatistics() AverageConsensusRate = %v, want ~%v", stats.AverageConsensusRate, expectedConsensusRate)
	}

	// Average response time calculation:
	// arbiter-1: (1h + 2h) / 2 = 1.5h
	// arbiter-2: (3h + 4h) / 2 = 3.5h
	// arbiter-3: 5h / 1 = 5h
	// Total average: (1.5h + 3.5h + 5h) / 3 = 10h / 3 = 3h20m
	expectedAvgResponse := (10 * time.Hour) / 3
	if stats.AverageResponseTime != expectedAvgResponse {
		t.Errorf("GetStatistics() AverageResponseTime = %v, want %v", stats.AverageResponseTime, expectedAvgResponse)
	}

	if stats.AverageScore <= 0 || stats.AverageScore > 100 {
		t.Errorf("GetStatistics() AverageScore = %v, want in range (0, 100]", stats.AverageScore)
	}
}

func TestGetStatistics_EmptyTracker(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	stats := tracker.GetStatistics()

	if stats.TotalArbiters != 0 {
		t.Errorf("GetStatistics() empty TotalArbiters = %d, want 0", stats.TotalArbiters)
	}
	if stats.TotalDecisions != 0 {
		t.Errorf("GetStatistics() empty TotalDecisions = %d, want 0", stats.TotalDecisions)
	}
	if stats.AverageScore != 0 {
		t.Errorf("GetStatistics() empty AverageScore = %v, want 0", stats.AverageScore)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tracker := NewArbiterReputationTracker()

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			arbiterID := "arbiter-" + string(rune('0'+id))
			tracker.RecordDecision(arbiterID, true, 1*time.Hour)
			tracker.RecordNonParticipation(arbiterID)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Test concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			tracker.ListReputations()
			tracker.GetStatistics()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data integrity
	stats := tracker.GetStatistics()
	if stats.TotalArbiters != 10 {
		t.Errorf("Concurrent access resulted in %d arbiters, want 10", stats.TotalArbiters)
	}
}
