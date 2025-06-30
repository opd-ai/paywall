// Package paywall provides HTTP handlers for Bitcoin payment processing
package paywall

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// mockTemplate is a simple template for testing
const mockTemplateContent = `
<!DOCTYPE html>
<html>
<head><title>Payment</title></head>
<body>
	<h1>Payment Required</h1>
	<p>BTC Address: {{.BTCAddress}}</p>
	<p>BTC Amount: {{.AmountBTC}}</p>
	<p>XMR Address: {{.XMRAddress}}</p>
	<p>XMR Amount: {{.AmountXMR}}</p>
	<p>Expires: {{.ExpiresAt}}</p>
	<p>Payment ID: {{.PaymentID}}</p>
	<script>{{.QrcodeJs}}</script>
</body>
</html>
`

// createTestPaywall creates a Paywall instance for testing
func createTestPaywall() *Paywall {
	tmpl, _ := template.New("payment").Parse(mockTemplateContent)
	return &Paywall{
		template: tmpl,
		prices: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
			wallet.Monero:  0.01,
		},
	}
}

// createHandlerTestPayment creates a valid Payment for testing handlers
func createHandlerTestPayment() *Payment {
	return &Payment{
		ID: "test-payment-123",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			wallet.Monero:  "49gCuLWHMxCSDSDKKKSDK5QGefi2DMPTfTL5SLmv7DivfNa",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
			wallet.Monero:  0.01,
		},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    StatusPending,
	}
}

func TestPaywall_renderPaymentPage_Success(t *testing.T) {
	tests := []struct {
		name        string
		payment     *Payment
		wantStatus  int
		wantContent []string
	}{
		{
			name:       "Valid payment renders correctly",
			payment:    createHandlerTestPayment(),
			wantStatus: http.StatusOK,
			wantContent: []string{
				"Payment Required",
				"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
				"0.001",
				"49gCuLWHMxCSDSDKKKSDK5QGefi2DMPTfTL5SLmv7DivfNa",
				"0.01",
				"test-payment-123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paywall := createTestPaywall()
			recorder := httptest.NewRecorder()

			paywall.renderPaymentPage(recorder, tt.payment)

			if recorder.Code != tt.wantStatus {
				t.Errorf("renderPaymentPage() status = %v, want %v", recorder.Code, tt.wantStatus)
			}

			body := recorder.Body.String()
			for _, content := range tt.wantContent {
				if !strings.Contains(body, content) {
					t.Errorf("renderPaymentPage() body missing content %q", content)
				}
			}
		})
	}
}

func TestPaywall_renderPaymentPage_NilPayment(t *testing.T) {
	paywall := createTestPaywall()
	recorder := httptest.NewRecorder()

	paywall.renderPaymentPage(recorder, nil)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("renderPaymentPage() with nil payment status = %v, want %v", recorder.Code, http.StatusBadRequest)
	}

	if !strings.Contains(recorder.Body.String(), "Invalid payment") {
		t.Error("renderPaymentPage() should return 'Invalid payment' error message")
	}
}

func TestPaywall_renderPaymentPage_NilMaps(t *testing.T) {
	tests := []struct {
		name    string
		payment *Payment
	}{
		{
			name: "Nil Addresses map",
			payment: &Payment{
				ID:        "test-123",
				Addresses: nil,
				Amounts: map[wallet.WalletType]float64{
					wallet.Bitcoin: 0.001,
				},
			},
		},
		{
			name: "Nil Amounts map",
			payment: &Payment{
				ID: "test-123",
				Addresses: map[wallet.WalletType]string{
					wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
				},
				Amounts: nil,
			},
		},
		{
			name: "Both maps nil",
			payment: &Payment{
				ID:        "test-123",
				Addresses: nil,
				Amounts:   nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paywall := createTestPaywall()
			recorder := httptest.NewRecorder()

			paywall.renderPaymentPage(recorder, tt.payment)

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("renderPaymentPage() with %s status = %v, want %v", tt.name, recorder.Code, http.StatusBadRequest)
			}

			if !strings.Contains(recorder.Body.String(), "Invalid payment data") {
				t.Error("renderPaymentPage() should return 'Invalid payment data' error message")
			}
		})
	}
}

func TestPaywall_renderPaymentPage_TemplateError(t *testing.T) {
	// Create paywall with invalid template that will fail during execution
	invalidTemplate, _ := template.New("invalid").Parse("{{.NonExistentField}}")
	paywall := &Paywall{
		template: invalidTemplate,
		prices: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
			wallet.Monero:  0.01,
		},
	}

	recorder := httptest.NewRecorder()
	payment := createHandlerTestPayment()

	paywall.renderPaymentPage(recorder, payment)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("renderPaymentPage() with template error status = %v, want %v", recorder.Code, http.StatusInternalServerError)
	}

	if !strings.Contains(recorder.Body.String(), "Failed to render payment page") {
		t.Error("renderPaymentPage() should return 'Failed to render payment page' error message")
	}
}

func TestPaywall_validatePaymentData_Success(t *testing.T) {
	paywall := createTestPaywall()
	recorder := httptest.NewRecorder()
	payment := createHandlerTestPayment()

	invalid := paywall.validatePaymentData(payment, recorder)

	if invalid {
		t.Error("validatePaymentData() should return false for valid payment")
	}

	if recorder.Code != 200 {
		t.Errorf("validatePaymentData() should not write error response for valid payment, got status %v", recorder.Code)
	}
}

func TestPaywall_validatePaymentData_NilPayment(t *testing.T) {
	paywall := createTestPaywall()
	recorder := httptest.NewRecorder()

	invalid := paywall.validatePaymentData(nil, recorder)

	if !invalid {
		t.Error("validatePaymentData() should return true for nil payment")
	}

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("validatePaymentData() status = %v, want %v", recorder.Code, http.StatusBadRequest)
	}

	if !strings.Contains(recorder.Body.String(), "Invalid payment") {
		t.Error("validatePaymentData() should return 'Invalid payment' error message")
	}
}

func TestPaywall_validatePaymentData_InvalidData(t *testing.T) {
	tests := []struct {
		name    string
		payment *Payment
	}{
		{
			name: "Nil Addresses",
			payment: &Payment{
				ID:        "test-123",
				Addresses: nil,
				Amounts: map[wallet.WalletType]float64{
					wallet.Bitcoin: 0.001,
				},
			},
		},
		{
			name: "Nil Amounts",
			payment: &Payment{
				ID: "test-123",
				Addresses: map[wallet.WalletType]string{
					wallet.Bitcoin: "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
				},
				Amounts: nil,
			},
		},
		{
			name: "Both nil",
			payment: &Payment{
				ID:        "test-123",
				Addresses: nil,
				Amounts:   nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paywall := createTestPaywall()
			recorder := httptest.NewRecorder()

			invalid := paywall.validatePaymentData(tt.payment, recorder)

			if !invalid {
				t.Errorf("validatePaymentData() should return true for %s", tt.name)
			}

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("validatePaymentData() status = %v, want %v", recorder.Code, http.StatusBadRequest)
			}

			if !strings.Contains(recorder.Body.String(), "Invalid payment data") {
				t.Error("validatePaymentData() should return 'Invalid payment data' error message")
			}
		})
	}
}

func TestPaywall_validatePaymentData_PriceValidation(t *testing.T) {
	tests := []struct {
		name      string
		btcPrice  float64
		xmrPrice  float64
		wantError bool
	}{
		{
			name:      "Valid prices",
			btcPrice:  0.001,
			xmrPrice:  0.01,
			wantError: false,
		},
		{
			name:      "BTC price at dust limit",
			btcPrice:  0.00001,
			xmrPrice:  0.0001,
			wantError: true,
		},
		{
			name:      "XMR price at minimum",
			btcPrice:  0.001,
			xmrPrice:  0.0001,
			wantError: true,
		},
		{
			name:      "Both prices at limits",
			btcPrice:  0.00001,
			xmrPrice:  0.0001,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paywall := &Paywall{
				prices: map[wallet.WalletType]float64{
					wallet.Bitcoin: tt.btcPrice,
					wallet.Monero:  tt.xmrPrice,
				},
			}
			recorder := httptest.NewRecorder()
			payment := createHandlerTestPayment()

			invalid := paywall.validatePaymentData(payment, recorder)

			if tt.wantError && !invalid {
				t.Errorf("validatePaymentData() should return true for invalid prices")
			}
			if !tt.wantError && invalid {
				t.Errorf("validatePaymentData() should return false for valid prices")
			}

			if tt.wantError {
				if recorder.Code != http.StatusInternalServerError {
					t.Errorf("validatePaymentData() status = %v, want %v", recorder.Code, http.StatusInternalServerError)
				}
				if !strings.Contains(recorder.Body.String(), "Failed to create payment") {
					t.Error("validatePaymentData() should return 'Failed to create payment' error message")
				}
			}
		})
	}
}

func TestPaywall_validatePaymentData_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		payment      *Payment
		btcPrice     float64
		xmrPrice     float64
		wantInvalid  bool
		wantStatus   int
		wantErrorMsg string
	}{
		{
			name:        "Valid payment and prices",
			payment:     createHandlerTestPayment(),
			btcPrice:    0.001,
			xmrPrice:    0.01,
			wantInvalid: false,
			wantStatus:  200,
		},
		{
			name:         "Nil payment",
			payment:      nil,
			btcPrice:     0.001,
			xmrPrice:     0.01,
			wantInvalid:  true,
			wantStatus:   http.StatusBadRequest,
			wantErrorMsg: "Invalid payment",
		},
		{
			name: "Nil addresses map",
			payment: &Payment{
				ID:        "test",
				Addresses: nil,
				Amounts:   map[wallet.WalletType]float64{wallet.Bitcoin: 0.001},
			},
			btcPrice:     0.001,
			xmrPrice:     0.01,
			wantInvalid:  true,
			wantStatus:   http.StatusBadRequest,
			wantErrorMsg: "Invalid payment data",
		},
		{
			name:         "Price validation failure",
			payment:      createHandlerTestPayment(),
			btcPrice:     0.00001, // At dust limit
			xmrPrice:     0.0001,  // At minimum
			wantInvalid:  true,
			wantStatus:   http.StatusInternalServerError,
			wantErrorMsg: "Failed to create payment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paywall := &Paywall{
				prices: map[wallet.WalletType]float64{
					wallet.Bitcoin: tt.btcPrice,
					wallet.Monero:  tt.xmrPrice,
				},
			}
			recorder := httptest.NewRecorder()

			invalid := paywall.validatePaymentData(tt.payment, recorder)

			if invalid != tt.wantInvalid {
				t.Errorf("validatePaymentData() = %v, want %v", invalid, tt.wantInvalid)
			}

			if tt.wantStatus != 200 && recorder.Code != tt.wantStatus {
				t.Errorf("validatePaymentData() status = %v, want %v", recorder.Code, tt.wantStatus)
			}

			if tt.wantErrorMsg != "" && !strings.Contains(recorder.Body.String(), tt.wantErrorMsg) {
				t.Errorf("validatePaymentData() error message should contain %q", tt.wantErrorMsg)
			}
		})
	}
}

// Test edge cases and boundary conditions
func TestPaywall_validatePaymentData_EdgeCases(t *testing.T) {
	t.Run("Empty payment ID", func(t *testing.T) {
		paywall := createTestPaywall()
		recorder := httptest.NewRecorder()
		payment := createHandlerTestPayment()
		payment.ID = ""

		invalid := paywall.validatePaymentData(payment, recorder)

		// Should still be valid as ID is not validated in this function
		if invalid {
			t.Error("validatePaymentData() should not validate ID field")
		}
	})

	t.Run("Empty address maps", func(t *testing.T) {
		paywall := createTestPaywall()
		recorder := httptest.NewRecorder()
		payment := &Payment{
			ID:        "test",
			Addresses: map[wallet.WalletType]string{},
			Amounts:   map[wallet.WalletType]float64{},
		}

		invalid := paywall.validatePaymentData(payment, recorder)

		// Should be valid as empty maps are not nil
		if invalid {
			t.Error("validatePaymentData() should accept empty maps")
		}
	})

	t.Run("Zero prices", func(t *testing.T) {
		paywall := &Paywall{
			prices: map[wallet.WalletType]float64{
				wallet.Bitcoin: 0.0,
				wallet.Monero:  0.0,
			},
		}
		recorder := httptest.NewRecorder()
		payment := createHandlerTestPayment()

		invalid := paywall.validatePaymentData(payment, recorder)

		// Should not trigger price validation error since both are below limits
		if invalid {
			t.Error("validatePaymentData() should not fail with zero prices")
		}
	})
}

// Benchmark tests for performance validation
func BenchmarkPaywall_validatePaymentData(b *testing.B) {
	paywall := createTestPaywall()
	payment := createHandlerTestPayment()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		paywall.validatePaymentData(payment, recorder)
	}
}

func BenchmarkPaywall_renderPaymentPage(b *testing.B) {
	paywall := createTestPaywall()
	payment := createHandlerTestPayment()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		paywall.renderPaymentPage(recorder, payment)
	}
}
