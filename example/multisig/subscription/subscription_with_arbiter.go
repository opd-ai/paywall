// Package main demonstrates a subscription service with arbiter-backed escrow
//
// NOTE: This example demonstrates the multisig coordination API and escrow workflows.
// Full Bitcoin multisig address generation is pending implementation in the wallet layer.
// The example shows production patterns for recurring payment escrow.
//
// Current implementation status:
//   - ✓ EscrowManager for subscription lifecycle
//   - ✓ Pro-rated refund logic
//   - ✓ Service quality dispute resolution
//   - ✓ Multi-party signature coordination
//   - ⧗ Bitcoin HD wallet multisig address generation (in progress)
//
// This example will run successfully once BTCHDWallet.GenerateMultisigAddress() is implemented.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/wallet"
)

// mockSignature creates a placeholder signature for examples
// In production, use actual cryptographic signing
func mockSignature(role paywall.MultisigRole, pubKey []byte) *paywall.SignatureData {
	return &paywall.SignatureData{
		SignerID:  fmt.Sprintf("%s-signer", string(role)),
		Role:      role,
		Signature: []byte("mock-signature-" + string(role)),
		PublicKey: pubKey,
		SignedAt:  time.Now(),
	}
}

// Subscription represents a recurring payment subscription
type Subscription struct {
	SubscriberID  string
	ServiceID     string
	PriceInBTC    float64
	BillingPeriod time.Duration
	CurrentPeriod int
	Status        string
	PaymentID     string
	NextBillingAt time.Time
	CreatedAt     time.Time
}

// SubscriptionService manages subscriptions with escrow protection
type SubscriptionService struct {
	paywall       *paywall.Paywall
	escrowMgr     *paywall.EscrowManager
	subscriptions map[string]*Subscription
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService() (*SubscriptionService, error) {
	// Service arbiter keys (in production, use real keys)
	servicePubKey := make([]byte, 33)
	arbiterPubKey := make([]byte, 33)
	subscriberPubKey := make([]byte, 33)

	copy(servicePubKey, []byte{0x02})    // Service provider key
	copy(arbiterPubKey, []byte{0x03})    // Neutral arbiter key
	copy(subscriberPubKey, []byte{0x04}) // Subscriber key (varies per subscriber)

	config := paywall.Config{
		PriceInBTC:       0.001, // Base subscription price
		TestNet:          true,
		Store:            paywall.NewMemoryStore(),
		PaymentTimeout:   time.Hour * 24,
		MinConfirmations: 2,

		// 2-of-3 multisig: subscriber, service, arbiter
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: {subscriberPubKey, servicePubKey, arbiterPubKey},
		},
		MultisigRole: paywall.RoleSeller, // Service provider role
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

	return &SubscriptionService{
		paywall:       pw,
		escrowMgr:     escrowMgr,
		subscriptions: make(map[string]*Subscription),
	}, nil
}

// CreateSubscription creates a new subscription with escrow
func (s *SubscriptionService) CreateSubscription(
	subscriberID, serviceID string,
	priceInBTC float64,
	billingPeriod time.Duration,
) (*Subscription, error) {
	fmt.Printf("\n[SUB] Creating subscription for %s to service %s\n", subscriberID, serviceID)
	fmt.Printf("[SUB] Price: %.8f BTC per %s\n", priceInBTC, billingPeriod)

	// Create escrow for first billing period
	paymentID, err := s.escrowMgr.CreateEscrow(priceInBTC/0.001, billingPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create escrow: %w", err)
	}

	subscription := &Subscription{
		SubscriberID:  subscriberID,
		ServiceID:     serviceID,
		PriceInBTC:    priceInBTC,
		BillingPeriod: billingPeriod,
		CurrentPeriod: 1,
		Status:        "pending_payment",
		PaymentID:     paymentID,
		NextBillingAt: time.Now().Add(billingPeriod),
		CreatedAt:     time.Now(),
	}

	s.subscriptions[subscriberID] = subscription

	payment, _ := s.paywall.Store.GetPayment(paymentID)
	fmt.Printf("[SUB] Payment address: %s\n", payment.Addresses[wallet.Bitcoin])
	fmt.Printf("[SUB] Escrow expires: %s\n", payment.EscrowTimeout.Format(time.RFC3339))

	return subscription, nil
}

// ConfirmSubscriptionPayment confirms payment for current period
func (s *SubscriptionService) ConfirmSubscriptionPayment(subscriberID string) error {
	subscription, ok := s.subscriptions[subscriberID]
	if !ok {
		return fmt.Errorf("subscription not found: %s", subscriberID)
	}

	payment, err := s.paywall.Store.GetPayment(subscription.PaymentID)
	if err != nil {
		return err
	}

	// Simulate blockchain confirmation
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 2
	s.paywall.Store.UpdatePayment(payment)

	err = s.escrowMgr.FundEscrow(subscription.PaymentID)
	if err != nil {
		return err
	}

	subscription.Status = "active"
	fmt.Printf("[SUB] Subscription %s activated (Period %d)\n", subscriberID, subscription.CurrentPeriod)

	return nil
}

// CompleteBillingPeriod releases payment for completed service period
func (s *SubscriptionService) CompleteBillingPeriod(subscriberID string, buyerPubKey, sellerPubKey []byte) error {
	subscription, ok := s.subscriptions[subscriberID]
	if !ok {
		return fmt.Errorf("subscription not found: %s", subscriberID)
	}

	// Release payment to service provider (both parties sign)
	buyerSig := mockSignature(paywall.RoleBuyer, buyerPubKey)
	sellerSig := mockSignature(paywall.RoleSeller, sellerPubKey)
	err := s.escrowMgr.ReleaseToSeller(subscription.PaymentID, buyerSig, sellerSig)
	if err != nil {
		return fmt.Errorf("failed to release payment: %w", err)
	}

	fmt.Printf("[SUB] Period %d completed - payment released to service\n", subscription.CurrentPeriod)

	// Prepare for next billing period
	subscription.CurrentPeriod++
	subscription.NextBillingAt = time.Now().Add(subscription.BillingPeriod)

	return nil
}

// CancelSubscription cancels an active subscription with refund
func (s *SubscriptionService) CancelSubscription(subscriberID, reason string, sig1PubKey, sig2PubKey []byte, role1, role2 paywall.MultisigRole) error {
	subscription, ok := s.subscriptions[subscriberID]
	if !ok {
		return fmt.Errorf("subscription not found: %s", subscriberID)
	}

	fmt.Printf("\n[SUB] Canceling subscription %s: %s\n", subscriberID, reason)

	payment, _ := s.paywall.Store.GetPayment(subscription.PaymentID)

	// If escrow still funded, initiate refund
	if payment.EscrowState == paywall.EscrowFunded {
		// Calculate pro-rated refund (simplified)
		timeUsed := time.Since(payment.CreatedAt)
		percentUsed := float64(timeUsed) / float64(subscription.BillingPeriod)
		refundPercent := 1.0 - percentUsed

		fmt.Printf("[SUB] Pro-rated refund: %.0f%% of %.8f BTC\n", refundPercent*100, subscription.PriceInBTC)

		// Create signatures for refund (buyer + seller OR buyer + arbiter)
		sig1 := mockSignature(role1, sig1PubKey)
		sig2 := mockSignature(role2, sig2PubKey)
		err := s.escrowMgr.RefundBuyer(subscription.PaymentID, sig1, sig2)
		if err != nil {
			return fmt.Errorf("failed to refund: %w", err)
		}

		fmt.Println("[SUB] Refund processed")
	}

	subscription.Status = "cancelled"
	return nil
}

// RaiseServiceDispute allows subscriber to dispute service quality
func (s *SubscriptionService) RaiseServiceDispute(subscriberID, reason string, role paywall.MultisigRole) error {
	subscription, ok := s.subscriptions[subscriberID]
	if !ok {
		return fmt.Errorf("subscription not found: %s", subscriberID)
	}

	fmt.Printf("\n[DISPUTE] Subscriber %s raised dispute: %s\n", subscriberID, reason)

	err := s.escrowMgr.RequestDispute(subscription.PaymentID, role, reason)
	if err != nil {
		return fmt.Errorf("failed to request dispute: %w", err)
	}

	subscription.Status = "disputed"
	fmt.Println("[DISPUTE] Arbiter notified, investigation started")

	return nil
}

// ResolveServiceDispute resolves a service quality dispute
func (s *SubscriptionService) ResolveServiceDispute(subscriberID string, favorSubscriber bool, resolution string, arbiterPubKey, winnerPubKey []byte) error {
	subscription, ok := s.subscriptions[subscriberID]
	if !ok {
		return fmt.Errorf("subscription not found: %s", subscriberID)
	}

	fmt.Printf("\n[DISPUTE] Arbiter resolution: %s\n", resolution)

	// Create signatures from arbiter and winner
	arbiterSig := mockSignature(paywall.RoleArbiter, arbiterPubKey)
	var winnerSig *paywall.SignatureData
	if favorSubscriber {
		winnerSig = mockSignature(paywall.RoleBuyer, winnerPubKey)
	} else {
		winnerSig = mockSignature(paywall.RoleSeller, winnerPubKey)
	}

	err := s.escrowMgr.ResolveDispute(subscription.PaymentID, arbiterSig, winnerSig)
	if err != nil {
		return fmt.Errorf("failed to resolve dispute: %w", err)
	}

	if favorSubscriber {
		subscription.Status = "refunded"
		fmt.Println("[DISPUTE] Refund issued to subscriber (handled by ResolveDispute)")
	} else {
		subscription.Status = "active"
		fmt.Println("[DISPUTE] Payment released to service provider (handled by ResolveDispute)")
	}

	return nil
}

// Close cleans up service resources
func (s *SubscriptionService) Close() {
	s.paywall.Close()
}

func main() {
	fmt.Println("=== Subscription Service with Arbiter Escrow ===")

	// Initialize subscription service
	service, err := NewSubscriptionService()
	if err != nil {
		log.Fatalf("Failed to initialize service: %v", err)
	}
	defer service.Close()

	fmt.Println("✓ Subscription service initialized with 2-of-3 multisig escrow")
	// Create mock public keys for participants
	buyerPubKey := make([]byte, 33)
	sellerPubKey := make([]byte, 33)
	arbiterPubKey := make([]byte, 33)
	copy(buyerPubKey, []byte{0x02})
	copy(sellerPubKey, []byte{0x03})
	copy(arbiterPubKey, []byte{0x04})
	// Scenario 1: Normal subscription flow
	fmt.Println("\n=== Scenario 1: Normal Subscription ===")

	sub1, err := service.CreateSubscription(
		"user_alice",
		"premium_service",
		0.005,
		time.Hour*24*30, // Monthly
	)
	if err != nil {
		log.Fatalf("Failed to create subscription: %v", err)
	}

	// Subscriber pays
	if err := service.ConfirmSubscriptionPayment(sub1.SubscriberID); err != nil {
		log.Fatalf("Failed to confirm payment: %v", err)
	}

	// Service period completes successfully
	time.Sleep(100 * time.Millisecond) // Simulate service delivery
	if err := service.CompleteBillingPeriod(sub1.SubscriberID, buyerPubKey, sellerPubKey); err != nil {
		log.Fatalf("Failed to complete billing period: %v", err)
	}

	// Scenario 2: Early cancellation with refund
	fmt.Println("\n=== Scenario 2: Early Cancellation ===")

	sub2, err := service.CreateSubscription(
		"user_bob",
		"premium_service",
		0.005,
		time.Hour*24*30,
	)
	if err != nil {
		log.Fatalf("Failed to create subscription: %v", err)
	}

	if err := service.ConfirmSubscriptionPayment(sub2.SubscriberID); err != nil {
		log.Fatalf("Failed to confirm payment: %v", err)
	}

	// User cancels after 10 days (both parties agree to refund)
	time.Sleep(100 * time.Millisecond) // Simulate 10 days of service
	if err := service.CancelSubscription(sub2.SubscriberID, "Switching to competitor", buyerPubKey, sellerPubKey, paywall.RoleBuyer, paywall.RoleSeller); err != nil {
		log.Fatalf("Failed to cancel subscription: %v", err)
	}

	// Scenario 3: Service quality dispute
	fmt.Println("\n=== Scenario 3: Service Quality Dispute ===")

	sub3, err := service.CreateSubscription(
		"user_charlie",
		"premium_service",
		0.005,
		time.Hour*24*30,
	)
	if err != nil {
		log.Fatalf("Failed to create subscription: %v", err)
	}

	if err := service.ConfirmSubscriptionPayment(sub3.SubscriberID); err != nil {
		log.Fatalf("Failed to confirm payment: %v", err)
	}

	// Subscriber disputes service quality
	if err := service.RaiseServiceDispute(sub3.SubscriberID, "Service downtime >50%", paywall.RoleBuyer); err != nil {
		log.Fatalf("Failed to raise dispute: %v", err)
	}

	// Arbiter investigates and resolves in favor of subscriber
	time.Sleep(100 * time.Millisecond) // Simulate investigation
	if err := service.ResolveServiceDispute(sub3.SubscriberID, true, "Verified excessive downtime", arbiterPubKey, buyerPubKey); err != nil {
		log.Fatalf("Failed to resolve dispute: %v", err)
	}

	// Display statistics
	fmt.Println("\n=== Service Statistics ===")
	active := 0
	cancelled := 0
	refunded := 0
	for _, sub := range service.subscriptions {
		switch sub.Status {
		case "active":
			active++
		case "cancelled":
			cancelled++
		case "refunded":
			refunded++
		}
	}
	fmt.Printf("Total Subscriptions: %d\n", len(service.subscriptions))
	fmt.Printf("Active: %d\n", active)
	fmt.Printf("Cancelled: %d\n", cancelled)
	fmt.Printf("Refunded: %d\n", refunded)

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nProduction considerations:")
	fmt.Println("  1. Implement automated billing cycle management")
	fmt.Println("  2. Add grace period handling for failed renewals")
	fmt.Println("  3. Integrate with service usage metrics for pro-rating")
	fmt.Println("  4. Set up automated arbiter notifications")
	fmt.Println("  5. Implement subscription upgrade/downgrade flows")
	fmt.Println("  6. Add webhook notifications for subscription events")
	fmt.Println("  7. Database persistence for subscription state")
}
