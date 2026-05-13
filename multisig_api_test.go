// Package paywall tests the multisig API client
package paywall

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

func TestMultisigClient_InitiateMultisig(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/multisig/initiate" {
			t.Errorf("Expected path /multisig/initiate, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Verify request body
		var req MultisigInitiateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.WalletType != wallet.Bitcoin {
			t.Errorf("Expected wallet type Bitcoin, got %s", req.WalletType)
		}
		if req.RequiredSigs != 2 {
			t.Errorf("Expected 2 required sigs, got %d", req.RequiredSigs)
		}

		// Send response
		resp := MultisigInitiateResponse{
			PaymentID:    "test-payment-123",
			Address:      "bc1qtest123",
			Amount:       0.001,
			RedeemScript: []byte("redeem-script"),
			ExpiresAt:    time.Now().Add(time.Hour),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	pubKeys := [][]byte{
		make([]byte, 33),
		make([]byte, 33),
		make([]byte, 33),
	}

	resp, err := client.InitiateMultisig(
		wallet.Bitcoin,
		2,
		pubKeys,
		RoleBuyer,
		1.0,
	)

	if err != nil {
		t.Fatalf("InitiateMultisig failed: %v", err)
	}

	if resp.PaymentID != "test-payment-123" {
		t.Errorf("Expected payment ID 'test-payment-123', got '%s'", resp.PaymentID)
	}
	if resp.Address != "bc1qtest123" {
		t.Errorf("Expected address 'bc1qtest123', got '%s'", resp.Address)
	}
	if resp.Amount != 0.001 {
		t.Errorf("Expected amount 0.001, got %f", resp.Amount)
	}
}

func TestMultisigClient_SubmitSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/multisig/sign" {
			t.Errorf("Expected path /multisig/sign, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Verify authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Expected Bearer token, got '%s'", auth)
		}

		var req MultisigSignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.PaymentID != "payment123" {
			t.Errorf("Expected payment ID 'payment123', got '%s'", req.PaymentID)
		}
		if req.SignerID != "signer1" {
			t.Errorf("Expected signer ID 'signer1', got '%s'", req.SignerID)
		}

		resp := MultisigSignResponse{
			Success:            true,
			SignatureCount:     1,
			RequiredSignatures: 2,
			Message:            "Signature accepted (1 of 2)",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	resp, err := client.SubmitSignature(
		"payment123",
		wallet.Bitcoin,
		"signer1",
		RoleBuyer,
		[]byte("signature-bytes"),
		[]byte("pubkey-bytes"),
	)

	if err != nil {
		t.Fatalf("SubmitSignature failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected successful signature submission")
	}
	if resp.SignatureCount != 1 {
		t.Errorf("Expected signature count 1, got %d", resp.SignatureCount)
	}
	if resp.RequiredSignatures != 2 {
		t.Errorf("Expected required signatures 2, got %d", resp.RequiredSignatures)
	}
}

func TestMultisigClient_GetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/multisig/status/payment123" {
			t.Errorf("Expected path /multisig/status/payment123, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		resp := MultisigStatusResponse{
			PaymentID:     "payment123",
			Status:        StatusPending,
			Confirmations: 0,
			Signatures: map[wallet.WalletType][]SignatureData{
				wallet.Bitcoin: {
					{SignerID: "signer1", Signature: []byte("sig1")},
				},
			},
			RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
			ReadyToBroadcast:   false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	resp, err := client.GetStatus("payment123")

	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if resp.PaymentID != "payment123" {
		t.Errorf("Expected payment ID 'payment123', got '%s'", resp.PaymentID)
	}
	if resp.Status != StatusPending {
		t.Errorf("Expected status pending, got '%s'", resp.Status)
	}
	if resp.ReadyToBroadcast {
		t.Error("Expected ReadyToBroadcast to be false")
	}
	if len(resp.Signatures[wallet.Bitcoin]) != 1 {
		t.Errorf("Expected 1 signature, got %d", len(resp.Signatures[wallet.Bitcoin]))
	}
}

func TestMultisigClient_BroadcastTransaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/multisig/broadcast" {
			t.Errorf("Expected path /multisig/broadcast, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		var req MultisigBroadcastRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.PaymentID != "payment123" {
			t.Errorf("Expected payment ID 'payment123', got '%s'", req.PaymentID)
		}
		if len(req.Transaction) == 0 {
			t.Error("Expected transaction bytes")
		}

		resp := MultisigBroadcastResponse{
			Success:       true,
			TransactionID: "tx123abc",
			Message:       "Transaction broadcast successful",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	resp, err := client.BroadcastTransaction(
		"payment123",
		wallet.Bitcoin,
		[]byte("signed-tx-bytes"),
	)

	if err != nil {
		t.Fatalf("BroadcastTransaction failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected successful broadcast")
	}
	if resp.TransactionID != "tx123abc" {
		t.Errorf("Expected transaction ID 'tx123abc', got '%s'", resp.TransactionID)
	}
}

func TestMultisigClient_WaitForSignatures(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// First call: not ready
		// Second call: ready
		readyToBroadcast := callCount >= 2

		resp := MultisigStatusResponse{
			PaymentID:          "payment123",
			Status:             StatusPending,
			ReadyToBroadcast:   readyToBroadcast,
			RequiredSignatures: map[wallet.WalletType]int{wallet.Bitcoin: 2},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	resp, err := client.WaitForSignatures("payment123", 5*time.Second, 100*time.Millisecond)

	if err != nil {
		t.Fatalf("WaitForSignatures failed: %v", err)
	}

	if !resp.ReadyToBroadcast {
		t.Error("Expected ReadyToBroadcast to be true")
	}

	if callCount < 2 {
		t.Errorf("Expected at least 2 API calls, got %d", callCount)
	}
}

func TestMultisigClient_WaitForSignatures_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return not ready
		resp := MultisigStatusResponse{
			PaymentID:        "payment123",
			ReadyToBroadcast: false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	_, err := client.WaitForSignatures("payment123", 200*time.Millisecond, 50*time.Millisecond)

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}
}

func TestMultisigClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
	}))
	defer server.Close()

	client := NewMultisigClient(server.URL, "test-token")

	_, err := client.InitiateMultisig(
		wallet.Bitcoin,
		2,
		[][]byte{make([]byte, 33)},
		RoleBuyer,
		1.0,
	)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestMultisigClient_SetTimeout(t *testing.T) {
	client := NewMultisigClient("https://example.com", "token")

	client.SetTimeout(5 * time.Second)

	if client.httpClient.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", client.httpClient.Timeout)
	}
}

func TestMultisigClient_SetAuthToken(t *testing.T) {
	client := NewMultisigClient("https://example.com", "old-token")

	client.SetAuthToken("new-token")

	if client.authToken != "new-token" {
		t.Errorf("Expected auth token 'new-token', got '%s'", client.authToken)
	}
}

func TestMultisigClient_NetworkError(t *testing.T) {
	// Use invalid URL to trigger network error
	client := NewMultisigClient("http://invalid-url-that-does-not-exist:99999", "token")
	client.SetTimeout(100 * time.Millisecond)

	_, err := client.GetStatus("payment123")

	if err == nil {
		t.Fatal("Expected network error, got nil")
	}
}
