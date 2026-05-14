// Package paywall tests dispute enhancements functionality
package paywall

import (
	"testing"
	"time"
)

func TestDefaultDisputeEnhancements(t *testing.T) {
	enhancements := DefaultDisputeEnhancements()

	if enhancements.DisputeFeePercentage != 0.01 {
		t.Errorf("DisputeFeePercentage = %v, want 0.01", enhancements.DisputeFeePercentage)
	}
	if enhancements.MinDisputeFee != 0.00001 {
		t.Errorf("MinDisputeFee = %v, want 0.00001", enhancements.MinDisputeFee)
	}
	if enhancements.MaxDisputeFee != 0.01 {
		t.Errorf("MaxDisputeFee = %v, want 0.01", enhancements.MaxDisputeFee)
	}
	if enhancements.MaxEvidenceSize != 10*1024*1024 {
		t.Errorf("MaxEvidenceSize = %v, want 10485760", enhancements.MaxEvidenceSize)
	}
	if enhancements.MaxEvidenceCount != 20 {
		t.Errorf("MaxEvidenceCount = %v, want 20", enhancements.MaxEvidenceCount)
	}
	if enhancements.DisputeRateLimit != 3 {
		t.Errorf("DisputeRateLimit = %v, want 3", enhancements.DisputeRateLimit)
	}
	if enhancements.DisputeRateWindow != 24*time.Hour {
		t.Errorf("DisputeRateWindow = %v, want 24h", enhancements.DisputeRateWindow)
	}
	if !enhancements.AllowTimeoutExtension {
		t.Error("AllowTimeoutExtension = false, want true")
	}
}

func TestDisputeFeeCalculator_CalculateFee(t *testing.T) {
	enhancements := DefaultDisputeEnhancements()
	calculator := NewDisputeFeeCalculator(enhancements)

	tests := []struct {
		name          string
		paymentAmount float64
		expectedFee   float64
	}{
		{
			name:          "typical payment",
			paymentAmount: 0.1,   // 0.1 BTC
			expectedFee:   0.001, // 1% = 0.001 BTC
		},
		{
			name:          "small payment hits minimum",
			paymentAmount: 0.0001,  // 0.0001 BTC
			expectedFee:   0.00001, // minimum fee
		},
		{
			name:          "large payment hits maximum",
			paymentAmount: 10.0, // 10 BTC
			expectedFee:   0.01, // maximum fee
		},
		{
			name:          "exact minimum",
			paymentAmount: 0.001,   // 0.001 BTC
			expectedFee:   0.00001, // 1% would be 0.00001, hits minimum
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee := calculator.CalculateFee(tt.paymentAmount)
			if fee != tt.expectedFee {
				t.Errorf("CalculateFee(%v) = %v, want %v", tt.paymentAmount, fee, tt.expectedFee)
			}
		})
	}
}

func TestDisputeFeeCalculator_ValidateFeePayment(t *testing.T) {
	enhancements := DefaultDisputeEnhancements()
	calculator := NewDisputeFeeCalculator(enhancements)

	tests := []struct {
		name        string
		requiredFee float64
		paidFee     float64
		wantErr     bool
	}{
		{
			name:        "exact payment",
			requiredFee: 0.001,
			paidFee:     0.001,
			wantErr:     false,
		},
		{
			name:        "overpayment",
			requiredFee: 0.001,
			paidFee:     0.002,
			wantErr:     false,
		},
		{
			name:        "underpayment",
			requiredFee: 0.001,
			paidFee:     0.0005,
			wantErr:     true,
		},
		{
			name:        "no payment",
			requiredFee: 0.001,
			paidFee:     0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := calculator.ValidateFeePayment(tt.requiredFee, tt.paidFee)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFeePayment() error = %v, wantErr %v", err, tt.wantErr)
			}
			// For error cases, just check that we got an error
			// The exact error message can vary
		})
	}
}

func TestDisputeRateLimiter(t *testing.T) {
	enhancements := &DisputeEnhancements{
		DisputeRateLimit:  2,
		DisputeRateWindow: 1 * time.Hour,
	}
	limiter := NewDisputeRateLimiter(enhancements)

	participantID := "participant-123"

	// First dispute should be allowed
	err := limiter.CheckRateLimit(participantID)
	if err != nil {
		t.Errorf("CheckRateLimit() first dispute error = %v, want nil", err)
	}
	limiter.RecordDispute(participantID)

	// Second dispute should be allowed
	err = limiter.CheckRateLimit(participantID)
	if err != nil {
		t.Errorf("CheckRateLimit() second dispute error = %v, want nil", err)
	}
	limiter.RecordDispute(participantID)

	// Third dispute should be blocked (limit is 2)
	err = limiter.CheckRateLimit(participantID)
	if err == nil {
		t.Error("CheckRateLimit() third dispute error = nil, want error")
	}

	// Check dispute count
	count := limiter.GetDisputeCount(participantID)
	if count != 2 {
		t.Errorf("GetDisputeCount() = %d, want 2", count)
	}

	// Different participant should be allowed
	err = limiter.CheckRateLimit("other-participant")
	if err != nil {
		t.Errorf("CheckRateLimit() different participant error = %v, want nil", err)
	}
}

func TestDisputeRateLimiter_TimeWindow(t *testing.T) {
	enhancements := &DisputeEnhancements{
		DisputeRateLimit:  2,
		DisputeRateWindow: 10 * time.Millisecond,
	}
	limiter := NewDisputeRateLimiter(enhancements)

	participantID := "participant-456"

	// File 2 disputes
	limiter.RecordDispute(participantID)
	limiter.RecordDispute(participantID)

	// Should be blocked immediately
	err := limiter.CheckRateLimit(participantID)
	if err == nil {
		t.Error("CheckRateLimit() immediate error = nil, want error")
	}

	// Wait for window to expire
	time.Sleep(20 * time.Millisecond)

	// Should be allowed now
	err = limiter.CheckRateLimit(participantID)
	if err != nil {
		t.Errorf("CheckRateLimit() after window error = %v, want nil", err)
	}
}

func TestEvidenceValidator_ValidateEvidence(t *testing.T) {
	enhancements := &DisputeEnhancements{
		MaxEvidenceSize:  100,
		MaxEvidenceCount: 3,
	}
	validator := NewEvidenceValidator(enhancements)

	tests := []struct {
		name                 string
		evidence             *Evidence
		currentEvidenceCount int
		wantErr              error
	}{
		{
			name: "valid evidence",
			evidence: &Evidence{
				Type:    EvidenceText,
				Content: "This is valid evidence",
			},
			currentEvidenceCount: 0,
			wantErr:              nil,
		},
		{
			name:                 "nil evidence",
			evidence:             nil,
			currentEvidenceCount: 0,
			wantErr:              ErrInvalidEvidence,
		},
		{
			name: "empty content",
			evidence: &Evidence{
				Type:    EvidenceText,
				Content: "",
			},
			currentEvidenceCount: 0,
			wantErr:              ErrInvalidEvidence,
		},
		{
			name: "evidence too large",
			evidence: &Evidence{
				Type:    EvidenceText,
				Content: string(make([]byte, 101)), // 101 bytes, exceeds limit
			},
			currentEvidenceCount: 0,
			wantErr:              ErrEvidenceTooLarge,
		},
		{
			name: "too many evidence items",
			evidence: &Evidence{
				Type:    EvidenceText,
				Content: "Valid content",
			},
			currentEvidenceCount: 3, // already at limit
			wantErr:              ErrInvalidEvidence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateEvidence(tt.evidence, tt.currentEvidenceCount)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateEvidence() error = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateEvidence() error = nil, want %v", tt.wantErr)
				}
			}
		})
	}
}

func TestTimeoutExtensionManager_RequestExtension(t *testing.T) {
	enhancements := &DisputeEnhancements{
		AllowTimeoutExtension: true,
		MaxTimeoutExtension:   7 * 24 * time.Hour,
	}
	manager := NewTimeoutExtensionManager(enhancements)

	// Test valid request
	err := manager.RequestExtension("payment-123", RoleBuyer, "Need more time", 24*time.Hour)
	if err != nil {
		t.Errorf("RequestExtension() valid error = %v, want nil", err)
	}

	// Test duplicate request
	err = manager.RequestExtension("payment-123", RoleBuyer, "Need more time", 24*time.Hour)
	if err == nil {
		t.Error("RequestExtension() duplicate error = nil, want error")
	}

	// Test extension too long
	err = manager.RequestExtension("payment-456", RoleSeller, "Need more time", 30*24*time.Hour)
	if err == nil {
		t.Error("RequestExtension() too long error = nil, want error")
	}
}

func TestTimeoutExtensionManager_ApproveExtension(t *testing.T) {
	enhancements := DefaultDisputeEnhancements()
	manager := NewTimeoutExtensionManager(enhancements)

	paymentID := "payment-789"

	// Create extension request
	manager.RequestExtension(paymentID, RoleBuyer, "Need time", 48*time.Hour)

	// Approve as seller
	err := manager.ApproveExtension(paymentID, RoleSeller)
	if err != nil {
		t.Errorf("ApproveExtension() error = %v, want nil", err)
	}

	// Check if approved
	approved, extension, err := manager.IsExtensionApproved(paymentID)
	if err != nil {
		t.Errorf("IsExtensionApproved() error = %v, want nil", err)
	}
	if !approved {
		t.Error("IsExtensionApproved() = false, want true")
	}
	if extension != 48*time.Hour {
		t.Errorf("IsExtensionApproved() extension = %v, want 48h", extension)
	}

	// Test duplicate approval
	err = manager.ApproveExtension(paymentID, RoleSeller)
	if err == nil {
		t.Error("ApproveExtension() duplicate error = nil, want error")
	}

	// Complete extension
	manager.CompleteExtension(paymentID)

	// Should not be found after completion
	_, err = manager.GetPendingExtension(paymentID)
	if err == nil {
		t.Error("GetPendingExtension() after completion error = nil, want error")
	}
}

func TestTimeoutExtensionManager_PartialApproval(t *testing.T) {
	enhancements := DefaultDisputeEnhancements()
	manager := NewTimeoutExtensionManager(enhancements)

	paymentID := "payment-partial"

	// Create extension request from buyer
	manager.RequestExtension(paymentID, RoleBuyer, "Need time", 24*time.Hour)

	// Check approval status (only buyer has approved so far)
	approved, _, err := manager.IsExtensionApproved(paymentID)
	if err != nil {
		t.Errorf("IsExtensionApproved() error = %v, want nil", err)
	}
	if approved {
		t.Error("IsExtensionApproved() with only buyer = true, want false")
	}

	// Now seller approves
	manager.ApproveExtension(paymentID, RoleSeller)

	// Should be approved now
	approved, _, err = manager.IsExtensionApproved(paymentID)
	if err != nil {
		t.Errorf("IsExtensionApproved() error = %v, want nil", err)
	}
	if !approved {
		t.Error("IsExtensionApproved() with both parties = false, want true")
	}
}

func TestTimeoutExtensionManager_DisallowExtensions(t *testing.T) {
	enhancements := &DisputeEnhancements{
		AllowTimeoutExtension: false,
		MaxTimeoutExtension:   7 * 24 * time.Hour,
	}
	manager := NewTimeoutExtensionManager(enhancements)

	err := manager.RequestExtension("payment-123", RoleBuyer, "Need time", 24*time.Hour)
	if err != ErrTimeoutExtensionDenied {
		t.Errorf("RequestExtension() with disallowed error = %v, want %v", err, ErrTimeoutExtensionDenied)
	}
}

func TestNewDisputeFeeCalculator_NilEnhancements(t *testing.T) {
	calculator := NewDisputeFeeCalculator(nil)
	if calculator == nil {
		t.Fatal("NewDisputeFeeCalculator(nil) = nil, want non-nil with defaults")
	}

	// Should use default enhancements
	fee := calculator.CalculateFee(0.1)
	if fee != 0.001 {
		t.Errorf("CalculateFee(0.1) with default = %v, want 0.001", fee)
	}
}

func TestNewDisputeRateLimiter_NilEnhancements(t *testing.T) {
	limiter := NewDisputeRateLimiter(nil)
	if limiter == nil {
		t.Fatal("NewDisputeRateLimiter(nil) = nil, want non-nil with defaults")
	}

	// Should not error on first dispute
	err := limiter.CheckRateLimit("test")
	if err != nil {
		t.Errorf("CheckRateLimit() with default error = %v, want nil", err)
	}
}

func TestNewEvidenceValidator_NilEnhancements(t *testing.T) {
	validator := NewEvidenceValidator(nil)
	if validator == nil {
		t.Fatal("NewEvidenceValidator(nil) = nil, want non-nil with defaults")
	}

	// Should validate normally with defaults
	evidence := &Evidence{
		Type:    EvidenceText,
		Content: "Test",
	}
	err := validator.ValidateEvidence(evidence, 0)
	if err != nil {
		t.Errorf("ValidateEvidence() with default error = %v, want nil", err)
	}
}
