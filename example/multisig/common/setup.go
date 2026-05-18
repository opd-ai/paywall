// Package common provides shared utilities for multisig examples
package common

import (
	"fmt"
	"time"

	"github.com/opd-ai/paywall"
)

// MockSignature creates a placeholder signature for examples.
// In production, use actual cryptographic signing.
func MockSignature(role paywall.MultisigRole, pubKey []byte) *paywall.SignatureData {
	return &paywall.SignatureData{
		SignerID:  fmt.Sprintf("%s-signer", string(role)),
		Role:      role,
		Signature: []byte("mock-signature-" + string(role)),
		PublicKey: pubKey,
		SignedAt:  time.Now(),
	}
}

// GenerateExamplePubKeys creates example public keys for buyer, seller, and arbiter.
// In production, these would come from actual participants.
func GenerateExamplePubKeys() (buyer, seller, arbiter []byte) {
	buyer = make([]byte, 33)
	seller = make([]byte, 33)
	arbiter = make([]byte, 33)

	// Fill with example data (in production, use real compressed public keys)
	copy(buyer, []byte{0x02})   // Buyer's compressed pubkey
	copy(seller, []byte{0x03})  // Seller's compressed pubkey
	copy(arbiter, []byte{0x04}) // Arbiter's compressed pubkey

	return buyer, seller, arbiter
}

// GenerateSinglePubKey creates a single example public key with the given prefix.
// In production, use real compressed public keys.
func GenerateSinglePubKey(prefix byte) []byte {
	pubKey := make([]byte, 33)
	copy(pubKey, []byte{prefix})
	return pubKey
}
