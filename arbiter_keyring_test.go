package paywall

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/opd-ai/paywall/wallet"
)

func TestNewArbiterKeyringService(t *testing.T) {
	// Generate test private key
	seed := sha256.Sum256([]byte("test-arbiter-seed"))
	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	t.Run("valid creation", func(t *testing.T) {
		keyring, err := NewArbiterKeyringService(privKey, nil, "test-arbiter")
		if err != nil {
			t.Fatalf("NewArbiterKeyringService() error = %v", err)
		}

		if keyring == nil {
			t.Fatal("keyring is nil")
		}

		if keyring.arbiterID != "test-arbiter" {
			t.Errorf("arbiterID = %s, want test-arbiter", keyring.arbiterID)
		}

		if keyring.btcPrivateKey == nil {
			t.Error("btcPrivateKey is nil")
		}
	})

	t.Run("nil private key should error", func(t *testing.T) {
		_, err := NewArbiterKeyringService(nil, nil, "test")
		if err == nil {
			t.Error("NewArbiterKeyringService() with nil privKey should error")
		}
	})

	t.Run("empty arbiter ID gets default", func(t *testing.T) {
		keyring, err := NewArbiterKeyringService(privKey, nil, "")
		if err != nil {
			t.Fatalf("NewArbiterKeyringService() error = %v", err)
		}

		if keyring.arbiterID != "arbiter-default" {
			t.Errorf("arbiterID = %s, want arbiter-default", keyring.arbiterID)
		}
	})
}

func TestArbiterKeyringService_SignTimeoutRefund(t *testing.T) {
	seed := sha256.Sum256([]byte("test-arbiter-seed"))
	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	keyring, err := NewArbiterKeyringService(privKey, nil, "test-arbiter")
	if err != nil {
		t.Fatalf("NewArbiterKeyringService() error = %v", err)
	}

	t.Run("sign valid payment", func(t *testing.T) {
		payment := &Payment{
			ID: "payment-123",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "tb1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq0z2zqy",
			},
			EscrowState:   EscrowFunded,
			EscrowTimeout: time.Now().Add(-1 * time.Hour),
		}

		sig, err := keyring.SignTimeoutRefund(payment)
		if err != nil {
			t.Fatalf("SignTimeoutRefund() error = %v", err)
		}

		if sig == nil {
			t.Fatal("signature is nil")
		}

		// Verify signature fields
		if sig.SignerID != "test-arbiter" {
			t.Errorf("SignerID = %s, want test-arbiter", sig.SignerID)
		}

		if sig.Role != RoleArbiter {
			t.Errorf("Role = %s, want %s", sig.Role, RoleArbiter)
		}

		if len(sig.Signature) == 0 {
			t.Error("Signature is empty")
		}

		if len(sig.PublicKey) == 0 {
			t.Error("PublicKey is empty")
		}

		if sig.PaymentID != payment.ID {
			t.Errorf("PaymentID = %s, want %s", sig.PaymentID, payment.ID)
		}

		if len(sig.Nonce) != 32 {
			t.Errorf("Nonce length = %d, want 32", len(sig.Nonce))
		}

		if sig.SignedAt.IsZero() {
			t.Error("SignedAt is zero")
		}
	})

	t.Run("nil payment should error", func(t *testing.T) {
		_, err := keyring.SignTimeoutRefund(nil)
		if err == nil {
			t.Error("SignTimeoutRefund() with nil payment should error")
		}
	})

	t.Run("signature is deterministic given same input", func(t *testing.T) {
		payment := &Payment{
			ID: "payment-456",
			Addresses: map[wallet.WalletType]string{
				wallet.Bitcoin: "tb1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq0z2zqy",
			},
			EscrowState:   EscrowFunded,
			EscrowTimeout: time.Unix(1234567890, 0), // Fixed timestamp
		}

		sig1, err := keyring.SignTimeoutRefund(payment)
		if err != nil {
			t.Fatalf("SignTimeoutRefund() error = %v", err)
		}

		sig2, err := keyring.SignTimeoutRefund(payment)
		if err != nil {
			t.Fatalf("SignTimeoutRefund() error = %v", err)
		}

		// Signatures should have different nonces (for replay protection)
		if bytes.Equal(sig1.Nonce, sig2.Nonce) {
			t.Error("signatures have identical nonces (should be unique)")
		}

		// But public keys should be the same
		if !bytes.Equal(sig1.PublicKey, sig2.PublicKey) {
			t.Error("public keys differ (should be same)")
		}
	})

	t.Run("monero payment returns not implemented", func(t *testing.T) {
		payment := &Payment{
			ID: "payment-xmr",
			Addresses: map[wallet.WalletType]string{
				wallet.Monero: "4xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
			EscrowState:   EscrowFunded,
			EscrowTimeout: time.Now().Add(-1 * time.Hour),
		}

		_, err := keyring.SignTimeoutRefund(payment)
		if err == nil {
			t.Error("SignTimeoutRefund() for Monero should return not implemented error")
		}
	})
}

func TestArbiterKeyringService_SignatureVerification(t *testing.T) {
	// This test verifies that signatures can be validated using ECDSA
	seed := sha256.Sum256([]byte("test-arbiter-seed"))
	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	keyring, err := NewArbiterKeyringService(privKey, nil, "test-arbiter")
	if err != nil {
		t.Fatalf("NewArbiterKeyringService() error = %v", err)
	}

	payment := &Payment{
		ID: "payment-verify",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "tb1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq0z2zqy",
		},
		EscrowState:   EscrowFunded,
		EscrowTimeout: time.Unix(1234567890, 0),
	}

	sig, err := keyring.SignTimeoutRefund(payment)
	if err != nil {
		t.Fatalf("SignTimeoutRefund() error = %v", err)
	}

	// Recreate the message hash
	message := "timeout_refund|payment-verify|1234567890"
	messageHash := sha256.Sum256([]byte(message))

	// Parse the public key
	pubKey, err := btcec.ParsePubKey(sig.PublicKey)
	if err != nil {
		t.Fatalf("ParsePubKey() error = %v", err)
	}

	// Parse the signature (remove SIGHASH byte)
	ecdsaSig, err := ecdsa.ParseDERSignature(sig.Signature[:len(sig.Signature)-1])
	if err != nil {
		t.Fatalf("ParseDERSignature() error = %v", err)
	}

	// Verify signature
	if !ecdsaSig.Verify(messageHash[:], pubKey) {
		t.Error("Signature verification failed")
	}
}

func TestArbiterKeyringService_GetBTCPublicKey(t *testing.T) {
	seed := sha256.Sum256([]byte("test-arbiter-seed"))
	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	keyring, err := NewArbiterKeyringService(privKey, nil, "test-arbiter")
	if err != nil {
		t.Fatalf("NewArbiterKeyringService() error = %v", err)
	}

	pubKey := keyring.GetBTCPublicKey()
	if len(pubKey) != 33 {
		t.Errorf("public key length = %d, want 33 (compressed)", len(pubKey))
	}

	// Verify it matches the expected public key
	expectedPubKey := privKey.PubKey().SerializeCompressed()
	if !bytes.Equal(pubKey, expectedPubKey) {
		t.Error("GetBTCPublicKey() returned incorrect public key")
	}
}

func TestArbiterKeyringService_GetXMRPublicKey(t *testing.T) {
	seed := sha256.Sum256([]byte("test-arbiter-seed"))
	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	xmrKey := []byte("xmr-private-key-placeholder")
	keyring, err := NewArbiterKeyringService(privKey, xmrKey, "test-arbiter")
	if err != nil {
		t.Fatalf("NewArbiterKeyringService() error = %v", err)
	}

	pubKey := keyring.GetXMRPublicKey()
	if !bytes.Equal(pubKey, xmrKey) {
		t.Error("GetXMRPublicKey() returned incorrect key")
	}
}

func TestNewArbiterKeyringFromSeed(t *testing.T) {
	t.Run("valid seed", func(t *testing.T) {
		seed := sha256.Sum256([]byte("deterministic-seed"))
		keyring, err := NewArbiterKeyringFromSeed(seed[:], "test-arbiter")
		if err != nil {
			t.Fatalf("NewArbiterKeyringFromSeed() error = %v", err)
		}

		if keyring == nil {
			t.Fatal("keyring is nil")
		}

		if keyring.arbiterID != "test-arbiter" {
			t.Errorf("arbiterID = %s, want test-arbiter", keyring.arbiterID)
		}

		// Verify key derivation is deterministic
		keyring2, err := NewArbiterKeyringFromSeed(seed[:], "test-arbiter-2")
		if err != nil {
			t.Fatalf("NewArbiterKeyringFromSeed() second call error = %v", err)
		}

		// Same seed should produce same public key
		pubKey1 := keyring.GetBTCPublicKey()
		pubKey2 := keyring2.GetBTCPublicKey()
		if !bytes.Equal(pubKey1, pubKey2) {
			t.Error("same seed produced different public keys (not deterministic)")
		}
	})

	t.Run("invalid seed length", func(t *testing.T) {
		_, err := NewArbiterKeyringFromSeed([]byte("short"), "test")
		if err == nil {
			t.Error("NewArbiterKeyringFromSeed() with short seed should error")
		}
	})
}

func TestArbiterKeyringService_ConcurrentAccess(t *testing.T) {
	seed := sha256.Sum256([]byte("test-arbiter-seed"))
	privKey, _ := btcec.PrivKeyFromBytes(seed[:])

	keyring, err := NewArbiterKeyringService(privKey, nil, "test-arbiter")
	if err != nil {
		t.Fatalf("NewArbiterKeyringService() error = %v", err)
	}

	payment := &Payment{
		ID: "payment-concurrent",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "tb1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq0z2zqy",
		},
		EscrowState:   EscrowFunded,
		EscrowTimeout: time.Now().Add(-1 * time.Hour),
	}

	// Test concurrent signing and public key access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := keyring.SignTimeoutRefund(payment)
			if err != nil {
				t.Errorf("concurrent SignTimeoutRefund() error = %v", err)
			}
			_ = keyring.GetBTCPublicKey()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
