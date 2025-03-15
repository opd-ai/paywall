// Package paywall provides Bitcoin payment protection for HTTP handlers
package paywall

import (
	"net/http"
	"time"
)

// Middleware wraps an http.Handler to enforce Bitcoin payment requirements
//
// Parameters:
//   - next: The HTTP handler to protect with payment verification
//
// Returns:
//   - http.Handler: A handler that checks payment status before allowing access
//
// Flow:
//  1. Checks for existing payment_id cookie
//  2. If cookie exists:
//     - Verifies payment status and expiration
//     - Allows access for confirmed, unexpired payments
//     - Shows payment page for pending, unexpired payments
//  3. If no valid payment:
//     - Creates new payment
//     - Sets secure payment_id cookie
//     - Shows payment page
//
// Error Handling:
//   - Returns 500 Internal Server Error if payment creation fails
//   - Invalid/expired payments result in new payment creation
//
// Security:
//   - Uses secure, HTTP-only cookies with SameSite=Strict
//   - Payment IDs are cryptographically random
//   - Validates payment status and expiration
//
// Related types: Payment, PaymentStore, PaymentStatus
func (p *Paywall) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First check for existing cookie
		cookie, err := r.Cookie("payment_id")
		if err == nil {
			// Cookie exists, verify payment
			// update expiration +15 minutes
			cookie.Expires = time.Now().Add(15 * time.Minute)
			http.SetCookie(w, cookie)
			payment, err := p.Store.GetPayment(cookie.Value)
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
		cookieExpiration := time.Now().Add(15 * time.Minute)

		// Set cookie for new payment
		http.SetCookie(w, &http.Cookie{
			Name:     "payment_id",
			Value:    payment.ID,
			Expires:  cookieExpiration,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		})

		// Show payment page
		p.renderPaymentPage(w, payment)
	})
}

func (p *Paywall) MiddlewareFunc(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(p.Middleware(next).(http.HandlerFunc))
}

func (p *Paywall) MiddlewareFuncFunc(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(p.Middleware(next).(http.HandlerFunc))
}
