package paywall

import (
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
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

func TestSignEvidence(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	tests := []struct {
		name     string
		evidence *Evidence
		privKey  *btcec.PrivateKey
		wantErr  bool
	}{
		{
			name:     "nil evidence",
			evidence: nil,
			privKey:  privKey,
			wantErr:  true,
		},
		{
			name: "nil private key",
			evidence: &Evidence{
				PaymentID:   "test-payment-1",
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "test content",
				Description: "test description",
			},
			privKey: nil,
			wantErr: true,
		},
		{
			name: "valid evidence and key",
			evidence: &Evidence{
				PaymentID:   "test-payment-1",
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "test content",
				Description: "test description",
			},
			privKey: privKey,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SignEvidence(tt.evidence, tt.privKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("SignEvidence() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(tt.evidence.Signature) == 0 {
					t.Error("SignEvidence() did not set signature")
				}
				if len(tt.evidence.PublicKey) == 0 {
					t.Error("SignEvidence() did not set public key")
				}
			}
		})
	}
}

func TestVerifyEvidenceSignature(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create valid signed evidence
	validEvidence := &Evidence{
		PaymentID:   "test-payment-1",
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     "test content",
		Description: "test description",
	}
	if err := SignEvidence(validEvidence, privKey); err != nil {
		t.Fatalf("Failed to sign evidence: %v", err)
	}

	tests := []struct {
		name     string
		evidence *Evidence
		wantOk   bool
		wantErr  bool
	}{
		{
			name:     "nil evidence",
			evidence: nil,
			wantOk:   false,
			wantErr:  true,
		},
		{
			name: "evidence without signature",
			evidence: &Evidence{
				PaymentID:   "test-payment-1",
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "test content",
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "evidence without public key",
			evidence: &Evidence{
				PaymentID:   "test-payment-1",
				Type:        EvidenceText,
				SubmittedBy: RoleBuyer,
				Content:     "test content",
				Signature:   []byte("fake signature"),
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name:     "valid signed evidence",
			evidence: validEvidence,
			wantOk:   true,
			wantErr:  false,
		},
		{
			name: "tampered evidence content",
			evidence: func() *Evidence {
				e := &Evidence{
					PaymentID:   validEvidence.PaymentID,
					Type:        validEvidence.Type,
					SubmittedBy: validEvidence.SubmittedBy,
					Content:     "tampered content", // Modified!
					Description: validEvidence.Description,
					Signature:   validEvidence.Signature,
					PublicKey:   validEvidence.PublicKey,
				}
				return e
			}(),
			wantOk:  false,
			wantErr: false,
		},
		{
			name: "tampered evidence description",
			evidence: func() *Evidence {
				e := &Evidence{
					PaymentID:   validEvidence.PaymentID,
					Type:        validEvidence.Type,
					SubmittedBy: validEvidence.SubmittedBy,
					Content:     validEvidence.Content,
					Description: "tampered description", // Modified!
					Signature:   validEvidence.Signature,
					PublicKey:   validEvidence.PublicKey,
				}
				return e
			}(),
			wantOk:  false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyEvidenceSignature(tt.evidence)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyEvidenceSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOk {
				t.Errorf("VerifyEvidenceSignature() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestSignResolution(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	tests := []struct {
		name       string
		resolution *Resolution
		privKey    *btcec.PrivateKey
		wantErr    bool
	}{
		{
			name:       "nil resolution",
			resolution: nil,
			privKey:    privKey,
			wantErr:    true,
		},
		{
			name: "nil private key",
			resolution: &Resolution{
				PaymentID: "test-payment-1",
				Decision:  RoleBuyer,
				Reason:    "test reason",
				ArbiterID: "arbiter-1",
			},
			privKey: nil,
			wantErr: true,
		},
		{
			name: "valid resolution and key",
			resolution: &Resolution{
				PaymentID: "test-payment-1",
				Decision:  RoleBuyer,
				Reason:    "test reason",
				ArbiterID: "arbiter-1",
				Evidence:  []string{"evidence-1", "evidence-2"},
			},
			privKey: privKey,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SignResolution(tt.resolution, tt.privKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("SignResolution() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if len(tt.resolution.Signature) == 0 {
					t.Error("SignResolution() did not set signature")
				}
				if len(tt.resolution.PublicKey) == 0 {
					t.Error("SignResolution() did not set public key")
				}
			}
		})
	}
}

func TestVerifyResolutionSignature(t *testing.T) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create valid signed resolution
	validResolution := &Resolution{
		PaymentID: "test-payment-1",
		Decision:  RoleBuyer,
		Reason:    "test reason",
		ArbiterID: "arbiter-1",
		Evidence:  []string{"evidence-1", "evidence-2"},
	}
	if err := SignResolution(validResolution, privKey); err != nil {
		t.Fatalf("Failed to sign resolution: %v", err)
	}

	tests := []struct {
		name       string
		resolution *Resolution
		wantOk     bool
		wantErr    bool
	}{
		{
			name:       "nil resolution",
			resolution: nil,
			wantOk:     false,
			wantErr:    true,
		},
		{
			name: "resolution without signature",
			resolution: &Resolution{
				PaymentID: "test-payment-1",
				Decision:  RoleBuyer,
				Reason:    "test reason",
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "resolution without public key",
			resolution: &Resolution{
				PaymentID: "test-payment-1",
				Decision:  RoleBuyer,
				Reason:    "test reason",
				Signature: []byte("fake signature"),
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name:       "valid signed resolution",
			resolution: validResolution,
			wantOk:     true,
			wantErr:    false,
		},
		{
			name: "tampered resolution reason",
			resolution: func() *Resolution {
				r := &Resolution{
					PaymentID: validResolution.PaymentID,
					Decision:  validResolution.Decision,
					Reason:    "tampered reason", // Modified!
					ArbiterID: validResolution.ArbiterID,
					Evidence:  validResolution.Evidence,
					Signature: validResolution.Signature,
					PublicKey: validResolution.PublicKey,
				}
				return r
			}(),
			wantOk:  false,
			wantErr: false,
		},
		{
			name: "tampered resolution decision",
			resolution: func() *Resolution {
				r := &Resolution{
					PaymentID: validResolution.PaymentID,
					Decision:  RoleSeller, // Modified!
					Reason:    validResolution.Reason,
					ArbiterID: validResolution.ArbiterID,
					Evidence:  validResolution.Evidence,
					Signature: validResolution.Signature,
					PublicKey: validResolution.PublicKey,
				}
				return r
			}(),
			wantOk:  false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyResolutionSignature(tt.resolution)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyResolutionSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOk {
				t.Errorf("VerifyResolutionSignature() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestLocalArbiter_SubmitEvidence_WithSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "test",
	}
	arbiter.RegisterDispute(payment, RoleBuyer)

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Test with valid signature
	validEvidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     "test content",
		Description: "test description",
	}
	if err := SignEvidence(validEvidence, privKey); err != nil {
		t.Fatalf("Failed to sign evidence: %v", err)
	}

	err = arbiter.SubmitEvidence(payment.ID, validEvidence)
	if err != nil {
		t.Errorf("SubmitEvidence() with valid signature error = %v", err)
	}

	// Test with invalid signature (tampered data)
	tamperedEvidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     "tampered content", // Different from signed content
		Description: "test description",
		Signature:   validEvidence.Signature, // Reusing old signature
		PublicKey:   validEvidence.PublicKey,
	}

	err = arbiter.SubmitEvidence(payment.ID, tamperedEvidence)
	if err == nil {
		t.Error("SubmitEvidence() with tampered evidence should have failed")
	}
	if err != nil && err.Error() != "invalid evidence signature" {
		t.Errorf("SubmitEvidence() expected 'invalid evidence signature' error, got: %v", err)
	}
}

func TestLocalArbiter_ResolveDispute_WithSignature(t *testing.T) {
	arbiter := NewLocalArbiter()
	payment := &Payment{
		ID:            "test-payment-1",
		DisputeReason: "test",
	}
	arbiter.RegisterDispute(payment, RoleBuyer)

	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Test with valid signature
	validResolution := &Resolution{
		Decision:  RoleBuyer,
		Reason:    "test reason",
		ArbiterID: "arbiter-1",
		Evidence:  []string{"evidence-1"},
	}
	if err := SignResolution(validResolution, privKey); err != nil {
		t.Fatalf("Failed to sign resolution: %v", err)
	}

	err = arbiter.ResolveDispute(payment.ID, validResolution)
	if err != nil {
		t.Errorf("ResolveDispute() with valid signature error = %v", err)
	}

	// Setup another dispute for tampered test
	payment2 := &Payment{
		ID:            "test-payment-2",
		DisputeReason: "test2",
	}
	arbiter.RegisterDispute(payment2, RoleSeller)

	// Test with invalid signature (tampered data)
	tamperedResolution := &Resolution{
		Decision:  RoleSeller, // Different decision!
		Reason:    "test reason",
		ArbiterID: "arbiter-1",
		Evidence:  []string{"evidence-1"},
		Signature: validResolution.Signature, // Reusing old signature
		PublicKey: validResolution.PublicKey,
	}

	err = arbiter.ResolveDispute(payment2.ID, tamperedResolution)
	if err == nil {
		t.Error("ResolveDispute() with tampered resolution should have failed")
	}
	if err != nil && err.Error() != "invalid resolution signature" {
		t.Errorf("ResolveDispute() expected 'invalid resolution signature' error, got: %v", err)
	}
}

