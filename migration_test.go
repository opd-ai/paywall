package paywall

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

func TestMigratePayment(t *testing.T) {
	tests := []struct {
		name    string
		payment *Payment
		wantErr bool
		check   func(*testing.T, *Payment)
	}{
		{
			name:    "nil payment returns error",
			payment: nil,
			wantErr: true,
		},
		{
			name: "payment missing ID returns error",
			payment: &Payment{
				Addresses: map[wallet.WalletType]string{},
				Amounts:   map[wallet.WalletType]float64{},
			},
			wantErr: true,
		},
		{
			name: "payment missing Addresses returns error",
			payment: &Payment{
				ID:      "test-id",
				Amounts: map[wallet.WalletType]float64{},
			},
			wantErr: true,
		},
		{
			name: "payment missing Amounts returns error",
			payment: &Payment{
				ID:        "test-id",
				Addresses: map[wallet.WalletType]string{},
			},
			wantErr: true,
		},
		{
			name: "legacy payment migrates successfully",
			payment: &Payment{
				ID:        "legacy-payment",
				Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "bc1qtest"},
				Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				Status:    StatusPending,
				CreatedAt: time.Now(),
			},
			wantErr: false,
			check: func(t *testing.T, p *Payment) {
				if p.MultisigEnabled {
					t.Error("legacy payment should not have multisig enabled")
				}
			},
		},
		{
			name: "multisig payment initializes nil maps",
			payment: &Payment{
				ID:              "multisig-payment",
				Addresses:       map[wallet.WalletType]string{wallet.Bitcoin: "bc1qtest"},
				Amounts:         map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				MultisigEnabled: true,
				Status:          StatusPending,
				CreatedAt:       time.Now(),
				// MultisigMetadata, RequiredSignatures, Signatures are nil
			},
			wantErr: false,
			check: func(t *testing.T, p *Payment) {
				if p.MultisigMetadata == nil {
					t.Error("multisig payment should have initialized MultisigMetadata map")
				}
				if p.RequiredSignatures == nil {
					t.Error("multisig payment should have initialized RequiredSignatures map")
				}
				if p.Signatures == nil {
					t.Error("multisig payment should have initialized Signatures map")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MigratePayment(tt.payment)
			if (err != nil) != tt.wantErr {
				t.Errorf("MigratePayment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, tt.payment)
			}
		})
	}
}

func TestValidatePaymentJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *Payment)
	}{
		{
			name:    "invalid JSON returns error",
			json:    `{invalid json}`,
			wantErr: true,
		},
		{
			name:    "missing required fields returns error",
			json:    `{"id":"test"}`,
			wantErr: true,
		},
		{
			name: "legacy payment JSON validates successfully",
			json: `{
				"id": "legacy-001",
				"addresses": {"BTC": "bc1qtest"},
				"amounts": {"BTC": 0.001},
				"created_at": "2024-01-01T00:00:00Z",
				"expires_at": "2024-01-02T00:00:00Z",
				"status": "pending",
				"confirmations": 0
			}`,
			wantErr: false,
			check: func(t *testing.T, p *Payment) {
				if p.ID != "legacy-001" {
					t.Errorf("expected ID legacy-001, got %s", p.ID)
				}
				if p.MultisigEnabled {
					t.Error("legacy payment should not have multisig enabled")
				}
			},
		},
		{
			name: "multisig payment JSON validates successfully",
			json: `{
				"id": "multisig-001",
				"addresses": {"BTC": "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC"},
				"amounts": {"BTC": 0.001},
				"created_at": "2024-01-01T00:00:00Z",
				"expires_at": "2024-01-02T00:00:00Z",
				"status": "pending",
				"confirmations": 0,
				"multisig_enabled": true,
				"required_signatures": {"BTC": 2}
			}`,
			wantErr: false,
			check: func(t *testing.T, p *Payment) {
				if !p.MultisigEnabled {
					t.Error("expected multisig to be enabled")
				}
				if p.MultisigMetadata == nil {
					t.Error("expected MultisigMetadata to be initialized")
				}
				if p.RequiredSignatures[wallet.Bitcoin] != 2 {
					t.Errorf("expected required signatures 2, got %d", p.RequiredSignatures[wallet.Bitcoin])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment, err := ValidatePaymentJSON([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePaymentJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, payment)
			}
		})
	}
}

func TestIsLegacyPayment(t *testing.T) {
	tests := []struct {
		name    string
		payment *Payment
		want    bool
	}{
		{
			name:    "nil payment returns false",
			payment: nil,
			want:    false,
		},
		{
			name: "payment without multisig fields is legacy",
			payment: &Payment{
				ID:        "legacy",
				Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "bc1qtest"},
				Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
			},
			want: true,
		},
		{
			name: "payment with multisig enabled is not legacy",
			payment: &Payment{
				ID:              "multisig",
				Addresses:       map[wallet.WalletType]string{wallet.Bitcoin: "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC"},
				Amounts:         map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				MultisigEnabled: true,
			},
			want: false,
		},
		{
			name: "payment with multisig metadata is not legacy",
			payment: &Payment{
				ID:        "multisig",
				Addresses: map[wallet.WalletType]string{wallet.Bitcoin: "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC"},
				Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				MultisigMetadata: map[wallet.WalletType]*wallet.MultisigMetadata{
					wallet.Bitcoin: {},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLegacyPayment(tt.payment); got != tt.want {
				t.Errorf("IsLegacyPayment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizePayment(t *testing.T) {
	tests := []struct {
		name    string
		payment *Payment
		check   func(*testing.T, *Payment)
	}{
		{
			name:    "nil payment doesn't panic",
			payment: nil,
			check:   func(t *testing.T, p *Payment) {},
		},
		{
			name: "multisig disabled payment clears multisig fields",
			payment: &Payment{
				ID:              "test",
				Addresses:       map[wallet.WalletType]string{wallet.Bitcoin: "bc1qtest"},
				Amounts:         map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				MultisigEnabled: false,
				MultisigMetadata: map[wallet.WalletType]*wallet.MultisigMetadata{
					wallet.Bitcoin: {},
				},
				RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
				Signatures:         map[wallet.WalletType][]SignatureData{},
			},
			check: func(t *testing.T, p *Payment) {
				if p.MultisigMetadata != nil {
					t.Error("expected MultisigMetadata to be nil after normalization")
				}
				if p.RequiredSignatures != nil {
					t.Error("expected RequiredSignatures to be nil after normalization")
				}
				if p.Signatures != nil {
					t.Error("expected Signatures to be nil after normalization")
				}
			},
		},
		{
			name: "multisig enabled payment preserves multisig fields",
			payment: &Payment{
				ID:              "test",
				Addresses:       map[wallet.WalletType]string{wallet.Bitcoin: "3QJmV3qfvL9SuYo34YihAf3sRCW3qSinyC"},
				Amounts:         map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
				MultisigEnabled: true,
				MultisigMetadata: map[wallet.WalletType]*wallet.MultisigMetadata{
					wallet.Bitcoin: {},
				},
				RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
			},
			check: func(t *testing.T, p *Payment) {
				if p.MultisigMetadata == nil {
					t.Error("expected MultisigMetadata to be preserved")
				}
				if p.RequiredSignatures == nil {
					t.Error("expected RequiredSignatures to be preserved")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NormalizePayment(tt.payment)
			tt.check(t, tt.payment)
		})
	}
}

// TestBackwardCompatibility ensures old payment JSON can be loaded and used
func TestBackwardCompatibility(t *testing.T) {
	// Simulate a payment created before multisig support
	legacyJSON := `{
		"id": "pre-multisig-payment",
		"addresses": {
			"BTC": "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq"
		},
		"amounts": {
			"BTC": 0.001
		},
		"created_at": "2024-01-01T00:00:00Z",
		"expires_at": "2024-01-02T00:00:00Z",
		"status": "pending",
		"confirmations": 0
	}`

	// Unmarshal the legacy payment
	var payment Payment
	if err := json.Unmarshal([]byte(legacyJSON), &payment); err != nil {
		t.Fatalf("failed to unmarshal legacy payment: %v", err)
	}

	// Migrate the payment
	if err := MigratePayment(&payment); err != nil {
		t.Fatalf("failed to migrate legacy payment: %v", err)
	}

	// Verify the payment is usable
	if payment.ID != "pre-multisig-payment" {
		t.Errorf("expected ID pre-multisig-payment, got %s", payment.ID)
	}
	if payment.MultisigEnabled {
		t.Error("legacy payment should not have multisig enabled")
	}
	if !IsLegacyPayment(&payment) {
		t.Error("expected payment to be identified as legacy")
	}
}
