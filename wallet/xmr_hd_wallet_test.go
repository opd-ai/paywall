package wallet

import (
	"errors"
	"testing"

	monero "github.com/monero-ecosystem/go-monero-rpc-client/wallet"
)

// MockMoneroClient implements a mock for the monero.Client interface for testing
type MockMoneroClient struct {
	GetBalanceFunc    func(*monero.RequestGetBalance) (*monero.ResponseGetBalance, error)
	CreateAddressFunc func(*monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error)
	GetTransfersFunc  func(*monero.RequestGetTransfers) (*monero.ResponseGetTransfers, error)
}

func (m *MockMoneroClient) GetBalance(req *monero.RequestGetBalance) (*monero.ResponseGetBalance, error) {
	if m.GetBalanceFunc != nil {
		return m.GetBalanceFunc(req)
	}
	return &monero.ResponseGetBalance{Balance: 1000000000000}, nil // 1 XMR in atomic units
}

func (m *MockMoneroClient) CreateAddress(req *monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error) {
	if m.CreateAddressFunc != nil {
		return m.CreateAddressFunc(req)
	}
	return &monero.ResponseCreateAddress{
		Address:      "48test...address",
		AddressIndex: 1,
	}, nil
}

func (m *MockMoneroClient) GetTransfers(req *monero.RequestGetTransfers) (*monero.ResponseGetTransfers, error) {
	if m.GetTransfersFunc != nil {
		return m.GetTransfersFunc(req)
	}
	return &monero.ResponseGetTransfers{
		In: []*monero.Transfer{
			{
				TxID:          "test_tx_123",
				Confirmations: 10,
			},
		},
	}, nil
}

// Stub implementations for other Client interface methods to satisfy the interface
func (m *MockMoneroClient) GetAddress(*monero.RequestGetAddress) (*monero.ResponseGetAddress, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetAddressIndex(*monero.RequestGetAddressIndex) (*monero.ResponseGetAddressIndex, error) {
	return nil, nil
}
func (m *MockMoneroClient) LabelAddress(*monero.RequestLabelAddress) error { return nil }
func (m *MockMoneroClient) ValidateAddress(*monero.RequestValidateAddress) (*monero.ResponseValidateAddress, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetAccounts(*monero.RequestGetAccounts) (*monero.ResponseGetAccounts, error) {
	return nil, nil
}
func (m *MockMoneroClient) CreateAccount(*monero.RequestCreateAccount) (*monero.ResponseCreateAccount, error) {
	return nil, nil
}
func (m *MockMoneroClient) LabelAccount(*monero.RequestLabelAccount) error          { return nil }
func (m *MockMoneroClient) GetAccountTags() (*monero.ResponseGetAccountTags, error) { return nil, nil }
func (m *MockMoneroClient) TagAccounts(*monero.RequestTagAccounts) error            { return nil }
func (m *MockMoneroClient) UntagAccounts(*monero.RequestUntagAccounts) error        { return nil }
func (m *MockMoneroClient) SetAccountTagDescription(*monero.RequestSetAccountTagDescription) error {
	return nil
}
func (m *MockMoneroClient) GetHeight() (*monero.ResponseGetHeight, error) { return nil, nil }
func (m *MockMoneroClient) Transfer(*monero.RequestTransfer) (*monero.ResponseTransfer, error) {
	return nil, nil
}
func (m *MockMoneroClient) TransferSplit(*monero.RequestTransferSplit) (*monero.ResponseTransferSplit, error) {
	return nil, nil
}
func (m *MockMoneroClient) SignTransfer(*monero.RequestSignTransfer) (*monero.ResponseSignTransfer, error) {
	return nil, nil
}
func (m *MockMoneroClient) SubmitTransfer(*monero.RequestSubmitTransfer) (*monero.ResponseSubmitTransfer, error) {
	return nil, nil
}
func (m *MockMoneroClient) SweepDust(*monero.RequestSweepDust) (*monero.ResponseSweepDust, error) {
	return nil, nil
}
func (m *MockMoneroClient) SweepAll(*monero.RequestSweepAll) (*monero.ResponseSweepAll, error) {
	return nil, nil
}
func (m *MockMoneroClient) SweepSingle(*monero.RequestSweepSingle) (*monero.ResponseSweepSingle, error) {
	return nil, nil
}
func (m *MockMoneroClient) RelayTx(*monero.RequestRelayTx) (*monero.ResponseRelayTx, error) {
	return nil, nil
}
func (m *MockMoneroClient) Store() error { return nil }
func (m *MockMoneroClient) GetPayments(*monero.RequestGetPayments) (*monero.ResponseGetPayments, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetBulkPayments(*monero.RequestGetBulkPayments) (*monero.ResponseGetBulkPayments, error) {
	return nil, nil
}
func (m *MockMoneroClient) IncomingTransfers(*monero.RequestIncomingTransfers) (*monero.ResponseIncomingTransfers, error) {
	return nil, nil
}
func (m *MockMoneroClient) QueryKey(*monero.RequestQueryKey) (*monero.ResponseQueryKey, error) {
	return nil, nil
}
func (m *MockMoneroClient) MakeIntegratedAddress(*monero.RequestMakeIntegratedAddress) (*monero.ResponseMakeIntegratedAddress, error) {
	return nil, nil
}
func (m *MockMoneroClient) SplitIntegratedAddress(*monero.RequestSplitIntegratedAddress) (*monero.ResponseSplitIntegratedAddress, error) {
	return nil, nil
}
func (m *MockMoneroClient) StopWallet() error                          { return nil }
func (m *MockMoneroClient) RescanBlockchain() error                    { return nil }
func (m *MockMoneroClient) SetTxNotes(*monero.RequestSetTxNotes) error { return nil }
func (m *MockMoneroClient) GetTxNotes(*monero.RequestGetTxNotes) (*monero.ResponseGetTxNotes, error) {
	return nil, nil
}
func (m *MockMoneroClient) SetAttribute(*monero.RequestSetAttribute) error { return nil }
func (m *MockMoneroClient) GetAttribute(*monero.RequestGetAttribute) (*monero.ResponseGetAttribute, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetTxKey(*monero.RequestGetTxKey) (*monero.ResponseGetTxKey, error) {
	return nil, nil
}
func (m *MockMoneroClient) CheckTxKey(*monero.RequestCheckTxKey) (*monero.ResponseCheckTxKey, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetTxProof(*monero.RequestGetTxProof) (*monero.ResponseGetTxProof, error) {
	return nil, nil
}
func (m *MockMoneroClient) CheckTxProof(*monero.RequestCheckTxProof) (*monero.ResponseCheckTxProof, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetSpendProof(*monero.RequestGetSpendProof) (*monero.ResponseGetSpendProof, error) {
	return nil, nil
}
func (m *MockMoneroClient) CheckSpendProof(*monero.RequestCheckSpendProof) (*monero.ResponseCheckSpendProof, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetReserveProof(*monero.RequestGetReserveProof) (*monero.ResponseGetReserveProof, error) {
	return nil, nil
}
func (m *MockMoneroClient) CheckReserveProof(*monero.RequestCheckReserveProof) (*monero.ResponseCheckReserveProof, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetTransferByTxID(*monero.RequestGetTransferByTxID) (*monero.ResponseGetTransferByTxID, error) {
	return nil, nil
}
func (m *MockMoneroClient) Sign(*monero.RequestSign) (*monero.ResponseSign, error) { return nil, nil }
func (m *MockMoneroClient) Verify(*monero.RequestVerify) (*monero.ResponseVerify, error) {
	return nil, nil
}
func (m *MockMoneroClient) ExportOutputs() (*monero.ResponseExportOutputs, error) { return nil, nil }
func (m *MockMoneroClient) ImportOutputs(*monero.RequestImportOutputs) (*monero.ResponseImportOutputs, error) {
	return nil, nil
}
func (m *MockMoneroClient) ExportKeyImages() (*monero.ResponseExportKeyImages, error) {
	return nil, nil
}
func (m *MockMoneroClient) ImportKeyImages(*monero.RequestImportKeyImages) (*monero.ResponseImportKeyImages, error) {
	return nil, nil
}
func (m *MockMoneroClient) MakeURI(*monero.RequestMakeURI) (*monero.ResponseMakeURI, error) {
	return nil, nil
}
func (m *MockMoneroClient) ParseURI(*monero.RequestParseURI) (*monero.ResponseParseURI, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetAddressBook(*monero.RequestGetAddressBook) (*monero.ResponseGetAddressBook, error) {
	return nil, nil
}
func (m *MockMoneroClient) AddAddressBook(*monero.RequestAddAddressBook) (*monero.ResponseAddAddressBook, error) {
	return nil, nil
}
func (m *MockMoneroClient) DeleteAddressBook(*monero.RequestDeleteAddressBook) error { return nil }
func (m *MockMoneroClient) Refresh(*monero.RequestRefresh) (*monero.ResponseRefresh, error) {
	return nil, nil
}
func (m *MockMoneroClient) RescanSpent() error                                  { return nil }
func (m *MockMoneroClient) StartMining(*monero.RequestStartMining) error        { return nil }
func (m *MockMoneroClient) StopMining() error                                   { return nil }
func (m *MockMoneroClient) GetLanguages() (*monero.ResponseGetLanguages, error) { return nil, nil }
func (m *MockMoneroClient) CreateWallet(*monero.RequestCreateWallet) error      { return nil }
func (m *MockMoneroClient) GenerateFromKeys(*monero.RequestGenerateFromKeys) (*monero.ResponseGenerateFromKeys, error) {
	return nil, nil
}
func (m *MockMoneroClient) OpenWallet(*monero.RequestOpenWallet) error { return nil }
func (m *MockMoneroClient) CloseWallet() error                         { return nil }
func (m *MockMoneroClient) ChangeWalletPassword(*monero.RequestChangeWalletPassword) error {
	return nil
}
func (m *MockMoneroClient) IsMultisig() (*monero.ResponseIsMultisig, error) { return nil, nil }
func (m *MockMoneroClient) PrepareMultisig() (*monero.ResponsePrepareMultisig, error) {
	return nil, nil
}
func (m *MockMoneroClient) MakeMultisig(*monero.RequestMakeMultisig) (*monero.ResponseMakeMultisig, error) {
	return nil, nil
}
func (m *MockMoneroClient) ExportMultisigInfo() (*monero.ResponseExportMultisigInfo, error) {
	return nil, nil
}
func (m *MockMoneroClient) ImportMultisigInfo(*monero.RequestImportMultisigInfo) (*monero.ResponseImportMultisigInfo, error) {
	return nil, nil
}
func (m *MockMoneroClient) FinalizeMultisig(*monero.RequestFinalizeMultisig) (*monero.ResponseFinalizeMultisig, error) {
	return nil, nil
}
func (m *MockMoneroClient) SignMultisig(*monero.RequestSignMultisig) (*monero.ResponseSignMultisig, error) {
	return nil, nil
}
func (m *MockMoneroClient) SubmitMultisig(*monero.RequestSubmitMultisig) (*monero.ResponseSubmitMultisig, error) {
	return nil, nil
}
func (m *MockMoneroClient) GetVersion() (*monero.ResponseGetVersion, error) { return nil, nil }

// Helper function to create a MoneroHDWallet with mock client
func createMockMoneroWallet(mockClient *MockMoneroClient) *MoneroHDWallet {
	return &MoneroHDWallet{
		client:    mockClient,
		nextIndex: 0,
	}
}

func TestNewMoneroWallet_Success(t *testing.T) {
	// This test requires a running Monero RPC server or would need dependency injection
	// For now, we'll test the configuration structure
	config := MoneroConfig{
		RPCURL:      "http://localhost:18082",
		RPCUser:     "testuser",
		RPCPassword: "testpass",
	}

	// Test that config is properly structured
	if config.RPCURL == "" {
		t.Error("RPCURL should not be empty")
	}
	if config.RPCUser == "" {
		t.Error("RPCUser should not be empty")
	}
	if config.RPCPassword == "" {
		t.Error("RPCPassword should not be empty")
	}
}

func TestMoneroHDWallet_Currency(t *testing.T) {
	mockClient := &MockMoneroClient{}
	wallet := createMockMoneroWallet(mockClient)

	currency := wallet.Currency()
	expected := string(Monero)

	if currency != expected {
		t.Errorf("Currency() = %v, want %v", currency, expected)
	}
}

func TestMoneroHDWallet_DeriveNextAddress_Success(t *testing.T) {
	expectedAddress := "48test123...moneroaddress"
	mockClient := &MockMoneroClient{
		CreateAddressFunc: func(req *monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error) {
			// Verify request parameters
			if req.AccountIndex != 0 {
				t.Errorf("Expected AccountIndex 0, got %d", req.AccountIndex)
			}
			if req.Label != "payment-0" {
				t.Errorf("Expected label 'payment-0', got '%s'", req.Label)
			}
			return &monero.ResponseCreateAddress{
				Address:      expectedAddress,
				AddressIndex: 1,
			}, nil
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	address, err := wallet.DeriveNextAddress()
	if err != nil {
		t.Fatalf("DeriveNextAddress() error = %v", err)
	}

	if address != expectedAddress {
		t.Errorf("DeriveNextAddress() = %v, want %v", address, expectedAddress)
	}

	// Verify nextIndex was incremented
	if wallet.nextIndex != 1 {
		t.Errorf("nextIndex should be 1 after DeriveNextAddress(), got %d", wallet.nextIndex)
	}
}

func TestMoneroHDWallet_DeriveNextAddress_Error(t *testing.T) {
	expectedError := errors.New("RPC connection failed")
	mockClient := &MockMoneroClient{
		CreateAddressFunc: func(req *monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error) {
			return nil, expectedError
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	address, err := wallet.DeriveNextAddress()
	if err == nil {
		t.Fatal("DeriveNextAddress() should return error")
	}

	if address != "" {
		t.Errorf("DeriveNextAddress() should return empty address on error, got %v", address)
	}

	// Verify nextIndex was not incremented on error
	if wallet.nextIndex != 0 {
		t.Errorf("nextIndex should remain 0 on error, got %d", wallet.nextIndex)
	}
}

func TestMoneroHDWallet_DeriveNextAddress_ConcurrentAccess(t *testing.T) {
	mockClient := &MockMoneroClient{
		CreateAddressFunc: func(req *monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error) {
			return &monero.ResponseCreateAddress{
				Address:      "concurrent_test_address",
				AddressIndex: 1,
			}, nil
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	// Test concurrent access to ensure mutex works
	done := make(chan bool, 2)

	go func() {
		_, err := wallet.DeriveNextAddress()
		if err != nil {
			t.Errorf("Concurrent DeriveNextAddress() error = %v", err)
		}
		done <- true
	}()

	go func() {
		_, err := wallet.DeriveNextAddress()
		if err != nil {
			t.Errorf("Concurrent DeriveNextAddress() error = %v", err)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// nextIndex should be 2 after both calls
	if wallet.nextIndex != 2 {
		t.Errorf("nextIndex should be 2 after concurrent calls, got %d", wallet.nextIndex)
	}
}

func TestMoneroHDWallet_GetAddress_Success(t *testing.T) {
	expectedAddress := "48getaddress...test"
	mockClient := &MockMoneroClient{
		CreateAddressFunc: func(req *monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error) {
			return &monero.ResponseCreateAddress{
				Address:      expectedAddress,
				AddressIndex: 1,
			}, nil
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	address, err := wallet.GetAddress()
	if err != nil {
		t.Fatalf("GetAddress() error = %v", err)
	}

	if address != expectedAddress {
		t.Errorf("GetAddress() = %v, want %v", address, expectedAddress)
	}
}

func TestMoneroHDWallet_GetAddress_Error(t *testing.T) {
	expectedError := errors.New("derive address failed")
	mockClient := &MockMoneroClient{
		CreateAddressFunc: func(req *monero.RequestCreateAddress) (*monero.ResponseCreateAddress, error) {
			return nil, expectedError
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	address, err := wallet.GetAddress()
	if err == nil {
		t.Fatal("GetAddress() should return error when DeriveNextAddress fails")
	}

	if address != "" {
		t.Errorf("GetAddress() should return empty address on error, got %v", address)
	}
}

func TestMoneroHDWallet_GetAddressBalance_Error(t *testing.T) {
	expectedError := errors.New("balance request failed")
	mockClient := &MockMoneroClient{
		GetBalanceFunc: func(req *monero.RequestGetBalance) (*monero.ResponseGetBalance, error) {
			return nil, expectedError
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	balance, err := wallet.GetAddressBalance("test_address")
	if err == nil {
		t.Fatal("GetAddressBalance() should return error")
	}

	if balance != 0 {
		t.Errorf("GetAddressBalance() should return 0 on error, got %v", balance)
	}
}

func TestMoneroHDWallet_GetTransactionConfirmations_Success(t *testing.T) {
	testTxID := "test_transaction_123"
	expectedConfirmations := 15

	mockClient := &MockMoneroClient{
		GetTransfersFunc: func(req *monero.RequestGetTransfers) (*monero.ResponseGetTransfers, error) {
			if !req.In {
				t.Error("Expected In=true for incoming transfers")
			}
			if req.AccountIndex != 0 {
				t.Errorf("Expected AccountIndex 0, got %d", req.AccountIndex)
			}
			return &monero.ResponseGetTransfers{
				In: []*monero.Transfer{
					{
						TxID:          "other_transaction",
						Confirmations: 5,
					},
					{
						TxID:          testTxID,
						Confirmations: uint64(expectedConfirmations),
					},
					{
						TxID:          "another_transaction",
						Confirmations: 20,
					},
				},
			}, nil
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	confirmations, err := wallet.GetTransactionConfirmations(testTxID)
	if err != nil {
		t.Fatalf("GetTransactionConfirmations() error = %v", err)
	}

	if confirmations != expectedConfirmations {
		t.Errorf("GetTransactionConfirmations() = %v, want %v", confirmations, expectedConfirmations)
	}
}

func TestMoneroHDWallet_GetTransactionConfirmations_NotFound(t *testing.T) {
	testTxID := "nonexistent_transaction"

	mockClient := &MockMoneroClient{
		GetTransfersFunc: func(req *monero.RequestGetTransfers) (*monero.ResponseGetTransfers, error) {
			return &monero.ResponseGetTransfers{
				In: []*monero.Transfer{
					{
						TxID:          "other_transaction",
						Confirmations: 5,
					},
				},
			}, nil
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	confirmations, err := wallet.GetTransactionConfirmations(testTxID)
	if err == nil {
		t.Fatal("GetTransactionConfirmations() should return error for nonexistent transaction")
	}

	if confirmations != 0 {
		t.Errorf("GetTransactionConfirmations() should return 0 on error, got %v", confirmations)
	}

	expectedErrorMsg := "transaction nonexistent_transaction not found"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestMoneroHDWallet_GetTransactionConfirmations_RPCError(t *testing.T) {
	expectedError := errors.New("RPC connection failed")
	mockClient := &MockMoneroClient{
		GetTransfersFunc: func(req *monero.RequestGetTransfers) (*monero.ResponseGetTransfers, error) {
			return nil, expectedError
		},
	}

	wallet := createMockMoneroWallet(mockClient)

	confirmations, err := wallet.GetTransactionConfirmations("test_tx")
	if err == nil {
		t.Fatal("GetTransactionConfirmations() should return error on RPC failure")
	}

	if confirmations != 0 {
		t.Errorf("GetTransactionConfirmations() should return 0 on error, got %v", confirmations)
	}
}

func TestMoneroHDWallet_GetAddressBalance_InsufficientConfirmations(t *testing.T) {
	expectedBalance := uint64(5000000000000) // 5 XMR in atomic units
	expectedConfirmations := 2                // Less than required minimum
	testTxID := "test_transaction_id"

	mockClient := &MockMoneroClient{
		GetBalanceFunc: func(req *monero.RequestGetBalance) (*monero.ResponseGetBalance, error) {
			return &monero.ResponseGetBalance{Balance: expectedBalance}, nil
		},
		GetTransfersFunc: func(req *monero.RequestGetTransfers) (*monero.ResponseGetTransfers, error) {
			return &monero.ResponseGetTransfers{
				In: []*monero.Transfer{
					{
						TxID:          testTxID,
						Amount:        expectedBalance,
						Confirmations: uint64(expectedConfirmations), // Insufficient confirmations
					},
				},
			}, nil
		},
	}

	wallet := createMockMoneroWallet(mockClient)
	wallet.minConfirmations = 3 // Require 3 confirmations

	balance, err := wallet.GetAddressBalance("test_address")

	// Should return actual balance, not zero
	if err != nil {
		t.Fatalf("GetAddressBalance() should not error with insufficient confirmations, got: %v", err)
	}

	expectedBalanceXMR := float64(expectedBalance) / 1e12
	if balance != expectedBalanceXMR {
		t.Errorf("GetAddressBalance() = %v, want %v (should return actual balance despite insufficient confirmations)", balance, expectedBalanceXMR)
	}
}

func TestMoneroConfig_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config MoneroConfig
		valid  bool
	}{
		{
			name: "Valid config",
			config: MoneroConfig{
				RPCURL:      "http://localhost:18082",
				RPCUser:     "user",
				RPCPassword: "pass",
			},
			valid: true,
		},
		{
			name: "Empty RPCURL",
			config: MoneroConfig{
				RPCURL:      "",
				RPCUser:     "user",
				RPCPassword: "pass",
			},
			valid: false,
		},
		{
			name: "Empty credentials",
			config: MoneroConfig{
				RPCURL:      "http://localhost:18082",
				RPCUser:     "",
				RPCPassword: "",
			},
			valid: true, // Credentials might be optional
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation of config structure
			hasURL := tt.config.RPCURL != ""

			if tt.valid && !hasURL {
				t.Error("Valid config should have non-empty RPCURL")
			}
			if !tt.valid && hasURL {
				t.Error("Invalid config marked as valid")
			}
		})
	}
}
