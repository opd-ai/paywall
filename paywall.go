// paywall.go
package paywall

import (
	"embed"
	"fmt"
	"html/template"
	"sync"

	"github.com/opd-ai/paywall/wallet"
)

//go:embed templates/payment.html
var templateFS embed.FS

type Paywall struct {
	wallet     *wallet.BitcoinWallet
	payments   map[string]Payment
	mu         sync.RWMutex
	priceInBTC float64
	template   *template.Template
}

func NewPaywall(priceInBTC float64, testnet bool) (*Paywall, error) {
	w, err := wallet.NewWallet(testnet)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	tmpl, err := template.ParseFS(templateFS, "templates/payment.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &Paywall{
		wallet:     w,
		payments:   make(map[string]Payment),
		priceInBTC: priceInBTC,
		template:   tmpl,
	}, nil
}
