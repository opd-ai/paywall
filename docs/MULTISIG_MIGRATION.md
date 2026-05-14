# Multisig Migration Guide

This guide helps existing Paywall users migrate from single-signature payments to multisig escrow functionality.

## Backward Compatibility Guarantees

**Important**: The multisig feature is 100% backward compatible with existing single-signature implementations.

### What Won't Change

- **Default behavior**: Single-signature mode remains the default
- **Existing APIs**: All current `Paywall` methods work identically
- **Payment storage**: Existing payment records remain valid
- **Configuration**: No configuration changes required for current users
- **Dependencies**: No new external dependencies for single-sig users

### What's New (Optional)

- Multisig payment support (opt-in via `MultisigEnabled` config flag)
- Escrow workflows for buyer/seller transactions
- Dispute resolution with third-party arbiters
- 2-of-3 and m-of-n signature schemes
- HTTP API for signature coordination

## Migration Scenarios

### Scenario 1: Keep Using Single-Signature (No Changes)

**Current Code** (remains valid):
```go
pw, err := paywall.NewPaywall(paywall.Config{
    PriceInBTC:     0.001,
    TestNet:        true,
    Store:          paywall.NewMemoryStore(),
    PaymentTimeout: time.Hour * 24,
})
```

**Result**: Your code continues to work exactly as before. No migration needed.

### Scenario 2: Enable Multisig for New Payments

**Before** (single-sig):
```go
config := paywall.Config{
    PriceInBTC:     0.001,
    TestNet:        true,
    Store:          paywall.NewMemoryStore(),
    PaymentTimeout: time.Hour * 24,
}
```

**After** (multisig enabled):
```go
// Generate or load participant public keys
buyerPubKey, sellerPubKey, arbiterPubKey := loadParticipantKeys()

config := paywall.Config{
    PriceInBTC:     0.001,
    TestNet:        true,
    Store:          paywall.NewMemoryStore(),
    PaymentTimeout: time.Hour * 24,
    
    // New multisig configuration
    MultisigEnabled:  true,
    MultisigRequired: 2,  // Require 2 signatures
    MultisigTotal:    3,  // Out of 3 participants
    ParticipantPubKeys: map[wallet.WalletType][][]byte{
        wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
    },
    MultisigRole: paywall.RoleBuyer,
}
```

**Payment Creation** (same API):
```go
payment, err := pw.CreatePayment()  // Works with both single-sig and multisig
```

### Scenario 3: Add Escrow to Existing System

**Step 1**: Enable multisig in configuration (see Scenario 2)

**Step 2**: Create escrow manager:
```go
pw, err := paywall.NewPaywall(config)
if err != nil {
    log.Fatal(err)
}

escrowMgr, err := paywall.NewEscrowManager(pw)
if err != nil {
    log.Fatal(err)
}
```

**Step 3**: Use escrow workflows:
```go
// Create escrow payment
paymentID, err := escrowMgr.CreateEscrow(1.0, time.Hour*48)

// Fund escrow (after blockchain confirmation)
err = escrowMgr.FundEscrow(paymentID)

// Release to seller (requires 2 signatures)
buyerSig := createSignature(buyerKey, paymentID)
sellerSig := createSignature(sellerKey, paymentID)
err = escrowMgr.ReleaseToSeller(paymentID, buyerSig, sellerSig)
```

## Configuration Field Changes

### New Optional Fields

The following fields are added to `Config` but are **completely optional**:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `MultisigEnabled` | `bool` | `false` | Enable multisig mode |
| `MultisigRequired` | `int` | `0` | Required signatures (m in m-of-n) |
| `MultisigTotal` | `int` | `0` | Total participants (n in m-of-n) |
| `ParticipantPubKeys` | `map[wallet.WalletType][][]byte` | `nil` | Participant public keys |
| `MultisigRole` | `MultisigRole` | `0` | This instance's role (buyer/seller/arbiter) |

### Existing Fields (Unchanged)

All existing configuration fields behave identically:

- `PriceInBTC` / `PriceInXMR`
- `TestNet`
- `Store`
- `PaymentTimeout`
- `MinConfirmations`
- `WebhookURL`

## Payment Structure Changes

### Single-Signature Payment (Before/After - No Change)

```go
payment := &Payment{
    ID: "payment-123",
    Addresses: map[wallet.WalletType]string{
        wallet.Bitcoin: "1A1zP1eP...",
    },
    Amounts: map[wallet.WalletType]float64{
        wallet.Bitcoin: 0.001,
    },
    Status: StatusPending,
}
```

### Multisig Payment (New Optional Fields)

When `MultisigEnabled: true`, payments include additional fields:

```go
payment := &Payment{
    // Existing fields (all present and unchanged)
    ID: "payment-123",
    Addresses: map[wallet.WalletType]string{
        wallet.Bitcoin: "3J98t1WpEZ73CNmYviecrnyiWrnqRhWNLy",  // P2SH address
    },
    Amounts: map[wallet.WalletType]float64{
        wallet.Bitcoin: 0.001,
    },
    Status: StatusPending,
    
    // New multisig fields (only populated when multisig enabled)
    MultisigEnabled: true,
    MultisigMetadata: map[wallet.WalletType]*wallet.MultisigMetadata{
        wallet.Bitcoin: {
            RedeemScript: []byte{...},
            ScriptHash:   "...",
            PublicKeys:   [][]byte{...},
        },
    },
    RequiredSignatures: map[wallet.WalletType]int{
        wallet.Bitcoin: 2,
    },
    Signatures: map[wallet.WalletType][]SignatureData{
        wallet.Bitcoin: {
            {SignerID: "buyer", Role: RoleBuyer, ...},
            {SignerID: "seller", Role: RoleSeller, ...},
        },
    },
    
    // Escrow-specific fields (optional)
    EscrowState:   EscrowFunded,
    EscrowTimeout: time.Now().Add(48 * time.Hour),
}
```

**Important**: Old payment records without multisig fields remain valid and work correctly.

## Breaking Changes

**None**. This is a non-breaking release. All existing code continues to work without modification.

## Common Migration Patterns

### Pattern 1: Gradual Rollout

Run single-sig and multisig modes side-by-side:

```go
func createPaymentForUser(userPreferences UserPrefs) (*Payment, error) {
    config := baseConfig()
    
    if userPreferences.EnableEscrow {
        config.MultisigEnabled = true
        config.MultisigRequired = 2
        config.MultisigTotal = 3
        config.ParticipantPubKeys = loadEscrowKeys(userPreferences)
    }
    
    pw, err := paywall.NewPaywall(config)
    if err != nil {
        return nil, err
    }
    
    return pw.CreatePayment()
}
```

### Pattern 2: Feature Flag

Use environment variables or feature flags:

```go
multisigEnabled := os.Getenv("ENABLE_MULTISIG") == "true"

config := paywall.Config{
    PriceInBTC:      0.001,
    MultisigEnabled: multisigEnabled,
    // ... rest of config
}
```

### Pattern 3: Per-Transaction Decision

Enable multisig only for high-value transactions:

```go
func createPayment(amount float64) (*Payment, error) {
    config := baseConfig()
    config.PriceInBTC = amount
    
    // Use multisig escrow for transactions > 0.01 BTC
    if amount > 0.01 {
        config.MultisigEnabled = true
        config.MultisigRequired = 2
        config.MultisigTotal = 3
        config.ParticipantPubKeys = loadEscrowKeys()
    }
    
    pw, err := paywall.NewPaywall(config)
    return pw.CreatePayment()
}
```

## Troubleshooting

### Issue: "generate multisig BTC address: multisig not supported by this wallet implementation"

**Cause**: The Bitcoin HD wallet doesn't yet have full multisig address generation implemented.

**Solution**: This is expected during development. The multisig API and escrow state machine are complete, but wallet-level address generation is in progress. The feature will work once `BTCHDWallet.GenerateMultisigAddress()` is implemented.

**Workaround**: Use the API and escrow examples to understand the workflow. Full functionality will be available in the next release.

### Issue: Payment creation fails with multisig enabled

**Check**:
1. `MultisigRequired` ≤ `MultisigTotal`
2. `MultisigRequired` ≥ 2 (minimum for multisig)
3. Number of public keys matches `MultisigTotal`
4. Public keys are 33 bytes (compressed format)

**Example**:
```go
// ❌ Invalid: required > total
config.MultisigRequired = 3
config.MultisigTotal = 2

// ✅ Valid
config.MultisigRequired = 2
config.MultisigTotal = 3
config.ParticipantPubKeys = map[wallet.WalletType][][]byte{
    wallet.Bitcoin: {key1, key2, key3},  // 3 keys for 3 total
}
```

### Issue: Existing payments not loading after upgrade

**Cause**: This should not happen - backward compatibility is maintained.

**Debug steps**:
1. Check payment store for corruption
2. Verify all required fields are present in stored JSON
3. Check for custom `PaymentStore` implementations that may need updates

**Solution**: Old payments work automatically. New optional fields default to zero values when missing.

### Issue: Monero RPC connection warnings

**Message**: `WARNING: XMR wallet configuration was provided but wallet creation failed`

**Cause**: Monero RPC connection not configured or unavailable.

**Solution**: 
- Set Monero RPC environment variables if using Monero:
  ```bash
  export XMR_WALLET_RPC_URL="http://localhost:18082"
  export XMR_WALLET_USER="username"
  export XMR_WALLET_PASS="password"
  ```
- Or ignore if only using Bitcoin (system continues with Bitcoin-only support)

## Testing Your Migration

### 1. Test Existing Functionality

Ensure single-sig payments still work:

```go
func TestBackwardCompatibility(t *testing.T) {
    config := paywall.Config{
        PriceInBTC:     0.001,
        TestNet:        true,
        Store:          paywall.NewMemoryStore(),
        PaymentTimeout: time.Hour * 24,
        // MultisigEnabled not set (defaults to false)
    }
    
    pw, err := paywall.NewPaywall(config)
    if err != nil {
        t.Fatal(err)
    }
    
    payment, err := pw.CreatePayment()
    if err != nil {
        t.Fatal(err)
    }
    
    if payment.MultisigEnabled {
        t.Error("Single-sig payment should not have multisig enabled")
    }
}
```

### 2. Test Multisig Opt-in

Verify multisig works when explicitly enabled:

```go
func TestMultisigEnable(t *testing.T) {
    config := paywall.Config{
        PriceInBTC:         0.001,
        TestNet:            true,
        Store:              paywall.NewMemoryStore(),
        PaymentTimeout:     time.Hour * 24,
        MultisigEnabled:    true,
        MultisigRequired:   2,
        MultisigTotal:      3,
        ParticipantPubKeys: map[wallet.WalletType][][]byte{
            wallet.Bitcoin: {key1, key2, key3},
        },
    }
    
    pw, err := paywall.NewPaywall(config)
    if err != nil {
        t.Fatal(err)
    }
    
    payment, err := pw.CreatePayment()
    if err != nil {
        t.Fatal(err)
    }
    
    if !payment.MultisigEnabled {
        t.Error("Multisig payment should have multisig enabled")
    }
}
```

### 3. Test Mixed Mode

Verify both modes work in the same application:

```go
func TestMixedMode(t *testing.T) {
    store := paywall.NewMemoryStore()
    
    // Create single-sig payment
    config1 := paywall.Config{
        PriceInBTC:     0.001,
        TestNet:        true,
        Store:          store,
        PaymentTimeout: time.Hour * 24,
    }
    pw1, _ := paywall.NewPaywall(config1)
    payment1, _ := pw1.CreatePayment()
    
    // Create multisig payment
    config2 := config1
    config2.MultisigEnabled = true
    config2.MultisigRequired = 2
    config2.MultisigTotal = 3
    config2.ParticipantPubKeys = map[wallet.WalletType][][]byte{
        wallet.Bitcoin: {key1, key2, key3},
    }
    pw2, _ := paywall.NewPaywall(config2)
    payment2, _ := pw2.CreatePayment()
    
    // Both payments should coexist
    p1, _ := store.GetPayment(payment1.ID)
    p2, _ := store.GetPayment(payment2.ID)
    
    if p1.MultisigEnabled {
        t.Error("First payment should be single-sig")
    }
    if !p2.MultisigEnabled {
        t.Error("Second payment should be multisig")
    }
}
```

## Support

For issues or questions:

- **GitHub Issues**: https://github.com/opd-ai/paywall/issues
- **Documentation**: See `docs/MULTISIG.md` for complete multisig guide
- **Examples**: Check `example/multisig/` for working code samples

## Next Steps

1. Review `docs/MULTISIG.md` for comprehensive multisig documentation
2. Explore example code in `example/multisig/` directory
3. Start with testnet configuration before moving to mainnet
4. Test escrow workflows thoroughly before production use
5. Consider implementing dispute resolution policies

## Summary

✅ **Backward Compatible**: Existing code works without changes  
✅ **Opt-In**: Multisig is disabled by default  
✅ **Gradual Adoption**: Mix single-sig and multisig in the same app  
✅ **Zero Breaking Changes**: All existing APIs unchanged  
✅ **Test Coverage**: Comprehensive test suite included
