# Boolean and Control Flow Logic Gaps — 2026-05-13

## Gap 1: Store Interface Implementation Consistency

**Stated Goal**: The `PaymentStore` interface (types.go) defines `ListPendingPayments() ([]*Payment, error)` to return payments awaiting blockchain confirmation. The system should treat pending payments consistently regardless of which storage backend is used (FileStore vs EncryptedFileStore). The Liskov Substitution Principle requires that implementations of the same interface produce equivalent results for the same inputs.

**Current State**: FileStore (filestore.go:160) uses `payment.Confirmations <= 1` while EncryptedFileStore (encryptedfilestore.go:203) uses `payment.Confirmations < 1`. This means a payment with exactly 1 confirmation is:
- **Included** by FileStore.ListPendingPayments() 
- **Excluded** by EncryptedFileStore.ListPendingPayments()

The comment in encryptedfilestore.go:186 explicitly states "returns all payment records with less than 1 confirmation", suggesting `< 1` is the intended behavior.

**Risk**: Applications switching from FileStore to EncryptedFileStore (or vice versa) will observe different payment tracking behavior without code changes. A payment monitoring dashboard could show inconsistent pending payment counts depending on storage backend. A payment that just reached 1 confirmation continues being monitored by FileStore but is ignored by EncryptedFileStore, potentially causing:
- Missed confirmation updates (payment stuck in "pending" state forever)
- Dashboard discrepancies (different pending payment counts reported)
- Integration test failures when testing against different storage backends

**Input Causing Incorrect Behavior**: 
```go
payment := &Payment{
    ID: "test123",
    Status: StatusPending,
    Confirmations: 1,  // exactly 1 confirmation
}
// Store with FileStore - payment IS returned
// Store with EncryptedFileStore - payment IS NOT returned
```

**Closing the Gap**: 
1. Change filestore.go line 160 from `if payment.Confirmations <= 1 {` to `if payment.Confirmations < 1 {`
2. Add interface contract test that verifies both implementations return identical results:
```go
func TestStoreImplementationsConsistency(t *testing.T) {
    payment := &Payment{ID: "test", Status: StatusPending, Confirmations: 1}
    
    fileStore := NewFileStore("./test-file")
    encStore, _ := NewEncryptedFileStore("./test-enc")
    
    fileStore.CreatePayment(payment)
    encStore.CreatePayment(payment)
    
    filePending, _ := fileStore.ListPendingPayments()
    encPending, _ := encStore.ListPendingPayments()
    
    // Should return same results
    assert.Equal(t, len(filePending), len(encPending))
}
```

---

## Gap 2: Bitcoin-Only Configuration Intent vs Validation Logic

**Stated Goal**: The README.md "Quick Start" example (lines 41-48) shows Bitcoin-only configuration:
```go
pw, err := paywall.NewPaywall(paywall.Config{
    PriceInBTC:     0.001,
    TestNet:        true,
    Store:          paywall.NewMemoryStore(),
    PaymentTimeout: time.Hour * 24,
})
```
No XMR fields are set, suggesting Bitcoin-only usage should work without XMR configuration. The project states it supports "Bitcoin (for compatibility)" separately from Monero support, implying Bitcoin can be used independently.

**Current State**: paywall.go lines 117-122 unconditionally check for XMR_WALLET_PASS environment variable and return error if not found, even when user provides no XMR configuration at all. The validation at line 100 correctly checks `if (XMRUser != "" || XMRPassword != "" || XMRRPC != "")` to determine if XMR is intended, but the password loading at lines 117-122 runs unconditionally afterward.

**Risk**: Users following the Quick Start example cannot create a Bitcoin-only paywall unless they set XMR_WALLET_PASS environment variable (to a dummy value they won't use). Test failure confirms: `TestPaywall_CreatePayment_RaceConditionFix` fails with "XMR wallet password not provided" even though XMR is not intended for that test. This blocks:
- Simple Bitcoin-only deployments for users who don't want Monero
- Development/testing without full Monero RPC infrastructure
- Migration path for existing Bitcoin-only users

**Input Causing Incorrect Behavior**:
```go
config := Config{
    PriceInBTC: 0.001,
    TestNet: true,
    Store: NewMemoryStore(),
    PaymentTimeout: time.Hour,
    // No XMR fields set - Bitcoin only intended
}
pw, err := NewPaywall(config)
// Returns error: "XMR wallet password not provided"
// Expected: Successfully creates Bitcoin-only paywall
```

**Closing the Gap**:
1. Wrap password loading logic (lines 117-122) in XMR intent check:
```go
if config.XMRUser != "" || config.XMRPassword != "" || config.XMRRPC != "" || config.PriceInXMR > 0 {
    if config.XMRUser == "" {
        config.XMRUser = os.Getenv("XMR_WALLET_USER")
    }
    if config.XMRPassword == "" {
        pass, exists := os.LookupEnv("XMR_WALLET_PASS")
        if !exists {
            return nil, fmt.Errorf("XMR wallet password not provided")
        }
        config.XMRPassword = pass
    }
    // ... rest of XMR setup
}
```
2. Update documentation to clarify: if ANY XMR config is provided (user, password, RPC URL, or price), all XMR fields become required
3. Add test case for Bitcoin-only configuration without XMR environment variables

---

## Gap 3: Monero Address-Specific Balance vs Account Balance

**Stated Goal**: The `CryptoClient` interface (verification.go:27-35) defines `GetAddressBalance(address string) (float64, error)` with documentation: "retrieves the current balance for a Bitcoin address". The parameter is named `address` (singular), suggesting per-address balance checking. The payment verification flow creates unique Monero addresses per payment (paywall.go:274) and stores them in `payment.Addresses[wallet.Monero]`. The verification logic (verification.go:129) calls `GetAddressBalance(payment.Addresses[walletType])`, expecting to check if that specific address received payment.

**Current State**: MoneroHDWallet.GetAddressBalance (wallet/xmr_hd_wallet.go:81-89) completely ignores the `address` parameter and returns the balance of account index 0 (line 82: `GetBalance(&monero.RequestGetBalance{AccountIndex: 0})`). This returns the sum of all subaddresses in account 0, not the balance of the specific address provided.

**Risk**: False positive payment confirmations. When payment A is created with Monero address X, but a different payment B is confirmed to address Y (same account), calling `GetAddressBalance(X)` returns the account balance (including Y's payment), causing payment A to be marked confirmed even though address X never received funds. This breaks the payment-to-address binding that the paywall system relies on for security.

**Input Causing Incorrect Behavior**:
```go
// Payment 1: address A, amount 1.0 XMR - no payment received
payment1 := &Payment{
    ID: "pay1",
    Addresses: map[WalletType]string{Monero: "addressA..."},
    Amounts: map[WalletType]float64{Monero: 1.0},
}

// Payment 2: address B, amount 1.0 XMR - payment received
payment2 := &Payment{
    ID: "pay2", 
    Addresses: map[WalletType]string{Monero: "addressB..."},
    Amounts: map[WalletType]float64{Monero: 1.0},
}

// Check payment1 - should return 0.0 (addressA has no funds)
balance := wallet.GetAddressBalance("addressA...")
// Returns 1.0 (account balance including addressB)
// payment1 incorrectly marked as confirmed
```

**Closing the Gap**:
1. **Option A (Correct but Inefficient)**: Iterate through incoming transfers and filter by address:
```go
func (w *MoneroHDWallet) GetAddressBalance(address string) (float64, error) {
    resp, err := w.client.GetTransfers(&monero.RequestGetTransfers{
        In: true,
        AccountIndex: 0,
    })
    if err != nil {
        return 0, fmt.Errorf("get transfers failed: %w", err)
    }
    
    var totalBalance float64
    for _, tx := range resp.In {
        if tx.Address == address {
            totalBalance += float64(tx.Amount) / 1e12
        }
    }
    return totalBalance, nil
}
```

2. **Option B (Architectural Change)**: Change the API contract to acknowledge Monero's account-based model:
   - Rename method to `GetAccountBalance(accountIndex uint32)` 
   - Store account index instead of address in payment records for Monero
   - Update documentation to clarify Bitcoin uses address-based balance, Monero uses account-based balance

3. **Option C (Hybrid)**: Keep interface but add explicit documentation that Monero implementation is account-level:
```go
// GetAddressBalance implements CryptoClient interface.
// For Monero: Returns account-level balance (account 0) ignoring address parameter
// due to Monero RPC limitations. Caller should use GetTransfers to verify specific
// address received payment.
func (w *MoneroHDWallet) GetAddressBalance(address string) (float64, error) {
    // ... existing implementation with clear warning
}
```

Current implementation silently breaks the security model by not verifying payment-to-address binding for Monero payments.

---

## Gap 4: Address Recovery vs BIP44 Address Reuse Prevention

**Stated Goal**: The wallet package implements BIP32/44 HD wallet standards (wallet/btc_hd_wallet.go:2-3). BIP44 specifies that addresses should not be reused for privacy and security. The RecoverNextIndex method (btc_hd_wallet.go:475-528) is intended to "query the blockchain for the highest used index" and set nextIndex accordingly to avoid reusing addresses after wallet recovery from backup.

**Current State**: RecoverNextIndex only checks `if balance > 0` (line 521) to determine if an address is "used". This misses addresses that received funds and then had all funds spent (balance = 0 but transaction history exists).

**Risk**: Address reuse after wallet recovery. If user receives payment to address #5, then spends all funds (balance → 0), then recovers wallet from seed on a new device, RecoverNextIndex scans and finds balance = 0, considers address #5 unused, and reuses it. This violates BIP44 privacy guarantees and can confuse users ("why am I seeing old transactions on my 'new' address?").

**Input Causing Incorrect Behavior**:
```go
// Initial state: address at index 5 received 0.01 BTC, then spent all
// Address 5 history: [+0.01 BTC, -0.01 BTC], current balance: 0 BTC

wallet.RecoverNextIndex()
// Scans index 0-999, finds balance=0 for all
// Sets nextIndex = 1 (highest used index + 1)

addr := wallet.DeriveNextAddress()
// Returns address at index 1, but index 5 was already used
// User's previous payment history at index 5 is ignored
```

**Closing the Gap**:
1. Query transaction history, not just balance:
```go
// Check if address has been used by querying transaction history
txCount, err := w.rpcClient.GetAddressTxIDs(address)
if err != nil {
    return fmt.Errorf("failed to check address history: %w", err)
}

// Address is "used" if it has any transaction history
if len(txCount) > 0 {
    highestIndex = i
}
```

2. Add persistent index storage to avoid expensive blockchain scanning on every recovery:
```go
// In SaveToFile, store nextIndex in encrypted wallet file (already done)
// In LoadFromFile, restore nextIndex from wallet file (already done) 
// Only call RecoverNextIndex if nextIndex restoration fails or user explicitly requests blockchain scan
```

3. Add test case for recovery with spent addresses:
```go
func TestRecoverNextIndex_SpentAddresses(t *testing.T) {
    // Create wallet, derive address at index 5
    // Simulate receiving and spending (balance = 0, but txHistory exists)
    // Save wallet, load into new wallet instance
    // Call RecoverNextIndex
    // Assert nextIndex = 6 (not reusing index 5)
}
```

Current implementation prioritizes balance checking (efficient) over transaction history checking (correct), creating a gap between BIP44 standards and actual behavior.

---

## Gap 5: Mutex Protection Timing vs Panic Safety

**Stated Goal**: The CryptoChainMonitor (verification.go:17-23) uses mutexes (`gmux`, `btcMux`, `xmrMux`) to protect concurrent access to shared resources. Go best practices require `defer mutex.Unlock()` immediately after `mutex.Lock()` to ensure unlock happens even if code panics.

**Current State**: verification.go:91-93 locks `gmux`, calls `ListPendingPayments()` (which can panic on storage errors), THEN defers unlock. This violates the immediately-defer pattern.

**Risk**: If `ListPendingPayments()` panics before line 93 executes, `gmux` remains locked permanently. Next call to `checkPendingPayments()` deadlocks on line 91 attempting to acquire the already-locked mutex. This cascades: monitoring goroutine hangs, payments stop being verified, all new payments remain in "pending" state forever even after blockchain confirms them.

**Input Causing Incorrect Behavior**:
```go
// Storage backend has corrupted data that causes panic in json.Unmarshal
// (or any other panic in storage layer)

monitor.checkPendingPayments()
// Line 91: gmux.Lock() - acquired
// Line 92: ListPendingPayments() - panics on corrupted data
// Line 93: defer gmux.Unlock() - NEVER EXECUTED
// Mutex remains locked

monitor.checkPendingPayments()  // second call
// Line 91: gmux.Lock() - DEADLOCK (waits forever for already-locked mutex)
```

**Closing the Gap**:
```go
func (m *CryptoChainMonitor) checkPendingPayments() error {
    m.gmux.Lock()
    defer m.gmux.Unlock()  // MOVED: defer immediately after lock
    
    payments, err := m.paywall.Store.ListPendingPayments()
    if err != nil {
        return fmt.Errorf("failed to list pending payments: %w", err)
    }
    // ... rest of function
}
```

This is a standard Go concurrency pattern violation that's easy to fix but has severe consequences if triggered.

---

## Gap 6: Dust Limit Validation Error Message

**Stated Goal**: handlers.go:78-99 validates payment data before rendering payment page. Lines 91-96 check if prices are below dust limits (minimum transaction amounts for Bitcoin and Monero). The comment states "Zero prices indicate disabled wallet types and should be allowed", suggesting zero-price currencies are intentionally supported.

**Current State**: The error message at line 95 is "Failed to create payment", which suggests a payment creation failure, but this is a validation error during payment *rendering*, not creation. The actual issue is "price below dust limit", not "creation failed".

**Risk**: Misleading error messages cause confusion during debugging. Developer sees "Failed to create payment" in logs, investigates payment creation logic (CreatePayment method), but the actual issue is in price configuration validation. This wastes debugging time and makes it harder to diagnose configuration issues.

**Input Causing Incorrect Behavior**:
```go
config := Config{
    PriceInBTC: 0.000001,  // Below dust limit (0.00001)
    // ... rest of config
}
pw, _ := NewPaywall(config)
payment, _ := pw.CreatePayment()  // succeeds

pw.renderPaymentPage(w, payment)
// Returns error: "Failed to create payment"
// Misleading: payment WAS created, issue is price below dust limit
```

**Closing the Gap**:
1. Change error message to be more specific:
```go
if (p.prices[wallet.Bitcoin] > 0 && p.prices[wallet.Bitcoin] <= minBTC) ||
    (p.prices[wallet.Monero] > 0 && p.prices[wallet.Monero] <= minXMR) {
    http.Error(w, "Payment amount below dust limit (Bitcoin: 0.00001, Monero: 0.0001)", http.StatusBadRequest)
    return true
}
```

2. Consider moving validation to NewPaywall to fail-fast:
```go
// In NewPaywall, after setting prices map
for walletType, price := range prices {
    if price > 0 && price < dustLimits[walletType] {
        return nil, fmt.Errorf("price for %s (%f) below dust limit (%f)", 
            walletType, price, dustLimits[walletType])
    }
}
```

This ensures configuration errors are caught at initialization, not during request handling.

---

## Summary Statistics

**Total Gaps Identified**: 6
- **Critical (Incorrect Results)**: 2 (Store consistency, Monero address balance)
- **High (Blocks Features)**: 2 (Bitcoin-only config, Address recovery)  
- **Medium (Robustness)**: 2 (Mutex panic safety, Error messaging)

**Root Cause Analysis**:
- **Interface Consistency**: 2 gaps (Store implementations, Monero address balance)
- **Configuration Logic**: 2 gaps (Bitcoin-only intent, Dust limit messaging)
- **Concurrency Patterns**: 1 gap (Mutex defer timing)
- **BIP44 Compliance**: 1 gap (Address reuse prevention)

**Affected Components**:
- Payment verification (3 gaps)
- Configuration validation (2 gaps)
- Wallet address management (1 gap)
