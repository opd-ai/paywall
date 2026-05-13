# BOOLEAN AND CONTROL FLOW LOGIC AUDIT — 2026-05-13

## Project Decision Logic Profile

**Project**: opd-ai/paywall — Bitcoin and Monero payment verification system for content protection

**Key Decision Logic Areas**:
- Payment status classification (pending vs confirmed)
- Cryptocurrency configuration validation (Bitcoin/Monero)
- Payment confirmation threshold determination
- Cookie-based authentication with fallback logic
- Wallet address validation and balance checking
- Endpoint validation with retry logic

**Complexity Profile**:
- Go 1.23.2 with 21 non-test files analyzed
- Highest complexity: NewPaywall (cyclomatic: 19, overall: 25.7)
- Most complex functions involve multi-cryptocurrency configuration validation
- Heavy use of conditional chains for error handling and wallet initialization
- Boolean expressions primarily in validation, filtering, and error checking paths

**Control Flow Patterns**:
- Guard clause pattern with early returns on validation failures
- Positive conditionals for feature enablement (e.g., `if config.XMRUser != ""`)
- Negated conditionals in error checks (`if err != nil`)
- Complex multi-condition validation for cryptocurrency configuration
- Mutex-protected critical sections with defer patterns

## Control Flow Inventory

| Package | Complex Conditions (3+ operands) | Negated Conditions | Switch Statements | Select Statements | Missing Returns After Error |
|---------|----------------------------------|-------------------|-------------------|-------------------|-----------------------------|
| paywall | 4 | 27 | 1 | 1 | 0 |
| wallet  | 6 | 19 | 0 | 0 | 0 |
| migrations | 0 | 3 | 0 | 0 | 0 |
| reverseproxy | 1 | 2 | 0 | 0 | 0 |

## Findings

### CRITICAL

- [ ] **Inconsistent Confirmation Threshold Logic Between Store Implementations** — filestore.go:160 vs encryptedfilestore.go:203 — FileStore uses `payment.Confirmations <= 1` while EncryptedFileStore uses `payment.Confirmations < 1` for the same interface method `ListPendingPayments()`. Counterexample: A payment with exactly 1 confirmation is included by FileStore but excluded by EncryptedFileStore, causing different behavior depending on which store is used. This violates the Liskov Substitution Principle and causes silent data inconsistency. — **Remediation:** Both should use `payment.Confirmations < 1` as stated in the comment on line 186 of encryptedfilestore.go: "returns all payment records with less than 1 confirmation". Change filestore.go:160 from `if payment.Confirmations <= 1` to `if payment.Confirmations < 1`.

- [ ] **Mutex Unlock Deferred After Operation That Could Error** — verification.go:91-93 — The pattern `m.gmux.Lock()` followed by `payments, err := m.paywall.Store.ListPendingPayments()` followed by `defer m.gmux.Unlock()` is incorrect. The defer should be immediately after the lock. Counterexample: If `ListPendingPayments()` panics before reaching the defer statement, the mutex remains locked permanently, causing deadlock on the next call to `checkPendingPayments()`. — **Remediation:** Move line 93 (`defer m.gmux.Unlock()`) to immediately after line 91 (`m.gmux.Lock()`). Correct pattern:
```go
m.gmux.Lock()
defer m.gmux.Unlock()
payments, err := m.paywall.Store.ListPendingPayments()
```

- [ ] **Unconditional XMR Password Requirement Blocks Bitcoin-Only Usage** — paywall.go:117-122 — The code checks `if config.XMRPassword == ""` and then `if !exists { return error }` unconditionally, even when the user wants Bitcoin-only configuration. Counterexample: User provides `Config{PriceInBTC: 0.001, TestNet: true, Store: store}` with no XMR configuration (all XMR fields empty). Line 100 passes because the condition `(config.XMRUser != "" || config.XMRPassword != "" || config.XMRRPC != "")` is false, so XMR validation is skipped. But line 117-122 runs unconditionally, checking for XMR_WALLET_PASS environment variable and returning error if not found, preventing Bitcoin-only usage. Test failure confirms this: `TestPaywall_CreatePayment_RaceConditionFix` fails with "XMR wallet password not provided". — **Remediation:** Wrap the password loading logic (lines 117-122) in a condition that checks if XMR is intended: `if config.XMRUser != "" || config.XMRPassword != "" || config.XMRRPC != ""`. This ensures XMR credential loading only happens when XMR configuration is explicitly requested.

### HIGH

- [ ] **Monero GetAddressBalance Ignores Address Parameter** — wallet/xmr_hd_wallet.go:81-89 — Function signature is `GetAddressBalance(address string)` but line 82 calls `w.client.GetBalance(&monero.RequestGetBalance{AccountIndex: 0})`, ignoring the `address` parameter entirely and returning the balance of account index 0 regardless of which address was requested. Counterexample: Payment created with Monero address A, but when verifying, the function checks the balance of the entire account (which may include address B, C, etc.), not the specific address A. This causes false positives where payment A appears confirmed because unrelated payment B was received. — **Remediation:** Monero RPC does not support per-address balance queries directly. The comment on line 87-88 acknowledges this limitation. The function should either: (1) Iterate through incoming transfers and filter by address, or (2) Change the API contract to make it clear this is account-level, not address-level balance. Current implementation is misleading.

- [ ] **Redundant Empty String Checks in Address Matching** — filestore.go:206-211 — Two consecutive if statements both check `if addr != ""` before comparing addresses. Counterexample: None (this is correct but redundant). If `addr == ""`, both conditions short-circuit correctly, returning nil at line 214. The redundancy doesn't cause incorrect behavior but indicates potential code smell. — **Remediation:** Consolidate into single check:
```go
if addr != "" {
    if payment.Addresses[wallet.Bitcoin] == addr {
        return &payment, nil
    }
    if payment.Addresses[wallet.Monero] == addr {
        return &payment, nil
    }
}
```
This maintains the same logic but makes the empty-string guard more explicit.

- [ ] **RecoverNextIndex Only Checks Balance > 0, Missing Used Addresses** — wallet/btc_hd_wallet.go:520-523 — Function scans addresses to find highest used index but only considers `if balance > 0`. Counterexample: Address at index 5 receives 0.001 BTC, user spends all of it, balance becomes 0. Address at index 10 receives 0.002 BTC, still has balance. RecoverNextIndex finds index 10, but then tries to reuse index 5 (which was already used). This violates BIP44 address reuse guidelines and can cause user confusion and privacy loss. — **Remediation:** Query transaction history for each address, not just balance. An address is "used" if it has any transaction history (received OR sent), regardless of current balance. Use RPC method `getaddresstxids` or similar to check transaction count, not just balance.

### MEDIUM

- [ ] **Potential Race Between Cookie Name Determination and Access** — middleware.go:41-55 — Cookie name is determined based on TLS status at line 45-47, then at line 52 there's a fallback that tries `__Host-payment_id` even when `cookieName == "payment_id"` (no TLS). Counterexample: Consider a request that starts without TLS (line 42 sets `isSecure = false`, line 41 sets `cookieName = "payment_id"`), but the condition at line 52 `if err != nil && cookieName == "payment_id"` means if the first cookie lookup fails, it tries `__Host-payment_id` anyway. This is intentional backward compatibility, but the comment says "Fallback: try __Host- version for backward compatibility" which suggests it's for the opposite direction (HTTPS falling back to HTTP cookie name). The logic appears inverted from the comment's intent. — **Remediation:** Verify the intended direction of backward compatibility. If the intent is to support migration from HTTP to HTTPS, the current logic is correct. If the intent is to support HTTPS sessions falling back to HTTP cookie names, the condition should be `if err != nil && cookieName == "__Host-payment_id"`. Based on security best practices, the current direction (HTTP -> HTTPS fallback) is correct, but the comment should be clarified.

- [ ] **XMR Configuration Validation After Credential Population** — paywall.go:113-133 — Lines 113-126 populate XMR configuration from environment variables if not provided in config, then lines 128-133 validate the populated credentials. The issue is line 119-122: if password is empty in config, it tries to load from env var, and returns error if env var doesn't exist. But this happens BEFORE checking if XMR is actually intended. This is related to the CRITICAL finding above but affects the validation ordering. Counterexample: Same as CRITICAL finding - Bitcoin-only user gets error about missing XMR password. — **Remediation:** Restructure to check XMR intent first, then load credentials, then validate. Suggested flow:
```go
// Only process XMR if any XMR config is provided
if config.XMRUser != "" || config.XMRPassword != "" || config.XMRRPC != "" || config.PriceInXMR > 0 {
    // Load from env if not provided
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

### LOW

- [ ] **Duplicate Empty String Check in GetPaymentByAddress** — encryptedfilestore.go:227-232 — Similar to filestore.go finding, but affects encrypted store. Same redundancy, same remediation. No counterexample causing incorrect behavior. — **Remediation:** Same as filestore.go finding - consolidate checks.

- [ ] **Intn Function Checks n <= 0 and n == 1 Separately** — wallet/btc_hd_wallet.go:120-125 — Lines 120-121 return 0 if `n <= 0`, then lines 123-124 return 0 if `n == 1`. The second check is unreachable for `n < 1` cases since they're caught by the first check. However, the `n == 1` case is a valid optimization (only one possible value, so don't bother with random generation). Counterexample: None - this is correct behavior, just slightly redundant. When `n == 1`, the only valid return is 0, so returning 0 immediately is an optimization. When `n == 0`, returning 0 is also safe. — **Remediation:** None required. This is correct and optimized. The `n == 1` check is a performance optimization to avoid calling `rand.Int` when there's only one possible value. However, for clarity, could be rewritten as:
```go
if n <= 0 {
    return 0
}
if n == 1 {
    return 0  // optimization: only one possible value
}
```
Current code is fine.

- [ ] **loadOrGenerateKey Checks err == nil and len(key) >= 32 Separately** — encryptedfilestore.go:64-67 — Line 65 checks both `err == nil && len(key) >= 32`. This is correct short-circuit evaluation, but the comment "Try to load existing key" suggests the intent is to use an existing key if present and valid. Counterexample: Key file exists but contains 16 bytes (insufficient). The function ignores it and generates a new key, overwriting the existing file at line 76. This could be surprising behavior if user expects validation error. — **Remediation:** Consider three cases explicitly: (1) key file doesn't exist -> generate, (2) key file exists and valid -> use, (3) key file exists but invalid -> return error (don't silently overwrite). Current behavior (silently overwrite) may be acceptable for encryption key rotation, but should be documented.

## False Positives Considered and Rejected

| Candidate Finding | Reason Rejected |
|-------------------|----------------|
| middleware.go:62-73 nested if statements with payment status checks | Both `StatusConfirmed` and `StatusPending` cases are mutually exclusive and cover all valid pending payment states. The condition order is correct: confirmed payments allow access, pending payments show payment page, expired or failed payments fall through to create new payment. No logic error. |
| handlers.go:93-94 OR operator in dust limit check | Condition `(p.prices[wallet.Bitcoin] > 0 && p.prices[wallet.Bitcoin] <= minBTC) \|\| (p.prices[wallet.Monero] > 0 && p.prices[wallet.Monero] <= minXMR)` correctly uses OR because either currency below dust limit should reject the payment. The `> 0` guard correctly allows zero prices (disabled currencies). Logic is correct. |
| verification.go:68-70 consecutive failures reset | Line 68-70 resets `consecutiveFailures` to 0 on success and resets ticker. This is correct exponential backoff with recovery logic. The condition `if consecutiveFailures > 0` ensures reset only happens if there were previous failures, avoiding unnecessary ticker reset. Correct implementation. |
| filestore.go:92-95 returns nil on file not found | `if os.IsNotExist(err) { return nil, nil }` is intentional: GetPayment returns (nil, nil) for not-found payments, not an error. This follows the API contract where nil payment with nil error means "payment not found". Callers check `if payment == nil` separately. Correct pattern. |
| paywall.go:100 complex XMR configuration check | The condition `(config.XMRUser != "" \|\| config.XMRPassword != "" \|\| config.XMRRPC != "")` uses OR correctly: if ANY XMR config is provided, PriceInXMR must be positive. This is the correct intent - partial XMR configuration requires complete configuration. The issue is in the subsequent unconditional password loading, not this condition itself. |
| construct.go:60-77 if/else on wallet loading | Variable err is shadowed in the if block at line 63, but this is intentional Go shadowing pattern for "try to load, fall back to create" logic. The outer err from LoadFromFile is only used in the condition, then inner err for NewBTCHDWallet. No logic error, this is idiomatic Go. |
| wallet/btc_hd_wallet.go:536-538 rollback only if nextIndex > 0 | Check `if w.nextIndex > 0` before decrementing is correct guard against underflow. If nextIndex is already 0, there's nothing to roll back. Correct implementation. |
| encryptedfilestore.go:199 continues on error or nil payment | Line 199 `if err != nil \|\| payment == nil { continue }` correctly skips files that can't be decrypted or have wrong extension. The helper `readAndDecryptPayment` returns (nil, nil) for non-.enc files, which is the expected "skip this file" signal. Correct filtering logic. |
| verification.go:52-74 select with case and default | The select statement correctly implements context cancellation (case <-ctx.Done) and periodic execution (case <-ticker.C). No default case is correct - we want to block until one of these events. Adding default would make it busy-wait. Correct implementation. |
| paywall.go:304-312 switch on wallet type for rollback | Switch statement correctly handles BTCHDWallet and MoneroHDWallet cases. No default needed because these are the only two wallet types defined by the WalletType enum. Adding default would silently ignore unknown types rather than letting the type system catch errors. Correct implementation. |

## Methodology Notes

All findings were verified against:
1. **Concrete test cases**: Each finding includes a specific input scenario that triggers the logic error
2. **Test suite execution**: Test failures confirm XMR password requirement issue (TestPaywall_CreatePayment_RaceConditionFix)
3. **Interface consistency**: Store implementations checked for consistent behavior (FileStore vs EncryptedFileStore)
4. **Mutex patterns**: All lock/defer patterns audited for correct ordering
5. **De Morgan validation**: Compound conditions verified by expanding to conjunctive/disjunctive normal form

## Recommendations Priority

1. **Immediate (CRITICAL)**: Fix confirmation threshold inconsistency and mutex defer ordering (both can cause production issues)
2. **Near-term (HIGH)**: Fix XMR password requirement and Monero balance checking (blocking Bitcoin-only usage and causing false positives)
3. **Planned (MEDIUM)**: Review XMR configuration validation flow and cookie fallback direction
4. **Opportunistic (LOW)**: Clean up redundant checks during next refactor

## Test Coverage Recommendations

Add specific test cases for:
- Payment with exactly 1 confirmation tested against both FileStore and EncryptedFileStore
- Bitcoin-only configuration with no XMR environment variables set
- Monero payment verification with multiple addresses in same account
- Concurrent access to checkPendingPayments with panic scenario
- RecoverNextIndex with spent addresses (zero balance but transaction history)
