// handlers.go
package paywall

import (
	"net/http"
	"time"
)

func (p *Paywall) servePaymentPage(w http.ResponseWriter, r *http.Request) {
	// Create new payment
	payment, err := p.CreatePayment()
	if err != nil {
		http.Error(w, "Failed to create payment", http.StatusInternalServerError)
		return
	}

	// Prepare template data
	data := PaymentPageData{
		Address:   payment.Address,
		AmountBTC: payment.AmountBTC,
		ExpiresAt: payment.ExpiresAt.Format(time.RFC3339),
		PaymentID: payment.ID,
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

	// Render payment page
	if err := p.template.Execute(w, data); err != nil {
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
		return
	}
}
