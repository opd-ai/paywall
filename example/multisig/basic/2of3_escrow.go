// Package main demonstrates a basic 2-of-3 multisig escrow workflow
//
// NOTE: This example demonstrates the multisig coordination API and escrow state machine
// using fully implemented Bitcoin multisig address generation.
//
// Current implementation status:
//   - ✓ Bitcoin HD wallet multisig (P2WSH/P2SH)
//   - ✓ Monero HD wallet multisig (via RPC workflow)
//   - ✓ Multisig coordination HTTP API (signature collection)
//   - ✓ Escrow state machine (pending, funded, completed, disputed, refunded)
//   - ✓ 2-of-3 signature coordination with roles (buyer, seller, arbiter)
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/opd-ai/paywall"
	"github.com/opd-ai/paywall/example/multisig/common"
	"github.com/opd-ai/paywall/wallet"
)

// This example demonstrates:
// 1. Creating a 2-of-3 multisig escrow payment
// 2. Coordinating signatures between buyer, seller, and arbiter
// 3. Releasing funds when agreement is reached

func main() {
	fmt.Println("=== 2-of-3 Multisig Escrow Example ===")

	// Generate example public keys (in production, these would come from participants)
	buyerPubKey, sellerPubKey, arbiterPubKey := common.GenerateExamplePubKeys()
	publicKeys := [][]byte{buyerPubKey, sellerPubKey, arbiterPubKey}

	// Create escrow paywall configuration
	config := paywall.Config{
		PriceInBTC:     0.001, // 0.001 BTC for the transaction
		TestNet:        true,  // Use testnet for safety
		Store:          paywall.NewMemoryStore(),
		PaymentTimeout: time.Hour * 24,

		// Multisig configuration for 2-of-3 escrow
		MultisigEnabled:  true,
		MultisigRequired: 2, // Require 2 signatures
		MultisigTotal:    3, // Out of 3 total participants
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: publicKeys,
		},
		MultisigRole: paywall.RoleBuyer, // This instance acts as buyer
	}

	// Initialize paywall with multisig escrow
	pw, err := paywall.NewPaywall(config)
	if err != nil {
		log.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	fmt.Println("✓ Paywall created with 2-of-3 multisig configuration")

	// Create escrow manager
	escrowMgr, err := paywall.NewEscrowManager(pw)
	if err != nil {
		log.Fatalf("Failed to create escrow manager: %v", err)
	}

	fmt.Println("✓ Escrow manager initialized")

	// Step 1: Create escrow payment
	paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72) // 72-hour timeout
	if err != nil {
		log.Fatalf("Failed to create escrow: %v", err)
	}

	fmt.Printf("✓ Escrow payment created: %s\n", paymentID)

	// Retrieve payment details
	payment, err := pw.Store.GetPayment(paymentID)
	if err != nil {
		log.Fatalf("Failed to retrieve payment: %v", err)
	}

	fmt.Printf("\nPayment Details:\n")
	fmt.Printf("  ID: %s\n", payment.ID)
	fmt.Printf("  Status: %s\n", payment.Status)
	fmt.Printf("  Escrow State: %s\n", payment.EscrowState.String())
	fmt.Printf("  Amount: %.8f BTC\n", payment.Amounts[wallet.Bitcoin])
	fmt.Printf("  Address: %s\n", payment.Addresses[wallet.Bitcoin])
	fmt.Printf("  Required Signatures: %d of %d\n",
		payment.RequiredSignatures[wallet.Bitcoin],
		len(payment.MultisigMetadata[wallet.Bitcoin].PublicKeys),
	)
	fmt.Printf("  Expires: %s\n", payment.ExpiresAt.Format(time.RFC3339))

	// Step 2: Simulate buyer funding the escrow
	fmt.Println("\n--- Buyer funds escrow ---")
	// In production: buyer sends BTC to payment.Addresses[wallet.Bitcoin]
	// Payment verification would happen via CryptoChainMonitor
	// For this example, we manually mark as funded

	// Simulate payment confirmation
	payment.Status = paywall.StatusConfirmed
	payment.Confirmations = 1
	pw.Store.UpdatePayment(payment)

	err = escrowMgr.FundEscrow(paymentID)
	if err != nil {
		log.Fatalf("Failed to fund escrow: %v", err)
	}

	fmt.Println("✓ Escrow funded and confirmed")

	// Step 3: Happy path - buyer and seller agree, release to seller
	fmt.Println("\n--- Buyer and seller agree, releasing to seller ---")

	// In production, this would involve:
	// 1. Buyer and seller both sign the transaction
	// 2. Signatures collected via MultisigCoordinator API
	// 3. Transaction broadcast when 2 signatures collected

	buyerSig := common.MockSignature(paywall.RoleBuyer, buyerPubKey)
	sellerSig := common.MockSignature(paywall.RoleSeller, sellerPubKey)
	err = escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
	if err != nil {
		log.Fatalf("Failed to release to seller: %v", err)
	}

	fmt.Println("✓ Funds released to seller")

	// Retrieve final payment state
	payment, _ = pw.Store.GetPayment(paymentID)
	fmt.Printf("\nFinal Escrow State: %s\n", payment.EscrowState.String())

	// Alternative scenario: Dispute resolution
	fmt.Println("\n--- Alternative: Dispute Scenario ---")

	// Create another escrow for dispute example
	disputePaymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*72)
	if err != nil {
		log.Fatalf("Failed to create dispute escrow: %v", err)
	}

	disputePayment, _ := pw.Store.GetPayment(disputePaymentID)
	disputePayment.Status = paywall.StatusConfirmed
	disputePayment.Confirmations = 1
	pw.Store.UpdatePayment(disputePayment)
	escrowMgr.FundEscrow(disputePaymentID)

	fmt.Printf("✓ Dispute escrow created: %s\n", disputePaymentID)

	// Buyer raises dispute
	err = escrowMgr.RequestDispute(disputePaymentID, paywall.RoleBuyer, "Item not as described")
	if err != nil {
		log.Fatalf("Failed to request dispute: %v", err)
	}

	fmt.Println("✓ Dispute requested by buyer")

	// Arbiter resolves in favor of buyer (refund)
	// ResolveDispute requires arbiter + winner signatures
	arbiterSig := common.MockSignature(paywall.RoleArbiter, arbiterPubKey)
	buyerSig2 := common.MockSignature(paywall.RoleBuyer, buyerPubKey)
	err = escrowMgr.ResolveDispute(disputePaymentID, arbiterSig, buyerSig2)
	if err != nil {
		log.Fatalf("Failed to resolve dispute: %v", err)
	}

	fmt.Println("✓ Arbiter resolved dispute in favor of buyer")

	// Note: ResolveDispute automatically handles the state transition to refunded
	// No need to call RefundBuyer separately

	fmt.Println("✓ Funds refunded to buyer")

	disputePayment, _ = pw.Store.GetPayment(disputePaymentID)
	fmt.Printf("\nFinal Dispute State: %s\n", disputePayment.EscrowState.String())

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\nIn production:")
	fmt.Println("  1. Use real compressed public keys from all participants")
	fmt.Println("  2. Integrate MultisigCoordinator HTTP API for signature collection")
	fmt.Println("  3. Monitor blockchain for actual payment confirmations")
	fmt.Println("  4. Implement proper key management and security")
	fmt.Println("  5. Use webhook notifications for real-time updates")
}
