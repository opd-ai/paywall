// Package main provides an example implementation of a Bitcoin paywall-protected HTTP server
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// Command-line flag for wallet seed initialization
var seed = flag.String("seed", "", "Sequence of bytes to use as a seed for the wallet")

// main initializes and runs a Bitcoin paywall-protected HTTP server.
// It demonstrates:
// - Wallet creation and persistence
// - Paywall configuration
// - HTTP middleware integration
// - Basic payment tracking
//
// The server protects content at /protected endpoint with Bitcoin payments

func main() {
	flag.Parse()
	key, err := wallet.GenerateEncryptionKey()
	if err != nil {
		log.Fatal(err)
	}

	config := wallet.StorageConfig{
		DataDir:       "./paywallet",
		EncryptionKey: key,
	}

	// Initialize paywall with minimal config
	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:     0.001,                    // 0.001 BTC
		TestNet:        true,                     // Use testnet
		Store:          paywall.NewMemoryStore(), // Required for payment tracking
		PaymentTimeout: time.Hour * 24,
	})
	// Attempt to load wallet from disk, if it fails store the new one
	if HDWallet, err := wallet.LoadFromFile(config); err != nil {
		// Save newly generated wallet
		if err := pw.HDWallet.SaveToFile(config); err != nil {
			log.Fatal(err)
		}
	} else {
		// Load stored wallet from disk
		pw.HDWallet = HDWallet
	}

	// Protected content handler
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Protected content"))
	})

	// Apply paywall middleware
	http.Handle("/protected", pw.Middleware(protected))

	log.Println("Server starting on :8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
