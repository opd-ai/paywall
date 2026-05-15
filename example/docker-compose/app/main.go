package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/opd-ai/paywall"
)

func main() {
	testNet := os.Getenv("TESTNET") == "true"
	priceStr := os.Getenv("PRICE_BTC")
	if priceStr == "" {
		priceStr = "0.0001"
	}
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		log.Fatalf("Invalid PRICE_BTC: %v", err)
	}

	paymentTimeoutStr := os.Getenv("PAYMENT_TIMEOUT")
	if paymentTimeoutStr == "" {
		paymentTimeoutStr = "24h"
	}
	paymentTimeout, err := time.ParseDuration(paymentTimeoutStr)
	if err != nil {
		log.Fatalf("Invalid PAYMENT_TIMEOUT: %v", err)
	}

	minConfStr := os.Getenv("MIN_CONFIRMATIONS")
	if minConfStr == "" {
		minConfStr = "1"
	}
	minConf, err := strconv.Atoi(minConfStr)
	if err != nil {
		log.Fatalf("Invalid MIN_CONFIRMATIONS: %v", err)
	}

	store := paywall.NewFileStore("/data/payments")

	pw, err := paywall.NewPaywall(paywall.Config{
		PriceInBTC:       price,
		TestNet:          testNet,
		Store:            store,
		PaymentTimeout:   paymentTimeout,
		MinConfirmations: minConf,
		XMRUser:          os.Getenv("XMR_WALLET_USER"),
		XMRPassword:      os.Getenv("XMR_WALLET_PASS"),
		XMRRPC:           os.Getenv("XMR_RPC_URL"),
	})
	if err != nil {
		log.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Paywall is running! Access /protected for gated content."))
	})

	http.Handle("/protected", pw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Welcome to protected content!"))
	})))

	log.Printf("Server starting on :8080 (testnet=%v, price=%f BTC)", testNet, price)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
