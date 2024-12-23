package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// MoneroHDWallet implements the HDWallet interface for Monero using RPC
type MoneroHDWallet struct {
	rpcURL      string
	rpcUser     string
	rpcPassword string
	mu          sync.Mutex
	nextIndex   uint32
}

// MoneroConfig holds Monero wallet RPC connection details
type MoneroConfig struct {
	RPCURL      string
	RPCUser     string
	RPCPassword string
}

// NewMoneroWallet creates a new Monero wallet instance
func NewMoneroWallet(config MoneroConfig) (*MoneroHDWallet, error) {
	w := &MoneroHDWallet{
		rpcURL:      config.RPCURL,
		rpcUser:     config.RPCUser,
		rpcPassword: config.RPCPassword,
		nextIndex:   0,
	}

	// Test RPC connection
	if err := w.testConnection(); err != nil {
		return nil, fmt.Errorf("monero RPC connection failed: %w", err)
	}

	return w, nil
}

// Currency implements HDWallet interface
func (w *MoneroHDWallet) Currency() string {
	return string(Monero)
}

// DeriveNextAddress implements HDWallet interface by creating a new subaddress
func (w *MoneroHDWallet) DeriveNextAddress() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Create new subaddress using RPC
	resp, err := w.rpcCall("create_address", map[string]interface{}{
		"account_index": 0,
		"label":         fmt.Sprintf("payment-%d", w.nextIndex),
	})
	if err != nil {
		return "", fmt.Errorf("create address failed: %w", err)
	}

	var result struct {
		Address    string `json:"address"`
		AddressIdx uint32 `json:"address_index"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse response failed: %w", err)
	}

	w.nextIndex++
	return result.Address, nil
}

// GetAddress implements HDWallet interface by returning the last derived address
func (w *MoneroHDWallet) GetAddress() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.nextIndex == 0 {
		return w.DeriveNextAddress()
	}

	// Get existing subaddress
	resp, err := w.rpcCall("get_address", map[string]interface{}{
		"account_index": 0,
		"address_index": w.nextIndex - 1,
	})
	if err != nil {
		return "", fmt.Errorf("get address failed: %w", err)
	}

	var result struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("parse response failed: %w", err)
	}

	return result.Address, nil
}

// rpcCall is a helper function for making Monero RPC calls
func (w *MoneroHDWallet) rpcCall(method string, params interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	req, err := http.NewRequest("POST", w.rpcURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.SetBasicAuth(w.rpcUser, w.rpcPassword)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", result.Error.Code, result.Error.Message)
	}

	return result.Result, nil
}

// testConnection verifies the RPC connection is working
func (w *MoneroHDWallet) testConnection() error {
	_, err := w.rpcCall("get_version", nil)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}
