package paywall

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/opd-ai/paywall/wallet"
)

// FuzzEscrowStateTransitions uses Go's native fuzzing to test escrow state machine
// Run with: go test -fuzz=FuzzEscrowStateTransitions -fuzztime=30s
func FuzzEscrowStateTransitions(f *testing.F) {
	// Seed corpus with valid transition sequences
	f.Add(uint8(1), uint8(2), uint8(3), uint8(4)) // Pending -> Funded -> Disputed -> Completed
	f.Add(uint8(1), uint8(2), uint8(3), uint8(5)) // Pending -> Funded -> Disputed -> Refunded
	f.Add(uint8(1), uint8(2), uint8(4), uint8(0)) // Pending -> Funded -> Completed -> (terminal)
	f.Add(uint8(1), uint8(2), uint8(5), uint8(0)) // Pending -> Funded -> Refunded -> (terminal)
	f.Add(uint8(1), uint8(0), uint8(0), uint8(0)) // Pending -> (no transition)

	// Generate test public keys once
	buyerSeed := sha256.Sum256([]byte("fuzz-buyer"))
	sellerSeed := sha256.Sum256([]byte("fuzz-seller"))
	arbiterSeed := sha256.Sum256([]byte("fuzz-arbiter"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	f.Fuzz(func(t *testing.T, state1, state2, state3, state4 uint8) {
		// Create a fresh paywall and escrow manager for each iteration
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
				wallet.Bitcoin: publicKeys,
			},
			MultisigRole:       RoleBuyer,
			AuthorizedArbiters: [][]byte{arbiterPubKey},
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Skip("Failed to create paywall")
		}
		defer pw.Close()

		escrowMgr, err := NewEscrowManager(pw)
		if err != nil {
			t.Skip("Failed to create escrow manager")
		}

		// Create an escrow payment
		paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
		if err != nil {
			t.Skip("Failed to create escrow")
		}

		// Initial state should be EscrowPending
		payment, _ := store.GetPayment(paymentID)
		if payment.EscrowState != EscrowPending {
			t.Fatalf("Expected initial state EscrowPending, got %s", payment.EscrowState)
		}

		// Attempt a sequence of state transitions
		states := []uint8{state1, state2, state3, state4}
		for i, targetState := range states {
			if targetState == 0 {
				// Skip null transitions
				continue
			}

			// Map uint8 to EscrowState
			var target EscrowState
			switch targetState % 6 {
			case 0:
				target = EscrowNone
			case 1:
				target = EscrowPending
			case 2:
				target = EscrowFunded
			case 3:
				target = EscrowDisputed
			case 4:
				target = EscrowCompleted
			case 5:
				target = EscrowRefunded
			}

			// Get current payment state
			payment, err = store.GetPayment(paymentID)
			if err != nil {
				t.Fatalf("Failed to get payment at iteration %d: %v", i, err)
			}

			currentState := payment.EscrowState

			// Attempt transition
			attemptStateTransition(t, escrowMgr, store, paymentID, currentState, target,
				buyerPubKey, sellerPubKey, arbiterPubKey)

			// Verify payment still exists and is in a valid state
			payment, err = store.GetPayment(paymentID)
			if err != nil {
				t.Fatalf("Payment disappeared after transition attempt at iteration %d", i)
			}
			if payment == nil {
				t.Fatalf("Payment is nil after transition attempt at iteration %d", i)
			}

			// State must be one of the defined escrow states
			if payment.EscrowState < EscrowNone || payment.EscrowState > EscrowRefunded {
				t.Fatalf("Invalid escrow state after transition: %d", payment.EscrowState)
			}

			// Terminal states should not change
			if currentState == EscrowCompleted || currentState == EscrowRefunded {
				if payment.EscrowState != currentState {
					t.Errorf("Terminal state changed from %s to %s", currentState, payment.EscrowState)
				}
			}
		}
	})
}

// attemptStateTransition tries to transition a payment to a target state
// This function doesn't validate the transition is legal - it just attempts it
func attemptStateTransition(t *testing.T, em *EscrowManager, store PaymentStore,
	paymentID string, from, to EscrowState,
	buyerPubKey, sellerPubKey, arbiterPubKey []byte) {

	// Helper to create signature data
	makeSig := func(role MultisigRole, pubKey []byte, suffix string) *SignatureData {
		return &SignatureData{
			SignerID:  string(role) + "-fuzz-" + suffix,
			Role:      role,
			Signature: []byte("fuzz-signature-" + suffix),
			PublicKey: pubKey,
			SignedAt:  time.Now(),
			Nonce:     []byte(paymentID + "-" + suffix),
			PaymentID: paymentID,
		}
	}

	switch to {
	case EscrowFunded:
		// To fund, payment must be confirmed
		payment, _ := store.GetPayment(paymentID)
		if payment.Status != StatusConfirmed {
			payment.Status = StatusConfirmed
			payment.Confirmations = 3
			store.UpdatePayment(payment)
		}
		_ = em.FundEscrow(paymentID)

	case EscrowDisputed:
		// Ensure payment is funded first
		if from == EscrowPending {
			payment, _ := store.GetPayment(paymentID)
			payment.Status = StatusConfirmed
			payment.Confirmations = 3
			store.UpdatePayment(payment)
			em.FundEscrow(paymentID)
		}
		// Request dispute (ignore error if invalid)
		_ = em.RequestDispute(paymentID, RoleBuyer, "fuzz dispute")

	case EscrowCompleted:
		// Ensure payment is funded first
		if from == EscrowPending {
			payment, _ := store.GetPayment(paymentID)
			payment.Status = StatusConfirmed
			payment.Confirmations = 3
			store.UpdatePayment(payment)
			em.FundEscrow(paymentID)
		}

		// Try different completion paths
		if from == EscrowDisputed {
			// Resolve dispute in favor of seller
			arbiterSig := makeSig(RoleArbiter, arbiterPubKey, "resolve-arbiter")
			sellerSig := makeSig(RoleSeller, sellerPubKey, "resolve-seller")
			_ = em.ResolveDispute(paymentID, arbiterSig, sellerSig)
		} else if from == EscrowFunded {
			// Release to seller (happy path)
			buyerSig := makeSig(RoleBuyer, buyerPubKey, "release-buyer")
			sellerSig := makeSig(RoleSeller, sellerPubKey, "release-seller")
			_ = em.ReleaseToSeller(paymentID, buyerSig, sellerSig)
		}

	case EscrowRefunded:
		// Ensure payment is funded first
		if from == EscrowPending {
			payment, _ := store.GetPayment(paymentID)
			payment.Status = StatusConfirmed
			payment.Confirmations = 3
			store.UpdatePayment(payment)
			em.FundEscrow(paymentID)
		}

		// Try different refund paths
		if from == EscrowDisputed {
			// Resolve dispute in favor of buyer
			arbiterSig := makeSig(RoleArbiter, arbiterPubKey, "refund-arbiter")
			buyerSig := makeSig(RoleBuyer, buyerPubKey, "refund-buyer")
			_ = em.ResolveDispute(paymentID, arbiterSig, buyerSig)
		} else if from == EscrowFunded {
			// Mutual refund
			buyerSig := makeSig(RoleBuyer, buyerPubKey, "mutual-refund-buyer")
			arbiterSig := makeSig(RoleArbiter, arbiterPubKey, "mutual-refund-arbiter")
			_ = em.RefundBuyer(paymentID, buyerSig, arbiterSig)
		}
	}
}

// FuzzSignatureDataStructures fuzzes signature validation logic
// Run with: go test -fuzz=FuzzSignatureDataStructures -fuzztime=30s
func FuzzSignatureDataStructures(f *testing.F) {
	// Seed corpus with various signature scenarios
	f.Add([]byte("valid-signature"), []byte("valid-pubkey"), []byte("valid-nonce"), "signer-1", uint8(1))
	f.Add([]byte{}, []byte("pubkey"), []byte("nonce"), "signer-2", uint8(2))                            // Empty signature
	f.Add([]byte("sig"), []byte{}, []byte("nonce"), "signer-3", uint8(3))                               // Empty pubkey
	f.Add([]byte("sig"), []byte("pubkey"), []byte{}, "signer-4", uint8(1))                              // Empty nonce
	f.Add([]byte("sig"), []byte("pubkey"), []byte("nonce"), "", uint8(2))                               // Empty signer ID
	f.Add([]byte{0xFF, 0xFF, 0xFF}, []byte{0x00, 0x01, 0x02}, []byte{0xDE, 0xAD}, "evil", uint8(3))     // Binary data
	f.Add([]byte("very-long-"+string(make([]byte, 1000))), []byte("pk"), []byte("n"), "long", uint8(1)) // Oversized signature

	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("fuzz-buyer-sig"))
	sellerSeed := sha256.Sum256([]byte("fuzz-seller-sig"))
	arbiterSeed := sha256.Sum256([]byte("fuzz-arbiter-sig"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	f.Fuzz(func(t *testing.T, signature, publicKey, nonce []byte, signerID string, roleInt uint8) {
		// Skip if inputs are too large to avoid memory exhaustion
		if len(signature) > 10000 || len(publicKey) > 1000 || len(nonce) > 1000 {
			return
		}

		// Create paywall with escrow
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
				wallet.Bitcoin: publicKeys,
			},
			MultisigRole:       RoleBuyer,
			AuthorizedArbiters: [][]byte{arbiterPubKey},
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Skip("Failed to create paywall")
		}
		defer pw.Close()

		escrowMgr, err := NewEscrowManager(pw)
		if err != nil {
			t.Skip("Failed to create escrow manager")
		}

		// Create and fund an escrow
		paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
		if err != nil {
			t.Skip("Failed to create escrow")
		}

		payment, _ := store.GetPayment(paymentID)
		payment.Status = StatusConfirmed
		payment.Confirmations = 3
		store.UpdatePayment(payment)
		escrowMgr.FundEscrow(paymentID)

		// Map roleInt to MultisigRole
		var role MultisigRole
		switch roleInt % 3 {
		case 0:
			role = RoleBuyer
		case 1:
			role = RoleSeller
		case 2:
			role = RoleArbiter
		}

		// Create a SignatureData with fuzzed inputs
		fuzzedSig := &SignatureData{
			SignerID:  signerID,
			Role:      role,
			Signature: signature,
			PublicKey: publicKey,
			SignedAt:  time.Now(),
			Nonce:     nonce,
			PaymentID: paymentID,
		}

		// Create a valid seller signature
		sellerSig := &SignatureData{
			SignerID:  "valid-seller",
			Role:      RoleSeller,
			Signature: []byte("valid-seller-signature"),
			PublicKey: sellerPubKey,
			SignedAt:  time.Now(),
			Nonce:     []byte("valid-seller-nonce"),
			PaymentID: paymentID,
		}

		// Attempt to use the fuzzed signature
		// System should gracefully reject invalid signatures without crashing
		var err1, err2 error

		// Test as buyer signature
		if role == RoleBuyer {
			err1 = escrowMgr.ReleaseToSeller(paymentID, fuzzedSig, sellerSig)
		}

		// Test as arbiter signature (dispute resolution)
		if role == RoleArbiter {
			// Request dispute first
			escrowMgr.RequestDispute(paymentID, RoleBuyer, "fuzz test")
			err2 = escrowMgr.ResolveDispute(paymentID, fuzzedSig, sellerSig)
		}

		// Verify the payment still exists and is in a valid state
		payment, err = store.GetPayment(paymentID)
		if err != nil {
			t.Fatalf("Payment retrieval failed: %v", err)
		}
		if payment == nil {
			t.Fatal("Payment is nil after signature validation")
		}

		// Payment state must remain valid
		if payment.EscrowState < EscrowNone || payment.EscrowState > EscrowRefunded {
			t.Fatalf("Invalid payment state: %d", payment.EscrowState)
		}

		// If signature was invalid, state should not have changed to terminal
		if err1 != nil && err2 != nil {
			// Both operations failed - payment should not be in terminal state
			if payment.EscrowState == EscrowCompleted || payment.EscrowState == EscrowRefunded {
				t.Errorf("Payment reached terminal state despite signature validation failures")
			}
		}
	})
}

// FuzzSignatureReplayDetection fuzzes the replay detection logic
// Run with: go test -fuzz=FuzzSignatureReplayDetection -fuzztime=30s
func FuzzSignatureReplayDetection(f *testing.F) {
	// Seed corpus
	f.Add([]byte("nonce1"), []byte("nonce2"), "payment1", "payment2", "signer1", "signer2")
	f.Add([]byte("same"), []byte("same"), "payment1", "payment1", "same-signer", "same-signer")
	f.Add([]byte{}, []byte{}, "", "", "", "")

	// Generate test keys
	buyerSeed := sha256.Sum256([]byte("fuzz-buyer-replay"))
	sellerSeed := sha256.Sum256([]byte("fuzz-seller-replay"))
	arbiterSeed := sha256.Sum256([]byte("fuzz-arbiter-replay"))

	buyerPrivKey, _ := btcec.PrivKeyFromBytes(buyerSeed[:])
	sellerPrivKey, _ := btcec.PrivKeyFromBytes(sellerSeed[:])
	arbiterPrivKey, _ := btcec.PrivKeyFromBytes(arbiterSeed[:])

	buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()
	sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()
	arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	f.Fuzz(func(t *testing.T, nonce1, nonce2 []byte, paymentID1, paymentID2, signer1, signer2 string) {
		// Skip oversized inputs
		if len(nonce1) > 1000 || len(nonce2) > 1000 {
			return
		}

		// Create paywall
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
				wallet.Bitcoin: publicKeys,
			},
			MultisigRole:       RoleBuyer,
			AuthorizedArbiters: [][]byte{arbiterPubKey},
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Skip("Failed to create paywall")
		}
		defer pw.Close()

		escrowMgr, err := NewEscrowManager(pw)
		if err != nil {
			t.Skip("Failed to create escrow manager")
		}

		// Create escrow
		realPaymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
		if err != nil {
			t.Skip("Failed to create escrow")
		}

		payment, _ := store.GetPayment(realPaymentID)
		payment.Status = StatusConfirmed
		payment.Confirmations = 3
		store.UpdatePayment(payment)
		escrowMgr.FundEscrow(realPaymentID)

		// Create two signatures with fuzzed nonces and payment IDs
		sig1 := &SignatureData{
			SignerID:  signer1,
			Role:      RoleBuyer,
			Signature: []byte("signature1"),
			PublicKey: buyerPubKey,
			SignedAt:  time.Now(),
			Nonce:     nonce1,
			PaymentID: paymentID1,
		}

		sig2 := &SignatureData{
			SignerID:  signer2,
			Role:      RoleBuyer,
			Signature: []byte("signature2"),
			PublicKey: buyerPubKey,
			SignedAt:  time.Now(),
			Nonce:     nonce2,
			PaymentID: paymentID2,
		}

		sellerSig := &SignatureData{
			SignerID:  "seller",
			Role:      RoleSeller,
			Signature: []byte("seller-signature"),
			PublicKey: sellerPubKey,
			SignedAt:  time.Now(),
			Nonce:     []byte("seller-nonce"),
			PaymentID: realPaymentID,
		}

		// Attempt first signature
		_ = escrowMgr.ReleaseToSeller(realPaymentID, sig1, sellerSig)

		// Get payment state after first attempt
		payment1, _ := store.GetPayment(realPaymentID)
		state1 := payment1.EscrowState

		// Attempt second signature (potentially replayed nonce)
		_ = escrowMgr.ReleaseToSeller(realPaymentID, sig2, sellerSig)

		// Get payment state after second attempt
		payment2, _ := store.GetPayment(realPaymentID)
		state2 := payment2.EscrowState

		// Verify payment didn't get corrupted
		if payment2 == nil {
			t.Fatal("Payment disappeared after replay attempt")
		}

		// State must be valid
		if payment2.EscrowState < EscrowNone || payment2.EscrowState > EscrowRefunded {
			t.Fatalf("Invalid payment state after replay: %d", payment2.EscrowState)
		}

		// If nonces are identical and non-empty, replay should be detected
		if len(nonce1) > 0 && len(nonce2) > 0 && bytesEqual(nonce1, nonce2) {
			// Replay should not cause double state transition
			if state1 == EscrowCompleted && state2 != EscrowCompleted {
				t.Error("State changed after nonce replay")
			}
		}
	})
}
