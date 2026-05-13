package paywall

import (
	"encoding/json"
	"fmt"

	"github.com/opd-ai/paywall/wallet"
)

// MigratePayment ensures a payment structure is compatible with the current schema.
// This function handles backward compatibility for payments created before multisig support.
// Zero-value multisig fields are initialized to prevent nil pointer dereferences.
//
// Related types: Payment
func MigratePayment(p *Payment) error {
	if p == nil {
		return fmt.Errorf("cannot migrate nil payment")
	}

	// Initialize multisig maps if they're nil and multisig is enabled
	if p.MultisigEnabled {
		if p.MultisigMetadata == nil {
			p.MultisigMetadata = make(map[wallet.WalletType]*wallet.MultisigMetadata)
		}
		if p.RequiredSignatures == nil {
			p.RequiredSignatures = make(map[wallet.WalletType]int)
		}
		if p.Signatures == nil {
			p.Signatures = make(map[wallet.WalletType][]SignatureData)
		}
	}

	// Validate required fields exist regardless of version
	if p.ID == "" {
		return fmt.Errorf("payment missing required ID field")
	}
	if p.Addresses == nil {
		return fmt.Errorf("payment %s missing required Addresses map", p.ID)
	}
	if p.Amounts == nil {
		return fmt.Errorf("payment %s missing required Amounts map", p.ID)
	}

	return nil
}

// ValidatePaymentJSON validates that JSON data can be unmarshaled into a Payment struct.
// This is useful for testing backward compatibility with legacy payment data.
// Returns the unmarshaled payment if successful, error otherwise.
//
// Related types: Payment
func ValidatePaymentJSON(data []byte) (*Payment, error) {
	var p Payment
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid payment JSON: %w", err)
	}

	if err := MigratePayment(&p); err != nil {
		return nil, fmt.Errorf("payment migration failed: %w", err)
	}

	return &p, nil
}

// IsLegacyPayment returns true if a payment was created before multisig support.
// Legacy payments have no multisig fields set.
//
// Related types: Payment
func IsLegacyPayment(p *Payment) bool {
	if p == nil {
		return false
	}
	return !p.MultisigEnabled &&
		p.MultisigMetadata == nil &&
		p.RequiredSignatures == nil &&
		p.Signatures == nil
}

// NormalizePayment ensures consistent representation of payment data.
// This includes:
// - Setting nil multisig maps to empty maps when multisig is disabled
// - Removing empty multisig metadata when not needed
//
// Related types: Payment
func NormalizePayment(p *Payment) {
	if p == nil {
		return
	}

	// If multisig is disabled, ensure multisig fields are consistently nil
	if !p.MultisigEnabled {
		p.MultisigMetadata = nil
		p.RequiredSignatures = nil
		p.Signatures = nil
	}
}
