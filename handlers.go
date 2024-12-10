// handlers.go
package paywall

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
)

func (p *Paywall) servePaymentPage(w http.ResponseWriter, r *http.Request) {
	paymentID := generatePaymentID()

	data := PaymentPageData{
		Address:   p.wallet.GetAddress(),
		Amount:    p.priceInBTC,
		ReturnURL: r.URL.String(),
		PaymentID: paymentID,
		Message:   fmt.Sprintf("Payment for %s: %s", r.URL.Path, paymentID),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusPaymentRequired)

	if err := p.template.Execute(w, data); err != nil {
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
		return
	}
}

func generatePaymentID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
