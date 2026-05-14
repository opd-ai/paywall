// Package main provides a Monero-only paywall example.
// This example demonstrates a privacy-focused paywall configuration
// using only Monero (no Bitcoin support).
//
// Prerequisites:
//   - Monero wallet RPC running on localhost:18081
//   - Set environment variables: XMR_WALLET_USER and XMR_WALLET_PASS
//
// Usage:
//
//	export XMR_WALLET_USER=myuser
//	export XMR_WALLET_PASS=mypassword
//	go run example/monero-only/main.go
//
// Then visit http://localhost:8002/protected to see the paywall in action.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	// Check for required Monero RPC credentials
	xmrUser := os.Getenv("XMR_WALLET_USER")
	xmrPass := os.Getenv("XMR_WALLET_PASS")

	if xmrUser == "" || xmrPass == "" {
		log.Fatal("Monero RPC credentials required. Set XMR_WALLET_USER and XMR_WALLET_PASS environment variables.")
	}

	// Monero-only paywall configuration
	// Note: No Bitcoin fields required when only accepting Monero payments
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInXMR:       0.01,                     // 0.01 XMR (~$2-3 at typical prices)
		TestNet:          false,                    // Monero mainnet (no testnet support)
		Store:            paywall.NewMemoryStore(), // In-memory storage for simplicity
		PaymentTimeout:   time.Hour * 24,           // Payment expires after 24 hours
		MinConfirmations: 10,                       // 10 confirmations (~20 minutes)
		XMRUser:          xmrUser,                  // From environment
		XMRPassword:      xmrPass,                  // From environment
		XMRRPC:           "http://localhost:18081", // Local Monero wallet RPC
	})
	if err != nil {
		log.Fatalf("Failed to create paywall: %v", err)
	}

	log.Println("Monero wallet RPC configured successfully")

	// Protected content handler
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>Protected Content</title></head><body><h1>Payment Confirmed!</h1><p>You have successfully paid with Monero.</p></body></html>"))
	})

	// Free content handler (no paywall)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><title>Monero-Only Paywall</title></head><body><h1>Monero-Only Paywall Example</h1><p><a href=\"/protected\">Access Protected Content</a> (requires 0.01 XMR)</p></body></html>"))
	})

	// Apply paywall middleware to protected endpoint
	http.Handle("/protected", pw.Middleware(protected))

	log.Println("Monero-only paywall server starting on :8002")
	log.Fatal(http.ListenAndServe(":8002", nil))
}
