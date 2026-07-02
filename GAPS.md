# Implementation Gaps — 2026-07-02

## Multisig API advertised as usable but can emit non-real payment addresses
- **Stated Goal**: Production-ready multisig payment support.
- **Current State**: `createMultisigPayment` falls back to `multisig-placeholder-address` on wallet derivation errors (`/home/runner/work/paywall/paywall/multisig_handlers.go:613-621`).
- **Impact**: Users can be instructed to pay to unusable addresses; funds can be lost and settlement impossible.
- **Closing the Gap**: Fail initiation on derivation errors; require real derived address+metadata before persisting/returning payment.

## Confirmation-based payment assurance is not upheld for Monero
- **Stated Goal**: Real-time payment verification with required confirmations.
- **Current State**: Monero wallet returns positive balance even when confirmations are below threshold (`/home/runner/work/paywall/paywall/wallet/xmr_hd_wallet.go:180-185`), and monitor confirms payment by balance (`/home/runner/work/paywall/paywall/verification.go:190-206`).
- **Impact**: Access can be granted before configured confirmation depth.
- **Closing the Gap**: Enforce confirmation threshold in wallet return contract (or separate API) and confirmation path logic before status update/webhook.

## Configured encrypted storage key is not actually honored
- **Stated Goal**: AES-256 encrypted storage with caller-provided keying control.
- **Current State**: `NewFileStoreWithConfig` validates `EncryptionKey` then ignores it and uses/generated `store.key` (`/home/runner/work/paywall/paywall/filestore.go:383-391`, `/home/runner/work/paywall/paywall/encryptedfilestore.go:63-79`).
- **Impact**: Integrators cannot reliably control key material; portability and deterministic recovery expectations break.
- **Closing the Gap**: Use supplied key directly (or enforce explicit key-file contract and remove misleading field).

## Monero credential contract differs from runtime behavior
- **Stated Goal**: Monero RPC support with `XMRUser`/`XMRPassword` configuration.
- **Current State**: Wallet and broadcaster constructors accept credentials but initialize client with URL only (`/home/runner/work/paywall/paywall/wallet/xmr_hd_wallet.go:22-33`, `/home/runner/work/paywall/paywall/xmr_broadcast.go:26-33`).
- **Impact**: Secured RPC deployments may fail or behave contrary to configuration expectations.
- **Closing the Gap**: Pass credentials through to client transport/auth config or update docs/config surface to match actual capability.

## “Blockchain time” timeout mode is not functionally complete for Bitcoin-only setups
- **Stated Goal**: Timeout automation supports blockchain-time checks.
- **Current State**: BTC timestamp provider is instantiated with empty URL (`/home/runner/work/paywall/paywall/timeout_automation.go:324`) and provider requires non-empty `rpcURL` (`:377-379`), preventing BTC path from supplying blockchain time.
- **Impact**: Blockchain-time mode silently degrades to system time in BTC-only scenarios.
- **Closing the Gap**: Implement a working BTC timestamp source path and test BTC-only blockchain-time behavior.
