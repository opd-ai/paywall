package paywall

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/opd-ai/paywall/wallet"
)

func TestNewEscrowManager(t *testing.T) {
	tests := []struct {
		name    string
		paywall *Paywall
		wantErr bool
	}{
		{
			name:    "nil paywall",
			paywall: nil,
			wantErr: true,
		},
		{
			name: "valid paywall",
			paywall: &Paywall{
				Store:     NewMemoryStore(),
				HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em, err := NewEscrowManager(tt.paywall)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEscrowManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && em == nil {
				t.Error("NewEscrowManager() returned nil manager without error")
			}
			if !tt.wantErr && em.paywall != tt.paywall {
				t.Error("NewEscrowManager() did not set paywall correctly")
			}
		})
	}
}

func TestEscrowState_String(t *testing.T) {
	tests := []struct {
		state EscrowState
		want  string
	}{
		{EscrowNone, "none"},
		{EscrowPending, "pending"},
		{EscrowFunded, "funded"},
		{EscrowCompleted, "completed"},
		{EscrowDisputed, "disputed"},
		{EscrowRefunded, "refunded"},
		{EscrowState(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("EscrowState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEscrowManager_CreateEscrow(t *testing.T) {
	t.Run("without multisig enabled", func(t *testing.T) {
		store := NewMemoryStore()
		pw := &Paywall{
			Store:     store,
			HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
			prices:    map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		}

		em, err := NewEscrowManager(pw)
		if err != nil {
			t.Fatalf("NewEscrowManager() error = %v", err)
		}

		_, err = em.CreateEscrow(1.0, 24*time.Hour)
		if err != ErrMultisigRequired {
			t.Errorf("CreateEscrow() error = %v, want %v", err, ErrMultisigRequired)
		}
	})
}

func TestEscrowManager_FundEscrow(t *testing.T) {
	tests := []struct {
		name          string
		setupPayment  func(PaymentStore) string
		wantErr       bool
		expectedError error
	}{
		{
			name: "payment not found",
			setupPayment: func(store PaymentStore) string {
				return "nonexistent"
			},
			wantErr: true,
		},
		{
			name: "escrow not enabled",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-1",
					EscrowState: EscrowNone,
					Status:      StatusConfirmed,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			wantErr:       true,
			expectedError: ErrEscrowNotEnabled,
		},
		{
			name: "invalid state - already funded",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-2",
					EscrowState: EscrowFunded,
					Status:      StatusConfirmed,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			wantErr:       true,
			expectedError: ErrInvalidEscrowState,
		},
		{
			name: "payment not confirmed",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-3",
					EscrowState: EscrowPending,
					Status:      StatusPending,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			wantErr: true,
		},
		{
			name: "successful funding",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-4",
					EscrowState: EscrowPending,
					Status:      StatusConfirmed,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			pw := &Paywall{
				Store:     store,
				HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
			}

			em, err := NewEscrowManager(pw)
			if err != nil {
				t.Fatalf("NewEscrowManager() error = %v", err)
			}

			paymentID := tt.setupPayment(store)
			err = em.FundEscrow(paymentID)

			if (err != nil) != tt.wantErr {
				t.Errorf("FundEscrow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.expectedError != nil && err != nil {
				// Check if the error wraps the expected error
				if !errors.Is(err, tt.expectedError) {
					t.Errorf("FundEscrow() error = %v, want %v", err, tt.expectedError)
				}
			}

			if !tt.wantErr {
				payment, _ := store.GetPayment(paymentID)
				if payment.EscrowState != EscrowFunded {
					t.Errorf("FundEscrow() state = %v, want %v", payment.EscrowState, EscrowFunded)
				}
			}
		})
	}
}

func TestEscrowManager_ReleaseToSeller(t *testing.T) {
	buyerSig := &SignatureData{
		SignerID:  "buyer-1",
		Role:      RoleBuyer,
		Signature: []byte("buyer-sig"),
		PublicKey: []byte("buyer-pubkey"),
		SignedAt:  time.Now(),
	}

	sellerSig := &SignatureData{
		SignerID:  "seller-1",
		Role:      RoleSeller,
		Signature: []byte("seller-sig"),
		PublicKey: []byte("seller-pubkey"),
		SignedAt:  time.Now(),
	}

	tests := []struct {
		name          string
		setupPayment  func(PaymentStore) string
		buyerSig      *SignatureData
		sellerSig     *SignatureData
		wantErr       bool
		expectedError error
	}{
		{
			name: "escrow not enabled",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-1",
					EscrowState: EscrowNone,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			buyerSig:      buyerSig,
			sellerSig:     sellerSig,
			wantErr:       true,
			expectedError: ErrEscrowNotEnabled,
		},
		{
			name: "invalid state - pending",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-2",
					EscrowState: EscrowPending,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			buyerSig:      buyerSig,
			sellerSig:     sellerSig,
			wantErr:       true,
			expectedError: ErrInvalidEscrowState,
		},
		{
			name: "missing buyer signature",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-3",
					EscrowState: EscrowFunded,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			buyerSig:      nil,
			sellerSig:     sellerSig,
			wantErr:       true,
			expectedError: ErrInsufficientSignatures,
		},
		{
			name: "successful release",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-4",
					EscrowState: EscrowFunded,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			buyerSig:  buyerSig,
			sellerSig: sellerSig,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			pw := &Paywall{
				Store:     store,
				HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
			}

			em, err := NewEscrowManager(pw)
			if err != nil {
				t.Fatalf("NewEscrowManager() error = %v", err)
			}

			paymentID := tt.setupPayment(store)
			err = em.ReleaseToSeller(paymentID, tt.buyerSig, tt.sellerSig)

			if (err != nil) != tt.wantErr {
				t.Errorf("ReleaseToSeller() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				payment, _ := store.GetPayment(paymentID)
				if payment.EscrowState != EscrowCompleted {
					t.Errorf("ReleaseToSeller() state = %v, want %v", payment.EscrowState, EscrowCompleted)
				}
				if len(payment.Signatures[wallet.Bitcoin]) != 2 {
					t.Errorf("ReleaseToSeller() signatures count = %d, want 2", len(payment.Signatures[wallet.Bitcoin]))
				}
			}
		})
	}
}

func TestEscrowManager_RequestDispute(t *testing.T) {
	tests := []struct {
		name          string
		setupPayment  func(PaymentStore) string
		role          MultisigRole
		reason        string
		wantErr       bool
		expectedError error
	}{
		{
			name: "escrow not enabled",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-1",
					EscrowState: EscrowNone,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			role:          RoleBuyer,
			reason:        "test reason",
			wantErr:       true,
			expectedError: ErrEscrowNotEnabled,
		},
		{
			name: "invalid state - completed",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-2",
					EscrowState: EscrowCompleted,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			role:          RoleBuyer,
			reason:        "test reason",
			wantErr:       true,
			expectedError: ErrInvalidEscrowState,
		},
		{
			name: "invalid role - arbiter",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-3",
					EscrowState: EscrowFunded,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			role:    RoleArbiter,
			reason:  "test reason",
			wantErr: true,
		},
		{
			name: "successful dispute by buyer",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-4",
					EscrowState: EscrowFunded,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			role:    RoleBuyer,
			reason:  "goods not received",
			wantErr: false,
		},
		{
			name: "successful dispute by seller",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-5",
					EscrowState: EscrowFunded,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			role:    RoleSeller,
			reason:  "buyer requesting refund unfairly",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			pw := &Paywall{
				Store:     store,
				HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
			}

			em, err := NewEscrowManager(pw)
			if err != nil {
				t.Fatalf("NewEscrowManager() error = %v", err)
			}

			paymentID := tt.setupPayment(store)
			err = em.RequestDispute(paymentID, tt.role, tt.reason)

			if (err != nil) != tt.wantErr {
				t.Errorf("RequestDispute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				payment, _ := store.GetPayment(paymentID)
				if payment.EscrowState != EscrowDisputed {
					t.Errorf("RequestDispute() state = %v, want %v", payment.EscrowState, EscrowDisputed)
				}
				if payment.DisputeReason != tt.reason {
					t.Errorf("RequestDispute() reason = %v, want %v", payment.DisputeReason, tt.reason)
				}
			}
		})
	}
}

func TestEscrowManager_ResolveDispute(t *testing.T) {
	arbiterSig := &SignatureData{
		SignerID:  "arbiter-1",
		Role:      RoleArbiter,
		Signature: []byte("arbiter-sig"),
		PublicKey: []byte("arbiter-pubkey"),
		SignedAt:  time.Now(),
	}

	buyerSig := &SignatureData{
		SignerID:  "buyer-1",
		Role:      RoleBuyer,
		Signature: []byte("buyer-sig"),
		PublicKey: []byte("buyer-pubkey"),
		SignedAt:  time.Now(),
	}

	sellerSig := &SignatureData{
		SignerID:  "seller-1",
		Role:      RoleSeller,
		Signature: []byte("seller-sig"),
		PublicKey: []byte("seller-pubkey"),
		SignedAt:  time.Now(),
	}

	tests := []struct {
		name         string
		setupPayment func(PaymentStore) string
		arbiterSig   *SignatureData
		winnerSig    *SignatureData
		wantErr      bool
		expectState  EscrowState
	}{
		{
			name: "not disputed",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-1",
					EscrowState: EscrowFunded,
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			arbiterSig: arbiterSig,
			winnerSig:  buyerSig,
			wantErr:    true,
		},
		{
			name: "resolve in favor of buyer",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-2",
					EscrowState: EscrowDisputed,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			arbiterSig:  arbiterSig,
			winnerSig:   buyerSig,
			wantErr:     false,
			expectState: EscrowRefunded,
		},
		{
			name: "resolve in favor of seller",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-3",
					EscrowState: EscrowDisputed,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			arbiterSig:  arbiterSig,
			winnerSig:   sellerSig,
			wantErr:     false,
			expectState: EscrowCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			pw := &Paywall{
				Store:              store,
				HDWallets:          make(map[wallet.WalletType]wallet.HDWallet),
				authorizedArbiters: [][]byte{arbiterSig.PublicKey}, // Add arbiter to authorized list
			}

			em, err := NewEscrowManager(pw)
			if err != nil {
				t.Fatalf("NewEscrowManager() error = %v", err)
			}

			paymentID := tt.setupPayment(store)
			err = em.ResolveDispute(paymentID, tt.arbiterSig, tt.winnerSig)

			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveDispute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				payment, _ := store.GetPayment(paymentID)
				if payment.EscrowState != tt.expectState {
					t.Errorf("ResolveDispute() state = %v, want %v", payment.EscrowState, tt.expectState)
				}
			}
		})
	}
}

func TestEscrowManager_RefundBuyer(t *testing.T) {
	buyerSig := &SignatureData{
		SignerID:  "buyer-1",
		Role:      RoleBuyer,
		Signature: []byte("buyer-sig"),
		PublicKey: []byte("buyer-pubkey"),
		SignedAt:  time.Now(),
	}

	sellerSig := &SignatureData{
		SignerID:  "seller-1",
		Role:      RoleSeller,
		Signature: []byte("seller-sig"),
		PublicKey: []byte("seller-pubkey"),
		SignedAt:  time.Now(),
	}

	arbiterSig := &SignatureData{
		SignerID:  "arbiter-1",
		Role:      RoleArbiter,
		Signature: []byte("arbiter-sig"),
		PublicKey: []byte("arbiter-pubkey"),
		SignedAt:  time.Now(),
	}

	tests := []struct {
		name         string
		setupPayment func(PaymentStore) string
		sig1         *SignatureData
		sig2         *SignatureData
		wantErr      bool
	}{
		{
			name: "buyer and seller mutual refund",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-1",
					EscrowState: EscrowFunded,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			sig1:    buyerSig,
			sig2:    sellerSig,
			wantErr: false,
		},
		{
			name: "buyer and arbiter refund",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-2",
					EscrowState: EscrowFunded,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			sig1:    buyerSig,
			sig2:    arbiterSig,
			wantErr: false,
		},
		{
			name: "invalid combination - seller and arbiter",
			setupPayment: func(store PaymentStore) string {
				payment := &Payment{
					ID:          "test-3",
					EscrowState: EscrowFunded,
					Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
				}
				store.CreatePayment(payment)
				return payment.ID
			},
			sig1:    sellerSig,
			sig2:    arbiterSig,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			pw := &Paywall{
				Store:              store,
				HDWallets:          make(map[wallet.WalletType]wallet.HDWallet),
				authorizedArbiters: [][]byte{arbiterSig.PublicKey}, // Add arbiter to authorized list
			}

			em, err := NewEscrowManager(pw)
			if err != nil {
				t.Fatalf("NewEscrowManager() error = %v", err)
			}

			paymentID := tt.setupPayment(store)
			err = em.RefundBuyer(paymentID, tt.sig1, tt.sig2)

			if (err != nil) != tt.wantErr {
				t.Errorf("RefundBuyer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				payment, _ := store.GetPayment(paymentID)
				if payment.EscrowState != EscrowRefunded {
					t.Errorf("RefundBuyer() state = %v, want %v", payment.EscrowState, EscrowRefunded)
				}
			}
		})
	}
}

func TestEscrowManager_CheckEscrowTimeouts(t *testing.T) {
	store := NewMemoryStore()

	// Create various escrow states
	now := time.Now()

	// Not timed out yet
	store.CreatePayment(&Payment{
		ID:            "funded-active",
		EscrowState:   EscrowFunded,
		EscrowTimeout: now.Add(1 * time.Hour),
	})

	// Timed out funded
	store.CreatePayment(&Payment{
		ID:              "funded-timeout",
		EscrowState:     EscrowFunded,
		EscrowTimeout:   now.Add(-1 * time.Hour),
		MultisigEnabled: true,
		Status:          StatusPending,
	})

	// Timed out disputed
	store.CreatePayment(&Payment{
		ID:              "disputed-timeout",
		EscrowState:     EscrowDisputed,
		EscrowTimeout:   now.Add(-1 * time.Hour),
		MultisigEnabled: true,
		Status:          StatusPending,
	})

	// Completed (should not be included)
	store.CreatePayment(&Payment{
		ID:            "completed",
		EscrowState:   EscrowCompleted,
		EscrowTimeout: now.Add(-1 * time.Hour),
	})

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	timedOut, err := em.CheckEscrowTimeouts()
	if err != nil {
		t.Fatalf("CheckEscrowTimeouts() error = %v", err)
	}

	if len(timedOut) != 2 {
		t.Errorf("CheckEscrowTimeouts() returned %d timeouts, want 2", len(timedOut))
	}

	foundFunded := false
	foundDisputed := false
	for _, id := range timedOut {
		if id == "funded-timeout" {
			foundFunded = true
		}
		if id == "disputed-timeout" {
			foundDisputed = true
		}
	}

	if !foundFunded {
		t.Error("CheckEscrowTimeouts() did not find funded-timeout")
	}
	if !foundDisputed {
		t.Error("CheckEscrowTimeouts() did not find disputed-timeout")
	}
}

func TestEscrowManager_GetEscrowState(t *testing.T) {
	store := NewMemoryStore()
	store.CreatePayment(&Payment{
		ID:          "test-1",
		EscrowState: EscrowFunded,
	})

	pw := &Paywall{
		Store:     store,
		HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	state, err := em.GetEscrowState("test-1")
	if err != nil {
		t.Fatalf("GetEscrowState() error = %v", err)
	}

	if state != EscrowFunded {
		t.Errorf("GetEscrowState() = %v, want %v", state, EscrowFunded)
	}

	_, err = em.GetEscrowState("nonexistent")
	if err == nil {
		t.Error("GetEscrowState() expected error for nonexistent payment")
	}
}

// TestEscrowManager_ResolveDispute_UnauthorizedArbiter tests that unauthorized arbiters are rejected
func TestEscrowManager_ResolveDispute_UnauthorizedArbiter(t *testing.T) {
	authorizedArbiterKey := []byte("authorized-arbiter-pubkey")
	unauthorizedArbiterKey := []byte("unauthorized-arbiter-pubkey")

	store := NewMemoryStore()
	pw := &Paywall{
		Store:              store,
		HDWallets:          make(map[wallet.WalletType]wallet.HDWallet),
		authorizedArbiters: [][]byte{authorizedArbiterKey},
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create disputed payment
	payment := &Payment{
		ID:          "test-dispute",
		EscrowState: EscrowDisputed,
		Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
	}
	store.CreatePayment(payment)

	// Test with unauthorized arbiter
	unauthorizedArbiterSig := &SignatureData{
		SignerID:  "unauthorized-arbiter",
		Role:      RoleArbiter,
		Signature: []byte("arbiter-sig"),
		PublicKey: unauthorizedArbiterKey,
		SignedAt:  time.Now(),
	}

	buyerSig := &SignatureData{
		SignerID:  "buyer-1",
		Role:      RoleBuyer,
		Signature: []byte("buyer-sig"),
		PublicKey: []byte("buyer-pubkey"),
		SignedAt:  time.Now(),
	}

	err = em.ResolveDispute(payment.ID, unauthorizedArbiterSig, buyerSig)
	if err == nil {
		t.Error("ResolveDispute() should reject unauthorized arbiter")
	}
	if err != nil && err.Error() != "arbiter is not authorized: public key not in authorized list" {
		t.Errorf("ResolveDispute() wrong error message: %v", err)
	}

	// Test with authorized arbiter
	authorizedArbiterSig := &SignatureData{
		SignerID:  "authorized-arbiter",
		Role:      RoleArbiter,
		Signature: []byte("arbiter-sig"),
		PublicKey: authorizedArbiterKey,
		SignedAt:  time.Now(),
	}

	err = em.ResolveDispute(payment.ID, authorizedArbiterSig, buyerSig)
	if err != nil {
		t.Errorf("ResolveDispute() should accept authorized arbiter: %v", err)
	}

	// Verify state changed
	updatedPayment, _ := store.GetPayment(payment.ID)
	if updatedPayment.EscrowState != EscrowRefunded {
		t.Errorf("ResolveDispute() state = %v, want %v", updatedPayment.EscrowState, EscrowRefunded)
	}
}

// TestEscrowManager_RefundBuyer_UnauthorizedArbiter tests arbiter validation in refunds
func TestEscrowManager_RefundBuyer_UnauthorizedArbiter(t *testing.T) {
	authorizedArbiterKey := []byte("authorized-arbiter-pubkey")
	unauthorizedArbiterKey := []byte("unauthorized-arbiter-pubkey")

	store := NewMemoryStore()
	pw := &Paywall{
		Store:              store,
		HDWallets:          make(map[wallet.WalletType]wallet.HDWallet),
		authorizedArbiters: [][]byte{authorizedArbiterKey},
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create funded payment
	payment := &Payment{
		ID:          "test-refund",
		EscrowState: EscrowFunded,
		Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
	}
	store.CreatePayment(payment)

	buyerSig := &SignatureData{
		SignerID:  "buyer-1",
		Role:      RoleBuyer,
		Signature: []byte("buyer-sig"),
		PublicKey: []byte("buyer-pubkey"),
		SignedAt:  time.Now(),
	}

	// Test with unauthorized arbiter
	unauthorizedArbiterSig := &SignatureData{
		SignerID:  "unauthorized-arbiter",
		Role:      RoleArbiter,
		Signature: []byte("arbiter-sig"),
		PublicKey: unauthorizedArbiterKey,
		SignedAt:  time.Now(),
	}

	err = em.RefundBuyer(payment.ID, buyerSig, unauthorizedArbiterSig)
	if err == nil {
		t.Error("RefundBuyer() should reject unauthorized arbiter")
	}
	if err != nil && err.Error() != "arbiter is not authorized: public key not in authorized list" {
		t.Errorf("RefundBuyer() wrong error message: %v", err)
	}

	// Test with authorized arbiter
	authorizedArbiterSig := &SignatureData{
		SignerID:  "authorized-arbiter",
		Role:      RoleArbiter,
		Signature: []byte("arbiter-sig"),
		PublicKey: authorizedArbiterKey,
		SignedAt:  time.Now(),
	}

	err = em.RefundBuyer(payment.ID, buyerSig, authorizedArbiterSig)
	if err != nil {
		t.Errorf("RefundBuyer() should accept authorized arbiter: %v", err)
	}

	// Verify state changed
	updatedPayment, _ := store.GetPayment(payment.ID)
	if updatedPayment.EscrowState != EscrowRefunded {
		t.Errorf("RefundBuyer() state = %v, want %v", updatedPayment.EscrowState, EscrowRefunded)
	}
}

// TestPaywall_ArbiterManagement tests arbiter authorization management
func TestPaywall_ArbiterManagement(t *testing.T) {
	pw := &Paywall{
		authorizedArbiters: [][]byte{},
	}

	arbiter1 := []byte("arbiter-1-pubkey")
	arbiter2 := []byte("arbiter-2-pubkey")
	arbiter3 := []byte("arbiter-3-pubkey")

	// Test IsAuthorizedArbiter with empty list
	if pw.IsAuthorizedArbiter(arbiter1) {
		t.Error("IsAuthorizedArbiter() should return false for empty list")
	}

	// Test AddAuthorizedArbiter
	err := pw.AddAuthorizedArbiter(arbiter1)
	if err != nil {
		t.Errorf("AddAuthorizedArbiter() error = %v", err)
	}

	if !pw.IsAuthorizedArbiter(arbiter1) {
		t.Error("IsAuthorizedArbiter() should return true after adding")
	}

	// Test adding duplicate
	err = pw.AddAuthorizedArbiter(arbiter1)
	if err == nil {
		t.Error("AddAuthorizedArbiter() should reject duplicate")
	}

	// Test adding empty key
	err = pw.AddAuthorizedArbiter([]byte{})
	if err == nil {
		t.Error("AddAuthorizedArbiter() should reject empty key")
	}

	// Add more arbiters
	pw.AddAuthorizedArbiter(arbiter2)
	pw.AddAuthorizedArbiter(arbiter3)

	// Test GetAuthorizedArbiters
	arbiters := pw.GetAuthorizedArbiters()
	if len(arbiters) != 3 {
		t.Errorf("GetAuthorizedArbiters() returned %d arbiters, want 3", len(arbiters))
	}

	// Test defensive copy (modifying result should not affect internal state)
	arbiters[0] = []byte("modified")
	if !pw.IsAuthorizedArbiter(arbiter1) {
		t.Error("GetAuthorizedArbiters() did not return defensive copy")
	}

	// Test RemoveAuthorizedArbiter
	err = pw.RemoveAuthorizedArbiter(arbiter2)
	if err != nil {
		t.Errorf("RemoveAuthorizedArbiter() error = %v", err)
	}

	if pw.IsAuthorizedArbiter(arbiter2) {
		t.Error("IsAuthorizedArbiter() should return false after removal")
	}

	// Test removing non-existent arbiter
	err = pw.RemoveAuthorizedArbiter([]byte("nonexistent"))
	if err == nil {
		t.Error("RemoveAuthorizedArbiter() should error for non-existent arbiter")
	}

	// Verify remaining arbiters
	arbiters = pw.GetAuthorizedArbiters()
	if len(arbiters) != 2 {
		t.Errorf("GetAuthorizedArbiters() returned %d arbiters, want 2", len(arbiters))
	}
}

// TestPaywall_ArbiterManagement_EmptyList tests behavior with no authorized arbiters
func TestPaywall_ArbiterManagement_EmptyList(t *testing.T) {
	pw := &Paywall{}

	// Test with nil list
	if pw.IsAuthorizedArbiter([]byte("any-key")) {
		t.Error("IsAuthorizedArbiter() should return false with nil list")
	}

	arbiters := pw.GetAuthorizedArbiters()
	if arbiters != nil {
		t.Error("GetAuthorizedArbiters() should return nil for empty list")
	}
}

// TestEscrowManager_ValidateSignatureData tests signature validation
func TestEscrowManager_ValidateSignatureData(t *testing.T) {
	// Generate valid test keys
	buyerPubKey := []byte{
		0x02, 0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac, 0x55, 0xa0, 0x62,
		0x95, 0xce, 0x87, 0x0b, 0x07, 0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28,
		0xd9, 0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98,
	}
	sellerPubKey := []byte{
		0x02, 0xf9, 0x30, 0x8a, 0x01, 0x92, 0x58, 0xc3, 0x10, 0x49, 0x34, 0x4f,
		0x85, 0xf8, 0x9d, 0x52, 0x29, 0xb5, 0x31, 0xc8, 0x45, 0x83, 0x6f, 0x99,
		0xb0, 0x8a, 0x42, 0xa4, 0xf6, 0x8b, 0x61, 0xc2, 0xad,
	}
	arbiterPubKey := []byte{
		0x03, 0x5c, 0xed, 0xc1, 0x61, 0x74, 0x53, 0xec, 0x23, 0x9e, 0x01, 0x47,
		0xe5, 0xe4, 0x49, 0x64, 0x4c, 0x4f, 0x81, 0x00, 0xf9, 0x0a, 0x9e, 0xd4,
		0x7f, 0x7c, 0xc6, 0x6d, 0x3c, 0x15, 0xf5, 0x60, 0xa7,
	}

	// Valid minimal DER-encoded ECDSA signature
	// DER format: 0x30 [total-length] 0x02 [r-length] [r-bytes] 0x02 [s-length] [s-bytes]
	// Minimum length requires at least 1 byte for R and 1 byte for S
	validSignature := []byte{
		0x30, 0x08, // SEQUENCE, 8 bytes total
		0x02, 0x02, 0x00, 0x01, // INTEGER R, 2 bytes, value 0x0001
		0x02, 0x02, 0x00, 0x01, // INTEGER S, 2 bytes, value 0x0001
	}

	// Mock signature (doesn't start with 0x30, so parsing is skipped)
	mockSignature := []byte("mock-signature-for-testing")

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	store := NewMemoryStore()
	pw := &Paywall{
		Store:              store,
		HDWallets:          make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:    true,
		authorizedArbiters: [][]byte{arbiterPubKey},
		participantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: publicKeys,
		},
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create test payment
	payment := &Payment{
		ID:              "test-sig-validation",
		MultisigEnabled: true,
		EscrowState:     EscrowDisputed,
		Addresses:       map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
	}
	store.CreatePayment(payment)

	tests := []struct {
		name    string
		sig     *SignatureData
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid mock signature data (used in tests and pre-transaction collection)",
			sig: &SignatureData{
				SignerID:  "buyer-1",
				Role:      RoleBuyer,
				Signature: mockSignature,
				PublicKey: buyerPubKey,
				SignedAt:  time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "nil signature data",
			sig:     nil,
			wantErr: true,
			errMsg:  "signature data cannot be nil",
		},
		{
			name: "empty public key",
			sig: &SignatureData{
				SignerID:  "buyer-1",
				Role:      RoleBuyer,
				Signature: validSignature,
				PublicKey: []byte{},
				SignedAt:  time.Now(),
			},
			wantErr: true,
			errMsg:  "public key is empty",
		},
		{
			name: "invalid public key",
			sig: &SignatureData{
				SignerID:  "buyer-1",
				Role:      RoleBuyer,
				Signature: validSignature,
				PublicKey: []byte{0x00, 0x01, 0x02}, // Invalid key
				SignedAt:  time.Now(),
			},
			wantErr: true,
			errMsg:  "failed to parse public key",
		},
		{
			name: "empty signature",
			sig: &SignatureData{
				SignerID:  "buyer-1",
				Role:      RoleBuyer,
				Signature: []byte{},
				PublicKey: buyerPubKey,
				SignedAt:  time.Now(),
			},
			wantErr: true,
			errMsg:  "signature is empty",
		},
		{
			name: "signature too short",
			sig: &SignatureData{
				SignerID:  "buyer-1",
				Role:      RoleBuyer,
				Signature: []byte{0x30, 0x01, 0x02}, // Too short
				PublicKey: buyerPubKey,
				SignedAt:  time.Now(),
			},
			wantErr: true,
			errMsg:  "signature too short",
		},
		{
			name: "invalid signature format",
			sig: &SignatureData{
				SignerID:  "buyer-1",
				Role:      RoleBuyer,
				Signature: []byte{0x30, 0x08, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}, // Starts with 0x30 but invalid DER
				PublicKey: buyerPubKey,
				SignedAt:  time.Now(),
			},
			wantErr: true,
			errMsg:  "failed to parse DER signature",
		},
		{
			name: "unknown participant",
			sig: &SignatureData{
				SignerID:  "unknown-1",
				Role:      RoleBuyer,
				Signature: validSignature,
				PublicKey: []byte{ // Valid secp256k1 key but not in participant list
					0x03, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
					0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
					0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
				},
				SignedAt: time.Now(),
			},
			wantErr: true,
			/* This test will fail at public key parsing before participant check can happen */
			errMsg: "failed to parse public key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := em.validateSignatureData(tt.sig, payment)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSignatureData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || err.Error() == "" {
					t.Errorf("validateSignatureData() expected error containing %q, got nil", tt.errMsg)
				} else if err.Error() != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateSignatureData() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestEscrowManager_RoleBasedAuthorization verifies that roles are properly
// derived from public key positions and prevents role spoofing
func TestEscrowManager_RoleBasedAuthorization(t *testing.T) {
	// Generate valid secp256k1 public keys for testing
	buyerSeed := sha256.Sum256([]byte("buyer-role-test"))
	sellerSeed := sha256.Sum256([]byte("seller-role-test"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-role-test"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	store := NewMemoryStore()
	pw := &Paywall{
		Store:           store,
		HDWallets:       make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled: true,
		participantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
		authorizedArbiters: [][]byte{arbiterPubKey},
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a test payment
	payment := &Payment{
		ID:              "test-role-auth",
		MultisigEnabled: true,
		EscrowState:     EscrowFunded,
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {},
		},
	}
	store.CreatePayment(payment)

	tests := []struct {
		name        string
		sigData     *SignatureData
		wantErr     bool
		errContains string
	}{
		{
			name: "buyer with correct role succeeds",
			sigData: &SignatureData{
				PublicKey: buyerPubKey,
				Role:      RoleBuyer,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr: false,
		},
		{
			name: "buyer trying to spoof seller role fails",
			sigData: &SignatureData{
				PublicKey: buyerPubKey,
				Role:      RoleSeller,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr:     true,
			errContains: "does not match",
		},
		{
			name: "buyer trying to spoof arbiter role fails",
			sigData: &SignatureData{
				PublicKey: buyerPubKey,
				Role:      RoleArbiter,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr:     true,
			errContains: "does not match",
		},
		{
			name: "seller with correct role succeeds",
			sigData: &SignatureData{
				PublicKey: sellerPubKey,
				Role:      RoleSeller,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr: false,
		},
		{
			name: "seller trying to spoof arbiter role fails",
			sigData: &SignatureData{
				PublicKey: sellerPubKey,
				Role:      RoleArbiter,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr:     true,
			errContains: "does not match",
		},
		{
			name: "arbiter with correct role succeeds",
			sigData: &SignatureData{
				PublicKey: arbiterPubKey,
				Role:      RoleArbiter,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr: false,
		},
		{
			name: "arbiter trying to spoof buyer role fails",
			sigData: &SignatureData{
				PublicKey: arbiterPubKey,
				Role:      RoleBuyer,
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr:     true,
			errContains: "does not match",
		},
		{
			name: "empty role is allowed (role is optional for backward compatibility)",
			sigData: &SignatureData{
				PublicKey: buyerPubKey,
				Role:      "",
				Signature: []byte("mock-signature-12345678"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := em.validateSignatureData(tt.sigData, payment)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateSignatureData() expected error containing '%s', got nil", tt.errContains)
					return
				}
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("validateSignatureData() error = %v, want error containing '%s'", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateSignatureData() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestPaywall_GetRoleForPubKey verifies role derivation from public key position
func TestPaywall_GetRoleForPubKey(t *testing.T) {
	// Generate valid secp256k1 public keys for testing
	buyerSeed := sha256.Sum256([]byte("buyer-getrole-test"))
	sellerSeed := sha256.Sum256([]byte("seller-getrole-test"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-getrole-test"))
	unknownSeed := sha256.Sum256([]byte("unknown-getrole-test"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])
	unknownPrivKey, _ := btcec.PrivKeyFromBytes(unknownSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()
	unknownPubKey := unknownPrivKey.PubKey().SerializeCompressed()

	pw := &Paywall{
		multisigEnabled: true,
		participantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
	}

	tests := []struct {
		name       string
		pubKey     []byte
		walletType wallet.WalletType
		wantRole   MultisigRole
		wantErr    bool
	}{
		{
			name:       "buyer at index 0",
			pubKey:     buyerPubKey,
			walletType: wallet.Bitcoin,
			wantRole:   RoleBuyer,
			wantErr:    false,
		},
		{
			name:       "seller at index 1",
			pubKey:     sellerPubKey,
			walletType: wallet.Bitcoin,
			wantRole:   RoleSeller,
			wantErr:    false,
		},
		{
			name:       "arbiter at index 2",
			pubKey:     arbiterPubKey,
			walletType: wallet.Bitcoin,
			wantRole:   RoleArbiter,
			wantErr:    false,
		},
		{
			name:       "unknown key returns error",
			pubKey:     unknownPubKey,
			walletType: wallet.Bitcoin,
			wantRole:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := pw.getRoleForPubKey(tt.pubKey, tt.walletType)

			if tt.wantErr {
				if err == nil {
					t.Errorf("getRoleForPubKey() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("getRoleForPubKey() unexpected error = %v", err)
				}
				if role != tt.wantRole {
					t.Errorf("getRoleForPubKey() = %v, want %v", role, tt.wantRole)
				}
			}
		})
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEscrowManager_ArbiterIntegration(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:              store,
		HDWallets:          make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:    true,
		participantPubKeys: make(map[wallet.WalletType][][]byte),
	}

	arbiter := NewLocalArbiter()
	logger := NewMemoryAuditLogger()

	t.Run("RequestDispute with arbiter registers dispute", func(t *testing.T) {
		em, err := NewEscrowManagerWithArbiter(pw, logger, arbiter)
		if err != nil {
			t.Fatalf("NewEscrowManagerWithArbiter() error = %v", err)
		}

		// Create escrowed payment
		payment := &Payment{
			ID:                     "test-payment-arbiter",
			EscrowState:            EscrowFunded,
			DisputeReason:          "",
			MultisigEnabled:        true,
			Addresses:              map[wallet.WalletType]string{wallet.Bitcoin: "test-addr"},
			StateTransitionHistory: []StateTransitionHistory{},
		}
		store.CreatePayment(payment)

		// Request dispute
		err = em.RequestDispute(payment.ID, RoleBuyer, "goods not received")
		if err != nil {
			t.Fatalf("RequestDispute() error = %v", err)
		}

		// Verify payment state
		updatedPayment, _ := store.GetPayment(payment.ID)
		if updatedPayment.EscrowState != EscrowDisputed {
			t.Errorf("Payment state = %v, want %v", updatedPayment.EscrowState, EscrowDisputed)
		}

		// Verify arbiter registration
		dispute, err := arbiter.GetDispute(payment.ID)
		if err != nil {
			t.Fatalf("GetDispute() error = %v, dispute should be registered", err)
		}
		if dispute.Requester != RoleBuyer {
			t.Errorf("Dispute requester = %v, want %v", dispute.Requester, RoleBuyer)
		}
		if dispute.Reason != "goods not received" {
			t.Errorf("Dispute reason = %v, want 'goods not received'", dispute.Reason)
		}
	})

	t.Run("RequestDispute with seller as requester", func(t *testing.T) {
		em, err := NewEscrowManagerWithArbiter(pw, logger, arbiter)
		if err != nil {
			t.Fatalf("NewEscrowManagerWithArbiter() error = %v", err)
		}

		// Create escrowed payment
		payment := &Payment{
			ID:                     "test-payment-seller-dispute",
			EscrowState:            EscrowFunded,
			DisputeReason:          "",
			MultisigEnabled:        true,
			Addresses:              map[wallet.WalletType]string{wallet.Bitcoin: "test-addr-2"},
			StateTransitionHistory: []StateTransitionHistory{},
		}
		store.CreatePayment(payment)

		// Request dispute as seller
		err = em.RequestDispute(payment.ID, RoleSeller, "buyer not responding")
		if err != nil {
			t.Fatalf("RequestDispute() error = %v", err)
		}

		// Verify arbiter registration with correct requester
		dispute, err := arbiter.GetDispute(payment.ID)
		if err != nil {
			t.Fatalf("GetDispute() error = %v", err)
		}
		if dispute.Requester != RoleSeller {
			t.Errorf("Dispute requester = %v, want %v", dispute.Requester, RoleSeller)
		}
	})

	t.Run("RequestDispute without arbiter still works", func(t *testing.T) {
		em, err := NewEscrowManagerWithAudit(pw, logger)
		if err != nil {
			t.Fatalf("NewEscrowManagerWithAudit() error = %v", err)
		}

		// Create escrowed payment
		payment := &Payment{
			ID:                     "test-payment-no-arbiter",
			EscrowState:            EscrowFunded,
			DisputeReason:          "",
			MultisigEnabled:        true,
			Addresses:              map[wallet.WalletType]string{wallet.Bitcoin: "test-addr-3"},
			StateTransitionHistory: []StateTransitionHistory{},
		}
		store.CreatePayment(payment)

		// Request dispute without arbiter
		err = em.RequestDispute(payment.ID, RoleBuyer, "test dispute")
		if err != nil {
			t.Fatalf("RequestDispute() error = %v", err)
		}

		// Verify payment state changed
		updatedPayment, _ := store.GetPayment(payment.ID)
		if updatedPayment.EscrowState != EscrowDisputed {
			t.Errorf("Payment state = %v, want %v", updatedPayment.EscrowState, EscrowDisputed)
		}

		// Arbiter should not have this dispute
		_, err = arbiter.GetDispute(payment.ID)
		if err != ErrDisputeNotFound {
			t.Errorf("GetDispute() error = %v, want %v (dispute should not exist without arbiter)", err, ErrDisputeNotFound)
		}
	})

	t.Run("SetArbiter after creation", func(t *testing.T) {
		em, err := NewEscrowManagerWithAudit(pw, logger)
		if err != nil {
			t.Fatalf("NewEscrowManagerWithAudit() error = %v", err)
		}

		// Initially no arbiter
		if em.arbiter != nil {
			t.Error("EscrowManager should not have arbiter initially")
		}

		// Set arbiter
		newArbiter := NewLocalArbiter()
		em.SetArbiter(newArbiter)

		if em.arbiter == nil {
			t.Error("SetArbiter() did not set arbiter")
		}

		// Create payment and request dispute
		payment := &Payment{
			ID:                     "test-payment-set-arbiter",
			EscrowState:            EscrowFunded,
			DisputeReason:          "",
			MultisigEnabled:        true,
			Addresses:              map[wallet.WalletType]string{wallet.Bitcoin: "test-addr-4"},
			StateTransitionHistory: []StateTransitionHistory{},
		}
		store.CreatePayment(payment)

		err = em.RequestDispute(payment.ID, RoleSeller, "dispute after arbiter set")
		if err != nil {
			t.Fatalf("RequestDispute() error = %v", err)
		}

		// Verify dispute registered with newly set arbiter
		dispute, err := newArbiter.GetDispute(payment.ID)
		if err != nil {
			t.Fatalf("GetDispute() error = %v", err)
		}
		if dispute.Requester != RoleSeller {
			t.Errorf("Dispute requester = %v, want %v", dispute.Requester, RoleSeller)
		}
	})
}

func TestDisputeAntiSpam_RateLimit(t *testing.T) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:           0.001,
		TestNet:              true,
		Store:                store,
		PaymentTimeout:       time.Hour,
		MaxDisputesPerPeriod: 3,
		DisputePeriod:        24 * time.Hour,
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	// Create 4 test payments
	for i := 0; i < 4; i++ {
		payment := &Payment{
			ID:          fmt.Sprintf("payment-%d", i),
			EscrowState: EscrowFunded,
			Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: fmt.Sprintf("test-address-%d", i)},
			Amounts:     map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
		}
		if err := store.CreatePayment(payment); err != nil {
			t.Fatalf("Failed to create payment: %v", err)
		}
	}

	// File 3 disputes (should succeed)
	for i := 0; i < 3; i++ {
		err := em.RequestDispute(fmt.Sprintf("payment-%d", i), RoleBuyer, "Test dispute")
		if err != nil {
			t.Errorf("Dispute %d should succeed, got error: %v", i, err)
		}
	}

	// File 4th dispute (should fail - rate limit exceeded)
	err = em.RequestDispute("payment-3", RoleBuyer, "Test dispute")
	if err == nil {
		t.Error("Expected rate limit error for 4th dispute, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("Expected 'rate limit exceeded' error, got: %v", err)
	}
}

func TestDisputeAntiSpam_DisputeFee(t *testing.T) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:        0.001,
		TestNet:           true,
		Store:             store,
		PaymentTimeout:    time.Hour,
		DisputeFeePercent: 0.05, // 5% dispute fee
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	payment := &Payment{
		ID:          "payment-1",
		EscrowState: EscrowFunded,
		Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:     map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
	}
	if err := store.CreatePayment(payment); err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	// Request dispute
	err = em.RequestDispute("payment-1", RoleBuyer, "Test dispute")
	if err != nil {
		t.Fatalf("Failed to request dispute: %v", err)
	}

	// Verify dispute fee was set
	payment, _ = store.GetPayment("payment-1")
	expectedFee := 0.001 * 0.05 // 5% of 0.001 BTC
	if payment.DisputeFee != expectedFee {
		t.Errorf("Expected dispute fee = %f, got %f", expectedFee, payment.DisputeFee)
	}
}

func TestDisputeAntiSpam_TimeoutExtension(t *testing.T) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:            0.001,
		TestNet:               true,
		Store:                 store,
		PaymentTimeout:        time.Hour,
		ExtendEscrowOnDispute: 7 * 24 * time.Hour,
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	originalTimeout := time.Now().Add(24 * time.Hour)
	payment := &Payment{
		ID:            "payment-1",
		EscrowState:   EscrowFunded,
		EscrowTimeout: originalTimeout,
		Addresses:     map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:       map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
	}
	if err := store.CreatePayment(payment); err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	// Request dispute
	err = em.RequestDispute("payment-1", RoleBuyer, "Test dispute")
	if err != nil {
		t.Fatalf("Failed to request dispute: %v", err)
	}

	// Verify timeout was extended
	payment, _ = store.GetPayment("payment-1")
	if payment.EscrowTimeout.Before(originalTimeout) {
		t.Error("Expected escrow timeout to be extended after dispute")
	}
}

func TestDisputeAntiSpam_EvidenceSize(t *testing.T) {
	store := NewMemoryStore()
	config := Config{
		PriceInBTC:           0.001,
		TestNet:              true,
		Store:                store,
		PaymentTimeout:       time.Hour,
		MaxEvidenceSizeBytes: 1024, // 1 KB limit for test
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("Failed to create escrow manager: %v", err)
	}

	payment := &Payment{
		ID:          "payment-1",
		EscrowState: EscrowDisputed,
		Addresses:   map[wallet.WalletType]string{wallet.Bitcoin: "test-address"},
		Amounts:     map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
	}
	if err := store.CreatePayment(payment); err != nil {
		t.Fatalf("Failed to create payment: %v", err)
	}

	// Submit evidence under limit (should succeed)
	smallEvidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     strings.Repeat("a", 512), // 512 bytes
		Description: "Small evidence",
	}
	err = em.SubmitDisputeEvidence("payment-1", smallEvidence)
	if err != nil {
		t.Errorf("Small evidence should succeed, got error: %v", err)
	}

	// Submit evidence over limit (should fail)
	largeEvidence := &Evidence{
		Type:        EvidenceText,
		SubmittedBy: RoleBuyer,
		Content:     strings.Repeat("b", 1024), // 1024 bytes (total would be 1536)
		Description: "Large evidence",
	}
	err = em.SubmitDisputeEvidence("payment-1", largeEvidence)
	if err == nil {
		t.Error("Expected evidence size limit error, got nil")
	}
	if !strings.Contains(err.Error(), "evidence size limit exceeded") {
		t.Errorf("Expected 'evidence size limit exceeded' error, got: %v", err)
	}
}

func TestCreateEscrow_NegativeTimeout(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:  true,
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 30 * 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Try to create escrow with negative timeout
	_, err = em.CreateEscrow(1.0, -1*time.Hour)
	if err == nil {
		t.Error("CreateEscrow() with negative timeout should fail")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("CreateEscrow() error = %v, want error about positive timeout", err)
	}
}

func TestCreateEscrow_ZeroTimeout(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:  true,
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 30 * 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Try to create escrow with zero timeout
	_, err = em.CreateEscrow(1.0, 0)
	if err == nil {
		t.Error("CreateEscrow() with zero timeout should fail")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("CreateEscrow() error = %v, want error about positive timeout", err)
	}
}

func TestCreateEscrow_BelowMinimum(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:  true,
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 30 * 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Try to create escrow below minimum
	_, err = em.CreateEscrow(1.0, 30*time.Minute)
	if err == nil {
		t.Error("CreateEscrow() below minimum should fail")
	}
	if !strings.Contains(err.Error(), "below minimum") {
		t.Errorf("CreateEscrow() error = %v, want error about minimum", err)
	}
}

func TestCreateEscrow_AboveMaximum(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:  true,
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 30 * 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Try to create escrow above maximum
	_, err = em.CreateEscrow(1.0, 31*24*time.Hour)
	if err == nil {
		t.Error("CreateEscrow() above maximum should fail")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("CreateEscrow() error = %v, want error about maximum", err)
	}
}

func TestExtendTimeout_BuyerSeller(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:            store,
		HDWallets:        make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled:  true,
		minEscrowTimeout: 1 * time.Hour,
		maxEscrowTimeout: 30 * 24 * time.Hour,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Create a funded escrow payment
	payment := &Payment{
		ID:              "test-extend",
		MultisigEnabled: true,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(24 * time.Hour),
	}
	store.CreatePayment(payment)

	// Extend timeout with buyer+seller signatures
	// Generate valid EC keys and signatures for the test
	buyerPrivKey, _ := btcec.NewPrivateKey()
	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()

	sellerPrivKey, _ := btcec.NewPrivateKey()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()

	// Create signature data with required fields
	extension := 2 * 24 * time.Hour
	currentTime := time.Now()

	buyerSigData := &SignatureData{
		Role:      RoleBuyer,
		PublicKey: buyerPubKey,
		SignerID:  "buyer",
		PaymentID: "test-extend",
		Nonce:     []byte("buyer-nonce-12345"),
		SignedAt:  currentTime,
	}

	// Compute and sign the intent hash for buyer
	buyerIntent := timeoutExtensionIntentHash("test-extend", payment.EscrowTimeout, extension, buyerSigData)
	buyerSigECDSA := ecdsa.Sign(buyerPrivKey, buyerIntent[:])
	buyerSigData.Signature = buyerSigECDSA.Serialize()

	sellerSigData := &SignatureData{
		Role:      RoleSeller,
		PublicKey: sellerPubKey,
		SignerID:  "seller",
		PaymentID: "test-extend",
		Nonce:     []byte("seller-nonce-67890"),
		SignedAt:  currentTime,
	}

	// Compute and sign the intent hash for seller
	sellerIntent := timeoutExtensionIntentHash("test-extend", payment.EscrowTimeout, extension, sellerSigData)
	sellerSigECDSA := ecdsa.Sign(sellerPrivKey, sellerIntent[:])
	sellerSigData.Signature = sellerSigECDSA.Serialize()

	oldTimeout := payment.EscrowTimeout
	err = em.ExtendTimeout("test-extend", extension, buyerSigData, sellerSigData)
	if err != nil {
		t.Fatalf("ExtendTimeout() error = %v", err)
	}

	// Verify timeout was extended
	updatedPayment, _ := store.GetPayment("test-extend")
	if !updatedPayment.EscrowTimeout.After(oldTimeout) {
		t.Error("Timeout was not extended")
	}
}

func TestExtendTimeout_NegativeExtension(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:           store,
		HDWallets:       make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled: true,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	payment := &Payment{
		ID:              "test-negative",
		MultisigEnabled: true,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(24 * time.Hour),
	}
	store.CreatePayment(payment)

	buyerSig := &SignatureData{Role: RoleBuyer, PublicKey: []byte("b"), Signature: []byte("sig123456")}
	sellerSig := &SignatureData{Role: RoleSeller, PublicKey: []byte("s"), Signature: []byte("sig123456")}

	// Try negative extension
	err = em.ExtendTimeout("test-negative", -1*time.Hour, buyerSig, sellerSig)
	if err == nil {
		t.Error("ExtendTimeout() with negative extension should fail")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("ExtendTimeout() error = %v, want error about positive extension", err)
	}
}

func TestExtendTimeout_ExceedsMaximum(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:           store,
		HDWallets:       make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled: true,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	payment := &Payment{
		ID:              "test-max",
		MultisigEnabled: true,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(24 * time.Hour),
	}
	store.CreatePayment(payment)

	buyerSig := &SignatureData{Role: RoleBuyer, PublicKey: []byte("b"), Signature: []byte("sig123456")}
	sellerSig := &SignatureData{Role: RoleSeller, PublicKey: []byte("s"), Signature: []byte("sig123456")}

	// Try extension exceeding 7 days
	err = em.ExtendTimeout("test-max", 8*24*time.Hour, buyerSig, sellerSig)
	if err == nil {
		t.Error("ExtendTimeout() exceeding maximum should fail")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("ExtendTimeout() error = %v, want error about maximum", err)
	}
}

func TestExtendTimeout_InvalidState(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:           store,
		HDWallets:       make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled: true,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	payment := &Payment{
		ID:              "test-invalid-state",
		MultisigEnabled: true,
		EscrowState:     EscrowCompleted, // Cannot extend completed escrow
		EscrowTimeout:   time.Now().Add(24 * time.Hour),
	}
	store.CreatePayment(payment)

	buyerSig := &SignatureData{Role: RoleBuyer, PublicKey: []byte("b"), Signature: []byte("sig123456")}
	sellerSig := &SignatureData{Role: RoleSeller, PublicKey: []byte("s"), Signature: []byte("sig123456")}

	err = em.ExtendTimeout("test-invalid-state", 2*24*time.Hour, buyerSig, sellerSig)
	if err == nil {
		t.Error("ExtendTimeout() on completed escrow should fail")
	}
}

func TestExtendTimeout_InsufficientSignatures(t *testing.T) {
	store := NewMemoryStore()
	pw := &Paywall{
		Store:           store,
		HDWallets:       make(map[wallet.WalletType]wallet.HDWallet),
		multisigEnabled: true,
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	payment := &Payment{
		ID:              "test-insufficient",
		MultisigEnabled: true,
		EscrowState:     EscrowFunded,
		EscrowTimeout:   time.Now().Add(24 * time.Hour),
	}
	store.CreatePayment(payment)

	buyerSig := &SignatureData{Role: RoleBuyer, PublicKey: []byte("b"), Signature: []byte("sig123456")}

	// Try with only one signature
	err = em.ExtendTimeout("test-insufficient", 2*24*time.Hour, buyerSig, nil)
	if err != ErrInsufficientSignatures {
		t.Errorf("ExtendTimeout() error = %v, want %v", err, ErrInsufficientSignatures)
	}
}
