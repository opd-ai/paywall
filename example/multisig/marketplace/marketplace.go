// Package main demonstrates a marketplace platform with multisig escrow and dispute resolution
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// MarketplaceTransaction represents a purchase in the marketplace
type MarketplaceTransaction struct {
	OrderID    string
	BuyerID    string
	SellerID   string
	Item       string
	PriceInBTC float64
	PaymentID  string
	Status     string
	CreatedAt  time.Time
}

// Marketplace manages escrow transactions for multiple vendors
type Marketplace struct {
	paywall      *paywall.Paywall
	escrowMgr    *paywall.EscrowManager
	coordinator  *paywall.MultisigCoordinator
	transactions map[string]*MarketplaceTransaction
}

// NewMarketplace creates a new marketplace instance
func NewMarketplace() (*Marketplace, error) {
	// Marketplace arbiter keys (in production, load from secure storage)
	arbiterPubKey := make([]byte, 33)
	copy(arbiterPubKey, []byte{0x02}) // Arbiter's public key

	// Create marketplace paywall configuration
	// Note: This uses placeholder keys; in production, combine with participant keys dynamically
	config := paywall.Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            paywall.NewMemoryStore(),
		PaymentTimeout:   time.Hour * 48, // 48-hour payment window
		MinConfirmations: 3,              // Require 3 confirmations for safety

		// Multisig is enabled, but participant keys are set per-transaction
		MultisigEnabled:  true,
		MultisigRequired: 2, // 2-of-3 signature requirement
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {arbiterPubKey, make([]byte, 33), make([]byte, 33)},
		},
		MultisigRole: paywall.RoleArbiter, // Marketplace acts as arbiter
	}

	pw, err := paywall.NewPaywall(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create paywall: %w", err)
	}

	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		pw.Close()
		return nil, fmt.Errorf("failed to create escrow manager: %w", err)
	}

	// Set up multisig coordinator for signature collection
	coordinator := paywall.NewMultisigCoordinator(pw, nil, &marketplaceNotifier{})

	return &Marketplace{
		paywall:      pw,
		escrowMgr:    escrowMgr,
		coordinator:  coordinator,
		transactions: make(map[string]*MarketplaceTransaction),
	}, nil
}

// marketplaceNotifier implements webhook notifications
type marketplaceNotifier struct{}

func (n *marketplaceNotifier) NotifySignatureReceived(paymentID string, signerID string, role paywall.MultisigRole) error {
	fmt.Printf("[WEBHOOK] Signature received from %s (%s) for payment %s\n", signerID, role, paymentID)
	return nil
}

func (n *marketplaceNotifier) NotifyReadyToBroadcast(paymentID string) error {
	fmt.Printf("[WEBHOOK] Payment %s ready to broadcast - all signatures collected\n", paymentID)
	return nil
}

func (n *marketplaceNotifier) NotifyBroadcastComplete(paymentID string, txID string) error {
	fmt.Printf("[WEBHOOK] Transaction %s broadcast for payment %s\n", txID, paymentID)
	return nil
}

// CreateOrder creates a new marketplace order with escrow
func (m *Marketplace) CreateOrder(orderID, buyerID, sellerID, item string, priceInBTC float64) (*MarketplaceTransaction, error) {
	fmt.Printf("\n[ORDER] Creating order %s: %s -> %s (%.8f BTC for %s)\n", orderID, buyerID, sellerID, priceInBTC, item)

	// Create escrow payment
	paymentID, err := m.escrowMgr.CreateEscrow(priceInBTC/0.001, time.Hour*72)
	if err != nil {
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}

	transaction := &MarketplaceTransaction{
		OrderID:    orderID,
		BuyerID:    buyerID,
		SellerID:   sellerID,
		Item:       item,
		PriceInBTC: priceInBTC,
		PaymentID:  paymentID,
		Status:     "pending_payment",
		CreatedAt:  time.Now(),
	}

	m.transactions[orderID] = transaction

	payment, _ := m.paywall.Store.GetPayment(paymentID)
	fmt.Printf("[ORDER] Payment address: %s\n", payment.Addresses[wallet.Bitcoin])
	fmt.Printf("[ORDER] Escrow timeout: %s\n", payment.EscrowTimeout.Format(time.RFC3339))

	return transaction, nil
}

// ConfirmPayment simulates payment confirmation on blockchain
func (m *Marketplace) ConfirmPayment(orderID string) error {
	transaction, ok := m.transactions[orderID]
	if !ok {
		return fmt.Errorf("order not found: %s", orderID)
	}

	payment, err := m.paywall.Store.GetPayment(transaction.PaymentID)
	if err != nil {
		return err
	}

	// Simulate blockchain confirmation
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 3
	m.paywall.Store.UpdatePayment(payment)

	err = m.escrowMgr.FundEscrow(transaction.PaymentID)
	if err != nil {
		return err
	}

	transaction.Status = "funded"
	fmt.Printf("[ORDER] Order %s funded and confirmed\n", orderID)

	return nil
}

// CompleteOrder releases funds to seller after successful delivery
func (m *Marketplace) CompleteOrder(orderID string, buyerPubKey, sellerPubKey []byte) error {
	transaction, ok := m.transactions[orderID]
	if !ok {
		return fmt.Errorf("order not found: %s", orderID)
	}

	// Release funds to seller (requires buyer + seller signatures)
	buyerSig := mockSignature(paywall.RoleBuyer, buyerPubKey)
	sellerSig := mockSignature(paywall.RoleSeller, sellerPubKey)
	err := m.escrowMgr.ReleaseToSeller(transaction.PaymentID, buyerSig, sellerSig)
	if err != nil {
		return fmt.Errorf("failed to release to seller: %w", err)
	}

	transaction.Status = "completed"
	fmt.Printf("[ORDER] Order %s completed - funds released to seller %s\n", orderID, transaction.SellerID)

	return nil
}

// mockSignature creates a placeholder signature for examples
func mockSignature(role paywall.MultisigRole, pubKey []byte) *paywall.SignatureData {
	return &paywall.SignatureData{
		SignerID:  fmt.Sprintf("%s-signer", string(role)),
		Role:      role,
		Signature: []byte("mock-signature-" + string(role)),
		PublicKey: pubKey,
		SignedAt:  time.Now(),
	}
}

// RaiseDispute allows buyer or seller to raise a dispute
func (m *Marketplace) RaiseDispute(orderID string, role paywall.MultisigRole, reason string) error {
	transaction, ok := m.transactions[orderID]
	if !ok {
		return fmt.Errorf("order not found: %s", orderID)
	}

	err := m.escrowMgr.RequestDispute(transaction.PaymentID, role, reason)
	if err != nil {
		return fmt.Errorf("failed to request dispute: %w", err)
	}

	transaction.Status = "disputed"
	fmt.Printf("[DISPUTE] Order %s disputed by %s: %s\n", orderID, string(role), reason)

	return nil
}

// ResolveDispute resolves a dispute in favor of buyer or seller
func (m *Marketplace) ResolveDispute(orderID string, favorBuyer bool, resolution string, arbiterPubKey, winnerPubKey []byte) error {
	transaction, ok := m.transactions[orderID]
	if !ok {
		return fmt.Errorf("order not found: %s", orderID)
	}

	// Create signatures from arbiter and winner
	arbiterSig := mockSignature(paywall.RoleArbiter, arbiterPubKey)
	var winnerSig *paywall.SignatureData
	if favorBuyer {
		winnerSig = mockSignature(paywall.RoleBuyer, winnerPubKey)
	} else {
		winnerSig = mockSignature(paywall.RoleSeller, winnerPubKey)
	}

	err := m.escrowMgr.ResolveDispute(transaction.PaymentID, arbiterSig, winnerSig)
	if err != nil {
		return fmt.Errorf("failed to resolve dispute: %w", err)
	}

	if favorBuyer {
		transaction.Status = "refunded"
		fmt.Printf("[DISPUTE] Order %s resolved in favor of buyer - refund issued\n", orderID)
	} else {
		transaction.Status = "completed"
		fmt.Printf("[DISPUTE] Order %s resolved in favor of seller - payment released\n", orderID)
	}

	return nil
}

// StartAPIServer starts the multisig coordinator HTTP API
func (m *Marketplace) StartAPIServer(addr string) error {
	http.HandleFunc("/multisig/initiate", m.coordinator.HandleInitiate)
	http.HandleFunc("/multisig/sign", m.coordinator.HandleSign)
	http.HandleFunc("/multisig/status/", m.coordinator.HandleStatus)
	http.HandleFunc("/multisig/broadcast", m.coordinator.HandleBroadcast)

	fmt.Printf("\n[API] Starting multisig coordinator API on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}

// Close cleans up marketplace resources
func (m *Marketplace) Close() {
	m.paywall.Close()
}

func main() {
	fmt.Println("\n=== Marketplace with Multisig Escrow Example ===")

	// Initialize marketplace
	marketplace, err := NewMarketplace()
	if err != nil {
		log.Fatalf("Failed to initialize marketplace: %v", err)
	}
	defer marketplace.Close()

	fmt.Println("✓ Marketplace initialized with 2-of-3 multisig escrow")

	// Create mock public keys for participants
	buyerPubKey := make([]byte, 33)
	sellerPubKey := make([]byte, 33)
	arbiterPubKey := make([]byte, 33)
	copy(buyerPubKey, []byte{0x02})
	copy(sellerPubKey, []byte{0x03})
	copy(arbiterPubKey, []byte{0x04})

	// Start API server in background (for signature coordination)
	go func() {
		if err := marketplace.StartAPIServer(":8080"); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Scenario 1: Successful transaction
	fmt.Println("\n=== Scenario 1: Successful Transaction ===")

	order1, err := marketplace.CreateOrder(
		"ORD-001",
		"buyer_alice",
		"seller_bob",
		"Vintage Camera",
		0.005,
	)
	if err != nil {
		log.Fatalf("Failed to create order: %v", err)
	}

	// Simulate buyer payment
	if err := marketplace.ConfirmPayment(order1.OrderID); err != nil {
		log.Fatalf("Failed to confirm payment: %v", err)
	}

	// Simulate successful delivery and completion
	time.Sleep(500 * time.Millisecond) // Simulate delivery time
	if err := marketplace.CompleteOrder(order1.OrderID, buyerPubKey, sellerPubKey); err != nil {
		log.Fatalf("Failed to complete order: %v", err)
	}

	// Scenario 2: Disputed transaction
	fmt.Println("\n=== Scenario 2: Disputed Transaction ===")

	order2, err := marketplace.CreateOrder(
		"ORD-002",
		"buyer_charlie",
		"seller_dave",
		"Laptop",
		0.02,
	)
	if err != nil {
		log.Fatalf("Failed to create order: %v", err)
	}

	if err := marketplace.ConfirmPayment(order2.OrderID); err != nil {
		log.Fatalf("Failed to confirm payment: %v", err)
	}

	// Buyer raises dispute
	if err := marketplace.RaiseDispute(order2.OrderID, paywall.RoleBuyer, "Item not working as described"); err != nil {
		log.Fatalf("Failed to raise dispute: %v", err)
	}

	// Marketplace arbiter investigates and resolves
	time.Sleep(500 * time.Millisecond) // Simulate investigation time
	if err := marketplace.ResolveDispute(order2.OrderID, true, "Verified defect, refund approved", arbiterPubKey, buyerPubKey); err != nil {
		log.Fatalf("Failed to resolve dispute: %v", err)
	}

	// Display final statistics
	fmt.Println("\n=== Marketplace Statistics ===")
	completed := 0
	refunded := 0
	for _, tx := range marketplace.transactions {
		if tx.Status == "completed" {
			completed++
		} else if tx.Status == "refunded" {
			refunded++
		}
	}
	fmt.Printf("Total Orders: %d\n", len(marketplace.transactions))
	fmt.Printf("Completed: %d\n", completed)
	fmt.Printf("Refunded: %d\n", refunded)

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nProduction considerations:")
	fmt.Println("  1. Integrate real payment verification via blockchain monitoring")
	fmt.Println("  2. Implement proper authentication for API endpoints")
	fmt.Println("  3. Add database backend for transaction persistence")
	fmt.Println("  4. Set up webhook notifications for real-time updates")
	fmt.Println("  5. Implement automated dispute resolution workflows")
	fmt.Println("  6. Add proper error handling and retry logic")
	fmt.Println("  7. Use secure key management for arbiter keys")
}
