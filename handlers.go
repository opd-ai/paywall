// Package paywall provides HTTP handlers for Bitcoin payment processing
package paywall

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// renderPaymentPage generates and serves the HTML payment page for a given payment
// Parameters:
//   - w: HTTP response writer for sending the rendered page
//   - payment: Payment record containing address and amount information
//
// The page includes:
//   - Bitcoin payment address
//   - Payment amount in BTC
//   - Payment expiration time
//   - QR code for the payment address
//
// Error handling:
//   - QR code library loading failures result in QR code feature being disabled
//   - Template rendering failures return 500 Internal Server Error
//
// Related types: Payment, PaymentPageData, template.Template
func (p *Paywall) renderPaymentPage(w http.ResponseWriter, payment *Payment) {
	if invalidPayment := p.validatePaymentData(payment, w); invalidPayment {
		return
	}
	qrCodeJsBytes, err := QrcodeJs.ReadFile("static/qrcode.min.js")
	if err != nil {
		log.Println("QR Code error", err)
		http.Error(w, "QR Code Error", http.StatusInternalServerError)
		qrCodeJsBytes = []byte("")
		// don't return here, let people manually type in the address
		// !return
	}
	// Properly format the Javascript bytes for inclusion in the HTML template as a <script>
	qrCodeJsString := template.JS(qrCodeJsBytes)
	// Prepare template data
	data := PaymentPageData{
		BTCAddress: payment.Addresses[wallet.Bitcoin],
		AmountBTC:  payment.Amounts[wallet.Bitcoin],
		XMRAddress: payment.Addresses[wallet.Monero],
		AmountXMR:  payment.Amounts[wallet.Monero],
		ExpiresAt:  payment.ExpiresAt.Format(time.RFC3339),
		PaymentID:  payment.ID,
		QrcodeJs:   qrCodeJsString,
	}

	if err := p.template.Execute(w, data); err != nil {
		log.Println("Failed to render payment page:", err)
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
		return
	}
}

// validatePaymentData checks if the payment data is valid before rendering the payment page
// Parameters:
//   - payment: Payment record to validate containing address and amount information
//   - w: HTTP response writer for sending error responses
//
// Returns:
//   - bool: true if payment data is invalid, false if valid
//
// Validation checks:
//   - Payment object is not nil
//   - Payment amounts and addresses maps are not nil
//   - Payment amounts are greater than configured prices
//   - Configured prices are greater than 0
//
// Error handling:
//   - Returns 400 Bad Request for nil payment or invalid payment data
//   - Returns 500 Internal Server Error for invalid amounts or prices
func (p *Paywall) validatePaymentData(payment *Payment, w http.ResponseWriter) bool {
	const minBTC = 0.00001 // Dust limit
	const minXMR = 0.0001
	if payment == nil {
		http.Error(w, "Invalid payment", http.StatusBadRequest)
		return true
	}

	if payment.Amounts == nil || payment.Addresses == nil {
		http.Error(w, "Invalid payment data", http.StatusBadRequest)
		return true
	}

	// Check if prices are below minimum thresholds (dust limits)
	// Zero prices indicate disabled wallet types and should be allowed
	if (p.prices[wallet.Bitcoin] > 0 && p.prices[wallet.Bitcoin] <= minBTC) ||
		(p.prices[wallet.Monero] > 0 && p.prices[wallet.Monero] <= minXMR) {
		http.Error(w, "Failed to create payment", http.StatusInternalServerError)
		return true
	}
	return false
}
