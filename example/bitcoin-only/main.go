// Package main provides a Bitcoin-only paywall example.
// This example demonstrates the simplest possible paywall configuration
// using only Bitcoin without Monero support.
//
// Usage:
//
//	go run example/bitcoin-only/main.go
//
// Then visit http://localhost:8001/protected to see the paywall in action.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

func main() {
	// Generate encryption key for wallet storage
	key, err := wallet.GenerateEncryptionKey()
	if err != nil {
		log.Fatalf("Failed to generate encryption key: %v", err)
	}

	config := wallet.StorageConfig{
		DataDir:       "./paywallet-btc-only",
		EncryptionKey: key,
	}

	// Bitcoin-only paywall configuration
	// Note: No XMR fields required when only accepting Bitcoin payments
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:       0.0001,                               // 0.0001 BTC (~$4.50 at $45k/BTC)
		TestNet:          true,                                 // Use Bitcoin testnet for development
		Store:            paywall.NewFileStore(config.DataDir), // File-based payment storage
		PaymentTimeout:   time.Hour * 24,                       // Payment expires after 24 hours
		MinConfirmations: 1,                                    // 1 confirmation for testnet
	})
	if err != nil {
		log.Fatalf("Failed to create paywall: %v", err)
	}

	// Attempt to load existing wallet, otherwise save the newly generated one
	if storedWallet, err := wallet.LoadFromFile(config); err == nil {
		// Use stored wallet
		pw.HDWallets[wallet.Bitcoin] = storedWallet
		log.Println("Loaded existing wallet from disk")
	} else {
		// Save newly generated wallet
		btcWallet := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet)
		if err := btcWallet.SaveToFile(config); err != nil {
			log.Fatalf("Failed to save wallet: %v", err)
		}
		log.Println("Created and saved new Bitcoin wallet")
	}

	// Protected content handler
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<html>
			<head><title>Protected Content</title></head>
			<body>
				<h1>🎉 Payment Confirmed!</h1>
				<p>You have successfully paid to access this content.</p>
				<p>This demonstrates a Bitcoin-only paywall without Monero support.</p>
			</body>
			</html>
		`))
	})

	// Free content handler (no paywall)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<html>
			<head><title>Bitcoin-Only Paywall Example</title></head>
			<body>
				<h1>Bitcoin-Only Paywall Example</h1>
				<p>This example demonstrates a simple Bitcoin-only paywall.</p>
				<p><a href="/protected">Access Protected Content</a> (requires 0.0001 BTC payment)</p>
				<p>Configuration:</p>
				<ul>
					<li>Price: 0.0001 BTC</li>
					<li>Network: Testnet</li>
					<li>Min Confirmations: 1</li>
					<li>Payment Timeout: 24 hours</li>
					<li>No Monero support (Bitcoin only)</li>
				</ul>
			</body>
			</html>
		`))
	})

	// Apply paywall middleware to protected endpoint
	http.Handle("/protected", pw.Middleware(protected))

	log.Println("Bitcoin-only paywall server starting on :8001")
	log.Println("Visit http://localhost:8001/ for info")
	log.Println("Visit http://localhost:8001/protected to test paywall")
	log.Fatal(http.ListenAndServe(":8001", nil))
}
