// handlers.go
package paywall

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

// Move the rendering logic to a separate method
func (p *Paywall) renderPaymentPage(w http.ResponseWriter, payment *Payment) {
	qrCodeJsBytes, err := QrcodeJs.ReadFile("static/qrcode.min.js")
	if err != nil {
		log.Println("QR Code error", err)
		qrCodeJsBytes = []byte("")
	}
	// Properly format the Javascript bytes for inclusion in the HTML template as a <script>
	qrCodeJsString := template.JS(qrCodeJsBytes)
	// Prepare template data
	data := PaymentPageData{
		Address:   payment.Address,
		AmountBTC: payment.AmountBTC,
		ExpiresAt: payment.ExpiresAt.Format(time.RFC3339),
		PaymentID: payment.ID,
		QrcodeJs:  qrCodeJsString,
	}

	if err := p.template.Execute(w, data); err != nil {
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
	}
}
