// middleware.go
package paywall

import (
	"net/http"
	"time"
)

// middleware.go
// middleware.go
func (p *Paywall) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First check for existing cookie
		cookie, err := r.Cookie("payment_id")
		if err == nil {
			// Cookie exists, verify payment
			payment, err := p.store.GetPayment(cookie.Value)
			if err == nil && payment != nil {
				if payment.Status == StatusConfirmed && time.Now().Before(payment.ExpiresAt) {
					// Payment confirmed and not expired, allow access
					next.ServeHTTP(w, r)
					return
				}
				if payment.Status == StatusPending && time.Now().Before(payment.ExpiresAt) {
					// Payment pending and not expired, show existing payment page
					p.renderPaymentPage(w, payment)
					return
				}
			}
		}

		// No valid payment found, create new one
		payment, err := p.CreatePayment()
		if err != nil {
			http.Error(w, "Failed to create payment", http.StatusInternalServerError)
			return
		}

		// Set cookie for new payment
		http.SetCookie(w, &http.Cookie{
			Name:     "payment_id",
			Value:    payment.ID,
			Expires:  payment.ExpiresAt,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})

		// Show payment page
		p.renderPaymentPage(w, payment)
	})
}

// Move the rendering logic to a separate method
func (p *Paywall) renderPaymentPage(w http.ResponseWriter, payment *Payment) {
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

	if err := p.template.Execute(w, data); err != nil {
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
	}
}
