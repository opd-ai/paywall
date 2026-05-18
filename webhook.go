package paywall

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// WebhookEventType represents the type of webhook event
type WebhookEventType string

const (
	// EventPaymentCreated is fired when a new payment is created
	EventPaymentCreated WebhookEventType = "payment_created"
	// EventPaymentConfirmed is fired when a payment receives required confirmations
	EventPaymentConfirmed WebhookEventType = "payment_confirmed"
	// EventEscrowFunded is fired when an escrow payment is funded
	EventEscrowFunded WebhookEventType = "escrow_funded"
	// EventDisputeResolved is fired when a dispute is resolved
	EventDisputeResolved WebhookEventType = "dispute_resolved"
	// EventEscrowCompleted is fired when an escrow is completed successfully
	EventEscrowCompleted WebhookEventType = "escrow_completed"
	// EventEscrowRefunded is fired when an escrow is refunded
	EventEscrowRefunded WebhookEventType = "escrow_refunded"
)

// WebhookConfig configures webhook notification behavior
type WebhookConfig struct {
	// URL is the HTTP endpoint to receive webhook notifications
	URL string
	// Secret is used to generate HMAC signatures for webhook authenticity
	Secret string
	// MaxRetries is the maximum number of retry attempts for failed webhooks
	MaxRetries int
	// RetryBackoff is the initial backoff duration, doubles on each retry
	RetryBackoff time.Duration
	// Timeout is the HTTP request timeout for webhook delivery
	Timeout time.Duration
	// EnabledEvents specifies which events should trigger webhooks
	// If nil or empty, all events are enabled
	EnabledEvents []WebhookEventType
}

// WebhookPayload contains the data sent to webhook endpoints
type WebhookPayload struct {
	// Event identifies the type of event
	Event WebhookEventType `json:"event"`
	// PaymentID is the ID of the affected payment
	PaymentID string `json:"payment_id"`
	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`
	// Data contains event-specific data
	Data map[string]interface{} `json:"data"`
}

// WebhookDispatcher manages webhook delivery with retries
type WebhookDispatcher struct {
	config     WebhookConfig
	client     *http.Client
	eventQueue chan WebhookPayload
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.RWMutex
	enabled    map[WebhookEventType]bool
}

// NewWebhookDispatcher creates a new webhook dispatcher
func NewWebhookDispatcher(config WebhookConfig) *WebhookDispatcher {
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.RetryBackoff == 0 {
		config.RetryBackoff = 1 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	enabled := make(map[WebhookEventType]bool)
	if len(config.EnabledEvents) == 0 {
		// Enable all events by default
		enabled[EventPaymentCreated] = true
		enabled[EventPaymentConfirmed] = true
		enabled[EventEscrowFunded] = true
		enabled[EventDisputeResolved] = true
		enabled[EventEscrowCompleted] = true
		enabled[EventEscrowRefunded] = true
	} else {
		for _, event := range config.EnabledEvents {
			enabled[event] = true
		}
	}

	wd := &WebhookDispatcher{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		eventQueue: make(chan WebhookPayload, 100),
		ctx:        ctx,
		cancel:     cancel,
		enabled:    enabled,
	}

	wd.wg.Add(1)
	go wd.processQueue()

	return wd
}

// Dispatch sends a webhook event asynchronously
func (wd *WebhookDispatcher) Dispatch(payload WebhookPayload) {
	wd.mu.RLock()
	enabled := wd.enabled[payload.Event]
	wd.mu.RUnlock()

	if !enabled {
		return
	}

	select {
	case wd.eventQueue <- payload:
	case <-wd.ctx.Done():
	default:
		// Queue full, drop event
	}
}

// processQueue handles webhook delivery in a background goroutine
func (wd *WebhookDispatcher) processQueue() {
	defer wd.wg.Done()

	for {
		select {
		case <-wd.ctx.Done():
			return
		case payload := <-wd.eventQueue:
			wd.deliverWithRetry(payload)
		}
	}
}

// deliverWithRetry attempts webhook delivery with exponential backoff
func (wd *WebhookDispatcher) deliverWithRetry(payload WebhookPayload) {
	backoff := wd.config.RetryBackoff

	for attempt := 0; attempt <= wd.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-wd.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		if err := wd.deliver(payload); err == nil {
			return
		}
	}
}

// deliver sends a single webhook HTTP request
func (wd *WebhookDispatcher) deliver(payload WebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(wd.ctx, "POST", wd.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "paywall-webhook/1.0")

	if wd.config.Secret != "" {
		signature := wd.generateSignature(body)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	resp, err := wd.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// generateSignature creates an HMAC-SHA256 signature for webhook authenticity
func (wd *WebhookDispatcher) generateSignature(body []byte) string {
	mac := hmac.New(sha256.New, []byte(wd.config.Secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Close stops the webhook dispatcher
func (wd *WebhookDispatcher) Close() {
	wd.cancel()
	wd.wg.Wait()
	close(wd.eventQueue)
}

// VerifyWebhookSignature verifies the HMAC signature of a webhook payload
// This is a helper function for webhook receivers to validate authenticity
func VerifyWebhookSignature(payload []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// GenerateWebhookSecret creates a cryptographically secure webhook secret
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
