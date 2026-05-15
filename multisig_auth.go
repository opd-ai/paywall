package paywall

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MultisigAuthenticator errors
var (
	// ErrMissingAuthHeader is returned when the Authorization header is missing
	ErrMissingAuthHeader = errors.New("missing Authorization header")
	// ErrInvalidAuthHeader is returned when the Authorization header format is invalid
	ErrInvalidAuthHeader = errors.New("invalid Authorization header format")
	// ErrInvalidToken is returned when JWT token is invalid or expired
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrUnauthorizedRole is returned when the authenticated user lacks permission for the role
	ErrUnauthorizedRole = errors.New("unauthorized for requested role")
)

// Note: ErrInvalidSignature is reused from escrow.go

// HMACMultisigAuthenticator implements MultisigAuthenticator using HMAC-SHA256 signatures
// The client must provide an Authorization header with format: "HMAC <signature>"
// where signature = HMAC-SHA256(secret, paymentID + role)
//
// This is a simple shared-secret authentication suitable for trusted parties.
// For production deployments with untrusted parties, consider JWTMultisigAuthenticator.
//
// Example usage:
//
//	auth := NewHMACMultisigAuthenticator(map[string]string{
//	    "buyer":   "buyer-secret-key",
//	    "seller":  "seller-secret-key",
//	    "arbiter": "arbiter-secret-key",
//	})
//	coordinator := NewMultisigCoordinator(paywall, auth, nil)
type HMACMultisigAuthenticator struct {
	// secrets maps role names to their HMAC secret keys
	secrets map[string]string
}

// NewHMACMultisigAuthenticator creates a new HMAC-based authenticator
// The secrets map should contain a secret key for each role that will participate
// in multisig operations (typically "buyer", "seller", "arbiter")
func NewHMACMultisigAuthenticator(secrets map[string]string) *HMACMultisigAuthenticator {
	if secrets == nil {
		secrets = make(map[string]string)
	}
	return &HMACMultisigAuthenticator{
		secrets: secrets,
	}
}

// Authenticate verifies the HMAC signature in the Authorization header
// Expected header format: "HMAC <hex-encoded-signature>"
// The signature should be: HMAC-SHA256(secret, paymentID + role)
func (h *HMACMultisigAuthenticator) Authenticate(r *http.Request, paymentID string, role MultisigRole) error {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ErrMissingAuthHeader
	}

	// Parse "HMAC <signature>" format
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "HMAC" {
		return ErrInvalidAuthHeader
	}

	providedSig := parts[1]

	// Get secret for this role
	secret, ok := h.secrets[string(role)]
	if !ok {
		return fmt.Errorf("no secret configured for role %s", role)
	}

	// Compute expected signature
	message := paymentID + string(role)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison
	if !hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
		return ErrInvalidSignature
	}

	return nil
}

// SimpleJWT represents a minimal JWT-like token for authentication
// This is a simplified JWT implementation suitable for basic authentication.
// For production use with external parties, consider using a full JWT library
// like github.com/golang-jwt/jwt/v5
type SimpleJWT struct {
	Role      string `json:"role"`
	PaymentID string `json:"payment_id"`
	ExpiresAt int64  `json:"exp"`
}

// JWTMultisigAuthenticator implements MultisigAuthenticator using JWT-like tokens
// The client must provide an Authorization header with format: "Bearer <token>"
//
// This authenticator uses a simplified JWT-like token format with HMAC-SHA256 signing.
// The token payload has the format: "role:payment_id:exp_timestamp"
// The signature is: HMAC-SHA256(secret, payload)
// Full token format: "payload.signature" (both hex-encoded)
//
// Example usage:
//
//	auth := NewJWTMultisigAuthenticator("your-secret-key", 15*time.Minute)
//	coordinator := NewMultisigCoordinator(paywall, auth, nil)
//
// Client creates token with:
//
//	payload := fmt.Sprintf("%s:%s:%d", role, paymentID, time.Now().Add(15*time.Minute).Unix())
//	signature := hex.EncodeToString(HMAC-SHA256(secret, payload))
//	token := hex.EncodeToString(payload) + "." + signature
//	header := "Bearer " + token
type JWTMultisigAuthenticator struct {
	secret     string
	expiration time.Duration
}

// NewJWTMultisigAuthenticator creates a new JWT-based authenticator
// The secret is used to sign and verify tokens
// The expiration parameter sets the maximum token lifetime
func NewJWTMultisigAuthenticator(secret string, expiration time.Duration) *JWTMultisigAuthenticator {
	return &JWTMultisigAuthenticator{
		secret:     secret,
		expiration: expiration,
	}
}

// Authenticate verifies the JWT token in the Authorization header
// Expected header format: "Bearer <token>"
// Token format: "<hex-payload>.<hex-signature>"
// Payload format: "role:payment_id:exp_timestamp"
func (j *JWTMultisigAuthenticator) Authenticate(r *http.Request, paymentID string, role MultisigRole) error {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ErrMissingAuthHeader
	}

	// Parse "Bearer <token>" format
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ErrInvalidAuthHeader
	}

	token := parts[1]

	// Parse token format: "payload.signature"
	tokenParts := strings.SplitN(token, ".", 2)
	if len(tokenParts) != 2 {
		return ErrInvalidToken
	}

	payloadHex := tokenParts[0]
	signatureHex := tokenParts[1]

	// Decode payload
	payload, err := hex.DecodeString(payloadHex)
	if err != nil {
		return fmt.Errorf("invalid token payload encoding: %w", err)
	}

	// Verify signature
	mac := hmac.New(sha256.New, []byte(j.secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signatureHex), []byte(expectedSig)) {
		return ErrInvalidSignature
	}

	// Parse payload: "role:payment_id:exp_timestamp"
	payloadStr := string(payload)
	payloadParts := strings.Split(payloadStr, ":")
	if len(payloadParts) != 3 {
		return ErrInvalidToken
	}

	tokenRole := payloadParts[0]
	tokenPaymentID := payloadParts[1]
	tokenExp := payloadParts[2]

	// Verify role matches
	if tokenRole != string(role) {
		return ErrUnauthorizedRole
	}

	// Verify payment ID matches
	if tokenPaymentID != paymentID {
		return fmt.Errorf("token payment ID mismatch: expected %s, got %s", paymentID, tokenPaymentID)
	}

	// Verify expiration
	var expTimestamp int64
	_, err = fmt.Sscanf(tokenExp, "%d", &expTimestamp)
	if err != nil {
		return fmt.Errorf("invalid expiration timestamp: %w", err)
	}

	if time.Now().Unix() > expTimestamp {
		return ErrInvalidToken
	}

	return nil
}

// NoAuthMultisigAuthenticator is a pass-through authenticator that allows all requests
// This is useful for testing or for trusted internal deployments where authentication
// is handled at a different layer (e.g., network isolation, VPN, etc.)
//
// WARNING: Using this authenticator in production without other security controls
// leaves multisig operations completely unauthenticated and vulnerable to abuse.
type NoAuthMultisigAuthenticator struct{}

// NewNoAuthMultisigAuthenticator creates a new pass-through authenticator
// that allows all requests without verification
func NewNoAuthMultisigAuthenticator() *NoAuthMultisigAuthenticator {
	return &NoAuthMultisigAuthenticator{}
}

// Authenticate always returns nil (allows all requests)
func (n *NoAuthMultisigAuthenticator) Authenticate(r *http.Request, paymentID string, role MultisigRole) error {
	return nil
}
