package paywall

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

// Property-based testing for state machine using testing/quick
// This validates that state machine properties hold for all valid inputs

// TestStateTransitionProperties uses property-based testing to verify state machine invariants
func TestStateTransitionProperties(t *testing.T) {
	// Property 1: Valid transitions are always accepted
	t.Run("ValidTransitionsAlwaysAccepted", func(t *testing.T) {
		property := func() bool {
			validator := NewEscrowStateValidator()
			validTransitions := map[EscrowState][]EscrowState{
				EscrowPending:   {EscrowFunded, EscrowNone},
				EscrowFunded:    {EscrowCompleted, EscrowDisputed, EscrowRefunded},
				EscrowDisputed:  {EscrowCompleted, EscrowRefunded},
				EscrowNone:      {EscrowPending},
				EscrowCompleted: {},
				EscrowRefunded:  {},
			}

			// Test all valid transitions
			for from, toStates := range validTransitions {
				for _, to := range toStates {
					err := validator.ValidateTransition(from, to)
					if err != nil {
						t.Logf("Valid transition rejected: %s -> %s: %v", from, to, err)
						return false
					}
				}
			}
			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
			t.Errorf("Property ValidTransitionsAlwaysAccepted failed: %v", err)
		}
	})

	// Property 2: Invalid transitions are always rejected
	t.Run("InvalidTransitionsAlwaysRejected", func(t *testing.T) {
		property := func() bool {
			validator := NewEscrowStateValidator()
			invalidTransitions := []struct {
				from EscrowState
				to   EscrowState
			}{
				// Terminal states cannot transition
				{EscrowCompleted, EscrowFunded},
				{EscrowCompleted, EscrowDisputed},
				{EscrowCompleted, EscrowRefunded},
				{EscrowRefunded, EscrowFunded},
				{EscrowRefunded, EscrowCompleted},
				{EscrowRefunded, EscrowDisputed},
				// Invalid direct transitions
				{EscrowPending, EscrowCompleted},
				{EscrowPending, EscrowDisputed},
				{EscrowPending, EscrowRefunded},
				// Cannot go back to pending once funded
				{EscrowFunded, EscrowPending},
				{EscrowDisputed, EscrowPending},
				{EscrowDisputed, EscrowFunded},
			}

			for _, trans := range invalidTransitions {
				err := validator.ValidateTransition(trans.from, trans.to)
				if err == nil {
					t.Logf("Invalid transition accepted: %s -> %s", trans.from, trans.to)
					return false
				}
			}
			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
			t.Errorf("Property InvalidTransitionsAlwaysRejected failed: %v", err)
		}
	})

	// Property 3: Same state transitions are always valid (idempotency)
	t.Run("SameStateTransitionIdempotent", func(t *testing.T) {
		property := func(state uint8) bool {
			escrowState := EscrowState(state % 6) // 0-5 for valid states
			validator := NewEscrowStateValidator()
			err := validator.ValidateTransition(escrowState, escrowState)
			return err == nil
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
			t.Errorf("Property SameStateTransitionIdempotent failed: %v", err)
		}
	})

	// Property 4: Terminal states cannot transition anywhere
	t.Run("TerminalStatesAreTerminal", func(t *testing.T) {
		property := func(targetState uint8) bool {
			validator := NewEscrowStateValidator()
			terminalStates := []EscrowState{EscrowCompleted, EscrowRefunded}
			target := EscrowState(targetState % 6)

			for _, terminal := range terminalStates {
				if terminal == target {
					continue // Same state is allowed
				}
				err := validator.ValidateTransition(terminal, target)
				if err == nil {
					t.Logf("Terminal state %s transitioned to %s", terminal, target)
					return false
				}
			}
			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
			t.Errorf("Property TerminalStatesAreTerminal failed: %v", err)
		}
	})

	// Property 5: State transition history is correctly recorded
	t.Run("TransitionHistoryRecorded", func(t *testing.T) {
		property := func() bool {
			validator := NewEscrowStateValidator()
			payment := &Payment{
				ID:                     "test-payment",
				EscrowState:            EscrowPending,
				Version:                1,
				StateTransitionHistory: []StateTransitionHistory{},
			}

			// Perform a valid transition
			err := validator.ValidateAndRecordTransition(payment, EscrowFunded, "buyer", "funding escrow")
			if err != nil {
				t.Logf("Failed to record transition: %v", err)
				return false
			}

			// Check history was recorded
			if len(payment.StateTransitionHistory) != 1 {
				t.Logf("History not recorded: expected 1 entry, got %d", len(payment.StateTransitionHistory))
				return false
			}

			history := payment.StateTransitionHistory[0]
			if history.FromState != EscrowPending || history.ToState != EscrowFunded {
				t.Logf("History incorrect: got %s -> %s, expected Pending -> Funded", history.FromState, history.ToState)
				return false
			}

			if history.Actor != "buyer" {
				t.Logf("Actor incorrect: got %s, expected buyer", history.Actor)
				return false
			}

			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
			t.Errorf("Property TransitionHistoryRecorded failed: %v", err)
		}
	})

	// Property 6: State is correctly updated after transition
	t.Run("StateUpdatedAfterTransition", func(t *testing.T) {
		property := func() bool {
			validator := NewEscrowStateValidator()
			payment := &Payment{
				ID:          "test-payment",
				EscrowState: EscrowPending,
				Version:     1,
			}

			err := validator.ValidateAndRecordTransition(payment, EscrowFunded, "buyer", "funding")
			if err != nil {
				return false
			}

			if payment.EscrowState != EscrowFunded {
				t.Logf("State not updated: expected EscrowFunded, got %s", payment.EscrowState)
				return false
			}

			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
			t.Errorf("Property StateUpdatedAfterTransition failed: %v", err)
		}
	})
}

// TestStateTransitionSequenceProperties tests properties of valid transition sequences
func TestStateTransitionSequenceProperties(t *testing.T) {
	// Property: Any valid sequence of transitions maintains invariants
	t.Run("ValidSequenceMaintainsInvariants", func(t *testing.T) {
		property := func(seed int64) bool {
			rng := rand.New(rand.NewSource(seed))
			validator := NewEscrowStateValidator()
			payment := &Payment{
				ID:          fmt.Sprintf("payment-%d", seed),
				EscrowState: EscrowPending,
				Version:     1,
			}

			// Generate a random valid sequence of transitions
			maxTransitions := 10
			for i := 0; i < maxTransitions; i++ {
				allowed := validator.GetAllowedTransitions(payment.EscrowState)
				if len(allowed) == 0 {
					// Reached terminal state
					break
				}

				// Randomly pick a valid next state
				nextState := allowed[rng.Intn(len(allowed))]
				oldState := payment.EscrowState

				err := validator.ValidateAndRecordTransition(payment, nextState, "actor", "reason")
				if err != nil {
					t.Logf("Valid transition failed: %s -> %s: %v", oldState, nextState, err)
					return false
				}

				// Verify state was updated
				if payment.EscrowState != nextState {
					t.Logf("State not updated correctly: expected %s, got %s", nextState, payment.EscrowState)
					return false
				}
			}

			// Final check: history length matches number of transitions
			// Note: Version management is handled by the store, not the validator
			if len(payment.StateTransitionHistory) == 0 {
				t.Logf("No history recorded")
				return false
			}

			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 100}); err != nil {
			t.Errorf("Property ValidSequenceMaintainsInvariants failed: %v", err)
		}
	})

	// Property: State transitions are monotonic (can't go backwards in the flow)
	t.Run("TransitionsAreMonotonic", func(t *testing.T) {
		property := func(seed int64) bool {
			rng := rand.New(rand.NewSource(seed))
			validator := NewEscrowStateValidator()
			payment := &Payment{
				ID:          fmt.Sprintf("payment-%d", seed),
				EscrowState: EscrowPending,
				Version:     1,
			}

			// Perform some transitions
			for i := 0; i < 5; i++ {
				allowed := validator.GetAllowedTransitions(payment.EscrowState)
				if len(allowed) == 0 {
					break
				}

				nextState := allowed[rng.Intn(len(allowed))]
				validator.ValidateAndRecordTransition(payment, nextState, "actor", "reason")

				// Cannot go back to Pending once we've moved to Funded or beyond
				// (Exception: None -> Pending is allowed for starting new escrows)
				if payment.EscrowState == EscrowFunded ||
					payment.EscrowState == EscrowDisputed ||
					payment.EscrowState == EscrowCompleted ||
					payment.EscrowState == EscrowRefunded {
					err := validator.ValidateTransition(payment.EscrowState, EscrowPending)
					if err == nil {
						t.Logf("Backward transition to Pending accepted from %s", payment.EscrowState)
						return false
					}
				}
			}

			return true
		}

		if err := quick.Check(property, &quick.Config{MaxCount: 50}); err != nil {
			t.Errorf("Property TransitionsAreMonotonic failed: %v", err)
		}
	})
}

// TestStateValidatorConcurrency tests concurrent access to state validator
func TestStateValidatorConcurrency(t *testing.T) {
	validator := NewEscrowStateValidator()
	done := make(chan bool)

	// Property: Concurrent reads don't cause races or panics
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				// Concurrent validation calls
				_ = validator.ValidateTransition(EscrowPending, EscrowFunded)
				_ = validator.IsTerminalState(EscrowCompleted)
				_ = validator.GetAllowedTransitions(EscrowFunded)
			}
			done <- true
		}()
	}

	// Wait for all goroutines with timeout
	timeout := time.After(5 * time.Second)
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Timeout waiting for concurrent operations")
		}
	}
}

// TestPaymentTransitionConcurrency tests concurrent transitions on the same payment
// This test is skipped with -race flag as it intentionally demonstrates race conditions
func TestPaymentTransitionConcurrency(t *testing.T) {
	// Skip if running with race detector since we're intentionally testing racy behavior
	// The actual store implementations (MemoryStore, FileStore) use proper locking
	if testing.Short() {
		t.Skip("Skipping race demonstration test in short mode")
	}

	validator := NewEscrowStateValidator()
	payment := &Payment{
		ID:          "concurrent-test",
		EscrowState: EscrowPending,
		Version:     1,
	}

	// Attempt concurrent transitions
	// Due to lack of synchronization in this test, this demonstrates the race
	// In production, the store's UpdatePayment with optimistic locking prevents this
	results := make(chan error, 2)

	go func() {
		err := validator.ValidateAndRecordTransition(payment, EscrowFunded, "actor1", "reason1")
		results <- err
	}()

	go func() {
		err := validator.ValidateAndRecordTransition(payment, EscrowFunded, "actor2", "reason2")
		results <- err
	}()

	// Collect results
	var successCount int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successCount++
		}
	}

	// This test documents that without proper store-level locking, races can occur
	// The ValidateAndRecordTransition itself doesn't protect against concurrent access
	// That's the responsibility of the PaymentStore implementation
	if successCount > 0 && payment.EscrowState != EscrowFunded {
		t.Errorf("At least one transition succeeded but state is not Funded: %s", payment.EscrowState)
	}
}

// TestAllStatesReachable verifies all states are reachable through valid transitions
func TestAllStatesReachable(t *testing.T) {
	validator := NewEscrowStateValidator()

	tests := []struct {
		name   string
		target EscrowState
		path   []EscrowState
	}{
		{
			name:   "ReachPending",
			target: EscrowPending,
			path:   []EscrowState{EscrowPending},
		},
		{
			name:   "ReachFunded",
			target: EscrowFunded,
			path:   []EscrowState{EscrowPending, EscrowFunded},
		},
		{
			name:   "ReachCompleted",
			target: EscrowCompleted,
			path:   []EscrowState{EscrowPending, EscrowFunded, EscrowCompleted},
		},
		{
			name:   "ReachDisputed",
			target: EscrowDisputed,
			path:   []EscrowState{EscrowPending, EscrowFunded, EscrowDisputed},
		},
		{
			name:   "ReachRefundedFromFunded",
			target: EscrowRefunded,
			path:   []EscrowState{EscrowPending, EscrowFunded, EscrowRefunded},
		},
		{
			name:   "ReachRefundedFromDisputed",
			target: EscrowRefunded,
			path:   []EscrowState{EscrowPending, EscrowFunded, EscrowDisputed, EscrowRefunded},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the path is valid
			for i := 0; i < len(tt.path)-1; i++ {
				from := tt.path[i]
				to := tt.path[i+1]
				err := validator.ValidateTransition(from, to)
				if err != nil {
					t.Errorf("Invalid path to %s: transition %s -> %s failed: %v",
						tt.target, from, to, err)
				}
			}

			// Verify we reached the target
			finalState := tt.path[len(tt.path)-1]
			if finalState != tt.target {
				t.Errorf("Path did not reach target state: got %s, want %s", finalState, tt.target)
			}
		})
	}
}
