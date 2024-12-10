// middleware.go
package paywall

import (
	"context"
	"net/http"
	"time"
)

func (p *Paywall) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for payment header
		paymentMsg := r.Header.Get("X-Payment-Message")
		paymentSig := r.Header.Get("X-Payment-Signature")

		// If no payment headers, show payment page
		if paymentMsg == "" || paymentSig == "" {
			p.servePaymentPage(w, r)
			return
		}

		// Check if payment is already verified
		p.mu.RLock()
		payment, exists := p.payments[paymentMsg]
		p.mu.RUnlock()

		if exists {
			if time.Now().After(payment.ExpiresAt) {
				p.servePaymentPage(w, r)
				return
			}

			if payment.Verified {
				ctx := context.WithValue(r.Context(), "payment", payment)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Verify new payment
		verified, err := p.verifyPayment(paymentMsg, paymentSig)
		if err != nil || !verified {
			p.servePaymentPage(w, r)
			return
		}

		// Get updated payment after verification
		p.mu.RLock()
		payment = p.payments[paymentMsg]
		p.mu.RUnlock()

		ctx := context.WithValue(r.Context(), "payment", payment)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
