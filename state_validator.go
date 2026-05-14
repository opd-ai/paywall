// Package paywall implements state transition validation for escrow payments
package paywall

import (
	"fmt"
	"time"
)

// StateTransitionHistory records a state change in the payment lifecycle
type StateTransitionHistory struct {
	// FromState is the previous escrow state
	FromState EscrowState `json:"from_state"`
	// ToState is the new escrow state
	ToState EscrowState `json:"to_state"`
	// Timestamp is when the transition occurred
	Timestamp time.Time `json:"timestamp"`
	// Actor identifies who initiated the transition
	Actor string `json:"actor"`
	// Reason provides context for the transition
	Reason string `json:"reason,omitempty"`
}

// EscrowStateValidator validates state transitions for escrow payments
type EscrowStateValidator struct {
	// validTransitions maps each state to its allowed next states
	validTransitions map[EscrowState][]EscrowState
}

// NewEscrowStateValidator creates a validator with standard transition rules
func NewEscrowStateValidator() *EscrowStateValidator {
	return &EscrowStateValidator{
		validTransitions: map[EscrowState][]EscrowState{
			// Pending can transition to Funded or back to None (cancelled)
			EscrowPending: {EscrowFunded, EscrowNone},
			// Funded can transition to Completed, Disputed, or Refunded
			EscrowFunded: {EscrowCompleted, EscrowDisputed, EscrowRefunded},
			// Disputed can transition to Completed or Refunded (via arbiter decision)
			EscrowDisputed: {EscrowCompleted, EscrowRefunded},
			// Terminal states cannot transition
			EscrowCompleted: {},
			EscrowRefunded:  {},
			EscrowNone:      {EscrowPending}, // Can start new escrow
		},
	}
}

// ValidateTransition checks if a state transition is allowed
// Parameters:
//   - from: Current escrow state
//   - to: Desired escrow state
//
// Returns:
//   - error: nil if valid, error describing why transition is invalid
func (v *EscrowStateValidator) ValidateTransition(from, to EscrowState) error {
	// Same state is always valid (no-op)
	if from == to {
		return nil
	}

	// Get allowed transitions for current state
	allowed, exists := v.validTransitions[from]
	if !exists {
		return fmt.Errorf("invalid source state: %s", from.String())
	}

	// Check if target state is allowed
	for _, validState := range allowed {
		if validState == to {
			return nil
		}
	}

	return fmt.Errorf("invalid transition: %s -> %s (allowed: %v)",
		from.String(), to.String(), v.stateSliceToStrings(allowed))
}

// IsTerminalState checks if a state is terminal (cannot transition further)
func (v *EscrowStateValidator) IsTerminalState(state EscrowState) bool {
	allowed, exists := v.validTransitions[state]
	return exists && len(allowed) == 0
}

// GetAllowedTransitions returns all valid next states for the current state
func (v *EscrowStateValidator) GetAllowedTransitions(from EscrowState) []EscrowState {
	allowed, exists := v.validTransitions[from]
	if !exists {
		return []EscrowState{}
	}

	// Return a copy to prevent modification
	result := make([]EscrowState, len(allowed))
	copy(result, allowed)
	return result
}

// stateSliceToStrings converts state slice to string slice for error messages
func (v *EscrowStateValidator) stateSliceToStrings(states []EscrowState) []string {
	strs := make([]string, len(states))
	for i, s := range states {
		strs[i] = s.String()
	}
	return strs
}

// ValidateAndRecordTransition validates a transition and creates a history entry
// Parameters:
//   - payment: Payment to update
//   - toState: Desired new state
//   - actor: Who is making the transition
//   - reason: Why the transition is happening
//
// Returns:
//   - error: nil if valid and recorded, error if invalid
func (v *EscrowStateValidator) ValidateAndRecordTransition(
	payment *Payment,
	toState EscrowState,
	actor string,
	reason string,
) error {
	// Validate transition
	if err := v.ValidateTransition(payment.EscrowState, toState); err != nil {
		return err
	}

	// Create history entry
	historyEntry := StateTransitionHistory{
		FromState: payment.EscrowState,
		ToState:   toState,
		Timestamp: time.Now(),
		Actor:     actor,
		Reason:    reason,
	}

	// Append to payment history
	if payment.StateTransitionHistory == nil {
		payment.StateTransitionHistory = []StateTransitionHistory{}
	}
	payment.StateTransitionHistory = append(payment.StateTransitionHistory, historyEntry)

	// Update payment state
	payment.EscrowState = toState

	return nil
}
