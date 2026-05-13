// Package paywall provides a client library for multisig signature coordination API
package paywall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// MultisigClient provides a client for the multisig coordination API
type MultisigClient struct {
	baseURL    string
	httpClient *http.Client
	authToken  string
}

// NewMultisigClient creates a new multisig API client
// Parameters:
//   - baseURL: The base URL of the paywall server (e.g., "https://api.example.com")
//   - authToken: Optional authentication token for API requests
//
// Returns:
//   - *MultisigClient: Configured client instance
func NewMultisigClient(baseURL string, authToken string) *MultisigClient {
	return &MultisigClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		authToken: authToken,
	}
}

// InitiateMultisig starts a new multisig payment setup
// Parameters:
//   - walletType: The cryptocurrency wallet type (Bitcoin or Monero)
//   - requiredSigs: Number of required signatures (m in m-of-n)
//   - publicKeys: All participant public keys
//   - role: The initiator's role (buyer, seller, or arbiter)
//   - priceMultiplier: Optional price multiplier (default 1.0)
//
// Returns:
//   - *MultisigInitiateResponse: Payment details including address and redeem script
//   - error: Any error encountered during the request
//
// Example:
//
//	client := NewMultisigClient("https://api.example.com", "token123")
//	resp, err := client.InitiateMultisig(
//	    wallet.Bitcoin,
//	    2,
//	    [][]byte{pubKey1, pubKey2, pubKey3},
//	    RoleBuyer,
//	    1.0,
//	)
func (mc *MultisigClient) InitiateMultisig(
	walletType wallet.WalletType,
	requiredSigs int,
	publicKeys [][]byte,
	role MultisigRole,
	priceMultiplier float64,
) (*MultisigInitiateResponse, error) {
	req := MultisigInitiateRequest{
		WalletType:      walletType,
		RequiredSigs:    requiredSigs,
		PublicKeys:      publicKeys,
		Role:            role,
		PriceMultiplier: priceMultiplier,
	}

	var resp MultisigInitiateResponse
	if err := mc.doRequest("POST", "/multisig/initiate", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SubmitSignature submits a partial signature for a multisig payment
// Parameters:
//   - paymentID: The payment to sign
//   - walletType: The wallet type for this signature
//   - signerID: Unique identifier for the signer
//   - role: The signer's role
//   - signature: The cryptographic signature bytes
//   - publicKey: The signer's public key
//
// Returns:
//   - *MultisigSignResponse: Confirmation and signature count
//   - error: Any error encountered during the request
//
// Example:
//
//	resp, err := client.SubmitSignature(
//	    "payment123",
//	    wallet.Bitcoin,
//	    "signer1",
//	    RoleBuyer,
//	    signatureBytes,
//	    pubKeyBytes,
//	)
func (mc *MultisigClient) SubmitSignature(
	paymentID string,
	walletType wallet.WalletType,
	signerID string,
	role MultisigRole,
	signature []byte,
	publicKey []byte,
) (*MultisigSignResponse, error) {
	req := MultisigSignRequest{
		PaymentID:  paymentID,
		WalletType: walletType,
		SignerID:   signerID,
		Role:       role,
		Signature:  signature,
		PublicKey:  publicKey,
	}

	var resp MultisigSignResponse
	if err := mc.doRequest("POST", "/multisig/sign", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// GetStatus retrieves the current signing status for a payment
// Parameters:
//   - paymentID: The payment to check
//
// Returns:
//   - *MultisigStatusResponse: Current status including signatures and readiness
//   - error: Any error encountered during the request
//
// Example:
//
//	status, err := client.GetStatus("payment123")
//	if err != nil {
//	    return err
//	}
//	if status.ReadyToBroadcast {
//	    fmt.Println("Ready to broadcast transaction")
//	}
func (mc *MultisigClient) GetStatus(paymentID string) (*MultisigStatusResponse, error) {
	var resp MultisigStatusResponse
	endpoint := fmt.Sprintf("/multisig/status/%s", paymentID)
	if err := mc.doRequest("GET", endpoint, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// BroadcastTransaction broadcasts a fully-signed multisig transaction
// Parameters:
//   - paymentID: The payment to broadcast
//   - walletType: The wallet type
//   - transaction: The fully-signed transaction bytes
//
// Returns:
//   - *MultisigBroadcastResponse: Broadcast result including transaction ID
//   - error: Any error encountered during the request
//
// Example:
//
//	resp, err := client.BroadcastTransaction(
//	    "payment123",
//	    wallet.Bitcoin,
//	    signedTxBytes,
//	)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Transaction ID: %s\n", resp.TransactionID)
func (mc *MultisigClient) BroadcastTransaction(
	paymentID string,
	walletType wallet.WalletType,
	transaction []byte,
) (*MultisigBroadcastResponse, error) {
	req := MultisigBroadcastRequest{
		PaymentID:   paymentID,
		WalletType:  walletType,
		Transaction: transaction,
	}

	var resp MultisigBroadcastResponse
	if err := mc.doRequest("POST", "/multisig/broadcast", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// WaitForSignatures polls for signatures until the required count is reached or timeout expires
// Parameters:
//   - paymentID: The payment to monitor
//   - timeout: Maximum time to wait
//   - pollInterval: How often to check status
//
// Returns:
//   - *MultisigStatusResponse: Final status when ready or timeout
//   - error: Any error encountered, including timeout
//
// Example:
//
//	status, err := client.WaitForSignatures("payment123", 5*time.Minute, 10*time.Second)
//	if err != nil {
//	    return err
//	}
//	if status.ReadyToBroadcast {
//	    // Proceed with broadcast
//	}
func (mc *MultisigClient) WaitForSignatures(
	paymentID string,
	timeout time.Duration,
	pollInterval time.Duration,
) (*MultisigStatusResponse, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := mc.GetStatus(paymentID)
		if err != nil {
			return nil, err
		}

		if status.ReadyToBroadcast {
			return status, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timeout waiting for signatures after %v", timeout)
}

// doRequest performs an HTTP request with JSON encoding/decoding
func (mc *MultisigClient) doRequest(method, endpoint string, reqBody interface{}, respBody interface{}) error {
	url := mc.baseURL + endpoint

	var body io.Reader
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if mc.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+mc.authToken)
	}

	resp, err := mc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	if respBody != nil && method != "GET" || method == "GET" {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// SetTimeout configures the HTTP client timeout
func (mc *MultisigClient) SetTimeout(timeout time.Duration) {
	mc.httpClient.Timeout = timeout
}

// SetAuthToken updates the authentication token
func (mc *MultisigClient) SetAuthToken(token string) {
	mc.authToken = token
}
