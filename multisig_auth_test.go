package paywall

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHMACMultisigAuthenticator_Success(t *testing.T) {
	secrets := map[string]string{
		"buyer":   "buyer-secret",
		"seller":  "seller-secret",
		"arbiter": "arbiter-secret",
	}
	auth := NewHMACMultisigAuthenticator(secrets)

	paymentID := "payment123"
	role := RoleBuyer

	// Create valid HMAC signature
	message := paymentID + string(role)
	mac := hmac.New(sha256.New, []byte(secrets[string(role)]))
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	// Create request with valid authorization header
	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("HMAC %s", signature))

	err := auth.Authenticate(req, paymentID, role)
	if err != nil {
		t.Errorf("Expected successful authentication, got error: %v", err)
	}
}

func TestHMACMultisigAuthenticator_MissingHeader(t *testing.T) {
	auth := NewHMACMultisigAuthenticator(map[string]string{
		"buyer": "secret",
	})

	req := httptest.NewRequest("POST", "/multisig/sign", nil)

	err := auth.Authenticate(req, "payment123", RoleBuyer)
	if err != ErrMissingAuthHeader {
		t.Errorf("Expected ErrMissingAuthHeader, got: %v", err)
	}
}

func TestHMACMultisigAuthenticator_InvalidHeaderFormat(t *testing.T) {
	auth := NewHMACMultisigAuthenticator(map[string]string{
		"buyer": "secret",
	})

	testCases := []struct {
		name   string
		header string
	}{
		{"No space", "HMACsignature"},
		{"Wrong scheme", "Bearer signature"},
		{"Missing signature", "HMAC"},
		{"Empty string", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/multisig/sign", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			err := auth.Authenticate(req, "payment123", RoleBuyer)
			if err != ErrMissingAuthHeader && err != ErrInvalidAuthHeader {
				t.Errorf("Expected ErrMissingAuthHeader or ErrInvalidAuthHeader, got: %v", err)
			}
		})
	}
}

func TestHMACMultisigAuthenticator_InvalidSignature(t *testing.T) {
	auth := NewHMACMultisigAuthenticator(map[string]string{
		"buyer": "secret",
	})

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", "HMAC invalidsignature")

	err := auth.Authenticate(req, "payment123", RoleBuyer)
	if err != ErrInvalidSignature {
		t.Errorf("Expected ErrInvalidSignature, got: %v", err)
	}
}

func TestHMACMultisigAuthenticator_WrongSecret(t *testing.T) {
	auth := NewHMACMultisigAuthenticator(map[string]string{
		"buyer": "correct-secret",
	})

	paymentID := "payment123"
	role := RoleBuyer

	// Create signature with wrong secret
	message := paymentID + string(role)
	mac := hmac.New(sha256.New, []byte("wrong-secret"))
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("HMAC %s", signature))

	err := auth.Authenticate(req, paymentID, role)
	if err != ErrInvalidSignature {
		t.Errorf("Expected ErrInvalidSignature, got: %v", err)
	}
}

func TestHMACMultisigAuthenticator_NoSecretForRole(t *testing.T) {
	auth := NewHMACMultisigAuthenticator(map[string]string{
		"buyer": "buyer-secret",
	})

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", "HMAC somesignature")

	err := auth.Authenticate(req, "payment123", RoleSeller)
	if err == nil {
		t.Error("Expected error for unconfigured role, got nil")
	}
	if err != nil && err != ErrInvalidSignature {
		// Should get error about missing secret
		expectedMsg := "no secret configured for role seller"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error message %q, got %q", expectedMsg, err.Error())
		}
	}
}

func TestHMACMultisigAuthenticator_AllRoles(t *testing.T) {
	secrets := map[string]string{
		"buyer":   "buyer-secret",
		"seller":  "seller-secret",
		"arbiter": "arbiter-secret",
	}
	auth := NewHMACMultisigAuthenticator(secrets)

	roles := []MultisigRole{RoleBuyer, RoleSeller, RoleArbiter}
	paymentID := "payment456"

	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			message := paymentID + string(role)
			mac := hmac.New(sha256.New, []byte(secrets[string(role)]))
			mac.Write([]byte(message))
			signature := hex.EncodeToString(mac.Sum(nil))

			req := httptest.NewRequest("POST", "/multisig/sign", nil)
			req.Header.Set("Authorization", fmt.Sprintf("HMAC %s", signature))

			err := auth.Authenticate(req, paymentID, role)
			if err != nil {
				t.Errorf("Expected successful authentication for %s, got error: %v", role, err)
			}
		})
	}
}

func TestJWTMultisigAuthenticator_Success(t *testing.T) {
	secret := "test-secret"
	expiration := 15 * time.Minute
	auth := NewJWTMultisigAuthenticator(secret, expiration)

	paymentID := "payment789"
	role := RoleBuyer

	// Create valid token
	exp := time.Now().Add(expiration).Unix()
	payload := fmt.Sprintf("%s:%s:%d", role, paymentID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := hex.EncodeToString([]byte(payload)) + "." + signature

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	err := auth.Authenticate(req, paymentID, role)
	if err != nil {
		t.Errorf("Expected successful authentication, got error: %v", err)
	}
}

func TestJWTMultisigAuthenticator_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	auth := NewJWTMultisigAuthenticator(secret, 15*time.Minute)

	paymentID := "payment789"
	role := RoleBuyer

	// Create expired token (expired 1 hour ago)
	exp := time.Now().Add(-1 * time.Hour).Unix()
	payload := fmt.Sprintf("%s:%s:%d", role, paymentID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := hex.EncodeToString([]byte(payload)) + "." + signature

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	err := auth.Authenticate(req, paymentID, role)
	if err != ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken for expired token, got: %v", err)
	}
}

func TestJWTMultisigAuthenticator_WrongRole(t *testing.T) {
	secret := "test-secret"
	auth := NewJWTMultisigAuthenticator(secret, 15*time.Minute)

	paymentID := "payment789"

	// Create token with buyer role
	exp := time.Now().Add(15 * time.Minute).Unix()
	payload := fmt.Sprintf("%s:%s:%d", RoleBuyer, paymentID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := hex.EncodeToString([]byte(payload)) + "." + signature

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Try to authenticate as seller (should fail)
	err := auth.Authenticate(req, paymentID, RoleSeller)
	if err != ErrUnauthorizedRole {
		t.Errorf("Expected ErrUnauthorizedRole for mismatched role, got: %v", err)
	}
}

func TestJWTMultisigAuthenticator_WrongPaymentID(t *testing.T) {
	secret := "test-secret"
	auth := NewJWTMultisigAuthenticator(secret, 15*time.Minute)

	paymentID := "payment789"
	role := RoleBuyer

	// Create token with specific payment ID
	exp := time.Now().Add(15 * time.Minute).Unix()
	payload := fmt.Sprintf("%s:%s:%d", role, paymentID, exp)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := hex.EncodeToString([]byte(payload)) + "." + signature

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Try to use with different payment ID (should fail)
	err := auth.Authenticate(req, "different-payment", role)
	if err == nil {
		t.Error("Expected error for mismatched payment ID, got nil")
	}
}

func TestJWTMultisigAuthenticator_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	auth := NewJWTMultisigAuthenticator(secret, 15*time.Minute)

	paymentID := "payment789"
	role := RoleBuyer

	// Create token with wrong secret
	exp := time.Now().Add(15 * time.Minute).Unix()
	payload := fmt.Sprintf("%s:%s:%d", role, paymentID, exp)
	mac := hmac.New(sha256.New, []byte("wrong-secret"))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))
	token := hex.EncodeToString([]byte(payload)) + "." + signature

	req := httptest.NewRequest("POST", "/multisig/sign", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	err := auth.Authenticate(req, paymentID, role)
	if err != ErrInvalidSignature {
		t.Errorf("Expected ErrInvalidSignature, got: %v", err)
	}
}

func TestJWTMultisigAuthenticator_InvalidTokenFormat(t *testing.T) {
	auth := NewJWTMultisigAuthenticator("secret", 15*time.Minute)

	testCases := []struct {
		name   string
		header string
	}{
		{"Missing dot", "Bearer invalidtoken"},
		{"No Bearer", "HMAC token"},
		{"Empty token", "Bearer"},
		{"Invalid hex", "Bearer xyz.abc"},
		{"Invalid payload format", "Bearer " + hex.EncodeToString([]byte("nocolons")) + "." + hex.EncodeToString([]byte("sig"))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/multisig/sign", nil)
			req.Header.Set("Authorization", tc.header)

			err := auth.Authenticate(req, "payment123", RoleBuyer)
			if err == nil {
				t.Error("Expected error for invalid token format, got nil")
			}
		})
	}
}

func TestNoAuthMultisigAuthenticator_AllowsAll(t *testing.T) {
	auth := NewNoAuthMultisigAuthenticator()

	testCases := []struct {
		name      string
		paymentID string
		role      MultisigRole
	}{
		{"Buyer", "payment1", RoleBuyer},
		{"Seller", "payment2", RoleSeller},
		{"Arbiter", "payment3", RoleArbiter},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/multisig/sign", nil)
			// No authorization header

			err := auth.Authenticate(req, tc.paymentID, tc.role)
			if err != nil {
				t.Errorf("Expected NoAuth to allow all requests, got error: %v", err)
			}
		})
	}
}

func TestHMACMultisigAuthenticator_NilSecretsMap(t *testing.T) {
	auth := NewHMACMultisigAuthenticator(nil)
	if auth.secrets == nil {
		t.Error("Expected secrets map to be initialized, got nil")
	}
}

func TestAuthenticatorInterfaces(t *testing.T) {
	// Verify all types implement MultisigAuthenticator interface
	var _ MultisigAuthenticator = &HMACMultisigAuthenticator{}
	var _ MultisigAuthenticator = &JWTMultisigAuthenticator{}
	var _ MultisigAuthenticator = &NoAuthMultisigAuthenticator{}
}
