package paywall

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/opd-ai/paywall/wallet"
)

// generateTestPublicKeys creates valid compressed public keys for testing timeout bounds
func generateTestPublicKeysTimeout() ([]byte, []byte, []byte) {
	buyerSeed := sha256.Sum256([]byte("buyer-timeout-test"))
	sellerSeed := sha256.Sum256([]byte("seller-timeout-test"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-timeout-test"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	return buyerPrivKey.PubKey().SerializeCompressed(),
		sellerPrivKey.PubKey().SerializeCompressed(),
		arbiterPrivKey.PubKey().SerializeCompressed()
}

func generateTestPrivateKeysTimeout() (*btcec.PrivateKey, *btcec.PrivateKey, *btcec.PrivateKey) {
	buyerSeed := sha256.Sum256([]byte("buyer-timeout-test"))
	sellerSeed := sha256.Sum256([]byte("seller-timeout-test"))
	arbiterSeed := sha256.Sum256([]byte("arbiter-timeout-test"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	return buyerPrivKey, sellerPrivKey, arbiterPrivKey
}

func buildTimeoutExtensionSignature(
	t *testing.T,
	paymentID string,
	currentTimeout time.Time,
	extension time.Duration,
	role MultisigRole,
	signerID string,
	privateKey *btcec.PrivateKey,
	nonce []byte,
) *SignatureData {
	t.Helper()

	signedAt := time.Now().UTC().Truncate(time.Nanosecond)
	sigPayload := &SignatureData{
		SignerID:  signerID,
		Role:      role,
		PublicKey: privateKey.PubKey().SerializeCompressed(),
		SignedAt:  signedAt,
		Nonce:     nonce,
		PaymentID: paymentID,
	}
	intent := timeoutExtensionIntentHash(paymentID, currentTimeout, extension, sigPayload)
	signature := append(ecdsa.Sign(privateKey, intent[:]).Serialize(), byte(0x01))
	sigPayload.Signature = signature
	return sigPayload
}

func createFundedEscrowForExtensionTest(
	t *testing.T,
	buyerPubKey, sellerPubKey, arbiterPubKey []byte,
) (*EscrowManager, *MemoryStore, string, time.Time) {
	t.Helper()

	store := NewMemoryStore()
	config := Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          store,
		PaymentTimeout: time.Hour * 24,

		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
		MinEscrowTimeout:   time.Hour,
		MaxEscrowTimeout:   30 * 24 * time.Hour,
		AuthorizedArbiters: [][]byte{arbiterPubKey},
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	t.Cleanup(func() { pw.Close() })

	em, _ := NewEscrowManager(pw)
	paymentID, err := em.CreateEscrow(1.0, 24*time.Hour)
	if err != nil {
		t.Fatalf("CreateEscrow failed: %v", err)
	}

	payment, _ := store.GetPayment(paymentID)
	payment.Status = StatusConfirmed
	if err := store.UpdatePayment(payment); err != nil {
		t.Fatalf("UpdatePayment failed: %v", err)
	}
	if err := em.FundEscrow(paymentID); err != nil {
		t.Fatalf("FundEscrow failed: %v", err)
	}

	payment, _ = store.GetPayment(paymentID)
	return em, store, paymentID, payment.EscrowTimeout
}

// TestEscrowTimeoutBounds verifies that escrow creation enforces timeout bounds
func TestEscrowTimeoutBounds(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()

	tests := []struct {
		name             string
		escrowTimeout    time.Duration
		minEscrowTimeout time.Duration
		maxEscrowTimeout time.Duration
		wantErr          bool
		errContains      string
	}{
		{
			name:             "valid timeout within bounds",
			escrowTimeout:    48 * time.Hour,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          false,
		},
		{
			name:             "timeout exactly at minimum",
			escrowTimeout:    24 * time.Hour,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          false,
		},
		{
			name:             "timeout exactly at maximum",
			escrowTimeout:    90 * 24 * time.Hour,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          false,
		},
		{
			name:             "timeout below minimum",
			escrowTimeout:    1 * time.Hour,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          true,
			errContains:      "below minimum",
		},
		{
			name:             "timeout above maximum",
			escrowTimeout:    100 * 24 * time.Hour,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          true,
			errContains:      "exceeds maximum",
		},
		{
			name:             "negative timeout",
			escrowTimeout:    -1 * time.Hour,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          true,
			errContains:      "below minimum",
		},
		{
			name:             "zero timeout",
			escrowTimeout:    0,
			minEscrowTimeout: 24 * time.Hour,
			maxEscrowTimeout: 90 * 24 * time.Hour,
			wantErr:          true,
			errContains:      "below minimum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()

			// Create a proper HD wallet for testing
			seed := make([]byte, 32)
			if _, err := rand.Read(seed); err != nil {
				t.Fatalf("Failed to generate seed: %v", err)
			}
			hdWallet, err := wallet.NewBTCHDWallet(seed, true, 1)
			if err != nil {
				t.Fatalf("Failed to create HD wallet: %v", err)
			}

			pw := &Paywall{
				Store:     store,
				HDWallets: map[wallet.WalletType]wallet.HDWallet{wallet.Bitcoin: hdWallet},
				prices:    map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				participantPubKeys: map[wallet.WalletType][][]byte{
					wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
				},
				multisigEnabled:  true,
				multisigRequired: 2,
				multisigTotal:    3,
				minEscrowTimeout: tt.minEscrowTimeout,
				maxEscrowTimeout: tt.maxEscrowTimeout,
			}

			em, err := NewEscrowManager(pw)
			if err != nil {
				t.Fatalf("NewEscrowManager() error = %v", err)
			}

			_, err = em.CreateEscrow(1.0, tt.escrowTimeout)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateEscrow() expected error, got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("CreateEscrow() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("CreateEscrow() unexpected error = %v", err)
				}
			}
		})
	}
}

// TestEscrowTimeoutDefaultBounds verifies that default timeout bounds are applied when not configured
func TestEscrowTimeoutDefaultBounds(t *testing.T) {
	store := NewMemoryStore()
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()

	// Create paywall with zero timeout bounds (should use defaults)
	config := Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
		MultisigRole:     RoleBuyer,
		MinEscrowTimeout: 0, // Zero - should use default
		MaxEscrowTimeout: 0, // Zero - should use default
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("NewPaywall() error = %v", err)
	}
	defer pw.Close()

	// Verify defaults were applied
	expectedMin := 24 * time.Hour
	expectedMax := 90 * 24 * time.Hour

	if pw.minEscrowTimeout != expectedMin {
		t.Errorf("minEscrowTimeout = %v, want %v", pw.minEscrowTimeout, expectedMin)
	}
	if pw.maxEscrowTimeout != expectedMax {
		t.Errorf("maxEscrowTimeout = %v, want %v", pw.maxEscrowTimeout, expectedMax)
	}

	// Test that timeout below default minimum is rejected
	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	_, err = em.CreateEscrow(1.0, 1*time.Hour) // Below 24h default
	if err == nil {
		t.Error("CreateEscrow() expected error for timeout below default minimum, got nil")
	}

	// Test that timeout within default bounds is accepted
	_, err = em.CreateEscrow(1.0, 48*time.Hour) // Within default bounds
	if err != nil {
		t.Errorf("CreateEscrow() unexpected error for valid timeout: %v", err)
	}

	// Test that timeout above default maximum is rejected
	_, err = em.CreateEscrow(1.0, 100*24*time.Hour) // Above 90 days default
	if err == nil {
		t.Error("CreateEscrow() expected error for timeout above default maximum, got nil")
	}
}

// TestEscrowTimeoutCustomBounds verifies that custom timeout bounds are respected
func TestEscrowTimeoutCustomBounds(t *testing.T) {
	store := NewMemoryStore()
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()

	// Create paywall with custom timeout bounds
	customMin := 12 * time.Hour
	customMax := 30 * 24 * time.Hour

	config := Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            store,
		PaymentTimeout:   time.Hour * 24,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
		},
		MultisigRole:     RoleBuyer,
		MinEscrowTimeout: customMin,
		MaxEscrowTimeout: customMax,
	}

	pw, err := NewPaywall(config)
	if err != nil {
		t.Fatalf("NewPaywall() error = %v", err)
	}
	defer pw.Close()

	// Verify custom bounds were applied
	if pw.minEscrowTimeout != customMin {
		t.Errorf("minEscrowTimeout = %v, want %v", pw.minEscrowTimeout, customMin)
	}
	if pw.maxEscrowTimeout != customMax {
		t.Errorf("maxEscrowTimeout = %v, want %v", pw.maxEscrowTimeout, customMax)
	}

	em, err := NewEscrowManager(pw)
	if err != nil {
		t.Fatalf("NewEscrowManager() error = %v", err)
	}

	// Test below custom minimum
	_, err = em.CreateEscrow(1.0, 6*time.Hour)
	if err == nil {
		t.Error("CreateEscrow() expected error for timeout below custom minimum, got nil")
	}

	// Test within custom bounds
	_, err = em.CreateEscrow(1.0, 24*time.Hour)
	if err != nil {
		t.Errorf("CreateEscrow() unexpected error for valid timeout: %v", err)
	}

	// Test above custom maximum
	_, err = em.CreateEscrow(1.0, 60*24*time.Hour)
	if err == nil {
		t.Error("CreateEscrow() expected error for timeout above custom maximum, got nil")
	}
}

// TestEscrowManager_ExtendTimeout tests timeout extension functionality
func TestEscrowManager_ExtendTimeout(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()
	buyerPrivKey, sellerPrivKey, _ := generateTestPrivateKeysTimeout()
	em, store, paymentID, originalTimeout := createFundedEscrowForExtensionTest(t, buyerPubKey, sellerPubKey, arbiterPubKey)

	extension := 48 * time.Hour
	buyerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		originalTimeout,
		extension,
		RoleBuyer,
		"buyer",
		buyerPrivKey,
		[]byte(paymentID+"-extend"),
	)
	sellerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		originalTimeout,
		extension,
		RoleSeller,
		"seller",
		sellerPrivKey,
		[]byte(paymentID+"-extend-2"),
	)

	err := em.ExtendTimeout(paymentID, extension, buyerSig, sellerSig)
	if err != nil {
		t.Fatalf("ExtendTimeout failed: %v", err)
	}

	payment, _ := store.GetPayment(paymentID)
	expectedTimeout := originalTimeout.Add(extension)
	if !payment.EscrowTimeout.Equal(expectedTimeout) {
		t.Errorf("Timeout not extended correctly: got %v, want %v",
			payment.EscrowTimeout, expectedTimeout)
	}

	t.Logf("✓ Timeout successfully extended from %v to %v",
		originalTimeout, payment.EscrowTimeout)
}

// TestEscrowManager_ExtendTimeout_MaxExtension tests maximum extension limit
func TestEscrowManager_ExtendTimeout_MaxExtension(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()
	buyerPrivKey, sellerPrivKey, _ := generateTestPrivateKeysTimeout()
	em, _, paymentID, currentTimeout := createFundedEscrowForExtensionTest(t, buyerPubKey, sellerPubKey, arbiterPubKey)

	extension := 8 * 24 * time.Hour // 8 days
	buyerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		currentTimeout,
		extension,
		RoleBuyer,
		"buyer",
		buyerPrivKey,
		[]byte(paymentID+"-extend-max"),
	)
	sellerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		currentTimeout,
		extension,
		RoleSeller,
		"seller",
		sellerPrivKey,
		[]byte(paymentID+"-extend-max-2"),
	)

	err := em.ExtendTimeout(paymentID, extension, buyerSig, sellerSig)
	if err == nil {
		t.Error("ExtendTimeout should reject extension beyond 7 days")
	}

	t.Logf("✓ Correctly rejected extension beyond max (7 days)")
}

// TestEscrowManager_ExtendTimeout_NegativeExtension tests negative extension rejection
func TestEscrowManager_ExtendTimeout_NegativeExtension(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()
	buyerPrivKey, sellerPrivKey, _ := generateTestPrivateKeysTimeout()
	em, _, paymentID, currentTimeout := createFundedEscrowForExtensionTest(t, buyerPubKey, sellerPubKey, arbiterPubKey)

	extension := -24 * time.Hour
	buyerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		currentTimeout,
		extension,
		RoleBuyer,
		"buyer",
		buyerPrivKey,
		[]byte(paymentID+"-extend-neg"),
	)
	sellerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		currentTimeout,
		extension,
		RoleSeller,
		"seller",
		sellerPrivKey,
		[]byte(paymentID+"-extend-neg-2"),
	)

	err := em.ExtendTimeout(paymentID, extension, buyerSig, sellerSig)
	if err == nil {
		t.Error("ExtendTimeout should reject negative extension")
	}

	t.Logf("✓ Correctly rejected negative extension")
}

func TestEscrowManager_ExtendTimeout_ArbiterPaths(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()
	buyerPrivKey, sellerPrivKey, arbiterPrivKey := generateTestPrivateKeysTimeout()

	cases := []struct {
		name string
		sig1 func(paymentID string, currentTimeout time.Time) *SignatureData
		sig2 func(paymentID string, currentTimeout time.Time) *SignatureData
	}{
		{
			name: "buyer plus arbiter",
			sig1: func(paymentID string, currentTimeout time.Time) *SignatureData {
				return buildTimeoutExtensionSignature(t, paymentID, currentTimeout, 24*time.Hour, RoleBuyer, "buyer", buyerPrivKey, []byte(paymentID+"-buyer-arbiter-buyer"))
			},
			sig2: func(paymentID string, currentTimeout time.Time) *SignatureData {
				return buildTimeoutExtensionSignature(t, paymentID, currentTimeout, 24*time.Hour, RoleArbiter, "arbiter", arbiterPrivKey, []byte(paymentID+"-buyer-arbiter-arbiter"))
			},
		},
		{
			name: "seller plus arbiter",
			sig1: func(paymentID string, currentTimeout time.Time) *SignatureData {
				return buildTimeoutExtensionSignature(t, paymentID, currentTimeout, 24*time.Hour, RoleSeller, "seller", sellerPrivKey, []byte(paymentID+"-seller-arbiter-seller"))
			},
			sig2: func(paymentID string, currentTimeout time.Time) *SignatureData {
				return buildTimeoutExtensionSignature(t, paymentID, currentTimeout, 24*time.Hour, RoleArbiter, "arbiter", arbiterPrivKey, []byte(paymentID+"-seller-arbiter-arbiter"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			em, store, paymentID, currentTimeout := createFundedEscrowForExtensionTest(t, buyerPubKey, sellerPubKey, arbiterPubKey)
			extension := 24 * time.Hour
			sig1 := tc.sig1(paymentID, currentTimeout)
			sig2 := tc.sig2(paymentID, currentTimeout)

			if err := em.ExtendTimeout(paymentID, extension, sig1, sig2); err != nil {
				t.Fatalf("ExtendTimeout failed: %v", err)
			}

			payment, _ := store.GetPayment(paymentID)
			if !payment.EscrowTimeout.Equal(currentTimeout.Add(extension)) {
				t.Fatalf("timeout not extended as expected")
			}
		})
	}
}

func TestEscrowManager_ExtendTimeout_UnauthorizedArbiter(t *testing.T) {
	buyerPubKey, sellerPubKey, arbiterPubKey := generateTestPublicKeysTimeout()
	buyerPrivKey, _, _ := generateTestPrivateKeysTimeout()

	unauthorizedSeed := sha256.Sum256([]byte("unauthorized-arbiter-timeout-test"))
	unauthorizedArbiterPriv, _ := btcec.PrivKeyFromBytes(unauthorizedSeed[:])
	unauthorizedArbiterPub := unauthorizedArbiterPriv.PubKey().SerializeCompressed()

	em, _, paymentID, currentTimeout := createFundedEscrowForExtensionTest(t, buyerPubKey, sellerPubKey, arbiterPubKey)
	extension := 24 * time.Hour

	buyerSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		currentTimeout,
		extension,
		RoleBuyer,
		"buyer",
		buyerPrivKey,
		[]byte(paymentID+"-buyer-unauth-arbiter-buyer"),
	)
	arbiterSig := buildTimeoutExtensionSignature(
		t,
		paymentID,
		currentTimeout,
		extension,
		RoleArbiter,
		"unauthorized-arbiter",
		unauthorizedArbiterPriv,
		[]byte(fmt.Sprintf("%s-unauth-arbiter", paymentID)),
	)
	arbiterSig.PublicKey = unauthorizedArbiterPub

	err := em.ExtendTimeout(paymentID, extension, buyerSig, arbiterSig)
	if err == nil {
		t.Fatal("ExtendTimeout should reject unauthorized arbiter")
	}
}
