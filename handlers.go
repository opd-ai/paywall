// handlers.go
package paywall

import (
	"net/http"
	"time"
)

// Move the rendering logic to a separate method
func (p *Paywall) renderPaymentPage(w http.ResponseWriter, payment *Payment) {
	// Prepare template data
	data := PaymentPageData{
		Address:   payment.Address,
		AmountBTC: payment.AmountBTC,
		ExpiresAt: payment.ExpiresAt.Format(time.RFC3339),
		PaymentID: payment.ID,
		QrcodeJs:  QrcodeJs,
	}

	// Set payment cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "payment_id",
		Value:    payment.ID,
		Expires:  payment.ExpiresAt,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	if err := p.template.Execute(w, data); err != nil {
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
	}
}
