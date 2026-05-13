// Package wallet defines multisig-specific types for wallet operations
package wallet

// MultisigConfig holds the configuration for multisig wallet operations.
// Used to configure m-of-n multisig addresses where m signatures are required from n total signers.
type MultisigConfig struct {
	// Enabled indicates whether multisig is enabled for this wallet
	Enabled bool `json:"enabled"`

	// RequiredSigs is the number of signatures required (m in m-of-n multisig)
	RequiredSigs int `json:"required_sigs"`

	// TotalSigners is the total number of signers (n in m-of-n multisig)
	TotalSigners int `json:"total_signers"`

	// PublicKeys contains all participant public keys in the multisig setup
	PublicKeys [][]byte `json:"public_keys"`

	// RedeemScript is the Bitcoin P2SH redeem script (Bitcoin-specific)
	// For Bitcoin, this contains the script used to redeem funds from the multisig address
	RedeemScript []byte `json:"redeem_script,omitempty"`

	// ScriptHash is the hash of the redeem script for verification (Bitcoin-specific)
	ScriptHash string `json:"script_hash,omitempty"`

	// MultisigInfo is the Monero multisig info export (Monero-specific)
	// For Monero, this contains the multisig setup information exported from the wallet
	MultisigInfo string `json:"multisig_info,omitempty"`
}

// MultisigMetadata contains per-address multisig data needed for transaction signing.
// This metadata is stored with each multisig address to enable signature creation and verification.
type MultisigMetadata struct {
	// Address is the multisig address
	Address string `json:"address"`

	// RedeemScript is the redeem script (Bitcoin) or setup info (Monero)
	RedeemScript []byte `json:"redeem_script"`

	// ScriptHash is the hash of the redeem script for verification
	ScriptHash string `json:"script_hash,omitempty"`

	// PublicKeys are the public keys used for this specific address
	PublicKeys [][]byte `json:"public_keys"`

	// RequiredSigs is the number of signatures required to spend from this address
	RequiredSigs int `json:"required_sigs"`
}
