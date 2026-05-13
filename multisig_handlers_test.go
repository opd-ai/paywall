// Package paywall tests multisig HTTP handlers
package paywall

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// mockAuthenticator implements MultisigAuthenticator for testing
type mockAuthenticator struct {
	shouldFail bool
}

func (m *mockAuthenticator) Authenticate(r *http.Request, paymentID string, role MultisigRole) error {
	if m.shouldFail {
		return ErrInvalidEscrowState // reuse existing error
	}
	return nil
}

// mockNotifier implements MultisigWebhookNotifier for testing
type mockNotifier struct {
	signatureReceived int
	readyToBroadcast  int
	broadcastComplete int
	lastPaymentID     string
	lastTxID          string
}

func (m *mockNotifier) NotifySignatureReceived(paymentID string, signerID string, role MultisigRole) error {
	m.signatureReceived++
	m.lastPaymentID = paymentID
	return nil
}

func (m *mockNotifier) NotifyReadyToBroadcast(paymentID string) error {
	m.readyToBroadcast++
	m.lastPaymentID = paymentID
	return nil
}

func (m *mockNotifier) NotifyBroadcastComplete(paymentID string, txID string) error {
	m.broadcastComplete++
	m.lastPaymentID = paymentID
	m.lastTxID = txID
	return nil
}

func TestMultisigCoordinator_HandleInitiate(t *testing.T) {
	// Create test paywall with multisig enabled
	pubKeys := [][]byte{
		make([]byte, 33),
		make([]byte, 33),
		make([]byte, 33),
	}
	pw, err := NewPaywall(Config{
		PriceInBTC:       0.001,
		TestNet:          true,
		Store:            NewMemoryStore(),
		PaymentTimeout:   time.Hour,
		MultisigEnabled:  true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{
			wallet.Bitcoin: pubKeys,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	coordinator := NewMultisigCoordinator(pw, nil, nil)

	tests := []struct {
		name       string
		request    MultisigInitiateRequest
		wantStatus int
		wantError  bool
	}{
		{
			name: "valid bitcoin 2-of-3",
			request: MultisigInitiateRequest{
				WalletType:   wallet.Bitcoin,
				RequiredSigs: 2,
				PublicKeys:   [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)},
				Role:         RoleBuyer,
			},
			wantStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name: "invalid wallet type",
			request: MultisigInitiateRequest{
				WalletType:   "invalid",
				RequiredSigs: 2,
				PublicKeys:   [][]byte{make([]byte, 33), make([]byte, 33)},
				Role:         RoleBuyer,
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "insufficient public keys",
			request: MultisigInitiateRequest{
				WalletType:   wallet.Bitcoin,
				RequiredSigs: 3,
				PublicKeys:   [][]byte{make([]byte, 33), make([]byte, 33)},
				Role:         RoleBuyer,
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
		{
			name: "too many public keys",
			request: MultisigInitiateRequest{
				WalletType:   wallet.Bitcoin,
				RequiredSigs: 10,
				PublicKeys:   make([][]byte, 20),
				Role:         RoleBuyer,
			},
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/multisig/initiate", bytes.NewReader(body))
			w := httptest.NewRecorder()

			coordinator.HandleInitiate(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("HandleInitiate() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if !tt.wantError && w.Code == http.StatusOK {
				var resp MultisigInitiateResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Errorf("Failed to decode response: %v", err)
				}
				if resp.PaymentID == "" {
					t.Error("Expected PaymentID in response")
				}
				if resp.Address == "" {
					t.Error("Expected Address in response")
				}
			}
		})
	}
}

func TestMultisigCoordinator_HandleInitiate_Authentication(t *testing.T) {
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	auth := &mockAuthenticator{shouldFail: true}
	coordinator := NewMultisigCoordinator(pw, auth, nil)

	req := MultisigInitiateRequest{
		WalletType:   wallet.Bitcoin,
		RequiredSigs: 2,
		PublicKeys:   [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)},
		Role:         RoleBuyer,
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/multisig/initiate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	coordinator.HandleInitiate(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestMultisigCoordinator_HandleSign(t *testing.T) {
	// Create paywall and payment
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	// Create a payment manually for testing
	payment := &Payment{
		ID:                 "test-payment",
		MultisigEnabled:    true,
		RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
		Signatures:         make(map[wallet.WalletType][]SignatureData),
		Status:             StatusPending,
		CreatedAt:          time.Now(),
		ExpiresAt:          time.Now().Add(time.Hour),
	}
	pw.Store.CreatePayment(payment)

	notifier := &mockNotifier{}
	coordinator := NewMultisigCoordinator(pw, nil, notifier)

	tests := []struct {
		name       string
		request    MultisigSignRequest
		wantStatus int
		wantNotify bool
	}{
		{
			name: "valid signature",
			request: MultisigSignRequest{
				PaymentID:  "test-payment",
				WalletType: wallet.Bitcoin,
				SignerID:   "signer1",
				Role:       RoleBuyer,
				Signature:  []byte("signature1"),
				PublicKey:  []byte("pubkey1"),
			},
			wantStatus: http.StatusOK,
			wantNotify: true,
		},
		{
			name: "missing payment id",
			request: MultisigSignRequest{
				WalletType: wallet.Bitcoin,
				SignerID:   "signer1",
				Role:       RoleBuyer,
				Signature:  []byte("signature1"),
				PublicKey:  []byte("pubkey1"),
			},
			wantStatus: http.StatusBadRequest,
			wantNotify: false,
		},
		{
			name: "invalid wallet type",
			request: MultisigSignRequest{
				PaymentID:  "test-payment",
				WalletType: "invalid",
				SignerID:   "signer1",
				Role:       RoleBuyer,
				Signature:  []byte("signature1"),
				PublicKey:  []byte("pubkey1"),
			},
			wantStatus: http.StatusBadRequest,
			wantNotify: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier.signatureReceived = 0
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/multisig/sign", bytes.NewReader(body))
			w := httptest.NewRecorder()

			coordinator.HandleSign(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("HandleSign() status = %v, want %v", w.Code, tt.wantStatus)
			}

			if tt.wantNotify && notifier.signatureReceived != 1 {
				t.Errorf("Expected notification, got %d", notifier.signatureReceived)
			}
		})
	}
}

func TestMultisigCoordinator_HandleSign_ReadyToBroadcast(t *testing.T) {
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	payment := &Payment{
		ID:                 "test-payment",
		MultisigEnabled:    true,
		RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {
				{SignerID: "signer1", Signature: []byte("sig1")},
			},
		},
		Status:    StatusPending,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	pw.Store.CreatePayment(payment)

	notifier := &mockNotifier{}
	coordinator := NewMultisigCoordinator(pw, nil, notifier)

	// Submit second signature (should trigger ready notification)
	req := MultisigSignRequest{
		PaymentID:  "test-payment",
		WalletType: wallet.Bitcoin,
		SignerID:   "signer2",
		Role:       RoleSeller,
		Signature:  []byte("signature2"),
		PublicKey:  []byte("pubkey2"),
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/multisig/sign", bytes.NewReader(body))
	w := httptest.NewRecorder()

	coordinator.HandleSign(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Should have received both signature and ready notifications
	time.Sleep(100 * time.Millisecond) // Wait for goroutines
	if notifier.signatureReceived != 1 {
		t.Errorf("Expected 1 signature notification, got %d", notifier.signatureReceived)
	}
	if notifier.readyToBroadcast != 1 {
		t.Errorf("Expected 1 ready notification, got %d", notifier.readyToBroadcast)
	}
}

func TestMultisigCoordinator_HandleStatus(t *testing.T) {
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	payment := &Payment{
		ID:                 "test-payment",
		MultisigEnabled:    true,
		RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {
				{SignerID: "signer1", Signature: []byte("sig1")},
			},
		},
		Status:        StatusPending,
		Confirmations: 0,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(time.Hour),
	}
	pw.Store.CreatePayment(payment)

	coordinator := NewMultisigCoordinator(pw, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/multisig/status/test-payment", nil)
	w := httptest.NewRecorder()

	coordinator.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp MultisigStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.PaymentID != "test-payment" {
		t.Errorf("Expected payment ID 'test-payment', got '%s'", resp.PaymentID)
	}
	if resp.ReadyToBroadcast {
		t.Error("Expected ReadyToBroadcast to be false (insufficient signatures)")
	}
	if len(resp.Signatures[wallet.Bitcoin]) != 1 {
		t.Errorf("Expected 1 signature, got %d", len(resp.Signatures[wallet.Bitcoin]))
	}
}

func TestMultisigCoordinator_HandleBroadcast(t *testing.T) {
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	payment := &Payment{
		ID:                 "test-payment",
		MultisigEnabled:    true,
		RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {
				{SignerID: "signer1", Signature: []byte("sig1")},
				{SignerID: "signer2", Signature: []byte("sig2")},
			},
		},
		Status:    StatusPending,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	pw.Store.CreatePayment(payment)

	notifier := &mockNotifier{}
	coordinator := NewMultisigCoordinator(pw, nil, notifier)

	req := MultisigBroadcastRequest{
		PaymentID:   "test-payment",
		WalletType:  wallet.Bitcoin,
		Transaction: []byte("signed-transaction-bytes"),
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/multisig/broadcast", bytes.NewReader(body))
	w := httptest.NewRecorder()

	coordinator.HandleBroadcast(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp MultisigBroadcastResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("Expected successful broadcast")
	}
	if resp.TransactionID == "" {
		t.Error("Expected transaction ID in response")
	}

	// Check notification
	time.Sleep(100 * time.Millisecond)
	if notifier.broadcastComplete != 1 {
		t.Errorf("Expected 1 broadcast notification, got %d", notifier.broadcastComplete)
	}
}

func TestMultisigCoordinator_HandleBroadcast_InsufficientSignatures(t *testing.T) {
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	payment := &Payment{
		ID:                 "test-payment",
		MultisigEnabled:    true,
		RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
		Signatures: map[wallet.WalletType][]SignatureData{
			wallet.Bitcoin: {
				{SignerID: "signer1", Signature: []byte("sig1")},
			},
		},
		Status:    StatusPending,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	pw.Store.CreatePayment(payment)

	coordinator := NewMultisigCoordinator(pw, nil, nil)

	req := MultisigBroadcastRequest{
		PaymentID:   "test-payment",
		WalletType:  wallet.Bitcoin,
		Transaction: []byte("signed-transaction-bytes"),
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest(http.MethodPost, "/multisig/broadcast", bytes.NewReader(body))
	w := httptest.NewRecorder()

	coordinator.HandleBroadcast(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestMultisigCoordinator_MethodNotAllowed(t *testing.T) {
	pubKeys := [][]byte{make([]byte, 33), make([]byte, 33), make([]byte, 33)}
	pw, err := NewPaywall(Config{
		PriceInBTC:     0.001,
		TestNet:        true,
		Store:          NewMemoryStore(),
		PaymentTimeout: time.Hour,
		MultisigEnabled: true,
		MultisigRequired: 2,
		MultisigTotal:    3,
		ParticipantPubKeys: map[wallet.WalletType][][]byte{wallet.Bitcoin: pubKeys},
	})
	if err != nil {
		t.Fatalf("Failed to create paywall: %v", err)
	}
	defer pw.Close()

	coordinator := NewMultisigCoordinator(pw, nil, nil)

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		method  string
		path    string
	}{
		{
			name:    "initiate GET not allowed",
			handler: coordinator.HandleInitiate,
			method:  http.MethodGet,
			path:    "/multisig/initiate",
		},
		{
			name:    "sign GET not allowed",
			handler: coordinator.HandleSign,
			method:  http.MethodGet,
			path:    "/multisig/sign",
		},
		{
			name:    "status POST not allowed",
			handler: coordinator.HandleStatus,
			method:  http.MethodPost,
			path:    "/multisig/status/test",
		},
		{
			name:    "broadcast GET not allowed",
			handler: coordinator.HandleBroadcast,
			method:  http.MethodGet,
			path:    "/multisig/broadcast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405, got %d", w.Code)
			}
		})
	}
}
