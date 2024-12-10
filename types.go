package paywall

import "time"

// Payment represents a verified payment record
type Payment struct {
	Amount    float64   // Amount in BTC
	Timestamp time.Time // When the payment was verified
	Message   string    // Payment message used for verification
	Signature string    // Payment signature
	Verified  bool      // Payment verification status
	ExpiresAt time.Time // When the payment expires
}

// PaymentPageData contains the data needed to render the payment page template
type PaymentPageData struct {
	Address   string  // Bitcoin address to receive payment
	Amount    float64 // Amount in BTC
	ReturnURL string  // URL to return to after payment
	PaymentID string  // Unique payment identifier
	Message   string  // Payment message to sign
}
