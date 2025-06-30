package paywall

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// mockPaymentStore implements PaymentStore interface for testing
type mockPaymentStore struct {
	payments map[string]*Payment
	getErr   error
}

func newMockPaymentStore() *mockPaymentStore {
	return &mockPaymentStore{
		payments: make(map[string]*Payment),
	}
}

func (m *mockPaymentStore) GetPayment(id string) (*Payment, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	payment, exists := m.payments[id]
	if !exists {
		return nil, errors.New("payment not found")
	}
	return payment, nil
}

func (m *mockPaymentStore) CreatePayment(payment *Payment) error {
	m.payments[payment.ID] = payment
	return nil
}

func (m *mockPaymentStore) GetPaymentByAddress(address string) (*Payment, error) {
	for _, payment := range m.payments {
		for _, addr := range payment.Addresses {
			if addr == address {
				return payment, nil
			}
		}
	}
	return nil, errors.New("payment not found")
}

func (m *mockPaymentStore) UpdatePayment(payment *Payment) error {
	m.payments[payment.ID] = payment
	return nil
}

func (m *mockPaymentStore) ListPendingPayments() ([]*Payment, error) {
	var pending []*Payment
	for _, payment := range m.payments {
		if payment.Status == StatusPending {
			pending = append(pending, payment)
		}
	}
	return pending, nil
}

// mockPaywall provides a testable Paywall instance
type mockPaywall struct {
	store           PaymentStore
	createPaymentFn func() (*Payment, error)
	renderPageFn    func(w http.ResponseWriter, payment *Payment)
}

func (m *mockPaywall) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Replicate middleware logic for testing
		cookie, err := r.Cookie("payment_id")
		if err == nil {
			// Cookie exists, verify payment
			cookie.Expires = time.Now().Add(1 * time.Hour)
			http.SetCookie(w, cookie)
			payment, err := m.store.GetPayment(cookie.Value)
			if err == nil && payment != nil {
				if payment.Status == StatusConfirmed && time.Now().Before(payment.ExpiresAt) {
					next.ServeHTTP(w, r)
					return
				}
				if payment.Status == StatusPending && time.Now().Before(payment.ExpiresAt) {
					m.renderPageFn(w, payment)
					return
				}
			}
		}

		// No valid payment found, create new one
		payment, err := m.createPaymentFn()
		if err != nil {
			http.Error(w, "Failed to create payment", http.StatusInternalServerError)
			return
		}
		cookieExpiration := time.Now().Add(1 * time.Hour)

		// Set cookie for new payment
		http.SetCookie(w, &http.Cookie{
			Name:     "__Host-payment_id",
			Value:    payment.ID,
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Domain:   "",
			Expires:  cookieExpiration,
		})

		m.renderPageFn(w, payment)
	})
}

func (m *mockPaywall) MiddlewareFunc(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(m.Middleware(next).(http.HandlerFunc))
}

func (m *mockPaywall) MiddlewareFuncFunc(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(m.Middleware(next).(http.HandlerFunc))
}

// Test data helpers
func createTestPaymentWithDetails(id string, status PaymentStatus, expiresAt time.Time) *Payment {
	return &Payment{
		ID:        id,
		Status:    status,
		ExpiresAt: expiresAt,
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2",
			wallet.Monero:  "48edfHu7V9Z84YzzMa6fUueoELZ9ZRXq9VetWzYGzKt52XU5xvqgzYnDK9URnRoJMk1j8nLwEVsaSWJ4fhdUyZijBGUicoD",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.001,
			wallet.Monero:  0.1,
		},
		CreatedAt: time.Now(),
	}
}

func TestMiddleware_NoExistingCookie_CreatesNewPayment(t *testing.T) {
	store := newMockPaymentStore()
	renderPageCalled := false
	newPayment := createTestPaymentWithDetails("new-payment-123", StatusPending, time.Now().Add(1*time.Hour))

	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			return newPayment, nil
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			renderPageCalled = true
			if payment.ID != newPayment.ID {
				t.Errorf("Expected payment ID %s, got %s", newPayment.ID, payment.ID)
			}
		},
	}

	// Create test handler
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected content"))
	})

	// Create request without payment cookie
	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	// Execute middleware
	paywall.Middleware(nextHandler).ServeHTTP(w, req)

	// Verify new payment cookie was set
	cookies := w.Result().Cookies()
	var paymentCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "__Host-payment_id" {
			paymentCookie = cookie
			break
		}
	}

	if paymentCookie == nil {
		t.Fatal("Expected payment_id cookie to be set")
	}

	if paymentCookie.Value != newPayment.ID {
		t.Errorf("Expected cookie value %s, got %s", newPayment.ID, paymentCookie.Value)
	}

	// Verify cookie security attributes
	if !paymentCookie.Secure {
		t.Error("Expected cookie to be Secure")
	}
	if !paymentCookie.HttpOnly {
		t.Error("Expected cookie to be HttpOnly")
	}
	if paymentCookie.SameSite != http.SameSiteStrictMode {
		t.Error("Expected cookie SameSite to be Strict")
	}

	if !renderPageCalled {
		t.Error("Expected render page function to be called")
	}
}

func TestMiddleware_ConfirmedPayment_AllowsAccess(t *testing.T) {
	store := newMockPaymentStore()
	confirmedPayment := createTestPaymentWithDetails("confirmed-123", StatusConfirmed, time.Now().Add(1*time.Hour))
	store.CreatePayment(confirmedPayment)

	nextHandlerCalled := false
	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			t.Error("Should not create new payment for confirmed payment")
			return nil, errors.New("unexpected call")
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			t.Error("Should not render payment page for confirmed payment")
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected content"))
	})

	// Create request with valid payment cookie
	req := httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "payment_id",
		Value: confirmedPayment.ID,
	})
	w := httptest.NewRecorder()

	paywall.Middleware(nextHandler).ServeHTTP(w, req)

	if !nextHandlerCalled {
		t.Error("Expected next handler to be called for confirmed payment")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "protected content") {
		t.Error("Expected protected content to be served")
	}
}

func TestMiddleware_PendingPayment_ShowsPaymentPage(t *testing.T) {
	store := newMockPaymentStore()
	pendingPayment := createTestPaymentWithDetails("pending-456", StatusPending, time.Now().Add(1*time.Hour))
	store.CreatePayment(pendingPayment)

	renderPageCalled := false
	var renderedPayment *Payment

	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			t.Error("Should not create new payment for pending payment")
			return nil, errors.New("unexpected call")
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			renderPageCalled = true
			renderedPayment = payment
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("payment page"))
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not call next handler for pending payment")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "payment_id",
		Value: pendingPayment.ID,
	})
	w := httptest.NewRecorder()

	paywall.Middleware(nextHandler).ServeHTTP(w, req)

	if !renderPageCalled {
		t.Error("Expected render page function to be called for pending payment")
	}

	if renderedPayment == nil || renderedPayment.ID != pendingPayment.ID {
		t.Error("Expected pending payment to be passed to render function")
	}

	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "payment page") {
		t.Error("Expected payment page content to be served")
	}
}

func TestMiddleware_ExpiredPayment_CreatesNewPayment(t *testing.T) {
	store := newMockPaymentStore()
	expiredPayment := createTestPaymentWithDetails("expired-789", StatusPending, time.Now().Add(-1*time.Hour))
	store.CreatePayment(expiredPayment)

	newPayment := createTestPaymentWithDetails("new-payment-456", StatusPending, time.Now().Add(1*time.Hour))
	renderPageCalled := false

	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			return newPayment, nil
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			renderPageCalled = true
			if payment.ID != newPayment.ID {
				t.Errorf("Expected new payment ID %s, got %s", newPayment.ID, payment.ID)
			}
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not call next handler for expired payment")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "payment_id",
		Value: expiredPayment.ID,
	})
	w := httptest.NewRecorder()

	paywall.Middleware(nextHandler).ServeHTTP(w, req)

	if !renderPageCalled {
		t.Error("Expected render page function to be called for expired payment")
	}

	// Verify new payment cookie was set
	cookies := w.Result().Cookies()
	var paymentCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "__Host-payment_id" {
			paymentCookie = cookie
			break
		}
	}

	if paymentCookie == nil {
		t.Fatal("Expected new payment_id cookie to be set for expired payment")
	}

	if paymentCookie.Value != newPayment.ID {
		t.Errorf("Expected new cookie value %s, got %s", newPayment.ID, paymentCookie.Value)
	}
}

func TestMiddleware_CreatePaymentError_Returns500(t *testing.T) {
	store := newMockPaymentStore()

	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			return nil, errors.New("payment creation failed")
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			t.Error("Should not render page when payment creation fails")
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not call next handler when payment creation fails")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()

	paywall.Middleware(nextHandler).ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "Failed to create payment") {
		t.Error("Expected error message in response body")
	}
}

func TestMiddleware_PaymentStoreError_CreatesNewPayment(t *testing.T) {
	store := newMockPaymentStore()
	store.getErr = errors.New("database error")

	newPayment := createTestPaymentWithDetails("new-payment-err", StatusPending, time.Now().Add(1*time.Hour))
	renderPageCalled := false

	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			return newPayment, nil
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			renderPageCalled = true
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not call next handler when store has error")
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "payment_id",
		Value: "some-id",
	})
	w := httptest.NewRecorder()

	paywall.Middleware(nextHandler).ServeHTTP(w, req)

	if !renderPageCalled {
		t.Error("Expected render page function to be called when store errors")
	}
}

// Table-driven test for different payment scenarios
func TestMiddleware_PaymentScenarios(t *testing.T) {
	tests := []struct {
		name               string
		payment            *Payment
		cookieExists       bool
		expectNextHandler  bool
		expectRenderPage   bool
		expectNewPayment   bool
		expectedStatusCode int
	}{
		{
			name:               "No cookie",
			payment:            nil,
			cookieExists:       false,
			expectNextHandler:  false,
			expectRenderPage:   true,
			expectNewPayment:   true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Confirmed payment not expired",
			payment:            createTestPaymentWithDetails("conf-123", StatusConfirmed, time.Now().Add(1*time.Hour)),
			cookieExists:       true,
			expectNextHandler:  true,
			expectRenderPage:   false,
			expectNewPayment:   false,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Pending payment not expired",
			payment:            createTestPaymentWithDetails("pend-456", StatusPending, time.Now().Add(1*time.Hour)),
			cookieExists:       true,
			expectNextHandler:  false,
			expectRenderPage:   true,
			expectNewPayment:   false,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Confirmed payment expired",
			payment:            createTestPaymentWithDetails("conf-exp", StatusConfirmed, time.Now().Add(-1*time.Hour)),
			cookieExists:       true,
			expectNextHandler:  false,
			expectRenderPage:   true,
			expectNewPayment:   true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "Pending payment expired",
			payment:            createTestPaymentWithDetails("pend-exp", StatusPending, time.Now().Add(-1*time.Hour)),
			cookieExists:       true,
			expectNextHandler:  false,
			expectRenderPage:   true,
			expectNewPayment:   true,
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMockPaymentStore()
			if tt.payment != nil {
				store.CreatePayment(tt.payment)
			}

			nextHandlerCalled := false
			renderPageCalled := false
			createPaymentCalled := false

			newPayment := createTestPaymentWithDetails("new-test", StatusPending, time.Now().Add(1*time.Hour))

			paywall := &mockPaywall{
				store: store,
				createPaymentFn: func() (*Payment, error) {
					createPaymentCalled = true
					return newPayment, nil
				},
				renderPageFn: func(w http.ResponseWriter, payment *Payment) {
					renderPageCalled = true
					w.WriteHeader(http.StatusOK)
				},
			}

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextHandlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/protected", nil)
			if tt.cookieExists && tt.payment != nil {
				req.AddCookie(&http.Cookie{
					Name:  "payment_id",
					Value: tt.payment.ID,
				})
			}
			w := httptest.NewRecorder()

			paywall.Middleware(nextHandler).ServeHTTP(w, req)

			if nextHandlerCalled != tt.expectNextHandler {
				t.Errorf("Expected nextHandler called: %v, got: %v", tt.expectNextHandler, nextHandlerCalled)
			}

			if renderPageCalled != tt.expectRenderPage {
				t.Errorf("Expected renderPage called: %v, got: %v", tt.expectRenderPage, renderPageCalled)
			}

			if createPaymentCalled != tt.expectNewPayment {
				t.Errorf("Expected createPayment called: %v, got: %v", tt.expectNewPayment, createPaymentCalled)
			}

			if w.Code != tt.expectedStatusCode {
				t.Errorf("Expected status code: %d, got: %d", tt.expectedStatusCode, w.Code)
			}
		})
	}
}

func TestMiddlewareFunc_ReturnsCorrectHandlerFunc(t *testing.T) {
	store := newMockPaymentStore()
	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			return createTestPaymentWithDetails("test", StatusPending, time.Now().Add(1*time.Hour)), nil
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			w.WriteHeader(http.StatusOK)
		},
	}

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Test that MiddlewareFunc returns a valid http.HandlerFunc
	handlerFunc := paywall.MiddlewareFunc(nextHandler)
	if handlerFunc == nil {
		t.Fatal("MiddlewareFunc should return a non-nil HandlerFunc")
	}

	// Test that the returned function behaves correctly
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handlerFunc.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestMiddlewareFuncFunc_ReturnsCorrectHandlerFunc(t *testing.T) {
	store := newMockPaymentStore()
	paywall := &mockPaywall{
		store: store,
		createPaymentFn: func() (*Payment, error) {
			return createTestPaymentWithDetails("test", StatusPending, time.Now().Add(1*time.Hour)), nil
		},
		renderPageFn: func(w http.ResponseWriter, payment *Payment) {
			w.WriteHeader(http.StatusOK)
		},
	}

	nextHandlerFunc := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	// Test that MiddlewareFuncFunc returns a valid http.HandlerFunc
	handlerFunc := paywall.MiddlewareFuncFunc(nextHandlerFunc)
	if handlerFunc == nil {
		t.Fatal("MiddlewareFuncFunc should return a non-nil HandlerFunc")
	}

	// Test that the returned function behaves correctly
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handlerFunc.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
