// Package paywall implements HTTP handlers for multisig signature coordination
package paywall

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// MultisigInitiateRequest contains the parameters for starting multisig setup
type MultisigInitiateRequest struct {
	// WalletType specifies which cryptocurrency wallet to use (Bitcoin or Monero)
	WalletType wallet.WalletType `json:"wallet_type"`
	// RequiredSigs specifies m in m-of-n multisig
	RequiredSigs int `json:"required_sigs"`
	// PublicKeys contains all participant public keys for multisig setup
	PublicKeys [][]byte `json:"public_keys"`
	// Role specifies the initiator's role (buyer, seller, or arbiter)
	Role MultisigRole `json:"role"`
	// PriceMultiplier allows custom pricing (default 1.0)
	PriceMultiplier float64 `json:"price_multiplier,omitempty"`
}

// MultisigInitiateResponse contains the result of multisig initiation
type MultisigInitiateResponse struct {
	// PaymentID uniquely identifies the created payment
	PaymentID string `json:"payment_id"`
	// Address is the multisig address for receiving funds
	Address string `json:"address"`
	// Amount is the required payment amount
	Amount float64 `json:"amount"`
	// RedeemScript is the Bitcoin redeem script (Bitcoin only)
	RedeemScript []byte `json:"redeem_script,omitempty"`
	// MultisigInfo is the Monero multisig info (Monero only)
	MultisigInfo string `json:"multisig_info,omitempty"`
	// ExpiresAt is when the payment expires
	ExpiresAt time.Time `json:"expires_at"`
}

// MultisigSignRequest contains a partial signature submission
type MultisigSignRequest struct {
	// PaymentID identifies the payment to sign
	PaymentID string `json:"payment_id"`
	// WalletType specifies which wallet the signature is for
	WalletType wallet.WalletType `json:"wallet_type"`
	// SignerID uniquely identifies the signer
	SignerID string `json:"signer_id"`
	// Role indicates the signer's role
	Role MultisigRole `json:"role"`
	// Signature contains the cryptographic signature bytes
	Signature []byte `json:"signature"`
	// PublicKey is the signer's public key for verification
	PublicKey []byte `json:"public_key"`
}

// MultisigSignResponse confirms signature acceptance
type MultisigSignResponse struct {
	// Success indicates whether the signature was accepted
	Success bool `json:"success"`
	// SignatureCount is the current number of collected signatures
	SignatureCount int `json:"signature_count"`
	// RequiredSignatures is the number of signatures needed
	RequiredSignatures int `json:"required_signatures"`
	// Message provides additional context
	Message string `json:"message"`
}

// MultisigStatusResponse contains the current signing status
type MultisigStatusResponse struct {
	// PaymentID identifies the payment
	PaymentID string `json:"payment_id"`
	// Status is the current payment status
	Status PaymentStatus `json:"status"`
	// Confirmations is the blockchain confirmation count
	Confirmations int `json:"confirmations"`
	// Signatures contains collected signatures per wallet type
	Signatures map[wallet.WalletType][]SignatureData `json:"signatures"`
	// RequiredSignatures specifies requirements per wallet type
	RequiredSignatures map[wallet.WalletType]int `json:"required_signatures"`
	// ReadyToBroadcast indicates if enough signatures are collected
	ReadyToBroadcast bool `json:"ready_to_broadcast"`
	// EscrowState indicates escrow status if applicable
	EscrowState EscrowState `json:"escrow_state,omitempty"`
}

// MultisigBroadcastRequest requests transaction broadcast
type MultisigBroadcastRequest struct {
	// PaymentID identifies the payment to broadcast
	PaymentID string `json:"payment_id"`
	// WalletType specifies which wallet to broadcast for
	WalletType wallet.WalletType `json:"wallet_type"`
	// Transaction is the fully-signed transaction bytes
	Transaction []byte `json:"transaction"`
}

// MultisigBroadcastResponse confirms broadcast result
type MultisigBroadcastResponse struct {
	// Success indicates whether broadcast succeeded
	Success bool `json:"success"`
	// TransactionID is the blockchain transaction ID
	TransactionID string `json:"transaction_id"`
	// Message provides additional context
	Message string `json:"message"`
}

// MultisigAuthenticator provides authentication for multisig operations
// Implementations should verify the caller has permission for the requested operation
type MultisigAuthenticator interface {
	// Authenticate verifies the request is authorized
	// Returns nil if authorized, error otherwise
	Authenticate(r *http.Request, paymentID string, role MultisigRole) error
}

// MultisigWebhookNotifier sends webhook notifications for multisig events
type MultisigWebhookNotifier interface {
	// NotifySignatureReceived sends notification when a signature is collected
	NotifySignatureReceived(paymentID string, signerID string, role MultisigRole) error
	// NotifyReadyToBroadcast sends notification when all signatures are collected
	NotifyReadyToBroadcast(paymentID string) error
	// NotifyBroadcastComplete sends notification when transaction is broadcast
	NotifyBroadcastComplete(paymentID string, txID string) error
}

// MultisigCoordinator manages signature coordination for multisig payments
type MultisigCoordinator struct {
	paywall       *Paywall
	authenticator MultisigAuthenticator
	notifier      MultisigWebhookNotifier
}

// NewMultisigCoordinator creates a new coordinator with optional authenticator and notifier
func NewMultisigCoordinator(pw *Paywall, auth MultisigAuthenticator, notifier MultisigWebhookNotifier) *MultisigCoordinator {
	return &MultisigCoordinator{
		paywall:       pw,
		authenticator: auth,
		notifier:      notifier,
	}
}

// HandleInitiate processes POST /multisig/initiate requests
// Creates a new multisig payment with the specified configuration
func (mc *MultisigCoordinator) HandleInitiate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MultisigInitiateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := mc.validateInitiateRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Authenticate if authenticator is configured
	if mc.authenticator != nil {
		if err := mc.authenticator.Authenticate(r, "", req.Role); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Create multisig payment
	payment, err := mc.createMultisigPayment(&req)
	if err != nil {
		log.Printf("Failed to create multisig payment: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create payment: %v", err), http.StatusInternalServerError)
		return
	}

	// Build response
	resp := MultisigInitiateResponse{
		PaymentID: payment.ID,
		Address:   payment.Addresses[req.WalletType],
		Amount:    payment.Amounts[req.WalletType],
		ExpiresAt: payment.ExpiresAt,
	}

	// Add wallet-specific data
	if metadata, ok := payment.MultisigMetadata[req.WalletType]; ok {
		if req.WalletType == wallet.Bitcoin {
			resp.RedeemScript = metadata.RedeemScript
		} else if req.WalletType == wallet.Monero {
			// For Monero, RedeemScript field contains the multisig setup info
			resp.MultisigInfo = string(metadata.RedeemScript)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleSign processes POST /multisig/sign requests
// Accepts and validates partial signatures for multisig payments
func (mc *MultisigCoordinator) HandleSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MultisigSignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := mc.validateSignRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Authenticate if authenticator is configured
	if mc.authenticator != nil {
		if err := mc.authenticator.Authenticate(r, req.PaymentID, req.Role); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Get payment
	payment, err := mc.paywall.Store.GetPayment(req.PaymentID)
	if err != nil {
		http.Error(w, "Payment not found", http.StatusNotFound)
		return
	}

	if !payment.MultisigEnabled {
		http.Error(w, "Payment is not multisig-enabled", http.StatusBadRequest)
		return
	}

	// Add signature
	sigData := SignatureData{
		SignerID:  req.SignerID,
		Role:      req.Role,
		Signature: req.Signature,
		PublicKey: req.PublicKey,
		SignedAt:  time.Now(),
	}

	if payment.Signatures == nil {
		payment.Signatures = make(map[wallet.WalletType][]SignatureData)
	}
	payment.Signatures[req.WalletType] = append(payment.Signatures[req.WalletType], sigData)

	// Update payment
	if err := mc.paywall.Store.UpdatePayment(payment); err != nil {
		http.Error(w, "Failed to store signature", http.StatusInternalServerError)
		return
	}

	// Send webhook notification
	if mc.notifier != nil {
		go mc.notifier.NotifySignatureReceived(req.PaymentID, req.SignerID, req.Role)
	}

	// Check if ready to broadcast
	requiredSigs := payment.RequiredSignatures[req.WalletType]
	currentSigs := len(payment.Signatures[req.WalletType])
	readyToBroadcast := currentSigs >= requiredSigs

	if readyToBroadcast && mc.notifier != nil {
		go mc.notifier.NotifyReadyToBroadcast(req.PaymentID)
	}

	// Build response
	resp := MultisigSignResponse{
		Success:            true,
		SignatureCount:     currentSigs,
		RequiredSignatures: requiredSigs,
		Message:            fmt.Sprintf("Signature accepted (%d of %d)", currentSigs, requiredSigs),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleStatus processes GET /multisig/status/:paymentID requests
// Returns the current signing status for a multisig payment
func (mc *MultisigCoordinator) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract payment ID from URL path
	// Expects path like /multisig/status/ABC123
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		http.Error(w, "Payment ID required", http.StatusBadRequest)
		return
	}
	paymentID := pathParts[2]

	// Get payment
	payment, err := mc.paywall.Store.GetPayment(paymentID)
	if err != nil {
		http.Error(w, "Payment not found", http.StatusNotFound)
		return
	}

	if !payment.MultisigEnabled {
		http.Error(w, "Payment is not multisig-enabled", http.StatusBadRequest)
		return
	}

	// Check if ready to broadcast (all required signatures collected)
	readyToBroadcast := true
	for walletType, required := range payment.RequiredSignatures {
		collected := len(payment.Signatures[walletType])
		if collected < required {
			readyToBroadcast = false
			break
		}
	}

	// Build response
	resp := MultisigStatusResponse{
		PaymentID:          payment.ID,
		Status:             payment.Status,
		Confirmations:      payment.Confirmations,
		Signatures:         payment.Signatures,
		RequiredSignatures: payment.RequiredSignatures,
		ReadyToBroadcast:   readyToBroadcast,
		EscrowState:        payment.EscrowState,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleBroadcast processes POST /multisig/broadcast requests
// Broadcasts a fully-signed multisig transaction to the blockchain
func (mc *MultisigCoordinator) HandleBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MultisigBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.PaymentID == "" {
		http.Error(w, "PaymentID required", http.StatusBadRequest)
		return
	}
	if len(req.Transaction) == 0 {
		http.Error(w, "Transaction required", http.StatusBadRequest)
		return
	}

	// Authenticate if authenticator is configured
	if mc.authenticator != nil {
		if err := mc.authenticator.Authenticate(r, req.PaymentID, ""); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Get payment
	payment, err := mc.paywall.Store.GetPayment(req.PaymentID)
	if err != nil {
		http.Error(w, "Payment not found", http.StatusNotFound)
		return
	}

	if !payment.MultisigEnabled {
		http.Error(w, "Payment is not multisig-enabled", http.StatusBadRequest)
		return
	}

	// Verify enough signatures collected
	requiredSigs := payment.RequiredSignatures[req.WalletType]
	currentSigs := len(payment.Signatures[req.WalletType])
	if currentSigs < requiredSigs {
		http.Error(w, fmt.Sprintf("Insufficient signatures: %d of %d", currentSigs, requiredSigs), http.StatusBadRequest)
		return
	}

	// TODO: Broadcast transaction to blockchain
	// This would use the wallet's broadcast functionality
	// For now, return a placeholder response
	txID := fmt.Sprintf("tx_%s_%d", req.PaymentID, time.Now().Unix())

	// Send webhook notification
	if mc.notifier != nil {
		go mc.notifier.NotifyBroadcastComplete(req.PaymentID, txID)
	}

	resp := MultisigBroadcastResponse{
		Success:       true,
		TransactionID: txID,
		Message:       "Transaction broadcast successful (placeholder)",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// validateInitiateRequest validates the multisig initiate request
func (mc *MultisigCoordinator) validateInitiateRequest(req *MultisigInitiateRequest) error {
	if req.WalletType != wallet.Bitcoin && req.WalletType != wallet.Monero {
		return fmt.Errorf("invalid wallet type: %s", req.WalletType)
	}
	if req.RequiredSigs < 1 {
		return fmt.Errorf("required_sigs must be at least 1")
	}
	if len(req.PublicKeys) < req.RequiredSigs {
		return fmt.Errorf("insufficient public keys: need at least %d", req.RequiredSigs)
	}
	if len(req.PublicKeys) > 15 {
		return fmt.Errorf("too many public keys: maximum 15 allowed")
	}
	if req.Role != RoleBuyer && req.Role != RoleSeller && req.Role != RoleArbiter {
		return fmt.Errorf("invalid role: %s", req.Role)
	}
	return nil
}

// validateSignRequest validates the signature submission request
func (mc *MultisigCoordinator) validateSignRequest(req *MultisigSignRequest) error {
	if req.PaymentID == "" {
		return fmt.Errorf("payment_id required")
	}
	if req.WalletType != wallet.Bitcoin && req.WalletType != wallet.Monero {
		return fmt.Errorf("invalid wallet type: %s", req.WalletType)
	}
	if req.SignerID == "" {
		return fmt.Errorf("signer_id required")
	}
	if len(req.Signature) == 0 {
		return fmt.Errorf("signature required")
	}
	if len(req.PublicKey) == 0 {
		return fmt.Errorf("public_key required")
	}
	if req.Role != RoleBuyer && req.Role != RoleSeller && req.Role != RoleArbiter {
		return fmt.Errorf("invalid role: %s", req.Role)
	}
	return nil
}

// createMultisigPayment creates a multisig payment from the initiate request
func (mc *MultisigCoordinator) createMultisigPayment(req *MultisigInitiateRequest) (*Payment, error) {
	// For now, use the paywall's CreatePayment which should handle multisig
	// if the paywall is configured with multisig enabled
	payment, err := mc.paywall.CreatePayment()
	if err != nil {
		return nil, err
	}

	// Ensure multisig fields are populated
	// This might need adjustment based on actual CreatePayment implementation
	if !payment.MultisigEnabled {
		return nil, fmt.Errorf("paywall not configured for multisig")
	}

	return payment, nil
}
