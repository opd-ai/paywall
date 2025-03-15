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
	if p.prices[wallet.Bitcoin] <= 0 || p.prices[wallet.Monero] <= 0 {
		http.Error(w, "Invalid payment amount", http.StatusBadRequest)
	}
	qrCodeJsBytes, err := QrcodeJs.ReadFile("static/qrcode.min.js")
	if err != nil {
		log.Println("QR Code error", err)
		qrCodeJsBytes = []byte("")
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
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
	}
}
