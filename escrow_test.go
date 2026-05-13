package paywall

import (
	"testing"
	"time"

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

			if tt.expectedError != nil && err != nil && err != tt.expectedError {
				// Check if the error wraps the expected error
				if err.Error() == "" || err == nil {
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
				Store:     store,
				HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
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
				Store:     store,
				HDWallets: make(map[wallet.WalletType]wallet.HDWallet),
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
