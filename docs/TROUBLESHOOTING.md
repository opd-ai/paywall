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

## Network Configuration

### Testnet vs Mainnet Confusion

**Problem**: "I sent real Bitcoin but the payment wasn't detected."

**Cause**: Paywall configured for testnet but user sent to mainnet (or vice versa).

**Symptoms**:
- Payment shows as pending forever
- Address doesn't appear on expected blockchain explorer
- Funds sent to wrong network are **unrecoverable**

**Prevention**:

1. **Check your configuration carefully**:
   ```go
   config := paywall.Config{
       TestNet: true,   // ⚠️ ONLY for testing with fake Bitcoin
       // TestNet: false,  // For production with real Bitcoin
   }
   ```

2. **Verify address format before accepting payment**:
   - **Bitcoin Testnet**: `tb1q...` (bech32) or `m...`/`2...` (legacy)
   - **Bitcoin Mainnet**: `bc1q...` (bech32) or `1...`/`3...` (legacy)
   - **Monero Testnet**: Starts with `9` or `B`
   - **Monero Mainnet**: Starts with `4` or `8`

3. **Test on testnet first**:
   ```bash
   # Get free testnet coins
   # Bitcoin: https://testnet-faucet.mempool.co/
   # Monero: https://stagenet.xmr-tw.org/faucet.html
   ```

4. **Add validation in UI**:
   ```go
   if config.TestNet {
       fmt.Println("⚠️ WARNING: Using TESTNET - do not send real funds!")
   }
   ```

### RPC Endpoint Configuration

**Problem**: "Blockchain verification failing" or "Connection refused"

**Bitcoin RPC**:

The paywall uses blockchain APIs to verify payments. If the default endpoint is down:

1. **Run your own Bitcoin node** (most reliable):
   ```bash
   # Install Bitcoin Core
   # Then in bitcoin.conf:
   server=1
   rpcuser=yourusername
   rpcpassword=yourpassword
   testnet=1  # For testnet
   
   # Access via: http://localhost:8332
   ```

2. **Use public API endpoints** (less reliable):
   - Testnet: `https://blockstream.info/testnet/api/`
   - Mainnet: `https://blockstream.info/api/`
   
   Note: The paywall currently uses embedded blockchain checking. To use custom RPC:
   - Extend `BTCBroadcaster` with custom RPC URL
   - Set environment variable (if supported by your deployment)

**Monero RPC**:

1. **Run monero-wallet-rpc locally**:
   ```bash
   # Testnet
   monero-wallet-rpc \
     --rpc-bind-port 18081 \
     --wallet-file ~/testnet/mywallet \
     --password mypass \
     --testnet \
     --daemon-address stagenet.xmr-tw.org:38081 \
     --rpc-login user:pass
   
   # Mainnet
   monero-wallet-rpc \
     --rpc-bind-port 18081 \
     --wallet-file ~/mywallet \
     --password mypass \
     --daemon-address node.moneroworld.com:18089 \
     --rpc-login user:pass
   ```

2. **Configure in paywall**:
   ```go
   config := paywall.Config{
       XMRRPC:      "http://localhost:18081",
       XMRUser:     "user",
       XMRPassword: "pass",
   }
   ```

3. **Test connection**:
   ```bash
   curl -X POST http://localhost:18081/json_rpc \
     -H 'Content-Type: application/json' \
     -u user:pass \
     -d '{"jsonrpc":"2.0","id":"0","method":"get_balance","params":{"account_index":0}}'
   ```

### Firewall Rules

**Problem**: "Connection refused" or "timeout" errors.

**Linux (ufw)**:
```bash
# Allow Bitcoin RPC (if running local node)
sudo ufw allow 8332/tcp

# Allow Monero wallet RPC
sudo ufw allow 18081/tcp

# Allow your paywall HTTP server
sudo ufw allow 8000/tcp

# Check rules
sudo ufw status
```

**Linux (iptables)**:
```bash
# Allow inbound on paywall port
sudo iptables -A INPUT -p tcp --dport 8000 -j ACCEPT

# Allow outbound to blockchain APIs
sudo iptables -A OUTPUT -p tcp --dport 443 -j ACCEPT

# Save rules
sudo iptables-save > /etc/iptables/rules.v4
```

**Docker**:
```dockerfile
# Expose paywall port
EXPOSE 8000

# If running RPC in same container
EXPOSE 18081
```

**Cloud providers** (AWS/GCP/Azure):
- Add security group rule allowing inbound port 8000
- Add security group rule allowing outbound HTTPS (443) for blockchain APIs
- If using RPC, add inbound rule for RPC ports

## Recovery Procedures

### Recovering Stuck Payments

**Symptom**: Payment shows "pending" but blockchain shows confirmed transaction.

**Diagnosis**:

1. **Check payment status**:
   ```go
   payment, err := paywall.Store.GetPayment(paymentID)
   if err != nil {
       log.Fatal(err)
   }
   log.Printf("Status: %s, Confirmations: %d", payment.Status, payment.Confirmations)
   ```

2. **Verify on blockchain explorer**:
   ```bash
   # Bitcoin testnet
   https://blockstream.info/testnet/address/YOUR_ADDRESS
   
   # Bitcoin mainnet
   https://blockstream.info/address/YOUR_ADDRESS
   ```

3. **Check confirmation threshold**:
   ```go
   if payment.Confirmations < config.MinConfirmations {
       // Still waiting for confirmations
   }
   ```

**Solutions**:

1. **Manual payment confirmation** (if verified on blockchain):
   ```go
   payment, _ := paywall.Store.GetPayment(paymentID)
   payment.Status = paywall.StatusConfirmed
   payment.Confirmations = 6  // Or actual confirmation count
   err := paywall.Store.UpdatePayment(payment)
   ```

2. **Restart payment verification**:
   ```go
   // The background goroutine checks periodically
   // Wait for next check cycle (typically 10-60 seconds)
   // Or restart your application to trigger immediate recheck
   ```

3. **Lower confirmation threshold temporarily** (for testing):
   ```go
   config := paywall.Config{
       MinConfirmations: 1,  // Accept after 1 confirmation
   }
   ```

### Recovering from Lost Wallet

**Problem**: "Lost wallet file" or "Need to restore from backup"

**Bitcoin Wallet Recovery**:

1. **If you have the mnemonic phrase** (12 or 24 words):
   ```go
   // Restore wallet from mnemonic
   seed, err := wallet.ImportFromMnemonic("word1 word2 ... word24", "")
   if err != nil {
       log.Fatal(err)
   }
   
   btcWallet, err := wallet.NewBTCHDWallet(seed[:32], testnet, 1)
   if err != nil {
       log.Fatal(err)
   }
   
   // Save to new encrypted file
   btcWallet.SaveToFile(wallet.StorageConfig{
       DataDir:       "./paywallet",
       EncryptionKey: encryptionKey,
   })
   ```

2. **If you have the encrypted wallet file**:
   ```go
   // Load from backup
   wallet, err := wallet.LoadFromFile(wallet.StorageConfig{
       DataDir:       "./paywallet_backup",
       EncryptionKey: originalKey,
   })
   if err != nil {
       log.Fatal(err)
   }
   ```

3. **If you have neither** (mnemonic nor file):
   - **Funds are UNRECOVERABLE**
   - This is why backup is critical
   - Always store mnemonic phrase in secure location

**Important**: After wallet recovery, the `nextIndex` counter (which tracks used addresses) is reset to 0. This means:
- Old addresses will regenerate in the same order
- Check payment history to find the highest used address index
- Manually increment `nextIndex` to avoid address reuse:
  ```go
  // After recovery, if you know you used 100 addresses:
  wallet.nextIndex = 100
  wallet.SaveToFile(config)
  ```

**Monero Wallet Recovery**:

1. **Restore from seed phrase**:
   ```bash
   # Stop wallet RPC
   killall monero-wallet-rpc
   
   # Restore wallet
   monero-wallet-cli --testnet --restore-deterministic-wallet
   # Enter your 25-word seed phrase
   # Set new wallet name and password
   
   # Start RPC with restored wallet
   monero-wallet-rpc --wallet-file restored_wallet --password newpass --testnet
   ```

2. **Restore from keys**:
   ```bash
   monero-wallet-cli --testnet --generate-from-keys restored_wallet
   # Enter private view key and spend key
   ```

### Recovering from Failed Transactions

**Problem**: "Transaction broadcast failed" or "Transaction rejected"

**Bitcoin Transaction Failures**:

1. **Insufficient fee**:
   - **Symptom**: Transaction stuck in mempool for hours/days
   - **Solution**: Wait for transaction to be dropped (3-7 days) or use RBF (Replace-By-Fee)
   - **Prevention**: Set reasonable fee rate (check https://mempool.space/ for current rates)

2. **Double-spend attempt**:
   - **Symptom**: "Transaction already exists" or "Input already spent"
   - **Cause**: Tried to spend same UTXO twice
   - **Solution**: Wait for first transaction to confirm or be dropped

3. **Invalid transaction**:
   - **Symptom**: "Script verification failed" or "Non-standard transaction"
   - **Cause**: Malformed transaction or invalid signatures
   - **Solution**: Regenerate transaction with correct signatures

**Monero Transaction Failures**:

1. **Insufficient funds**:
   ```bash
   # Check wallet balance
   curl -X POST http://localhost:18081/json_rpc \
     -u user:pass \
     -d '{"jsonrpc":"2.0","id":"0","method":"get_balance","params":{"account_index":0}}'
   ```

2. **Unlock time not reached**:
   - Monero has 10-block lock time after receiving funds
   - Wait ~20 minutes for funds to unlock

3. **Daemon not synchronized**:
   ```bash
   # Check daemon sync status
   curl -X POST http://localhost:18081/json_rpc \
     -d '{"jsonrpc":"2.0","id":"0","method":"get_info"}'
   # Look for "synchronized": true
   ```

### Recovering from Escrow Timeout

**Problem**: "Escrow timed out" but buyer claims they paid.

**Investigation**:

1. **Check escrow state**:
   ```go
   payment, _ := paywall.Store.GetPayment(paymentID)
   log.Printf("EscrowState: %v, EscrowTimeout: %v", payment.EscrowState, payment.EscrowTimeout)
   log.Printf("TransactionID: %s, BroadcastedAt: %v", payment.TransactionID, payment.BroadcastedAt)
   ```

2. **Verify on blockchain**:
   - If `TransactionID` is set, check blockchain explorer
   - If transaction confirmed, escrow should not have timed out

3. **Check audit log**:
   ```go
   entries, _ := auditLogger.GetAuditTrail(paymentID)
   for _, entry := range entries {
       log.Printf("%s: %s -> %s by %s",
           entry.Timestamp, entry.PreviousState, entry.NewState, entry.ActorRole)
   }
   ```

**Manual Resolution** (if payment verified on blockchain):

```go
// If transaction is confirmed but escrow timed out due to bug:
// 1. Verify transaction on blockchain
// 2. Manually update escrow state
payment, _ := paywall.Store.GetPayment(paymentID)
payment.EscrowState = paywall.EscrowFunded
payment.TransactionID = "blockchain_tx_id"
payment.BroadcastedAt = time.Now()
paywall.Store.UpdatePayment(payment)

// 3. Release to seller or refund buyer as appropriate
// Use EscrowManager.ReleaseToSeller() or RefundBuyer()
```

**Timeout Extension** (if both parties agree):

```go
// Request extension (requires 2-of-3 signatures)
err := escrowManager.ExtendTimeout(
    paymentID,
    7 * 24 * time.Hour,  // Extend by 7 days
    buyerSig,
    sellerSig,
)
```

---

If you can't find the solution here, check the project's GitHub issues or open a new one with:
- An error message (full text, with context)
- Steps to reproduce
- Your configuration (without sensitive data)
- Go version and OS
