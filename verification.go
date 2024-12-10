// verification.go
package paywall

import (
	"encoding/hex"
	"fmt"
	"time"
)

func (p *Paywall) verifyPayment(message, signatureHex string) (bool, error) {
	// Check existing verified payment
	p.mu.RLock()
	if payment, exists := p.payments[message]; exists && payment.Verified {
		if time.Now().Before(payment.ExpiresAt) {
			p.mu.RUnlock()
			return true, nil
		}
	}
	p.mu.RUnlock()

	// Decode signature
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature format: %w", err)
	}

	// Verify signature
	valid, err := p.wallet.VerifyMessage([]byte(message), signature)
	if err != nil {
		return false, fmt.Errorf("signature verification failed: %w", err)
	}

	if valid {
		p.mu.Lock()
		p.payments[message] = Payment{
			Amount:    p.priceInBTC,
			Timestamp: time.Now(),
			Message:   message,
			Signature: signatureHex,
			Verified:  true,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		p.mu.Unlock()
	}

	return valid, nil
}
