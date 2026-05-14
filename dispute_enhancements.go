// Package paywall implements enhanced dispute resolution features
package paywall

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrDisputeFeeRequired indicates a dispute fee must be paid
	ErrDisputeFeeRequired = errors.New("dispute fee required")
	// ErrDisputeRateLimitExceeded indicates too many disputes filed
	ErrDisputeRateLimitExceeded = errors.New("dispute rate limit exceeded")
	// ErrEvidenceTooLarge indicates evidence exceeds size limits
	ErrEvidenceTooLarge = errors.New("evidence exceeds maximum size")
	// ErrTimeoutExtensionDenied indicates timeout extension was rejected
	ErrTimeoutExtensionDenied = errors.New("timeout extension denied")
)

// DisputeEnhancements extends the dispute system with fees, rate limiting, and other improvements
type DisputeEnhancements struct {
	// DisputeFeePercentage is the fee as a percentage of payment amount (e.g., 0.01 = 1%)
	DisputeFeePercentage float64
	// MinDisputeFee is the minimum fee in base currency units
	MinDisputeFee float64
	// MaxDisputeFee is the maximum fee in base currency units
	MaxDisputeFee float64
	// MaxEvidenceSize is the maximum evidence size in bytes
	MaxEvidenceSize int64
	// MaxEvidenceCount is the maximum number of evidence items per dispute
	MaxEvidenceCount int
	// DisputeRateLimit is the maximum disputes allowed per time window
	DisputeRateLimit int
	// DisputeRateWindow is the time window for rate limiting
	DisputeRateWindow time.Duration
	// AllowTimeoutExtension enables timeout extension requests
	AllowTimeoutExtension bool
	// MaxTimeoutExtension is the maximum time a timeout can be extended
	MaxTimeoutExtension time.Duration
}

// DefaultDisputeEnhancements returns sensible default values
func DefaultDisputeEnhancements() *DisputeEnhancements {
	return &DisputeEnhancements{
		DisputeFeePercentage:  0.01,             // 1% of payment
		MinDisputeFee:         0.00001,          // 1000 satoshis minimum
		MaxDisputeFee:         0.01,             // 0.01 BTC maximum
		MaxEvidenceSize:       10 * 1024 * 1024, // 10 MB
		MaxEvidenceCount:      20,               // 20 pieces of evidence
		DisputeRateLimit:      3,                // 3 disputes
		DisputeRateWindow:     24 * time.Hour,   // per 24 hours
		AllowTimeoutExtension: true,
		MaxTimeoutExtension:   7 * 24 * time.Hour, // 7 days max extension
	}
}

// DisputeFeeCalculator calculates dispute fees
type DisputeFeeCalculator struct {
	enhancements *DisputeEnhancements
}

// NewDisputeFeeCalculator creates a new fee calculator
func NewDisputeFeeCalculator(enhancements *DisputeEnhancements) *DisputeFeeCalculator {
	if enhancements == nil {
		enhancements = DefaultDisputeEnhancements()
	}
	return &DisputeFeeCalculator{
		enhancements: enhancements,
	}
}

// CalculateFee calculates the dispute fee for a payment
func (dfc *DisputeFeeCalculator) CalculateFee(paymentAmount float64) float64 {
	fee := paymentAmount * dfc.enhancements.DisputeFeePercentage

	// Apply minimum
	if fee < dfc.enhancements.MinDisputeFee {
		fee = dfc.enhancements.MinDisputeFee
	}

	// Apply maximum
	if fee > dfc.enhancements.MaxDisputeFee {
		fee = dfc.enhancements.MaxDisputeFee
	}

	return fee
}

// ValidateFeePayment checks if the dispute fee has been paid
func (dfc *DisputeFeeCalculator) ValidateFeePayment(requiredFee, paidFee float64) error {
	if paidFee < requiredFee {
		return fmt.Errorf("%w: required %.8f, paid %.8f", ErrDisputeFeeRequired, requiredFee, paidFee)
	}
	return nil
}

// DisputeRateLimiter manages rate limiting for disputes
type DisputeRateLimiter struct {
	enhancements *DisputeEnhancements
	// disputes tracks dispute timestamps per participant
	disputes map[string][]time.Time
	mu       sync.RWMutex
}

// NewDisputeRateLimiter creates a new rate limiter
func NewDisputeRateLimiter(enhancements *DisputeEnhancements) *DisputeRateLimiter {
	if enhancements == nil {
		enhancements = DefaultDisputeEnhancements()
	}
	return &DisputeRateLimiter{
		enhancements: enhancements,
		disputes:     make(map[string][]time.Time),
	}
}

// CheckRateLimit verifies if a participant can file a dispute
func (drl *DisputeRateLimiter) CheckRateLimit(participantID string) error {
	drl.mu.Lock()
	defer drl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-drl.enhancements.DisputeRateWindow)

	// Get recent disputes for this participant
	disputes, exists := drl.disputes[participantID]
	if !exists {
		// First dispute for this participant
		return nil
	}

	// Filter to disputes within the time window
	recentDisputes := make([]time.Time, 0)
	for _, disputeTime := range disputes {
		if disputeTime.After(windowStart) {
			recentDisputes = append(recentDisputes, disputeTime)
		}
	}

	// Update filtered list
	drl.disputes[participantID] = recentDisputes

	// Check if limit exceeded
	if len(recentDisputes) >= drl.enhancements.DisputeRateLimit {
		oldestDispute := recentDisputes[0]
		timeUntilAllowed := oldestDispute.Add(drl.enhancements.DisputeRateWindow).Sub(now)
		return fmt.Errorf("%w: %d disputes in last %s, try again in %s",
			ErrDisputeRateLimitExceeded,
			len(recentDisputes),
			drl.enhancements.DisputeRateWindow,
			timeUntilAllowed.Round(time.Minute))
	}

	return nil
}

// RecordDispute records a new dispute for rate limiting
func (drl *DisputeRateLimiter) RecordDispute(participantID string) {
	drl.mu.Lock()
	defer drl.mu.Unlock()

	if drl.disputes[participantID] == nil {
		drl.disputes[participantID] = make([]time.Time, 0)
	}
	drl.disputes[participantID] = append(drl.disputes[participantID], time.Now())
}

// GetDisputeCount returns the number of recent disputes for a participant
func (drl *DisputeRateLimiter) GetDisputeCount(participantID string) int {
	drl.mu.RLock()
	defer drl.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-drl.enhancements.DisputeRateWindow)

	disputes, exists := drl.disputes[participantID]
	if !exists {
		return 0
	}

	count := 0
	for _, disputeTime := range disputes {
		if disputeTime.After(windowStart) {
			count++
		}
	}
	return count
}

// EvidenceValidator validates evidence submissions
type EvidenceValidator struct {
	enhancements *DisputeEnhancements
}

// NewEvidenceValidator creates a new evidence validator
func NewEvidenceValidator(enhancements *DisputeEnhancements) *EvidenceValidator {
	if enhancements == nil {
		enhancements = DefaultDisputeEnhancements()
	}
	return &EvidenceValidator{
		enhancements: enhancements,
	}
}

// ValidateEvidence checks if evidence meets size and count requirements
func (ev *EvidenceValidator) ValidateEvidence(evidence *Evidence, currentEvidenceCount int) error {
	if evidence == nil {
		return ErrInvalidEvidence
	}

	// Check evidence count limit
	if currentEvidenceCount >= ev.enhancements.MaxEvidenceCount {
		return fmt.Errorf("%w: maximum %d pieces allowed, already have %d",
			ErrInvalidEvidence,
			ev.enhancements.MaxEvidenceCount,
			currentEvidenceCount)
	}

	// Check evidence size
	evidenceSize := int64(len(evidence.Content))
	if evidenceSize > ev.enhancements.MaxEvidenceSize {
		return fmt.Errorf("%w: evidence size %d bytes exceeds maximum %d bytes",
			ErrEvidenceTooLarge,
			evidenceSize,
			ev.enhancements.MaxEvidenceSize)
	}

	// Validate content is not empty
	if len(evidence.Content) == 0 {
		return fmt.Errorf("%w: evidence content cannot be empty", ErrInvalidEvidence)
	}

	return nil
}

// TimeoutExtensionRequest represents a request to extend an escrow timeout
type TimeoutExtensionRequest struct {
	// PaymentID is the payment to extend
	PaymentID string
	// RequestedBy is the participant requesting the extension
	RequestedBy MultisigRole
	// Reason is why the extension is needed
	Reason string
	// RequestedExtension is how much time to add
	RequestedExtension time.Duration
	// ApprovedBy tracks who has approved the extension
	ApprovedBy []MultisigRole
	// CreatedAt is when the request was made
	CreatedAt time.Time
}

// TimeoutExtensionManager manages timeout extension requests
type TimeoutExtensionManager struct {
	enhancements *DisputeEnhancements
	// requests tracks pending extension requests by payment ID
	requests map[string]*TimeoutExtensionRequest
	mu       sync.RWMutex
}

// NewTimeoutExtensionManager creates a new timeout extension manager
func NewTimeoutExtensionManager(enhancements *DisputeEnhancements) *TimeoutExtensionManager {
	if enhancements == nil {
		enhancements = DefaultDisputeEnhancements()
	}
	return &TimeoutExtensionManager{
		enhancements: enhancements,
		requests:     make(map[string]*TimeoutExtensionRequest),
	}
}

// RequestExtension creates a timeout extension request
func (tem *TimeoutExtensionManager) RequestExtension(paymentID string, requestedBy MultisigRole, reason string, extension time.Duration) error {
	tem.mu.Lock()
	defer tem.mu.Unlock()

	if !tem.enhancements.AllowTimeoutExtension {
		return ErrTimeoutExtensionDenied
	}

	// Check if extension is within allowed limits
	if extension > tem.enhancements.MaxTimeoutExtension {
		return fmt.Errorf("%w: requested %s exceeds maximum %s",
			ErrTimeoutExtensionDenied,
			extension,
			tem.enhancements.MaxTimeoutExtension)
	}

	// Check if there's already a pending request
	if _, exists := tem.requests[paymentID]; exists {
		return fmt.Errorf("timeout extension request already exists for payment %s", paymentID)
	}

	request := &TimeoutExtensionRequest{
		PaymentID:          paymentID,
		RequestedBy:        requestedBy,
		Reason:             reason,
		RequestedExtension: extension,
		ApprovedBy:         []MultisigRole{requestedBy},
		CreatedAt:          time.Now(),
	}

	tem.requests[paymentID] = request
	return nil
}

// ApproveExtension adds approval from a participant
func (tem *TimeoutExtensionManager) ApproveExtension(paymentID string, approver MultisigRole) error {
	tem.mu.Lock()
	defer tem.mu.Unlock()

	request, exists := tem.requests[paymentID]
	if !exists {
		return fmt.Errorf("no extension request found for payment %s", paymentID)
	}

	// Check if already approved by this participant
	for _, existingApprover := range request.ApprovedBy {
		if existingApprover == approver {
			return fmt.Errorf("participant %s already approved this extension", approver)
		}
	}

	request.ApprovedBy = append(request.ApprovedBy, approver)
	return nil
}

// IsExtensionApproved checks if an extension has been approved by all parties
// Requires approval from both buyer and seller
func (tem *TimeoutExtensionManager) IsExtensionApproved(paymentID string) (bool, time.Duration, error) {
	tem.mu.RLock()
	defer tem.mu.RUnlock()

	request, exists := tem.requests[paymentID]
	if !exists {
		return false, 0, fmt.Errorf("no extension request found for payment %s", paymentID)
	}

	// Check if both buyer and seller have approved
	hasBuyer := false
	hasSeller := false
	for _, approver := range request.ApprovedBy {
		if approver == RoleBuyer {
			hasBuyer = true
		}
		if approver == RoleSeller {
			hasSeller = true
		}
	}

	approved := hasBuyer && hasSeller
	return approved, request.RequestedExtension, nil
}

// CompleteExtension removes the extension request after it's been applied
func (tem *TimeoutExtensionManager) CompleteExtension(paymentID string) {
	tem.mu.Lock()
	defer tem.mu.Unlock()
	delete(tem.requests, paymentID)
}

// GetPendingExtension retrieves a pending extension request
func (tem *TimeoutExtensionManager) GetPendingExtension(paymentID string) (*TimeoutExtensionRequest, error) {
	tem.mu.RLock()
	defer tem.mu.RUnlock()

	request, exists := tem.requests[paymentID]
	if !exists {
		return nil, fmt.Errorf("no extension request found for payment %s", paymentID)
	}

	// Return a copy to prevent external modification
	requestCopy := *request
	requestCopy.ApprovedBy = make([]MultisigRole, len(request.ApprovedBy))
	copy(requestCopy.ApprovedBy, request.ApprovedBy)

	return &requestCopy, nil
}
