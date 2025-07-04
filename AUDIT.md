# COMPREHENSIVE FUNCTIONAL AUDIT REPORT

## AUDIT SUMMARY

**Total Issues Found: 12**
- CRITICAL BUG: 3
- FUNCTIONAL MISMATCH: 4  
- MISSING FEATURE: 3
- EDGE CASE BUG: 2
- PERFORMANCE ISSUE: 0

**Overall Assessment**: The codebase has several critical issues that prevent proper functionality and create security vulnerabilities. The documentation promises features that are either missing or incorrectly implemented.

## DETAILED FINDINGS

### CRITICAL BUG: Incorrect Cookie Name in Middleware
**File:** middleware.go:32
**Severity:** High
**Description:** The middleware attempts to read a cookie named "__Host-payment_id" but sets a cookie with the same name. However, the documented secure cookie implementation uses "__Host-" prefix which requires specific conditions that may not be met in all deployment scenarios.
**Expected Behavior:** Should use consistent cookie naming and handle __Host- prefix requirements properly
**Actual Behavior:** Sets __Host- prefixed cookie without ensuring HTTPS and proper domain requirements
**Impact:** Cookie may not be set or read properly in non-HTTPS environments, breaking payment flow
**Reproduction:** Deploy on HTTP-only server and attempt to make payment
**Code Reference:**
```go
cookie, err := r.Cookie("__Host-payment_id")
// Later:
http.SetCookie(w, &http.Cookie{
    Name:     "__Host-payment_id",
    Value:    payment.ID,
    Path:     "/",
    Secure:   true,  // This breaks on HTTP
    HttpOnly: true,
    SameSite: http.SameSiteStrictMode,
    Domain:   "",
    Expires:  cookieExpiration,
})
```

### CRITICAL BUG: Memory Store Returns Wrong Logic for Pending Payments 
**File:** memstore.go:75-80
**Severity:** High  
**Description:** The ListPendingPayments method has inverted logic - it returns payments with MORE than 1 confirmation instead of pending payments with LESS than minimum confirmations
**Expected Behavior:** Should return payments with status StatusPending OR confirmations < minConfirmations
**Actual Behavior:** Returns payments with confirmations > 1, which are confirmed payments
**Impact:** Payment monitoring system will never find pending payments to check, breaking automatic payment confirmation
**Reproduction:** Create payments and call ListPendingPayments - will return empty list even for pending payments
**Code Reference:**
```go
for _, p := range m.payments {
    if p.Confirmations > 1 {  // BUG: Should be <= minConfirmations
        payments = append(payments, p)
    }
}
```

### CRITICAL BUG: File Store Has Same Logic Error for Pending Payments
**File:** filestore.go:134-138
**Severity:** High
**Description:** Same logic error as memory store - returns confirmed payments instead of pending ones
**Expected Behavior:** Should return payments with confirmations <= minimum required confirmations  
**Actual Behavior:** Returns payments with confirmations <= 1, missing the actual confirmation threshold
**Impact:** Payment monitoring fails to process pending payments correctly
**Reproduction:** Use file store and check pending payments - wrong payments returned
**Code Reference:**
```go
if payment.Confirmations <= 1 {  // Should use configurable minConfirmations
    payments = append(payments, &payment)
}
```

### FUNCTIONAL MISMATCH: Missing Monero Support Documentation Discrepancy
**File:** README.md vs actual implementation
**Severity:** Medium
**Description:** README claims "Support for Monero wallets via RPC interface" but implementation has significant gaps and will fail in most configurations
**Expected Behavior:** Monero should work as documented with basic RPC configuration
**Actual Behavior:** Monero wallet creation fails silently and features are incomplete
**Impact:** Users expecting Monero support will be disappointed and configurations will fail
**Reproduction:** Try to use Monero with default configuration from README
**Code Reference:**
```go
// From README: Claims Monero support, but:
if err != nil {
    log.Printf("error creating XMR wallet %s,\n\tXMR will be disabled", err)
}
// Silently disables XMR without user awareness
```

### FUNCTIONAL MISMATCH: Wallet Management API Mismatch
**File:** README.md vs wallet implementations  
**Severity:** Medium
**Description:** README shows wallet.LoadFromFile() and SaveToFile() but actual HD wallet interface doesn't include these methods
**Expected Behavior:** HD wallet interface should include persistence methods as documented
**Actual Behavior:** Only BTCHDWallet implements SaveToFile, not the interface
**Impact:** Code using interface types cannot save/load wallets, breaking documented patterns
**Reproduction:** Try to use HDWallet interface with SaveToFile method
**Code Reference:**
```go
// README shows:
wallet, err := wallet.LoadFromFile(config)
err = pw.HDWallet.SaveToFile(config)

// But HDWallet interface doesn't include these methods:
type HDWallet interface {
    DeriveNextAddress() (string, error)
    GetAddress() (string, error)
    Currency() string
    GetAddressBalance(address string) (float64, error)
    GetTransactionConfirmations(txID string) (int, error)
}
```

### FUNCTIONAL MISMATCH: Configuration Example Inconsistency
**File:** README.md vs Config struct
**Severity:** Medium  
**Description:** README shows NewFileStore() taking a string parameter but also shows NewFileStoreWithConfig taking FileStoreConfig
**Expected Behavior:** Consistent API documentation for file store creation
**Actual Behavior:** Multiple conflicting ways to create file stores in documentation
**Impact:** Developer confusion and integration difficulties
**Reproduction:** Follow README examples - some will fail due to wrong function signatures
**Code Reference:**
```go
// README shows both:
store := paywall.NewFileStore("./payments")  // Simple version
store, err := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{...}) // Complex version
```

### FUNCTIONAL MISMATCH: Quick Start Example Missing Required Store
**File:** README.md Quick Start section
**Severity:** Medium
**Description:** Quick Start example shows creating paywall but the Store field is required and would cause panic if not provided
**Expected Behavior:** Example should work without modification
**Actual Behavior:** Example will panic because Store is required but not shown as such in quick start
**Impact:** New users following documentation will get runtime errors
**Reproduction:** Copy quick start example exactly - will panic on payment creation
**Code Reference:**
```go
// README Quick Start - missing Store requirement:
pw, err := paywall.NewPaywall(paywall.Config{
    PriceInBTC:     0.001,
    TestNet:        true,
    Store:          paywall.NewMemoryStore(), // This line is missing in README
    PaymentTimeout: time.Hour * 24,
})
```

### MISSING FEATURE: Payment Expiration Handling
**File:** verification.go, paywall.go
**Severity:** Medium
**Description:** Documentation implies automatic payment expiration but no code exists to mark expired payments as StatusExpired
**Expected Behavior:** Expired payments should automatically transition to StatusExpired
**Actual Behavior:** Expired payments remain StatusPending indefinitely
**Impact:** Database bloat and confusion about payment states
**Reproduction:** Create payment, wait for expiration time, check status - still pending
**Code Reference:**
```go
// Status constants exist but no expiration logic:
const (
    StatusPending PaymentStatus = "pending"
    StatusConfirmed PaymentStatus = "confirmed" 
    StatusExpired PaymentStatus = "expired"  // Never set anywhere
)
```

### MISSING FEATURE: Error Handling for Wallet Connection Failures
**File:** paywall.go:125-135
**Severity:** Medium
**Description:** When XMR wallet creation fails, it's silently disabled but paywall continues. Users aren't notified that their XMR configuration is invalid.
**Expected Behavior:** Should return error or provide clear notification when wallet setup fails
**Actual Behavior:** Silently continues with only BTC wallet, potentially confusing users who expect XMR
**Impact:** Users may not realize their XMR wallet isn't working until payment attempts fail
**Reproduction:** Provide invalid XMR credentials - paywall appears to work but XMR is disabled
**Code Reference:**
```go
if err != nil {
    log.Printf("error creating XMR wallet %s,\n\tXMR will be disabled", err)
    // No user notification or config validation
}
```

### MISSING FEATURE: Dust Limit Validation for Bitcoin
**File:** handlers.go:67-75, createpayment.go
**Severity:** Low
**Description:** Documentation mentions "dust limit validation" but no validation exists during payment creation, only during render validation
**Expected Behavior:** Should validate amounts against dust limits when creating payments
**Actual Behavior:** Only validates during payment page rendering, not at creation time
**Impact:** Could create unspendable Bitcoin payments
**Reproduction:** Configure very small BTC amount and create payment - may be unspendable
**Code Reference:**
```go
// Validation only in render, not in CreatePayment:
const minBTC = 0.00001 // Dust limit
if (p.prices[wallet.Bitcoin] > 0 && p.prices[wallet.Bitcoin] <= minBTC) {
    http.Error(w, "Failed to create payment", http.StatusInternalServerError)
    return true
}
```

### EDGE CASE BUG: Race Condition in Payment Creation
**File:** paywall.go:255-267
**Severity:** Medium
**Description:** CreatePayment method generates addresses by calling DeriveNextAddress which increments wallet index, but if store.CreatePayment fails, the address index is already incremented
**Expected Behavior:** Address derivation should be atomic with payment storage
**Actual Behavior:** Failed payment creation can skip address indexes
**Impact:** Address gaps in HD wallet derivation path
**Reproduction:** Make store.CreatePayment fail after address generation - address index will be incremented but payment not stored
**Code Reference:**
```go
address, err := hdWallet.DeriveNextAddress()  // Increments index
if err != nil {
    return nil, fmt.Errorf("generate %s address: %w", walletType, err)
}
payment.Addresses[walletType] = address
// Later:
if err := p.Store.CreatePayment(payment); err != nil {  // If this fails, index already incremented
    return nil, fmt.Errorf("store payment: %w", err)
}
```

### EDGE CASE BUG: Monero Balance Check Logic Flaw
**File:** wallet/xmr_hd_wallet.go:77-91
**Severity:** Medium
**Description:** GetAddressBalance for Monero gets total wallet balance instead of address-specific balance, and returns 0 if confirmations are insufficient rather than actual balance
**Expected Behavior:** Should return actual balance for specific address regardless of confirmations, or clearly separate balance vs confirmed balance
**Actual Behavior:** Returns 0 balance for unconfirmed transactions, making payment verification unreliable
**Impact:** Monero payments may appear as unpaid even when balance is received
**Reproduction:** Send Monero payment with fewer than required confirmations - will show as 0 balance
**Code Reference:**
```go
if conf < w.minConfirmations {
    return 0, fmt.Errorf("unconfirmed, balance considered 0(this is temporary): %w", err)
}
// Should return actual balance with confirmation info separately
```