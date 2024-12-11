// middleware.go
package paywall

import (
	"net/http"
	"time"
)

func (p *Paywall) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for payment ID in cookie
		cookie, err := r.Cookie("payment_id")
		if err != nil {
			p.servePaymentPage(w, r)
			return
		}

		// Verify payment
		payment, err := p.store.GetPayment(cookie.Value)
		if err != nil {
			p.servePaymentPage(w, r)
			return
		}

		if payment.Status != StatusConfirmed || time.Now().After(payment.ExpiresAt) {
			p.servePaymentPage(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
