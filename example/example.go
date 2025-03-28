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
		PriceInBTC:       0.0001,                               // 0.0001 BTC
		TestNet:          true,                                 // Use testnet
		Store:            paywall.NewFileStore(config.DataDir), // Required for payment tracking
		PaymentTimeout:   time.Hour * 24,
		MinConfirmations: 1,
		XMRUser:          "user",
		XMRPassword:      "password",
		XMRRPC:           "http://localhost:18081/",
	})
	if err != nil {
		log.Fatal(err)
	}
	// Attempt to load wallet from disk, if it fails store the new one
	if HDWallet, err := wallet.LoadFromFile(config); err != nil {
		// Save newly generated wallet
		if err := pw.HDWallets[wallet.Bitcoin].(*wallet.BTCHDWallet).SaveToFile(config); err != nil {
			log.Fatal(err)
		}
	} else {
		// Load stored wallet from disk
		pw.HDWallets[wallet.Bitcoin] = HDWallet
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
