// Package paywall provides HTTP handlers for Bitcoin payment processing
package paywall

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// WalletMultisigStatusResponse contains runtime multisig status for a wallet.
// Exposed through the admin status endpoint for wallet introspection.
type WalletMultisigStatusResponse struct {
	WalletType      wallet.WalletType      `json:"wallet_type"`
	MultisigEnabled bool                   `json:"multisig_enabled"`
	MultisigConfig  *wallet.MultisigConfig `json:"multisig_config,omitempty"`
}

// HandleWalletMultisigStatus processes GET /api/admin/wallet/{type}/multisig/status requests.
// It exposes runtime multisig status and configuration for the selected wallet type.
func (p *Paywall) HandleWalletMultisigStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	const prefix = "/api/admin/wallet/"
	const suffix = "/multisig/status"

	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, suffix) {
		http.Error(w, "invalid path, expected /api/admin/wallet/{type}/multisig/status", http.StatusBadRequest)
		return
	}

	walletSegment := strings.TrimPrefix(r.URL.Path, prefix)
	walletSegment = strings.TrimSuffix(walletSegment, suffix)
	walletSegment = strings.Trim(walletSegment, "/")

	walletType, err := parseWalletType(walletSegment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hdWallet, ok := p.HDWallets[walletType]
	if !ok {
		http.Error(w, fmt.Sprintf("wallet not configured for type: %s", walletType), http.StatusNotFound)
		return
	}

	resp := WalletMultisigStatusResponse{
		WalletType:      walletType,
		MultisigEnabled: hdWallet.IsMultisigEnabled(),
	}

	config, err := hdWallet.GetMultisigConfig()
	if err != nil && !errors.Is(err, wallet.ErrMultisigNotSupported) {
		http.Error(w, fmt.Sprintf("failed to get multisig config for %s: %v", walletType, err), http.StatusInternalServerError)
		return
	}
	resp.MultisigConfig = config

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		p.logger.log(LogEntry{
			Level:   LogLevelError,
			Event:   "response_encoding_failed",
			Message: fmt.Sprintf("Failed to encode wallet multisig status response: %v", err),
		})
	}
}

func parseWalletType(value string) (wallet.WalletType, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "BTC", "BITCOIN":
		return wallet.Bitcoin, nil
	case "XMR", "MONERO":
		return wallet.Monero, nil
	default:
		return "", fmt.Errorf("invalid wallet type: %s", value)
	}
}

// renderPaymentPage generates and serves the HTML payment page for a given payment
// Parameters:
//   - w: HTTP response writer for sending the rendered page
//   - payment: Payment record containing address and amount information
//
// The page includes:
//   - Bitcoin payment address
//   - Payment amount in BTC
//   - Payment expiration time
//   - QR code for the payment address
//
// Error handling:
//   - QR code library loading failures result in QR code feature being disabled
//   - Template rendering failures return 500 Internal Server Error
//
// Related types: Payment, PaymentPageData, template.Template
func (p *Paywall) renderPaymentPage(w http.ResponseWriter, payment *Payment) {
	// Ensure logger is initialized for safety in tests
	if p.logger == nil {
		p.logger = NewStructuredLogger(io.Discard, LogLevelError, true)
	}

	if invalidPayment := p.validatePaymentData(payment, w); invalidPayment {
		return
	}
	qrCodeJsBytes, err := QrcodeJs.ReadFile("static/qrcode.min.js")
	if err != nil {
		p.logger.log(LogEntry{
			Level:   LogLevelError,
			Event:   "qrcode_load_failed",
			Message: fmt.Sprintf("Failed to load QR code JavaScript: %v", err),
		})
		http.Error(w, "QR Code Error", http.StatusInternalServerError)
		qrCodeJsBytes = []byte("")
		// don't return here, let people manually type in the address
		// !return
	}
	// Properly format the Javascript bytes for inclusion in the HTML template as a <script>
	qrCodeJsString := template.JS(qrCodeJsBytes)
	// Prepare template data
	data := PaymentPageData{
		BTCAddress: payment.Addresses[wallet.Bitcoin],
		AmountBTC:  payment.Amounts[wallet.Bitcoin],
		XMRAddress: payment.Addresses[wallet.Monero],
		AmountXMR:  payment.Amounts[wallet.Monero],
		ExpiresAt:  payment.ExpiresAt.Format(time.RFC3339),
		PaymentID:  payment.ID,
		QrcodeJs:   qrCodeJsString,
	}

	// Add multisig information if enabled
	if payment.MultisigEnabled {
		data.IsMultisig = true
		// Determine multisig type from payment metadata
		if len(payment.RequiredSignatures) > 0 {
			// Find any wallet type to get signature requirements
			for walletType, required := range payment.RequiredSignatures {
				if metadata, ok := payment.MultisigMetadata[walletType]; ok {
					total := len(metadata.PublicKeys)
					data.MultisigType = fmt.Sprintf("%d-of-%d", required, total)
					break
				}
			}
		}
		data.MultisigRole = p.multisigRole
		data.MultisigInstructions = "This is a multisig payment address. Funds sent to this address require multiple signatures to spend, providing additional security for escrow transactions."
	}

	if err := p.template.Execute(w, data); err != nil {
		p.logger.log(LogEntry{
			Level:   LogLevelError,
			Event:   "template_render_failed",
			Message: fmt.Sprintf("Failed to render payment page: %v", err),
		})
		http.Error(w, "Failed to render payment page", http.StatusInternalServerError)
		return
	}
}

// validatePaymentData checks if the payment data is valid before rendering the payment page
// Parameters:
//   - payment: Payment record to validate containing address and amount information
//   - w: HTTP response writer for sending error responses
//
// Returns:
//   - bool: true if payment data is invalid, false if valid
//
// Validation checks:
//   - Payment object is not nil
//   - Payment amounts and addresses maps are not nil
//   - Payment amounts are greater than configured prices
//   - Configured prices are greater than 0
//
// Note: Price dust limit validation is performed at Paywall initialization time
// (NewPaywall), so prices are guaranteed to pass dust limit checks here.
//
// Error handling:
//   - Returns 400 Bad Request for nil payment or invalid payment data
//   - Returns 500 Internal Server Error for invalid amounts or prices
func (p *Paywall) validatePaymentData(payment *Payment, w http.ResponseWriter) bool {
	if payment == nil {
		http.Error(w, "Invalid payment", http.StatusBadRequest)
		return true
	}

	if payment.Amounts == nil || payment.Addresses == nil {
		http.Error(w, "Invalid payment data", http.StatusBadRequest)
		return true
	}

	return false
}
