package paywall

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPWebhookNotifier_NotifySignatureReceived(t *testing.T) {
	var callCount int32
	var receivedEvent WebhookEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)

		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEvent)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		SignatureReceivedURL: server.URL,
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifySignatureReceived("payment123", "signer456", RoleBuyer)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected 1 webhook call, got %d", callCount)
	}

	if receivedEvent.EventType != "signature_received" {
		t.Errorf("Expected event_type 'signature_received', got '%s'", receivedEvent.EventType)
	}

	if receivedEvent.PaymentID != "payment123" {
		t.Errorf("Expected payment_id 'payment123', got '%s'", receivedEvent.PaymentID)
	}

	if receivedEvent.SignerID != "signer456" {
		t.Errorf("Expected signer_id 'signer456', got '%s'", receivedEvent.SignerID)
	}

	if receivedEvent.Role != RoleBuyer {
		t.Errorf("Expected role 'buyer', got '%s'", receivedEvent.Role)
	}
}

func TestHTTPWebhookNotifier_NotifyReadyToBroadcast(t *testing.T) {
	var receivedEvent WebhookEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		ReadyToBroadcastURL: server.URL,
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifyReadyToBroadcast("payment789")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if receivedEvent.EventType != "ready_to_broadcast" {
		t.Errorf("Expected event_type 'ready_to_broadcast', got '%s'", receivedEvent.EventType)
	}

	if receivedEvent.PaymentID != "payment789" {
		t.Errorf("Expected payment_id 'payment789', got '%s'", receivedEvent.PaymentID)
	}
}

func TestHTTPWebhookNotifier_NotifyBroadcastComplete(t *testing.T) {
	var receivedEvent WebhookEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		BroadcastCompleteURL: server.URL,
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifyBroadcastComplete("payment999", "tx123abc")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if receivedEvent.EventType != "broadcast_complete" {
		t.Errorf("Expected event_type 'broadcast_complete', got '%s'", receivedEvent.EventType)
	}

	if receivedEvent.PaymentID != "payment999" {
		t.Errorf("Expected payment_id 'payment999', got '%s'", receivedEvent.PaymentID)
	}

	if receivedEvent.TransactionID != "tx123abc" {
		t.Errorf("Expected transaction_id 'tx123abc', got '%s'", receivedEvent.TransactionID)
	}
}

func TestHTTPWebhookNotifier_Retry(t *testing.T) {
	var callCount int32
	attemptToSucceed := int32(2) // Fail first, succeed on second attempt

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count < attemptToSucceed {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		SignatureReceivedURL: server.URL,
		MaxRetries:           3,
		RetryDelay:           50 * time.Millisecond,
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifySignatureReceived("payment123", "signer456", RoleBuyer)
	if err != nil {
		t.Fatalf("Expected eventual success, got error: %v", err)
	}

	if atomic.LoadInt32(&callCount) != attemptToSucceed {
		t.Errorf("Expected %d attempts, got %d", attemptToSucceed, callCount)
	}
}

func TestHTTPWebhookNotifier_MaxRetriesExceeded(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		SignatureReceivedURL: server.URL,
		MaxRetries:           2,
		RetryDelay:           10 * time.Millisecond,
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifySignatureReceived("payment123", "signer456", RoleBuyer)
	if err == nil {
		t.Fatal("Expected error after max retries, got nil")
	}

	// Should try: initial + 2 retries = 3 attempts
	expected := int32(3)
	if atomic.LoadInt32(&callCount) != expected {
		t.Errorf("Expected %d attempts, got %d", expected, callCount)
	}
}

func TestHTTPWebhookNotifier_ClientError_NoRetry(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		SignatureReceivedURL: server.URL,
		MaxRetries:           3,
		RetryDelay:           10 * time.Millisecond,
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifySignatureReceived("payment123", "signer456", RoleBuyer)
	if err == nil {
		t.Fatal("Expected error for 4xx response, got nil")
	}

	// Client errors (4xx) should not retry
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("Expected 1 attempt (no retries for 4xx), got %d", callCount)
	}
}

func TestHTTPWebhookNotifier_CustomHeaders(t *testing.T) {
	var receivedAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := HTTPWebhookConfig{
		SignatureReceivedURL: server.URL,
		Headers: map[string]string{
			"Authorization": "Bearer secret-token",
		},
	}
	notifier := NewHTTPWebhookNotifier(config)

	err := notifier.NotifySignatureReceived("payment123", "signer456", RoleBuyer)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedAuth := "Bearer secret-token"
	if receivedAuthHeader != expectedAuth {
		t.Errorf("Expected Authorization header '%s', got '%s'", expectedAuth, receivedAuthHeader)
	}
}

func TestHTTPWebhookNotifier_EmptyURL_NoError(t *testing.T) {
	config := HTTPWebhookConfig{
		SignatureReceivedURL: "", // Empty URL
	}
	notifier := NewHTTPWebhookNotifier(config)

	// Should return nil (no error) when URL is not configured
	err := notifier.NotifySignatureReceived("payment123", "signer456", RoleBuyer)
	if err != nil {
		t.Errorf("Expected no error for empty URL, got: %v", err)
	}
}

func TestHTTPWebhookNotifier_Defaults(t *testing.T) {
	config := HTTPWebhookConfig{
		SignatureReceivedURL: "http://example.com",
	}
	notifier := NewHTTPWebhookNotifier(config)

	if notifier.config.Timeout != 10*time.Second {
		t.Errorf("Expected default timeout 10s, got %v", notifier.config.Timeout)
	}

	if notifier.config.MaxRetries != 3 {
		t.Errorf("Expected default max retries 3, got %d", notifier.config.MaxRetries)
	}

	if notifier.config.RetryDelay != 1*time.Second {
		t.Errorf("Expected default retry delay 1s, got %v", notifier.config.RetryDelay)
	}

	if notifier.config.Headers == nil {
		t.Error("Expected headers map to be initialized, got nil")
	}
}

func TestLoggingWebhookNotifier(t *testing.T) {
	// Capture log output
	var logOutput strings.Builder
	logger := log.New(&logOutput, "", 0)

	notifier := NewLoggingWebhookNotifier(logger)

	// Test signature received
	notifier.NotifySignatureReceived("payment1", "signer1", RoleBuyer)
	if !strings.Contains(logOutput.String(), "Signature received") {
		t.Error("Expected log output for signature received")
	}

	logOutput.Reset()

	// Test ready to broadcast
	notifier.NotifyReadyToBroadcast("payment2")
	if !strings.Contains(logOutput.String(), "Ready to broadcast") {
		t.Error("Expected log output for ready to broadcast")
	}

	logOutput.Reset()

	// Test broadcast complete
	notifier.NotifyBroadcastComplete("payment3", "tx123")
	if !strings.Contains(logOutput.String(), "Broadcast complete") {
		t.Error("Expected log output for broadcast complete")
	}
}

func TestLoggingWebhookNotifier_NilLogger(t *testing.T) {
	// Should use default logger when nil is provided
	notifier := NewLoggingWebhookNotifier(nil)
	if notifier.logger == nil {
		t.Error("Expected default logger to be set, got nil")
	}

	// Should not panic
	err := notifier.NotifySignatureReceived("payment1", "signer1", RoleBuyer)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestNoOpWebhookNotifier(t *testing.T) {
	notifier := NewNoOpWebhookNotifier()

	// All methods should return nil without doing anything
	err := notifier.NotifySignatureReceived("payment1", "signer1", RoleBuyer)
	if err != nil {
		t.Errorf("Expected no error from no-op notifier, got: %v", err)
	}

	err = notifier.NotifyReadyToBroadcast("payment2")
	if err != nil {
		t.Errorf("Expected no error from no-op notifier, got: %v", err)
	}

	err = notifier.NotifyBroadcastComplete("payment3", "tx123")
	if err != nil {
		t.Errorf("Expected no error from no-op notifier, got: %v", err)
	}
}

func TestWebhookNotifierInterfaces(t *testing.T) {
	// Verify all types implement MultisigWebhookNotifier interface
	var _ MultisigWebhookNotifier = &HTTPWebhookNotifier{}
	var _ MultisigWebhookNotifier = &LoggingWebhookNotifier{}
	var _ MultisigWebhookNotifier = &NoOpWebhookNotifier{}
}

func TestWebhookEvent_JSONMarshaling(t *testing.T) {
	event := WebhookEvent{
		EventType:     "test_event",
		PaymentID:     "payment123",
		SignerID:      "signer456",
		Role:          RoleSeller,
		TransactionID: "tx789",
		Timestamp:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	var decoded WebhookEvent
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	if decoded.EventType != event.EventType {
		t.Errorf("EventType mismatch: expected %s, got %s", event.EventType, decoded.EventType)
	}

	if decoded.PaymentID != event.PaymentID {
		t.Errorf("PaymentID mismatch: expected %s, got %s", event.PaymentID, decoded.PaymentID)
	}
}
