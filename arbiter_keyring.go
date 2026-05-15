package paywall

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/opd-ai/paywall/wallet"
)

// ArbiterKeyringService provides production implementation of ArbiterSigner
// It securely stores arbiter private keys and generates signatures for timeout refunds
type ArbiterKeyringService struct {
	// btcPrivateKey is the arbiter's Bitcoin private key for ECDSA signing
	btcPrivateKey *btcec.PrivateKey
	// xmrPrivateKey would store Monero private key (future enhancement)
	xmrPrivateKey []byte
	// mu protects concurrent access to private keys
	mu sync.RWMutex
	// arbiterID is a unique identifier for this arbiter
	arbiterID string
}

// NewArbiterKeyringService creates a new arbiter keyring with provided private keys
// Parameters:
//   - btcPrivateKey: Bitcoin private key for ECDSA signatures (required for BTC escrows)
//   - xmrPrivateKey: Monero private key for EdDSA signatures (optional, for future XMR support)
//   - arbiterID: Unique identifier for this arbiter instance
//
// Returns error if btcPrivateKey is nil (minimum requirement for operation)
func NewArbiterKeyringService(btcPrivateKey *btcec.PrivateKey, xmrPrivateKey []byte, arbiterID string) (*ArbiterKeyringService, error) {
	if btcPrivateKey == nil {
		return nil, fmt.Errorf("btcPrivateKey is required")
	}
	if arbiterID == "" {
		arbiterID = "arbiter-default"
	}

	return &ArbiterKeyringService{
		btcPrivateKey: btcPrivateKey,
		xmrPrivateKey: xmrPrivateKey,
		arbiterID:     arbiterID,
	}, nil
}

// SignTimeoutRefund implements the ArbiterSigner interface
// It signs a timeout refund transaction for the given payment
//
// The signature process:
//  1. Determines wallet type from payment data
//  2. Creates a canonical message to sign: "timeout_refund|<payment_id>|<escrow_timeout>"
//  3. Signs the message hash using ECDSA (Bitcoin) or EdDSA (Monero)
//  4. Returns properly formatted SignatureData with nonce for replay protection
//
// Returns error if the payment is invalid, missing required fields, or signing fails
func (a *ArbiterKeyringService) SignTimeoutRefund(payment *Payment) (*SignatureData, error) {
	if payment == nil {
		return nil, fmt.Errorf("payment is nil")
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// Determine which wallet type to use (prioritize BTC if multiple addresses exist)
	walletType := wallet.Bitcoin
	if len(payment.Addresses) > 0 {
		// Check which addresses are present
		if _, hasBTC := payment.Addresses[wallet.Bitcoin]; !hasBTC {
			if _, hasXMR := payment.Addresses[wallet.Monero]; hasXMR {
				walletType = wallet.Monero
			}
		}
	}

	// Generate unique nonce for replay protection
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Create canonical message for signing
	// Format: "timeout_refund|<payment_id>|<escrow_timeout_unix>"
	message := fmt.Sprintf("timeout_refund|%s|%d", payment.ID, payment.EscrowTimeout.Unix())
	messageHash := sha256.Sum256([]byte(message))

	var signature []byte
	var publicKey []byte

	switch walletType {
	case wallet.Bitcoin:
		// Sign using Bitcoin ECDSA
		if a.btcPrivateKey == nil {
			return nil, fmt.Errorf("bitcoin private key not configured")
		}

		// Create ECDSA signature
		ecdsaSig := ecdsa.Sign(a.btcPrivateKey, messageHash[:])
		// Serialize signature in DER format with SIGHASH_ALL appended
		signature = append(ecdsaSig.Serialize(), byte(0x01))
		// Get compressed public key
		publicKey = a.btcPrivateKey.PubKey().SerializeCompressed()

	case wallet.Monero:
		// Monero EdDSA signing would go here
		// For now, return error indicating XMR refunds not yet supported
		return nil, fmt.Errorf("monero timeout refunds not yet implemented")

	default:
		return nil, fmt.Errorf("unsupported wallet type: %s", walletType)
	}

	// Create and return signature data
	return &SignatureData{
		SignerID:  a.arbiterID,
		Role:      RoleArbiter,
		Signature: signature,
		PublicKey: publicKey,
		SignedAt:  time.Now(),
		Nonce:     nonce,
		PaymentID: payment.ID,
	}, nil
}

// GetBTCPublicKey returns the arbiter's Bitcoin public key
// This can be used to verify the arbiter's identity before processing signatures
func (a *ArbiterKeyringService) GetBTCPublicKey() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.btcPrivateKey == nil {
		return nil
	}
	return a.btcPrivateKey.PubKey().SerializeCompressed()
}

// GetXMRPublicKey returns the arbiter's Monero public key (future enhancement)
func (a *ArbiterKeyringService) GetXMRPublicKey() []byte {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// Placeholder for Monero public key derivation
	return a.xmrPrivateKey
}

// NewArbiterKeyringFromSeed creates a keyring from a 32-byte seed
// This is useful for deterministic key generation from a backed-up seed
func NewArbiterKeyringFromSeed(seed []byte, arbiterID string) (*ArbiterKeyringService, error) {
	if len(seed) != 32 {
		return nil, fmt.Errorf("seed must be exactly 32 bytes, got %d", len(seed))
	}

	// Derive Bitcoin private key from seed
	btcPrivKey, _ := btcec.PrivKeyFromBytes(seed)

	return NewArbiterKeyringService(btcPrivKey, nil, arbiterID)
}
