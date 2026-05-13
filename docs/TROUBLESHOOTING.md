# Troubleshooting

This guide covers common issues and their solutions.

## Configuration Issues

### "XMR wallet password not provided"

**Error**:
```
Error: XMR wallet password not provided
```

**Cause**: You have Monero configuration but didn't provide the password via `config.XMRPassword` or `XMR_WALLET_PASS` environment variable.

**Solution**:

**Option 1: Use Bitcoin-only** (remove Monero configuration):
```go
config := paywall.Config{
    PriceInBTC:     0.001,
    // Don't set XMRUser, XMRPassword, or XMRRPC
}
```

**Option 2: Provide Monero password explicitly**:
```go
config := paywall.Config{
    PriceInBTC:     0.001,
    PriceInXMR:     0.01,
    XMRUser:        "wallet_user",
    XMRPassword:    "secure_password",  // Explicitly provided
    XMRRPC:         "http://localhost:18081",
}
```

**Option 3: Set environment variable**:
```bash
export XMR_WALLET_USER="wallet_user"
export XMR_WALLET_PASS="secure_password"
go run my_app.go
```

### "PriceInBTC must be positive"

**Error**:
```
Error: PriceInBTC must be positive, got: 0.000000
```

**Cause**: `PriceInBTC` is 0 or negative, but the code expects a positive price.

**Solution**: Set a valid positive price:
```go
config := paywall.Config{
    PriceInBTC: 0.001,  // Must be > 0
}
```

### "PriceInBTC below dust limit"

**Error**:
```
Error: PriceInBTC 0.000001 is below dust limit (minimum: 0.00001)
```

**Cause**: Bitcoin payment price is too low to be economical on the blockchain.

**Solution**: Increase the price to at least 0.00001 BTC:
```go
config := paywall.Config{
    PriceInBTC: 0.00001,  // Minimum acceptable
    // Or higher:
    // PriceInBTC: 0.001,
}
```

The dust limit exists because:
- Bitcoin transactions have minimum fees (~200 satoshis)
- Payments below this range would cost more in fees than the payment value
- The dust limit prevents users from wasting funds

### "payment timeout must be positive"

**Error**:
```
Error: payment timeout must be positive
```

**Cause**: `PaymentTimeout` is 0 or negative.

**Solution**: Set a valid positive timeout:
```go
config := paywall.Config{
    PaymentTimeout: 24 * time.Hour,  // 24 hours
    // Or: 1 * time.Hour, 5 * time.Minute, etc.
}
```

### "Store must not be nil"

**Error**:
```
panic: Store must not be nil
```

**Cause**: No payment store was provided in the Config.

**Solution**: Provide a store (Memory, File, or EncryptedFile):
```go
config := paywall.Config{
    Store: paywall.NewMemoryStore(),
    // Or:
    // Store: paywall.NewFileStore("./payments"),
}
```

## Runtime Issues

### Payment page not showing

**Symptom**: Accessing a protected endpoint returns a blank response or error instead of the payment page.

**Cause**: The embedded template might have failed to load, or there's an HTTP error.

**Solution**:

1. **Check logs**: Look for error messages during paywall initialization:
   ```
   log.Printf("Paywall initialized: %+v", pw)
   ```

2. **Verify embedded templates**: The templates/ directory must exist and contain `payment.html`:
   ```bash
   ls -la templates/payment.html
   ```

3. **Check static files**: The `static/qrcode.min.js` file must exist:
   ```bash
   ls -la static/qrcode.min.js
   ```

4. **Debug HTTP response**:
   ```go
   // Check status code manually
   resp, err := http.Get("http://localhost:8000/protected")
   if err != nil {
       log.Fatal(err)
   }
   log.Printf("Status: %d", resp.StatusCode)
   body, _ := io.ReadAll(resp.Body)
   log.Printf("Body: %s", body)
   ```

### Cookies not being set (HTTPS required)

**Symptom**: Cookie shows as empty or not persisted across requests.

**Error in logs**:
```
Secure flag on cookie but using HTTP
```

**Cause**: The paywall middleware requires HTTPS for security. HTTP connections cannot use Secure cookies.

**Solution**:

1. **Use HTTPS in production**:
   ```go
   log.Fatal(http.ListenAndServeTLS(":443", "cert.pem", "key.pem", nil))
   ```

2. **For development/testing only**, modify middleware to accept HTTP (NOT RECOMMENDED for production):
   ```go
   // Edit middleware.go to conditionally set Secure flag
   // This is a security risk and should only be done for development
   ```

3. **Use a TLS reverse proxy** (recommended for testing):
   ```bash
   # Use nginx or Caddy to terminate TLS and forward to your HTTP app
   caddy reverse-proxy --from localhost:8443 --to localhost:8000
   ```

### Payment not confirming

**Symptom**: User sends Bitcoin but payment stays in "pending" status indefinitely.

**Causes and Solutions**:

**Cause 1: Wrong address**
- User sent to wrong address
- Solution: Check public blockchain explorer to verify transaction:
  ```bash
  # Bitcoin testnet explorer
  https://testnet.blockchain.info/address/PASTE_ADDRESS_HERE
  ```

**Cause 2: Insufficient confirmations**
```
Payment created: 1 confirmation
MinConfirmations: 6
Status: Still pending
```
- Solution: Wait for more confirmations (~10 minutes per confirmation on testnet)
- Or lower `MinConfirmations` for testing:
  ```go
  config := paywall.Config{
      MinConfirmations: 1,  // Accept faster
  }
  ```

**Cause 3: RPC endpoint timeout**
```go
// In logs:
2026/05/12 10:00:00 ERROR: Failed to verify payment xyz: blockchain timeout
```
- Solution: Change blockchain endpoint or run a local node
- Try adding a longer timeout for your HTTP client:
  ```go
  client := &http.Client{
      Timeout: 30 * time.Second,  // Increase from default 10s
  }
  ```

**Cause 4: Test on wrong network**
```
// Sent on testnet but configured for mainnet (or vice versa)
config := paywall.Config{
    TestNet: true,  // But sent to mainnet address
}
```
- Solution: Ensure TestNet setting matches where you sent funds
- Testnet Bitcoin addresses start with `tb1q` or `2` or `m`
- Mainnet Bitcoin addresses start with `bc1q` or `1` or `3`

### Monero RPC connection failed

**Error**:
```
ERROR: monero RPC connection failed: Post "": unsupported protocol scheme ""
```

**Cause**: Monero RPC endpoint is not configured or unreachable.

**Solution**:

1. **Check Monero wallet RPC is running**:
   ```bash
   ps aux | grep monero-wallet-rpc
   # Should show running process
   ```

2. **Verify RPC port is listening**:
   ```bash
   netstat -tlnp | grep 18081
   # Should show: tcp 127.0.0.1:18081 LISTEN
   ```

3. **Test connectivity**:
   ```bash
   curl -X POST http://localhost:18081/json_rpc \
      -H "Content-Type: application/json" \
      -d '{"jsonrpc":"2.0","id":"0","method":"on_transfer"}'
   # Should get a response
   ```

4. **Check XMRRPC URL**:
   ```go
   config := paywall.Config{
       XMRRPC: "http://localhost:18081",  // Verify this is correct
   }
   ```

5. **Check RPC credentials**:
   ```bash
   # Verify username/password are correct
   export XMR_WALLET_USER="correct_user"
   export XMR_WALLET_PASS="correct_password"
   ```

### Address generation failed

**Error**:
```
ERROR: wallet initialization failed: key derivation failed
```

**Cause**: Usually indicates corrupted seed or random source issues.

**Solution**:

1. **Check system entropy** (if using /dev/urandom):
   ```bash
   cat /proc/sys/kernel/random/entropy_avail
   # Should be > 1000
   ```

2. **Verify seed generation**:
   ```go
   seed := make([]byte, 32)
   n, err := rand.Read(seed)
   if err != nil || n != 32 {
       log.Fatal("Failed to generate seed")
   }
   ```

3. **Ensure crypto/rand is available**:
   ```bash
   ls -la /dev/urandom
   # Should exist and be readable
   ```

### Payments stuck in pending (not updating)

**Symptom**: Payments don't update status even after confirmations appear on blockchain.

**Cause**: Verification background goroutine encountered an error and may have stopped.

**Solution**:

1. **Check if paywall is still running**:
   ```go
   // Monitor goroutine health
   log.Printf("Paywall still alive: monitoring = %v", pw.monitor != nil)
   ```

2. **Check logs for verification errors**:
   ```bash
   # Look for repeated error messages
   grep "verification failed" app.log
   ```

3. **Restart the application**:
   ```bash
   # Kill and restart the process
   pkill -f "my_app"
   go run my_app.go
   ```

4. **Call Close() properly**:
   ```go
   defer pw.Close()  // Ensure cleanup happens
   ```

## Storage Issues

### "Directory does not exist"

**Error**:
```
mkdir ./payments: no such file or directory
```

**Cause**: File store directory path is invalid or parent directory doesn't exist.

**Solution**:

1. **Create the directory manually**:
   ```bash
   mkdir -p ./payments
   chmod 700 ./payments
   ```

2. **Or use FileStore default location**:
   ```go
   store := paywall.NewFileStore("./payments")
   // Created if it doesn't exist
   ```

3. **Or use absolute path**:
   ```go
   store := paywall.NewFileStore("/tmp/paywall-payments")
   ```

### "Permission denied" accessing payment files

**Error**:
```
permission denied: ./payments/payment.json
```

**Cause**: File permissions prevent reading/writing payment files.

**Solution**:

1. **Fix directory permissions**:
   ```bash
   chmod 700 ./payments
   chmod 600 ./payments/*.json
   ```

2. **Or change owner**:
   ```bash
   chown -R appuser:appuser ./payments
   sudo -u appuser go run my_app.go
   ```

3. **Or run as correct user**:
   ```bash
   # Check who owns the directory
   ls -ld ./payments
   # Run as that user
   sudo -u paywall_user go run my_app.go
   ```

### "Encryption key too short"

**Error**:
```
encryption key must be 32 bytes, got 16
```

**Cause**: Encryption key is not 256 bits (32 bytes).

**Solution**: Generate a proper key:
```bash
# Generate 32 random bytes (256 bits) in hex
export PAYWALL_ENCRYPTION_KEY=$(openssl rand -hex 32)

# Verify it's correct
echo $PAYWALL_ENCRYPTION_KEY | wc -c
# Should print 65 (64 hex chars + newline)
```

## Network Issues

### "Connection refused" for blockchain API

**Error**:
```
ERROR: Failed to reach blockchain API: connection refused
```

**Cause**: Public blockchain API endpoints are unreachable or all endpoints are down.

**Solution**:

1. **Check internet connectivity**:
   ```bash
   ping 8.8.8.8
   ```

2. **Test specific endpoint**:
   ```bash
   curl -I https://api.blockcypher.com/v1/btc/test3
   # Should return 200 OK
   ```

3. **Use local Bitcoin node** (recommended for production):
   ```bash
   # Run local Bitcoin full node
   bitcoind -testnet -rpcport=18332
   
   # Configure paywall to use it
   # (Currently requires code modification)
   ```

4. **Check firewall/proxy**:
   ```bash
   # Verify outbound HTTPS allowed
   curl -I https://www.google.com
   # Should work
   ```

## Testing & Development

### "How do I test payment verification locally?"

1. **Use Bitcoin testnet** (as shown in examples):
   ```go
   config := paywall.Config{
       TestNet: true,
   }
   ```

2. **Send real testnet Bitcoin**:
   - Use a testnet faucet: https://testnet-faucet.mempool.space
   - Send to generated address via your wallet or command line
   - Wait for confirmations (~10 minutes per confirmation)

3. **Or use regtest** (fully controlled local testing):
   ```bash
   # Start Bitcoin regtest node
   bitcoind -regtest -rpcuser=test -rpcpassword=test
   
   # Generate blocks
   bitcoin-cli -regtest generatetoaddress 101 "bcrt1..."
   
   # Mine instantly as you test
   bitcoin-cli -regtest generatetoaddress 1 "bcrt1..."
   ```

### "How do I inspect payment records?"

**For Memory Store**:
- Data is lost on restart, no persistent inspection

**For File Store**:
```bash
# List all payment files
ls -la ./payments/

# View a payment as JSON
cat ./payments/1461540575152f03fedd677cb87cdc62.json | jq .

# Find payments with specific status
grep -l "confirmed" ./payments/*.json
```

**For Encrypted File Store**:
```bash
# Files are encrypted, can't view directly
# Use the GetPaymentByID API instead
```

### "How do I reset all payments?"

**For Memory Store**:
- Automatic on restart

**For File Store**:
```bash
rm -rf ./payments/*
```

**For Encrypted File Store**:
```bash
rm -rf /var/lib/paywall/payments/*
```

## Production Issues

### High CPU usage from verification

**Cause**: Background verification goroutine checking too many pending payments.

**Solution**:

1. **Reduce payment timeout** (fewer old payments):
   ```go
   config := paywall.Config{
       PaymentTimeout: 12 * time.Hour,  // Instead of 24 hours
   }
   ```

2. **Increase MinConfirmations** (payments confirm faster):
   ```go
   config := paywall.Config{
       MinConfirmations: 1,  // Confirm faster, stop checking sooner
   }
   ```

3. **Monitor and alert**:
   ```go
   pending, _ := pw.Store.ListPendingPayments()
   if len(pending) > 1000 {
       log.Fatalf("Too many pending payments: %d", len(pending))
   }
   ```

### Memory usage grows over time

**Cause**: Pending payments accumulating without being cleaned up.

**Solution**:

1. **Verify expired payments are being removed**:
   - Expired payments (past PaymentTimeout) should be automatically removed
   - Check logs for cleanup messages

2. **Lower PaymentTimeout**:
   ```go
   config := paywall.Config{
       PaymentTimeout: 12 * time.Hour,  // Faster cleanup
   }
   ```

3. **Monitor memory usage**:
   ```bash
   # Linux
   ps aux | grep my_app | awk '{print $6}'  # RSS in kilobytes
   
   # Or use pprof:
   import _ "net/http/pprof"
   go func() {
       log.Println(http.ListenAndServe("localhost:6060", nil))
   }()
   # Then: curl http://localhost:6060/debug/pprof/heap?debug=1
   ```

## Getting Help

### Enable Debug Logging

```go
import "log"

// Add a custom logger prefix for paywall debugging
log.SetPrefix("[PAYWALL] ")
log.SetFlags(log.LstdFlags | log.Lshortfile)

// Then all errors will include file:line numbers
// Example: [PAYWALL] 2026/05/12 10:00:00 handlers.go:50: Failed to create payment
```

### Check Project Issues

Visit https://github.com/opd-ai/paywall/issues to:
- Search for similar problems
- Report new bugs with logs and reproduction steps
- Request features or improvements

### Read Project Documentation

- [README.md](../README.md) - Quick start and features
- [CONFIGURATION.md](CONFIGURATION.md) - Detailed config guide
- [SECURITY.md](SECURITY.md) - Security considerations
- [EXAMPLES.md](EXAMPLES.md) - Code examples

### Enable Detailed Logging

To capture more debugging information:

```go
// Add to your main()
log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

// Log payment lifecycle events
handler := func(w http.ResponseWriter, r *http.Request) {
    log.Printf("Request: %s %s", r.Method, r.URL)
    // ... rest of handler
}
```

## Common Gotchas

### Don't Use TestNet in Production

```go
config := paywall.Config{
    TestNet: true,  // ❌ WRONG for production
}
// This will generate testnet addresses where users expect mainnet
// You will lose all payments
```

### Use Unique HTTP Handler for Each Content

```go
// ❌ Wrong: reusing same handler
protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Content"))
})
http.Handle("/article1", pw.Middleware(protected))
http.Handle("/article2", pw.Middleware(protected))

// ✅ Right: different content
article1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Article 1 content"))
})
article2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Article 2 content"))
})
http.Handle("/article1", pw.Middleware(article1))
http.Handle("/article2", pw.Middleware(article2))
```

### Close Paywall Before Exiting

```go
// ✅ Right: use defer
pw, _ := paywall.NewPaywall(config)
defer pw.Close()
// Close is called on exit, cleaning up resources

// ❌ Wrong: no cleanup
pw, _ := paywall.NewPaywall(config)
// If program exits, background goroutine may not stop cleanly
```

---

If you can't find the solution here, check the project's GitHub issues or open a new one with:
- An error message (full text, with context)
- Steps to reproduce
- Your configuration (without sensitive data)
- Go version and OS
