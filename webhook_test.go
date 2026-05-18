package paywall

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWebhookDispatcher_Basic(t *testing.T) {
	received := make(chan WebhookPayload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload WebhookPayload
		json.Unmarshal(body, &payload)
		received <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		Secret:       "test-secret",
		MaxRetries:   3,
		RetryBackoff: 10 * time.Millisecond,
		Timeout:      1 * time.Second,
	}

	dispatcher := NewWebhookDispatcher(config)
	defer dispatcher.Close()

	// Dispatch test event
	testPayload := WebhookPayload{
		Event:     EventPaymentCreated,
		PaymentID: "test-payment-123",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"amount": 0.001,
		},
	}

	dispatcher.Dispatch(testPayload)

	// Wait for webhook delivery
	select {
	case recv := <-received:
		if recv.Event != EventPaymentCreated {
			t.Errorf("Expected event %s, got %s", EventPaymentCreated, recv.Event)
		}
		if recv.PaymentID != "test-payment-123" {
			t.Errorf("Expected payment ID test-payment-123, got %s", recv.PaymentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Webhook not received within timeout")
	}
}

func TestWebhookDispatcher_SignatureVerification(t *testing.T) {
	var mu sync.Mutex
	var receivedSignature string
	var receivedBody []byte
	secret := "test-secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-Webhook-Signature")
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedSignature = sig
		receivedBody = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		Secret:       secret,
		MaxRetries:   1,
		RetryBackoff: 10 * time.Millisecond,
		Timeout:      1 * time.Second,
	}

	dispatcher := NewWebhookDispatcher(config)
	defer dispatcher.Close()

	dispatcher.Dispatch(WebhookPayload{
		Event:     EventPaymentConfirmed,
		PaymentID: "test-payment-456",
		Timestamp: time.Now(),
		Data:      map[string]interface{}{},
	})

	// Wait for delivery
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	sig := receivedSignature
	body := make([]byte, len(receivedBody))
	copy(body, receivedBody)
	mu.Unlock()

	if sig == "" {
		t.Fatal("No signature received")
	}

	if !VerifyWebhookSignature(body, sig, secret) {
		t.Error("Signature verification failed")
	}
}

func TestWebhookDispatcher_EventFiltering(t *testing.T) {
	receivedCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:           server.URL,
		Secret:        "test-secret",
		MaxRetries:    1,
		RetryBackoff:  10 * time.Millisecond,
		Timeout:       1 * time.Second,
		EnabledEvents: []WebhookEventType{EventPaymentCreated, EventPaymentConfirmed},
	}

	dispatcher := NewWebhookDispatcher(config)
	defer dispatcher.Close()

	// Dispatch enabled event
	dispatcher.Dispatch(WebhookPayload{
		Event:     EventPaymentCreated,
		PaymentID: "test-1",
		Timestamp: time.Now(),
	})

	// Dispatch disabled event
	dispatcher.Dispatch(WebhookPayload{
		Event:     EventEscrowFunded,
		PaymentID: "test-2",
		Timestamp: time.Now(),
	})

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := receivedCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("Expected 1 webhook delivery, got %d", count)
	}
}

func TestWebhookDispatcher_Retry(t *testing.T) {
	attemptCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		// Fail first 2 attempts, succeed on 3rd
		if currentAttempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:          server.URL,
		Secret:       "test-secret",
		MaxRetries:   3,
		RetryBackoff: 50 * time.Millisecond,
		Timeout:      1 * time.Second,
	}

	dispatcher := NewWebhookDispatcher(config)
	defer dispatcher.Close()

	dispatcher.Dispatch(WebhookPayload{
		Event:     EventPaymentCreated,
		PaymentID: "retry-test",
		Timestamp: time.Now(),
	})

	// Wait for retries
	time.Sleep(1 * time.Second)

	mu.Lock()
	count := attemptCount
	mu.Unlock()

	if count != 3 {
		t.Errorf("Expected 3 attempts, got %d", count)
	}
}

func TestGenerateWebhookSecret(t *testing.T) {
	secret1, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("Failed to generate webhook secret: %v", err)
	}

	if len(secret1) != 64 { // 32 bytes hex-encoded = 64 characters
		t.Errorf("Expected secret length 64, got %d", len(secret1))
	}

	secret2, err := GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("Failed to generate second webhook secret: %v", err)
	}

	if secret1 == secret2 {
		t.Error("Generated secrets should be unique")
	}
}

func TestVerifyWebhookSignature_Invalid(t *testing.T) {
	payload := []byte(`{"event":"payment_created"}`)
	secret := "test-secret"
	invalidSignature := "0000000000000000000000000000000000000000000000000000000000000000"

	if VerifyWebhookSignature(payload, invalidSignature, secret) {
		t.Error("Expected signature verification to fail for invalid signature")
	}
}

func TestWebhookDispatcher_Close(t *testing.T) {
	config := WebhookConfig{
		URL:          "http://example.com/webhook",
		Secret:       "test-secret",
		MaxRetries:   1,
		RetryBackoff: 10 * time.Millisecond,
		Timeout:      1 * time.Second,
	}

	dispatcher := NewWebhookDispatcher(config)

	// Dispatch some events
	for i := 0; i < 5; i++ {
		dispatcher.Dispatch(WebhookPayload{
			Event:     EventPaymentCreated,
			PaymentID: "test",
			Timestamp: time.Now(),
		})
	}

	// Close should not hang
	done := make(chan bool)
	go func() {
		dispatcher.Close()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not complete within timeout")
	}
}
