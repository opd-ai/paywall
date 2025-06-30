package paywall

import (
	"encoding/json"
	"html/template"
	"testing"
	"time"

	"github.com/opd-ai/paywall/wallet"
)

// TestPaymentStatusConstants verifies all payment status constants are defined correctly
func TestPaymentStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		status   PaymentStatus
		expected string
	}{
		{
			name:     "StatusPending_ValidConstant",
			status:   StatusPending,
			expected: "pending",
		},
		{
			name:     "StatusConfirmed_ValidConstant",
			status:   StatusConfirmed,
			expected: "confirmed",
		},
		{
			name:     "StatusExpired_ValidConstant",
			status:   StatusExpired,
			expected: "expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

// TestPaymentStruct_Creation verifies Payment struct can be created and populated correctly
func TestPaymentStruct_Creation_ValidData(t *testing.T) {
	createdAt := time.Now()
	expiresAt := createdAt.Add(time.Hour)

	addresses := map[wallet.WalletType]string{
		wallet.Bitcoin: "bc1qtest123",
		wallet.Monero:  "43H3Uqnc9test123",
	}

	amounts := map[wallet.WalletType]float64{
		wallet.Bitcoin: 0.001,
		wallet.Monero:  0.01,
	}

	payment := Payment{
		ID:            "test-payment-id",
		Addresses:     addresses,
		Amounts:       amounts,
		CreatedAt:     createdAt,
		ExpiresAt:     expiresAt,
		Status:        StatusPending,
		Confirmations: 0,
		TransactionID: "",
	}

	// Verify all fields are set correctly
	if payment.ID != "test-payment-id" {
		t.Errorf("Expected ID 'test-payment-id', got '%s'", payment.ID)
	}

	if payment.Addresses[wallet.Bitcoin] != "bc1qtest123" {
		t.Errorf("Expected BTC address 'bc1qtest123', got '%s'", payment.Addresses[wallet.Bitcoin])
	}

	if payment.Addresses[wallet.Monero] != "43H3Uqnc9test123" {
		t.Errorf("Expected XMR address '43H3Uqnc9test123', got '%s'", payment.Addresses[wallet.Monero])
	}

	if payment.Amounts[wallet.Bitcoin] != 0.001 {
		t.Errorf("Expected BTC amount 0.001, got %f", payment.Amounts[wallet.Bitcoin])
	}

	if payment.Amounts[wallet.Monero] != 0.01 {
		t.Errorf("Expected XMR amount 0.01, got %f", payment.Amounts[wallet.Monero])
	}

	if payment.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, payment.Status)
	}

	if payment.Confirmations != 0 {
		t.Errorf("Expected confirmations 0, got %d", payment.Confirmations)
	}

	if !payment.CreatedAt.Equal(createdAt) {
		t.Errorf("Expected CreatedAt %v, got %v", createdAt, payment.CreatedAt)
	}

	if !payment.ExpiresAt.Equal(expiresAt) {
		t.Errorf("Expected ExpiresAt %v, got %v", expiresAt, payment.ExpiresAt)
	}
}

// TestPaymentStruct_JSONSerialization verifies Payment struct can be serialized to/from JSON
func TestPaymentStruct_JSONSerialization_SuccessfulRoundTrip(t *testing.T) {
	original := Payment{
		ID: "json-test-id",
		Addresses: map[wallet.WalletType]string{
			wallet.Bitcoin: "bc1qjsontest123",
			wallet.Monero:  "43H3Uqnc9jsontest",
		},
		Amounts: map[wallet.WalletType]float64{
			wallet.Bitcoin: 0.002,
			wallet.Monero:  0.02,
		},
		CreatedAt:     time.Unix(1640995200, 0).UTC(), // Fixed time for consistent testing
		ExpiresAt:     time.Unix(1640998800, 0).UTC(), // Fixed time for consistent testing
		Status:        StatusConfirmed,
		Confirmations: 3,
		TransactionID: "tx123abc",
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Payment to JSON: %v", err)
	}

	// Deserialize from JSON
	var deserialized Payment
	err = json.Unmarshal(jsonData, &deserialized)
	if err != nil {
		t.Fatalf("Failed to unmarshal Payment from JSON: %v", err)
	}

	// Verify all fields match
	if deserialized.ID != original.ID {
		t.Errorf("ID mismatch: expected %s, got %s", original.ID, deserialized.ID)
	}

	if deserialized.Addresses[wallet.Bitcoin] != original.Addresses[wallet.Bitcoin] {
		t.Errorf("BTC address mismatch: expected %s, got %s",
			original.Addresses[wallet.Bitcoin], deserialized.Addresses[wallet.Bitcoin])
	}

	if deserialized.Amounts[wallet.Bitcoin] != original.Amounts[wallet.Bitcoin] {
		t.Errorf("BTC amount mismatch: expected %f, got %f",
			original.Amounts[wallet.Bitcoin], deserialized.Amounts[wallet.Bitcoin])
	}

	if deserialized.Status != original.Status {
		t.Errorf("Status mismatch: expected %s, got %s", original.Status, deserialized.Status)
	}

	if deserialized.Confirmations != original.Confirmations {
		t.Errorf("Confirmations mismatch: expected %d, got %d", original.Confirmations, deserialized.Confirmations)
	}

	if deserialized.TransactionID != original.TransactionID {
		t.Errorf("TransactionID mismatch: expected %s, got %s", original.TransactionID, deserialized.TransactionID)
	}
}

// TestPaymentStruct_EmptyValues verifies Payment struct handles empty/nil values correctly
func TestPaymentStruct_EmptyValues_HandlesGracefully(t *testing.T) {
	payment := Payment{} // Zero value

	// Verify zero values are as expected
	if payment.ID != "" {
		t.Errorf("Expected empty ID, got '%s'", payment.ID)
	}

	if payment.Addresses != nil {
		t.Errorf("Expected nil Addresses, got %v", payment.Addresses)
	}

	if payment.Amounts != nil {
		t.Errorf("Expected nil Amounts, got %v", payment.Amounts)
	}

	if !payment.CreatedAt.IsZero() {
		t.Errorf("Expected zero CreatedAt, got %v", payment.CreatedAt)
	}

	if !payment.ExpiresAt.IsZero() {
		t.Errorf("Expected zero ExpiresAt, got %v", payment.ExpiresAt)
	}

	if payment.Status != "" {
		t.Errorf("Expected empty Status, got '%s'", payment.Status)
	}

	if payment.Confirmations != 0 {
		t.Errorf("Expected zero Confirmations, got %d", payment.Confirmations)
	}
}

// TestPaymentPageDataStruct_Creation verifies PaymentPageData struct creation and field assignment
func TestPaymentPageDataStruct_Creation_ValidData(t *testing.T) {
	jsCode := template.JS("console.log('test')")

	data := PaymentPageData{
		BTCAddress: "bc1qpagetest123",
		AmountBTC:  0.005,
		XMRAddress: "43H3Uqnc9pagetest",
		AmountXMR:  0.05,
		ExpiresAt:  "2024-01-15T10:30:00Z",
		PaymentID:  "page-test-id",
		QrcodeJs:   jsCode,
	}

	// Verify all fields are set correctly
	if data.BTCAddress != "bc1qpagetest123" {
		t.Errorf("Expected BTCAddress 'bc1qpagetest123', got '%s'", data.BTCAddress)
	}

	if data.AmountBTC != 0.005 {
		t.Errorf("Expected AmountBTC 0.005, got %f", data.AmountBTC)
	}

	if data.XMRAddress != "43H3Uqnc9pagetest" {
		t.Errorf("Expected XMRAddress '43H3Uqnc9pagetest', got '%s'", data.XMRAddress)
	}

	if data.AmountXMR != 0.05 {
		t.Errorf("Expected AmountXMR 0.05, got %f", data.AmountXMR)
	}

	if data.ExpiresAt != "2024-01-15T10:30:00Z" {
		t.Errorf("Expected ExpiresAt '2024-01-15T10:30:00Z', got '%s'", data.ExpiresAt)
	}

	if data.PaymentID != "page-test-id" {
		t.Errorf("Expected PaymentID 'page-test-id', got '%s'", data.PaymentID)
	}

	if string(data.QrcodeJs) != "console.log('test')" {
		t.Errorf("Expected QrcodeJs 'console.log('test')', got '%s'", string(data.QrcodeJs))
	}
}

// TestPaymentPageDataStruct_JSONSerialization verifies PaymentPageData can be serialized to JSON
func TestPaymentPageDataStruct_JSONSerialization_SuccessfulMarshaling(t *testing.T) {
	data := PaymentPageData{
		BTCAddress: "bc1qjsonpagetest",
		AmountBTC:  0.001,
		XMRAddress: "43H3Uqnc9jsonpage",
		AmountXMR:  0.01,
		ExpiresAt:  "2024-12-31T23:59:59Z",
		PaymentID:  "json-page-id",
		QrcodeJs:   template.JS("var test = true;"),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal PaymentPageData to JSON: %v", err)
	}

	// Verify JSON contains expected fields
	var unmarshaled PaymentPageData
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal PaymentPageData from JSON: %v", err)
	}

	if unmarshaled.BTCAddress != data.BTCAddress {
		t.Errorf("BTCAddress mismatch after JSON round trip: expected %s, got %s",
			data.BTCAddress, unmarshaled.BTCAddress)
	}

	if unmarshaled.PaymentID != data.PaymentID {
		t.Errorf("PaymentID mismatch after JSON round trip: expected %s, got %s",
			data.PaymentID, unmarshaled.PaymentID)
	}
}

// TestPaymentStruct_StatusTransitions verifies valid payment status transitions
func TestPaymentStruct_StatusTransitions_ValidFlow(t *testing.T) {
	payment := Payment{
		ID:     "status-test",
		Status: StatusPending,
	}

	// Test transition from Pending to Confirmed
	payment.Status = StatusConfirmed
	if payment.Status != StatusConfirmed {
		t.Errorf("Expected status %s, got %s", StatusConfirmed, payment.Status)
	}

	// Test setting to Expired
	payment.Status = StatusExpired
	if payment.Status != StatusExpired {
		t.Errorf("Expected status %s, got %s", StatusExpired, payment.Status)
	}
}

// TestPaymentStruct_WalletTypeMaps verifies map operations with wallet types
func TestPaymentStruct_WalletTypeMaps_MultipleWalletTypes(t *testing.T) {
	payment := Payment{
		Addresses: make(map[wallet.WalletType]string),
		Amounts:   make(map[wallet.WalletType]float64),
	}

	// Add Bitcoin data
	payment.Addresses[wallet.Bitcoin] = "bc1qmaptest"
	payment.Amounts[wallet.Bitcoin] = 0.001

	// Add Monero data
	payment.Addresses[wallet.Monero] = "43H3Uqnc9maptest"
	payment.Amounts[wallet.Monero] = 0.01

	// Verify Bitcoin data
	btcAddr, btcExists := payment.Addresses[wallet.Bitcoin]
	if !btcExists {
		t.Error("Bitcoin address not found in map")
	}
	if btcAddr != "bc1qmaptest" {
		t.Errorf("Expected Bitcoin address 'bc1qmaptest', got '%s'", btcAddr)
	}

	// Verify Monero data
	xmrAddr, xmrExists := payment.Addresses[wallet.Monero]
	if !xmrExists {
		t.Error("Monero address not found in map")
	}
	if xmrAddr != "43H3Uqnc9maptest" {
		t.Errorf("Expected Monero address '43H3Uqnc9maptest', got '%s'", xmrAddr)
	}

	// Verify map length
	if len(payment.Addresses) != 2 {
		t.Errorf("Expected 2 addresses in map, got %d", len(payment.Addresses))
	}
	if len(payment.Amounts) != 2 {
		t.Errorf("Expected 2 amounts in map, got %d", len(payment.Amounts))
	}
}
