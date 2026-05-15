package paywall

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewLocalArbiter(t *testing.T) {
	arbiter := NewLocalArbiter()
	if arbiter == nil {
		t.Fatal("NewLocalArbiter() returned nil")
	}
	if arbiter.disputes == nil {
		t.Error("NewLocalArbiter() did not initialize disputes map")
	}
}

func TestLocalArbiter_RegisterDispute(t *testing.T) {
	tests := []struct {
		name      string
		payment   *Payment
		requester MultisigRole
		wantErr   bool
	}{
		{
			name:      "nil payment",
			payment:   nil,
			requester: RoleBuyer,
			wantErr:   true,
		},
		{
			name: "valid payment with buyer requester",
			payment: &Payment{
				ID:            "test-payment-1",
				DisputeReason: "goods not received",
			},
			requester: RoleBuyer,
			wantErr:   false,
		},
		{
			name: "valid payment with seller requester",
			payment: &Payment{
				ID:            "test-payment-2",
				DisputeReason: "buyer not responding",
			},
			requester: RoleSeller,
			wantErr:   false,
		},
		{
			name: "invalid requester - arbiter",
			payment: &Payment{
				ID:            "test-payment-3",
				DisputeReason: "some reason",
			},
			requester: RoleArbiter,
			wantErr:   true,
		},
		{
			name: "duplicate registration",
			payment: &Payment{
				ID:            "test-payment-1",
				DisputeReason: "different reason",
			},
			requester: RoleBuyer,
			wantErr:   true,
		},
	}

	arbiter := NewLocalArbiter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := arbiter.RegisterDispute(tt.payment, tt.requester)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterDispute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.payment != nil {
				dispute, err := arbiter.GetDispute(tt.payment.ID)
				if err != nil {
					t.Errorf("GetDispute() failed after successful registration: %v", err)
				}
				if dispute.PaymentID != tt.payment.ID {
					t.Errorf("Registered dispute paymentID = %v, want %v", dispute.PaymentID, tt.payment.ID)
				}
				if dispute.Status != DisputeOpen {
					t.Errorf("Registered dispute status = %v, want %v", dispute.Status, DisputeOpen)
				}
				if dispute.Requester != tt.requester {
					t.Errorf("Registered dispute requester = %v, want %v", dispute.Requester, tt.requester)
				}
			}
		})
	}
}

func TestLocalArbiter_SubmitEvidence(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "goods not received",
	}
	arbiter.RegisterDispute(payment, RoleBuyer)

	tests := []struct {
		name      string
		paymentID string
		evidence  *Evidence
		wantErr   bool
	}{
		{
			name:      "dispute not found",
			paymentID: "nonexistent",
			evidence: &Evidence{
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "some evidence",
			},
			wantErr: true,
		},
		{
			name:      "nil evidence",
			paymentID: payment.ID,
			evidence:  nil,
			wantErr:   true,
		},
		{
			name:      "empty content",
			paymentID: payment.ID,
			evidence: &Evidence{
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "",
			},
			wantErr: true,
		},
		{
			name:      "valid text evidence",
			paymentID: payment.ID,
			evidence: &Evidence{
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "Tracking number shows package never arrived",
				Description: "Shipping proof",
			},
			wantErr: false,
		},
		{
			name:      "valid image evidence",
			paymentID: payment.ID,
			evidence: &Evidence{
				Type:        EvidenceImage,
				SubmittedBy: RoleSeller,
				Content:     "https://example.com/shipping-receipt.jpg",
				Description: "Shipping receipt",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := arbiter.SubmitEvidence(tt.paymentID, tt.evidence)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitEvidence() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				dispute, _ := arbiter.GetDispute(tt.paymentID)
				if len(dispute.Evidence) == 0 {
					t.Error("SubmitEvidence() did not add evidence to dispute")
				}
				lastEvidence := dispute.Evidence[len(dispute.Evidence)-1]
				if lastEvidence.PaymentID != tt.paymentID {
					t.Errorf("Evidence paymentID = %v, want %v", lastEvidence.PaymentID, tt.paymentID)
				}
				if lastEvidence.ID == "" {
					t.Error("SubmitEvidence() did not set evidence ID")
				}
				if lastEvidence.Timestamp.IsZero() {
					t.Error("SubmitEvidence() did not set evidence timestamp")
				}
			}
		})
	}
}

func TestLocalArbiter_SubmitEvidence_AfterResolution(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "test reason",
	}
	arbiter.RegisterDispute(payment, RoleBuyer)

	// Resolve the dispute
	resolution := &Resolution{
		Decision:  RoleBuyer,
		Reason:    "Buyer was right",
		ArbiterID: "arbiter-1",
		Evidence:  []string{},
	}
	arbiter.ResolveDispute(payment.ID, resolution)

	// Try to submit evidence after resolution
	evidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleSeller,
		Content:     "late evidence",
	}
	err := arbiter.SubmitEvidence(payment.ID, evidence)

	if err != ErrDisputeAlreadyResolved {
		t.Errorf("SubmitEvidence() after resolution error = %v, want %v", err, ErrDisputeAlreadyResolved)
	}
}

func TestLocalArbiter_GetResolution(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "test reason",
	}
	arbiter.RegisterDispute(payment, RoleSeller)

	// Before resolution
	_, err := arbiter.GetResolution(payment.ID)
	if err == nil {
		t.Error("GetResolution() should return error before resolution")
	}

	// After resolution
	resolution := &Resolution{
		Decision:  RoleBuyer,
		Reason:    "Evidence favors buyer",
		ArbiterID: "arbiter-1",
		Evidence:  []string{"evidence-1", "evidence-2"},
	}
	arbiter.ResolveDispute(payment.ID, resolution)

	gotResolution, err := arbiter.GetResolution(payment.ID)
	if err != nil {
		t.Fatalf("GetResolution() error = %v", err)
	}

	if gotResolution.Decision != resolution.Decision {
		t.Errorf("GetResolution() decision = %v, want %v", gotResolution.Decision, resolution.Decision)
	}
	if gotResolution.Reason != resolution.Reason {
		t.Errorf("GetResolution() reason = %v, want %v", gotResolution.Reason, resolution.Reason)
	}
	if gotResolution.ArbiterID != resolution.ArbiterID {
		t.Errorf("GetResolution() arbiterID = %v, want %v", gotResolution.ArbiterID, resolution.ArbiterID)
	}
	if gotResolution.Timestamp.IsZero() {
		t.Error("GetResolution() timestamp not set")
	}
}

func TestLocalArbiter_GetDispute(t *testing.T) {
	arbiter := NewLocalArbiter()

	// Nonexistent dispute
	_, err := arbiter.GetDispute("nonexistent")
	if err != ErrDisputeNotFound {
		t.Errorf("GetDispute() for nonexistent = %v, want %v", err, ErrDisputeNotFound)
	}

	// Create and retrieve dispute
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "test reason",
	}
	arbiter.RegisterDispute(payment, RoleSeller)

	dispute, err := arbiter.GetDispute(payment.ID)
	if err != nil {
		t.Fatalf("GetDispute() error = %v", err)
	}

	if dispute.PaymentID != payment.ID {
		t.Errorf("GetDispute() paymentID = %v, want %v", dispute.PaymentID, payment.ID)
	}
	if dispute.Reason != payment.DisputeReason {
		t.Errorf("GetDispute() reason = %v, want %v", dispute.Reason, payment.DisputeReason)
	}
}

func TestLocalArbiter_ListOpenDisputes(t *testing.T) {
	arbiter := NewLocalArbiter()

	// Initially empty
	disputes, err := arbiter.ListOpenDisputes()
	if err != nil {
		t.Fatalf("ListOpenDisputes() error = %v", err)
	}
	if len(disputes) != 0 {
		t.Errorf("ListOpenDisputes() count = %d, want 0", len(disputes))
	}

	// Add some disputes
	for i := 1; i <= 3; i++ {
		payment := &Payment{
			ID:            fmt.Sprintf("payment-%d", i),
			DisputeReason: "test reason",
		}
		requester := RoleBuyer
		if i%2 == 0 {
			requester = RoleSeller
		}
		arbiter.RegisterDispute(payment, requester)
	}

	disputes, err = arbiter.ListOpenDisputes()
	if err != nil {
		t.Fatalf("ListOpenDisputes() error = %v", err)
	}
	if len(disputes) != 3 {
		t.Errorf("ListOpenDisputes() count = %d, want 3", len(disputes))
	}

	// Resolve one
	resolution := &Resolution{
		Decision:  RoleBuyer,
		Reason:    "test",
		ArbiterID: "arbiter-1",
	}
	arbiter.ResolveDispute("payment-1", resolution)

	disputes, err = arbiter.ListOpenDisputes()
	if err != nil {
		t.Fatalf("ListOpenDisputes() error = %v", err)
	}
	if len(disputes) != 2 {
		t.Errorf("ListOpenDisputes() after resolution count = %d, want 2", len(disputes))
	}
}

func TestLocalArbiter_ResolveDispute(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*LocalArbiter) string
		resolution *Resolution
		wantErr    bool
	}{
		{
			name: "dispute not found",
			setup: func(la *LocalArbiter) string {
				return "nonexistent"
			},
			resolution: &Resolution{
				Decision:  RoleBuyer,
				Reason:    "test",
				ArbiterID: "arbiter-1",
			},
			wantErr: true,
		},
		{
			name: "nil resolution",
			setup: func(la *LocalArbiter) string {
				payment := &Payment{
					ID:            "test-payment-1",
					DisputeReason: "test",
				}
				la.RegisterDispute(payment, RoleBuyer)
				return payment.ID
			},
			resolution: nil,
			wantErr:    true,
		},
		{
			name: "valid resolution",
			setup: func(la *LocalArbiter) string {
				payment := &Payment{
					ID:            "test-payment-2",
					DisputeReason: "test",
				}
				la.RegisterDispute(payment, RoleSeller)
				return payment.ID
			},
			resolution: &Resolution{
				Decision:  RoleSeller,
				Reason:    "Evidence shows seller fulfilled obligations",
				ArbiterID: "arbiter-1",
				Evidence:  []string{"evidence-1"},
			},
			wantErr: false,
		},
		{
			name: "already resolved",
			setup: func(la *LocalArbiter) string {
				payment := &Payment{
					ID:            "test-payment-3",
					DisputeReason: "test",
				}
				la.RegisterDispute(payment, RoleBuyer)
				la.ResolveDispute(payment.ID, &Resolution{
					Decision:  RoleBuyer,
					Reason:    "first resolution",
					ArbiterID: "arbiter-1",
				})
				return payment.ID
			},
			resolution: &Resolution{
				Decision:  RoleSeller,
				Reason:    "second resolution",
				ArbiterID: "arbiter-1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arbiter := NewLocalArbiter()
			paymentID := tt.setup(arbiter)

			err := arbiter.ResolveDispute(paymentID, tt.resolution)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveDispute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				dispute, _ := arbiter.GetDispute(paymentID)
				if dispute.Status != DisputeResolved {
					t.Errorf("ResolveDispute() status = %v, want %v", dispute.Status, DisputeResolved)
				}
				if dispute.Resolution == nil {
					t.Error("ResolveDispute() did not set resolution")
				}
				if dispute.ResolvedAt.IsZero() {
					t.Error("ResolveDispute() did not set resolvedAt timestamp")
				}
				if dispute.Resolution.Timestamp.IsZero() {
					t.Error("ResolveDispute() did not set resolution timestamp")
				}
			}
		})
	}
}

func TestLocalArbiter_CloseDispute(t *testing.T) {
	arbiter := NewLocalArbiter()

	// Nonexistent dispute
	err := arbiter.CloseDispute("nonexistent")
	if err != ErrDisputeNotFound {
		t.Errorf("CloseDispute() nonexistent error = %v, want %v", err, ErrDisputeNotFound)
	}

	// Close open dispute
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "test",
	}
	arbiter.RegisterDispute(payment, RoleBuyer)

	err = arbiter.CloseDispute(payment.ID)
	if err != nil {
		t.Fatalf("CloseDispute() error = %v", err)
	}

	dispute, _ := arbiter.GetDispute(payment.ID)
	if dispute.Status != DisputeClosed {
		t.Errorf("CloseDispute() status = %v, want %v", dispute.Status, DisputeClosed)
	}
	if dispute.ResolvedAt.IsZero() {
		t.Error("CloseDispute() did not set resolvedAt timestamp")
	}

	// Try to close already closed
	err = arbiter.CloseDispute(payment.ID)
	if err != ErrDisputeAlreadyResolved {
		t.Errorf("CloseDispute() already closed error = %v, want %v", err, ErrDisputeAlreadyResolved)
	}
}

func TestEvidence_Types(t *testing.T) {
	types := []EvidenceType{
		EvidenceText,
		EvidenceImage,
		EvidenceDocument,
		EvidenceTransaction,
	}

	if len(types) != 4 {
		t.Errorf("Expected 4 evidence types, got %d", len(types))
	}

	// Ensure types are distinct
	seen := make(map[EvidenceType]bool)
	for _, typ := range types {
		if seen[typ] {
			t.Errorf("Duplicate evidence type: %v", typ)
		}
		seen[typ] = true
	}
}

func TestDisputeStatus_Values(t *testing.T) {
	statuses := []DisputeStatus{
		DisputeOpen,
		DisputeUnderReview,
		DisputeResolved,
		DisputeClosed,
	}

	if len(statuses) != 4 {
		t.Errorf("Expected 4 dispute statuses, got %d", len(statuses))
	}

	// Ensure statuses are distinct
	seen := make(map[DisputeStatus]bool)
	for _, status := range statuses {
		if seen[status] {
			t.Errorf("Duplicate dispute status: %v", status)
		}
		seen[status] = true
	}
}

func TestArbiterInterface(t *testing.T) {
	// Verify LocalArbiter implements Arbiter interface
	var _ Arbiter = (*LocalArbiter)(nil)
}

func TestSubmitEvidence_WithSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:          "test-payment-sig",
		EscrowState: EscrowDisputed,
	}

	err := arbiter.RegisterDispute(payment, RoleBuyer)
	if err != nil {
		t.Fatalf("RegisterDispute() error = %v", err)
	}

	evidence := &Evidence{
		Type:            EvidenceText,
		SubmittedBy:     RoleBuyer,
		Content:         "Payment not received",
		Description:     "Buyer claims non-delivery",
		Signature:       []byte("valid-signature-12345678"),
		SubmitterPubKey: []byte("buyer-public-key"),
	}

	err = arbiter.SubmitEvidence("test-payment-sig", evidence)
	if err != nil {
		t.Errorf("SubmitEvidence() with valid signature error = %v", err)
	}
}

func TestSubmitEvidence_InvalidSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:          "test-payment-badsig",
		EscrowState: EscrowDisputed,
	}

	err := arbiter.RegisterDispute(payment, RoleBuyer)
	if err != nil {
		t.Fatalf("RegisterDispute() error = %v", err)
	}

	// Signature too short
	evidence := &Evidence{
		Type:            EvidenceText,
		SubmittedBy:     RoleBuyer,
		Content:         "Payment not received",
		Description:     "Buyer claims non-delivery",
		Signature:       []byte("short"),
		SubmitterPubKey: []byte("buyer-public-key"),
	}

	err = arbiter.SubmitEvidence("test-payment-badsig", evidence)
	if err == nil {
		t.Error("SubmitEvidence() with short signature should fail")
	}
	if !strings.Contains(err.Error(), "signature too short") {
		t.Errorf("SubmitEvidence() error = %v, want signature too short error", err)
	}
}

func TestSubmitEvidence_MissingSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:          "test-payment-nosig",
		EscrowState: EscrowDisputed,
	}

	err := arbiter.RegisterDispute(payment, RoleBuyer)
	if err != nil {
		t.Fatalf("RegisterDispute() error = %v", err)
	}

	// No signature - should be allowed for backward compatibility
	evidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     "Payment not received",
		Description: "Buyer claims non-delivery",
	}

	err = arbiter.SubmitEvidence("test-payment-nosig", evidence)
	if err != nil {
		t.Errorf("SubmitEvidence() without signature error = %v, should allow for backward compatibility", err)
	}
}

func TestResolveDispute_WithSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:          "test-resolution-sig",
		EscrowState: EscrowDisputed,
	}

	err := arbiter.RegisterDispute(payment, RoleBuyer)
	if err != nil {
		t.Fatalf("RegisterDispute() error = %v", err)
	}

	resolution := &Resolution{
		Decision:      RoleBuyer,
		Reason:        "Seller failed to deliver",
		ArbiterID:     "arbiter-1",
		Signature:     []byte("valid-arbiter-signature-12345678"),
		ArbiterPubKey: []byte("arbiter-public-key"),
	}

	err = arbiter.ResolveDispute("test-resolution-sig", resolution)
	if err != nil {
		t.Errorf("ResolveDispute() with valid signature error = %v", err)
	}
}

func TestResolveDispute_InvalidSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:          "test-resolution-badsig",
		EscrowState: EscrowDisputed,
	}

	err := arbiter.RegisterDispute(payment, RoleBuyer)
	if err != nil {
		t.Fatalf("RegisterDispute() error = %v", err)
	}

	// Signature too short
	resolution := &Resolution{
		Decision:      RoleBuyer,
		Reason:        "Seller failed to deliver",
		ArbiterID:     "arbiter-1",
		Signature:     []byte("bad"),
		ArbiterPubKey: []byte("arbiter-public-key"),
	}

	err = arbiter.ResolveDispute("test-resolution-badsig", resolution)
	if err == nil {
		t.Error("ResolveDispute() with short signature should fail")
	}
	if !strings.Contains(err.Error(), "signature too short") {
		t.Errorf("ResolveDispute() error = %v, want signature too short error", err)
	}
}

func TestResolveDispute_MissingSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:          "test-resolution-nosig",
		EscrowState: EscrowDisputed,
	}

	err := arbiter.RegisterDispute(payment, RoleBuyer)
	if err != nil {
		t.Fatalf("RegisterDispute() error = %v", err)
	}

	// No signature - should be allowed for backward compatibility
	resolution := &Resolution{
		Decision:  RoleBuyer,
		Reason:    "Seller failed to deliver",
		ArbiterID: "arbiter-1",
	}

	err = arbiter.ResolveDispute("test-resolution-nosig", resolution)
	if err != nil {
		t.Errorf("ResolveDispute() without signature error = %v, should allow for backward compatibility", err)
	}
}

func TestValidateEvidenceSignature_NilEvidence(t *testing.T) {
	err := validateEvidenceSignature(nil)
	if err == nil {
		t.Error("validateEvidenceSignature(nil) should fail")
	}
}

func TestValidateResolutionSignature_NilResolution(t *testing.T) {
	err := validateResolutionSignature(nil)
	if err == nil {
		t.Error("validateResolutionSignature(nil) should fail")
	}
}

func TestSignEvidence(t *testing.T) {
	evidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     "Test content",
	}

	err := SignEvidence(evidence, []byte("private-key"))
	if err != nil {
		t.Errorf("SignEvidence() error = %v", err)
	}

	if len(evidence.Signature) == 0 {
		t.Error("SignEvidence() did not set signature")
	}
}

func TestSignResolution(t *testing.T) {
	resolution := &Resolution{
		Decision:  RoleBuyer,
		Reason:    "Test reason",
		ArbiterID: "arbiter-1",
	}

	err := SignResolution(resolution, []byte("arbiter-private-key"))
	if err != nil {
		t.Errorf("SignResolution() error = %v", err)
	}

	if len(resolution.Signature) == 0 {
		t.Error("SignResolution() did not set signature")
	}
}
