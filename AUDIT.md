# UNIVERSAL BUG AUDIT (END-TO-END) — 2026-07-02

## Project Profile
Bitcoin/Monero paywall middleware for HTTP services, with escrow/multisig flows, wallet persistence, and webhook integrations. Target users are developers and content monetization services. Deployment model includes long-running web services with background payment monitors, file-backed persistence, and optional RPC/network dependencies. Critical paths: `NewPaywall` initialization, `CreatePayment`, `CryptoChainMonitor.checkWalletPayment`, wallet balance/confirmation logic, escrow state transitions, and multisig API handlers.

## Audit Scope
- Packages audited: `github.com/opd-ai/paywall`, `github.com/opd-ai/paywall/wallet`, `github.com/opd-ai/paywall/migration`, `github.com/opd-ai/paywall/migration/cmd/encrypt`, `github.com/opd-ai/paywall/integration_test`, and all `example/*` packages listed by `go list ./...`.
- Baseline tools run: `go-stats-generator`, `go test -race ./...`, `go vet ./...`.
- Research pass: repository issues/PRs + dependency advisory check.

## Coverage Log
| Package | 3b Logic | 3c Nil | 3d Errors | 3e Resources | 3f Concurrency | 3g Security | 3h Aliasing | 3i Init | 3j API |
|---------|----------|--------|-----------|--------------|----------------|-------------|-------------|---------|--------|
| github.com/opd-ai/paywall | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/wallet | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/migration | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/migration/cmd/encrypt | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/api-monetization | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/bitcoin-only | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/digital-downloads | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/docker-compose/app | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/monero-only | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/multisig/basic | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/multisig/common | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/multisig/marketplace | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/multisig/subscription | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/reverseproxy | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/example/reverseproxy/proxy | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| github.com/opd-ai/paywall/integration_test | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

## Goal-Achievement Summary
| Stated Goal | Status | Blocking Findings |
|-------------|--------|-------------------|
| Secure Bitcoin/Monero payment verification | ⚠️ | C-1, H-1 |
| Production-ready multisig support | ❌ | C-2, H-2 |
| AES-256 encrypted storage with configurable keying | ⚠️ | H-3 |
| Real-time payment verification with confirmations | ⚠️ | C-1, H-1 |
| Testnet/deployment reliability | ⚠️ | M-2, L-2 |

## Findings

### CRITICAL
- [ ] **C-1: Monero payments can confirm before required confirmations** — `/home/runner/work/paywall/paywall/wallet/xmr_hd_wallet.go:180-185` + `/home/runner/work/paywall/paywall/verification.go:190-206` — **logic/error handling** — `GetAddressBalance` returns spendable balance even when confirmations are below `minConfirmations`, and `checkWalletPayment` confirms solely on returned balance; this can grant paid access on under-confirmed transfers. **Remediation:** In `wallet/xmr_hd_wallet.go` (`GetAddressBalance`), return `0,nil` (or a structured "pending confirmations" signal) until `confirmations >= minConfirmations`; in `verification.go` (`checkWalletPayment`), handle `UpdatePayment` errors and avoid confirming until confirmation criteria are explicit per wallet. **Validation:** `go test -race ./... && go vet ./...`

- [ ] **C-2: Multisig API returns placeholder payment addresses on wallet errors** — `/home/runner/work/paywall/paywall/multisig_handlers.go:611-621` — **API/security/logic** — on `DeriveMultisigAddress` failure, handler stores and returns `multisig-placeholder-address` with placeholder script while still returning success; users can pay to a non-real address, causing irreversible fund loss. **Remediation:** In `createMultisigPayment` (`multisig_handlers.go`), remove placeholder fallback and return an error response when multisig address derivation fails; never persist synthetic addresses/scripts. **Validation:** `go test -race ./...`

### HIGH
- [ ] **H-1: Payment state update failure is ignored on confirmation path** — `/home/runner/work/paywall/paywall/verification.go:206` — **error handling** — `UpdatePayment` error is discarded, but logs/webhooks continue as if confirmation persisted; causes false-positive confirmations and state divergence across restarts. **Remediation:** In `checkWalletPayment` (`verification.go`), check `UpdatePayment` result, return wrapped error on failure, and dispatch webhook only after successful persistence. **Validation:** `go test -race ./...`

- [ ] **H-2: Nil payment dereference panic in multisig endpoints** — `/home/runner/work/paywall/paywall/multisig_handlers.go:255`, `:330`, `:419` — **nil/boundary safety** — `GetPayment` implementations return `(nil,nil)` for missing IDs, but handlers dereference `payment.MultisigEnabled` without nil checks, leading to server panic/DoS on unknown payment IDs. **Remediation:** Add explicit `if payment == nil { ...not found... }` checks in `HandleSign`, `HandleStatus`, and `validateBroadcastPayment`. **Validation:** `go test -race ./...`

- [ ] **H-3: File-store encryption key configuration is ignored** — `/home/runner/work/paywall/paywall/filestore.go:383-391` + `/home/runner/work/paywall/paywall/encryptedfilestore.go:63-79` — **API/initialization** — `NewFileStoreWithConfig` validates `config.EncryptionKey` but does not use it; instead it loads/generates `store.key`, violating caller expectations and breaking deterministic key control. **Remediation:** Wire `config.EncryptionKey` into encrypted store initialization (or remove field and document key-file ownership); fail if on-disk key mismatches provided key. **Validation:** `go test -race ./...`

### MEDIUM
- [ ] **M-1: Wallet restore forces mainnet network regardless of original wallet mode** — `/home/runner/work/paywall/paywall/wallet/storage.go:153` — **initialization/API contract** — `LoadFromFile` reconstructs `BTCHDWallet` with `MainNetParams` unconditionally, so restored testnet wallets derive wrong-network addresses. **Remediation:** Persist and restore network identity in wallet payload (`SaveToFile` + `LoadFromFile`) and enforce compatibility checks. **Validation:** `go test -race ./wallet/...`

- [ ] **M-2: Bitcoin blockchain-time path is effectively dead in timeout monitor** — `/home/runner/work/paywall/paywall/timeout_automation.go:324` + `:377-379` — **logic/API** — monitor builds `NewBitcoinTimestampProvider("", false)`, while provider hard-fails on empty `rpcURL`, so Bitcoin branch never returns blockchain time. **Remediation:** Provide a real configured endpoint/client path, or remove the `rpcURL` gate when public APIs are used; add tests for BTC-only blockchain-time mode. **Validation:** `go test -race ./...`

- [ ] **M-3: Monero RPC credentials accepted by API but not applied to client setup** — `/home/runner/work/paywall/paywall/wallet/xmr_hd_wallet.go:22-33` and `/home/runner/work/paywall/paywall/xmr_broadcast.go:26-33` — **API/behavioral contract** — constructors take RPC user/password, but client config uses only URL; this contradicts config contract and can fail secured deployments unexpectedly. **Remediation:** Pass credentials into Monero RPC client config (or drop params and docs if unsupported by library). **Validation:** `go test -race ./...`

- [ ] **M-4: Multisig client response decode condition is precedence-broken** — `/home/runner/work/paywall/paywall/multisig_api.go:278` — **logic/API** — `if respBody != nil && method != "GET" || method == "GET"` always decodes for GET, even when `respBody` is nil, yielding avoidable decode errors and brittle API behavior. **Remediation:** Require a non-nil response target before decoding (e.g., `if respBody != nil { ...Decode(respBody)... }`). **Validation:** `go test -race ./...`

### LOW
- [ ] **L-1: Timeout telemetry logs meaningless elapsed duration** — `/home/runner/work/paywall/paywall/timeout_automation.go:159` — **logic/observability** — `time.Since(time.Now())` records ~0ms every time, defeating timeout diagnostics. **Remediation:** Log a duration derived from stable payment timestamps (for example `now - payment.CreatedAt` or `payment.EscrowTimeout - now`), not from `time.Now()` inside the same call. **Validation:** `go test -race ./...`

- [ ] **L-2: Blockchain timestamp HTTP calls have no request timeout** — `/home/runner/work/paywall/paywall/timeout_automation.go:385`, `:402`, `:429` — **resource/performance** — bare `http.Get` can block monitor work under network stalls, degrading timeout processing. **Remediation:** Use `http.Client{Timeout: ...}` or context-bound requests in `BitcoinTimestampProvider.GetLatestBlockTime`. **Validation:** `go test -race ./...`

- [ ] **L-3: Example catalog renders unescaped filenames into HTML** — `/home/runner/work/paywall/paywall/example/digital-downloads/main.go:84` — **security (example app)** — unescaped file names inserted via `fmt.Fprintf` can inject HTML/JS in demo UI if crafted filenames exist. **Remediation:** HTML-escape file names or render via `html/template`. **Validation:** `go test -race ./...`

## Metrics Snapshot
| Metric | Value |
|--------|-------|
| Total functions | 462 |
| Functions above complexity 15 | 8 |
| Avg cyclomatic complexity | 3.84 |
| Doc coverage | 82.3% |
| Duplication ratio | 0.72% |
| Test pass rate | 15/17 packages (`go test -race ./...`) |
| go vet warnings | 0 |

## False Positives Considered and Rejected
| Candidate | Reason Rejected |
|-----------|----------------|
| `panic` in `Intn` (`wallet/btc_hd_wallet.go:131`) | Panic is explicit fail-closed behavior on cryptographic RNG failure; no safer fallback for security-sensitive randomness. |
| Placeholder signature verification comments in `dispute.go` | Code explicitly documents intentional placeholder behavior; tracked as implementation gap rather than duplicate speculative exploit report. |
| Middleware type assertion to `http.HandlerFunc` (`middleware.go:103,107`) | Current `Middleware` implementation always returns `http.HandlerFunc`; no reachable panic path in present code path. |

## Remaining Scope (if session ended before completion)
| Package | Status | Notes |
|---------|--------|-------|
| N/A | Completed | Full package pass completed for repository packages and examples. |
