package paywall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookEvent represents a webhook payload for multisig events
type WebhookEvent struct {
	// EventType identifies the type of webhook event
	EventType string `json:"event_type"`
	// PaymentID identifies the payment this event relates to
	PaymentID string `json:"payment_id"`
	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`
	// SignerID is populated for signature_received events
	SignerID string `json:"signer_id,omitempty"`
	// Role is populated for signature_received events
	Role MultisigRole `json:"role,omitempty"`
	// TransactionID is populated for broadcast_complete events
	TransactionID string `json:"transaction_id,omitempty"`
}

// HTTPWebhookNotifier implements MultisigWebhookNotifier by sending HTTP POST requests
// to configured webhook URLs. It supports retry with exponential backoff and configurable timeouts.
//
// Example usage:
//
//	config := HTTPWebhookConfig{
//	    SignatureReceivedURL: "https://api.example.com/webhooks/signature",
//	    ReadyToBroadcastURL:  "https://api.example.com/webhooks/ready",
//	    BroadcastCompleteURL: "https://api.example.com/webhooks/complete",
//	    Timeout:              10 * time.Second,
//	    MaxRetries:           3,
//	}
//	notifier := NewHTTPWebhookNotifier(config)
//	coordinator := NewMultisigCoordinator(paywall, auth, notifier)
type HTTPWebhookNotifier struct {
	config HTTPWebhookConfig
	client *http.Client
}

// HTTPWebhookConfig contains configuration for HTTP webhook notifications
type HTTPWebhookConfig struct {
	// SignatureReceivedURL is the endpoint to notify when a signature is received
	SignatureReceivedURL string
	// ReadyToBroadcastURL is the endpoint to notify when all signatures are collected
	ReadyToBroadcastURL string
	// BroadcastCompleteURL is the endpoint to notify when transaction is broadcast
	BroadcastCompleteURL string
	// Timeout for each HTTP request (default: 10 seconds)
	Timeout time.Duration
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int
	// RetryDelay is the initial delay between retries, doubled after each attempt (default: 1 second)
	RetryDelay time.Duration
	// Headers to include in webhook requests (e.g., for authentication)
	Headers map[string]string
}

// NewHTTPWebhookNotifier creates a new HTTP webhook notifier with the given configuration
func NewHTTPWebhookNotifier(config HTTPWebhookConfig) *HTTPWebhookNotifier {
	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.Headers == nil {
		config.Headers = make(map[string]string)
	}

	return &HTTPWebhookNotifier{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// NotifySignatureReceived sends a webhook when a signature is collected
func (h *HTTPWebhookNotifier) NotifySignatureReceived(paymentID, signerID string, role MultisigRole) error {
	if h.config.SignatureReceivedURL == "" {
		return nil // No URL configured, skip silently
	}

	event := WebhookEvent{
		EventType: "signature_received",
		PaymentID: paymentID,
		SignerID:  signerID,
		Role:      role,
		Timestamp: time.Now(),
	}

	return h.sendWebhook(h.config.SignatureReceivedURL, event)
}

// NotifyReadyToBroadcast sends a webhook when all signatures are collected
func (h *HTTPWebhookNotifier) NotifyReadyToBroadcast(paymentID string) error {
	if h.config.ReadyToBroadcastURL == "" {
		return nil // No URL configured, skip silently
	}

	event := WebhookEvent{
		EventType: "ready_to_broadcast",
		PaymentID: paymentID,
		Timestamp: time.Now(),
	}

	return h.sendWebhook(h.config.ReadyToBroadcastURL, event)
}

// NotifyBroadcastComplete sends a webhook when transaction is broadcast
func (h *HTTPWebhookNotifier) NotifyBroadcastComplete(paymentID, txID string) error {
	if h.config.BroadcastCompleteURL == "" {
		return nil // No URL configured, skip silently
	}

	event := WebhookEvent{
		EventType:     "broadcast_complete",
		PaymentID:     paymentID,
		TransactionID: txID,
		Timestamp:     time.Now(),
	}

	return h.sendWebhook(h.config.BroadcastCompleteURL, event)
}

// sendWebhook sends an HTTP POST request with retry logic
func (h *HTTPWebhookNotifier) sendWebhook(url string, event WebhookEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	var lastErr error
	delay := h.config.RetryDelay

	for attempt := 0; attempt <= h.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		for key, value := range h.config.Headers {
			req.Header.Set(key, value)
		}

		resp, err := h.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed (attempt %d/%d): %w", attempt+1, h.config.MaxRetries+1, err)
			log.Printf("[WEBHOOK] %v", lastErr)
			continue
		}

		// Read and close response body
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Check HTTP status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil // Success
		}

		lastErr = fmt.Errorf("webhook returned status %d (attempt %d/%d): %s", resp.StatusCode, attempt+1, h.config.MaxRetries+1, string(body))
		log.Printf("[WEBHOOK] %v", lastErr)

		// Don't retry on client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", h.config.MaxRetries+1, lastErr)
}

// LoggingWebhookNotifier implements MultisigWebhookNotifier by logging events
// This is useful for development, testing, or when webhook delivery is not required
type LoggingWebhookNotifier struct {
	logger *log.Logger
}

// NewLoggingWebhookNotifier creates a new logging-based webhook notifier
// If logger is nil, uses the default logger
func NewLoggingWebhookNotifier(logger *log.Logger) *LoggingWebhookNotifier {
	if logger == nil {
		logger = log.Default()
	}
	return &LoggingWebhookNotifier{
		logger: logger,
	}
}

// NotifySignatureReceived logs when a signature is collected
func (l *LoggingWebhookNotifier) NotifySignatureReceived(paymentID, signerID string, role MultisigRole) error {
	l.logger.Printf("[WEBHOOK] Signature received: payment=%s signer=%s role=%s", paymentID, signerID, role)
	return nil
}

// NotifyReadyToBroadcast logs when all signatures are collected
func (l *LoggingWebhookNotifier) NotifyReadyToBroadcast(paymentID string) error {
	l.logger.Printf("[WEBHOOK] Ready to broadcast: payment=%s", paymentID)
	return nil
}

// NotifyBroadcastComplete logs when transaction is broadcast
func (l *LoggingWebhookNotifier) NotifyBroadcastComplete(paymentID, txID string) error {
	l.logger.Printf("[WEBHOOK] Broadcast complete: payment=%s txid=%s", paymentID, txID)
	return nil
}

// NoOpWebhookNotifier implements MultisigWebhookNotifier with no-op methods
// This is useful when webhook notifications are not needed
type NoOpWebhookNotifier struct{}

// NewNoOpWebhookNotifier creates a new no-op webhook notifier
func NewNoOpWebhookNotifier() *NoOpWebhookNotifier {
	return &NoOpWebhookNotifier{}
}

// NotifySignatureReceived does nothing
func (n *NoOpWebhookNotifier) NotifySignatureReceived(paymentID, signerID string, role MultisigRole) error {
	return nil
}

// NotifyReadyToBroadcast does nothing
func (n *NoOpWebhookNotifier) NotifyReadyToBroadcast(paymentID string) error {
	return nil
}

// NotifyBroadcastComplete does nothing
func (n *NoOpWebhookNotifier) NotifyBroadcastComplete(paymentID, txID string) error {
	return nil
}
