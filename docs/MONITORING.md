# Monitoring Guide

This guide provides comprehensive monitoring strategies for the paywall system, including metrics collection, structured logging, and observability best practices for production deployments.

## Table of Contents

- [Metrics](#metrics)
- [Structured Logging](#structured-logging)
- [Dashboard Examples](#dashboard-examples)
- [Alerting](#alerting)
- [Performance Monitoring](#performance-monitoring)
- [Security Monitoring](#security-monitoring)

## Metrics

### MetricsCollector

The paywall package includes a built-in `MetricsCollector` that provides Prometheus-style metrics for comprehensive system observability.

#### Creating a Metrics Collector

```go
import "github.com/opd-ai/paywall"

metrics := paywall.NewMetricsCollector()
```

#### Available Metrics

##### Multisig Operations
- `MultisigAddressGenerated` - Counter for multisig addresses created
- `PartialSignatureSubmitted` - Counter for partial signatures submitted
- `PartialSignatureVerified` - Counter for partial signatures verified
- `MultisigTransactionCompleted` - Counter for multisig transactions completed
- `MultisigTransactionBroadcast` - Counter for multisig transactions broadcast

##### Escrow State Transitions
- `EscrowCreated` - Counter for escrows created
- `EscrowFunded` - Counter for escrows funded
- `EscrowCompleted` - Counter for escrows completed
- `EscrowRefunded` - Counter for escrows refunded
- `EscrowDisputed` - Counter for escrows disputed
- `EscrowDisputeResolved` - Counter for escrows with resolved disputes

##### Dispute Resolution
- `DisputeResolutionCount` - Counter for total dispute resolutions
- `DisputeResolutionAvgMs` - Average dispute resolution time in milliseconds
- `DisputeResolutionTotalMs` - Total dispute resolution time in milliseconds

##### Payment Operations
- `PaymentCreated` - Counter for payments created
- `PaymentConfirmed` - Counter for payments confirmed
- `PaymentExpired` - Counter for payments expired

##### Error Metrics
- `SignatureVerificationFailed` - Counter for failed signature verifications
- `TransactionBroadcastFailed` - Counter for failed transaction broadcasts
- `EscrowTimeoutTriggered` - Counter for escrow timeouts
- `ArbiterConsensusRequired` - Counter for arbitrations requiring consensus

##### Performance Metrics
- `AddressGenerationDurationMs` - Cumulative time spent generating addresses
- `SignatureVerificationDurationMs` - Cumulative time spent verifying signatures
- `StateTransitionDurationMs` - Cumulative time spent on state transitions

#### Retrieving Metrics

```go
snapshot := metrics.Snapshot()

fmt.Printf("Payments created: %d\n", snapshot.PaymentCreated)
fmt.Printf("Escrows completed: %d\n", snapshot.EscrowCompleted)
fmt.Printf("Average dispute resolution: %dms\n", snapshot.DisputeResolutionAvgMs)
```

#### Integration Example

```go
// Initialize metrics collector
metrics := paywall.NewMetricsCollector()

// Instrument payment creation
payment, err := pw.CreatePayment()
if err == nil {
    metrics.IncrementPaymentCreated()
    if payment.MultisigEnabled {
        metrics.IncrementMultisigAddressGenerated()
    }
}

// Instrument escrow state transitions
start := time.Now()
err = manager.TransitionState(payment, paywall.EscrowFunded, paywall.RoleBuyer)
if err == nil {
    metrics.RecordStateTransitionDuration(time.Since(start))
    metrics.IncrementEscrowFunded()
}
```

### Prometheus Integration

To integrate with Prometheus, wrap the MetricsCollector with Prometheus collectors:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    paymentsCreated = promauto.NewCounter(prometheus.CounterOpts{
        Name: "paywall_payments_created_total",
        Help: "Total number of payments created",
    })
    
    escrowStateTransitions = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "paywall_escrow_state_transitions_total",
            Help: "Total number of escrow state transitions",
        },
        []string{"from_state", "to_state"},
    )
    
    disputeResolutionDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "paywall_dispute_resolution_duration_seconds",
            Help:    "Dispute resolution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),
        },
    )
)

// Expose metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

## Structured Logging

The paywall package provides a `StructuredLogger` that emits JSON-formatted logs for easy parsing and analysis.

### Creating a Logger

```go
import "os"

// JSON logging to stdout (production)
logger := paywall.NewDefaultLogger()

// Custom configuration
logger := paywall.NewStructuredLogger(
    os.Stdout,                   // Output writer
    paywall.LogLevelInfo,        // Minimum log level
    true,                        // JSON output
)

// Human-readable logging (development)
logger := paywall.NewStructuredLogger(
    os.Stdout,
    paywall.LogLevelDebug,
    false,                       // Human-readable format
)
```

### Log Levels

- `LogLevelDebug` - Detailed debugging information
- `LogLevelInfo` - Informational messages (default)
- `LogLevelWarn` - Warning messages
- `LogLevelError` - Error messages

### Event Types

#### Multisig Events
```go
logger.LogMultisigAddressGenerated(paymentID, address, walletType, required, total)
logger.LogPartialSignatureSubmitted(paymentID, role, signatureIndex)
logger.LogPartialSignatureVerified(paymentID, role, signatureIndex)
logger.LogSignatureThresholdReached(paymentID, requiredSigs, collectedSigs)
logger.LogMultisigTransactionBroadcast(paymentID, txHash, walletType)
```

#### Escrow Events
```go
logger.LogEscrowCreated(paymentID, amount, currency, participants)
logger.LogEscrowStateTransition(paymentID, fromState, toState, role)
logger.LogEscrowFunded(paymentID, txHash, amount, currency)
logger.LogEscrowCompleted(paymentID, releasedToRole)
logger.LogEscrowRefunded(paymentID, refundedToRole)
```

#### Dispute Events
```go
logger.LogDisputeInitiated(paymentID, initiatedBy, reason)
logger.LogArbiterVoteSubmitted(paymentID, arbiterIndex, votedFor)
logger.LogDisputeResolved(paymentID, winner, consensusReached, resolutionTimeMs)
```

#### Error Events
```go
logger.LogSignatureVerificationFailed(paymentID, role, reason)
logger.LogTransactionBroadcastFailed(paymentID, txHash, err)
logger.LogInvalidStateTransition(paymentID, fromState, toState)
```

### Log Format

JSON format (production):
```json
{
  "timestamp": "2024-01-15T10:30:45.123456Z",
  "level": "INFO",
  "event": "escrow_created",
  "message": "Escrow created",
  "payment_id": "pay_abc123",
  "amount": 0.001,
  "currency": "btc",
  "state": "pending",
  "data": {
    "participants": ["buyer", "seller", "arbiter"]
  }
}
```

Human-readable format (development):
```
[2024-01-15T10:30:45.123456Z] INFO - escrow_created: Escrow created (payment=pay_abc123)
```

### Integration with ELK Stack

For centralized logging with Elasticsearch, Logstash, and Kibana:

```go
// Configure logger to write to a file that Filebeat can tail
logFile, err := os.OpenFile("/var/log/paywall/app.log", 
    os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
if err != nil {
    log.Fatal(err)
}
defer logFile.Close()

logger := paywall.NewStructuredLogger(logFile, paywall.LogLevelInfo, true)
```

Filebeat configuration:
```yaml
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/paywall/*.log
  json.keys_under_root: true
  json.add_error_key: true

output.elasticsearch:
  hosts: ["localhost:9200"]
  index: "paywall-%{+yyyy.MM.dd}"
```

## Dashboard Examples

### Grafana Dashboard

Sample Prometheus queries for Grafana dashboards:

#### Payment Volume
```promql
rate(paywall_payments_created_total[5m])
```

#### Escrow Success Rate
```promql
sum(rate(paywall_escrow_completed_total[5m])) / 
sum(rate(paywall_escrow_created_total[5m]))
```

#### Dispute Rate
```promql
rate(paywall_escrow_disputed_total[5m])
```

#### Average Dispute Resolution Time
```promql
rate(paywall_dispute_resolution_duration_seconds_sum[5m]) /
rate(paywall_dispute_resolution_duration_seconds_count[5m])
```

#### Signature Verification Success Rate
```promql
1 - (rate(paywall_signature_verification_failed_total[5m]) / 
     rate(paywall_partial_signature_submitted_total[5m]))
```

### Sample Dashboard Layout

```
+------------------+------------------+------------------+
| Payment Volume   | Escrows Active   | Dispute Rate    |
| 125 ops/sec      | 2,341           | 2.3%            |
+------------------+------------------+------------------+
| Multisig Usage % | Signature Fails  | Avg Resolution  |
| 68%              | 3/hour          | 45 minutes      |
+------------------+------------------+------------------+
|                                                         |
|        Payment Flow (last 24h)                         |
|        [Graph showing Created/Confirmed/Expired]       |
|                                                         |
+------------------+------------------+------------------+
|                                                         |
|        Escrow State Distribution                       |
|        [Pie chart: Pending/Funded/Completed/Disputed]  |
|                                                         |
+------------------+------------------+------------------+
```

## Alerting

### Recommended Alerts

#### Critical Alerts

**High Signature Failure Rate**
```promql
rate(paywall_signature_verification_failed_total[5m]) > 0.1
```
Trigger: More than 0.1 signature failures per second
Action: Investigate potential security issue or software bug

**Transaction Broadcast Failures**
```promql
rate(paywall_transaction_broadcast_failed_total[5m]) > 0.05
```
Trigger: More than 0.05 broadcast failures per second
Action: Check Bitcoin/Monero node connectivity

**Excessive Escrow Timeouts**
```promql
rate(paywall_escrow_timeout_triggered_total[5m]) > 1
```
Trigger: More than 1 timeout per second
Action: Review timeout configuration or buyer/seller activity

#### Warning Alerts

**High Dispute Rate**
```promql
rate(paywall_escrow_disputed_total[5m]) / 
rate(paywall_escrow_created_total[5m]) > 0.05
```
Trigger: More than 5% of escrows disputed
Action: Review dispute patterns, potential fraud

**Slow Dispute Resolution**
```promql
avg(paywall_dispute_resolution_duration_seconds) > 7200
```
Trigger: Average resolution time exceeds 2 hours
Action: Check arbiter responsiveness

**Low Escrow Completion Rate**
```promql
rate(paywall_escrow_completed_total[5m]) / 
rate(paywall_escrow_funded_total[5m]) < 0.9
```
Trigger: Less than 90% completion rate
Action: Investigate refund patterns

### Alert Configuration Example (AlertManager)

```yaml
groups:
- name: paywall_alerts
  interval: 1m
  rules:
  - alert: HighSignatureFailureRate
    expr: rate(paywall_signature_verification_failed_total[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High signature verification failure rate"
      description: "{{ $value }} signature failures per second detected"

  - alert: HighDisputeRate
    expr: |
      rate(paywall_escrow_disputed_total[5m]) / 
      rate(paywall_escrow_created_total[5m]) > 0.05
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Elevated dispute rate"
      description: "{{ $value | humanizePercentage }} of escrows disputed"
```

## Performance Monitoring

### Key Performance Indicators

1. **Throughput**
   - Payments created per second
   - Escrows completed per second
   - Signatures verified per second

2. **Latency**
   - Address generation time
   - Signature verification time
   - State transition duration

3. **Success Rates**
   - Payment confirmation rate
   - Escrow completion rate
   - Signature verification success rate

### Benchmarking

Run performance benchmarks to establish baselines:

```bash
go test -bench=. -benchmem -benchtime=10s
```

Expected performance baselines (reference hardware: AMD Ryzen 7):
- Address generation (single-sig): ~240 µs/op
- Address generation (multisig): ~74 µs/op
- Signature verification: ~13 µs/op
- Payment verification (single-sig): ~555 ns/op
- Payment verification (multisig): ~1.5 µs/op
- State transition: ~132 ns/op

## Security Monitoring

### Security Events to Monitor

1. **Signature Verification Failures**
   - Track source IP addresses
   - Monitor for patterns (same payment ID, repeated failures)
   - Alert on anomalies

2. **Invalid State Transitions**
   - Log all attempts to perform invalid state changes
   - Track by payment ID and role
   - Investigate patterns of malicious behavior

3. **Dispute Patterns**
   - Monitor dispute initiation frequency by role
   - Track arbiter vote patterns
   - Alert on potential collusion

4. **Timeout Abuse**
   - Monitor escrows approaching timeout
   - Track patterns of intentional delays
   - Alert on systematic timeout exploitation

### Security Dashboard Queries

**Failed Signature Attempts by Payment**
```elasticsearch
GET /paywall-*/_search
{
  "query": {
    "term": { "event": "signature_verification_failed" }
  },
  "aggs": {
    "by_payment": {
      "terms": { "field": "payment_id.keyword", "size": 20 }
    }
  }
}
```

**Dispute Initiation by Role**
```elasticsearch
GET /paywall-*/_search
{
  "query": {
    "term": { "event": "dispute_initiated" }
  },
  "aggs": {
    "by_role": {
      "terms": { "field": "role.keyword" }
    }
  }
}
```

### Audit Trail

Maintain comprehensive audit trails for:
- All signature submissions
- All state transitions
- All arbiter votes
- All dispute resolutions
- All timeout automations

Example audit query:
```elasticsearch
GET /paywall-*/_search
{
  "query": {
    "bool": {
      "must": [
        { "term": { "payment_id": "pay_abc123" } }
      ]
    }
  },
  "sort": [
    { "timestamp": "asc" }
  ]
}
```

## Best Practices

1. **Set Appropriate Log Levels**
   - Production: `LogLevelInfo` or `LogLevelWarn`
   - Development: `LogLevelDebug`
   - Testing: `LogLevelError`

2. **Monitor Key Metrics**
   - Set up dashboards for payment flow, escrow lifecycle, and disputes
   - Configure alerts for critical failures and anomalies
   - Review metrics regularly for trends

3. **Centralize Logs**
   - Use JSON format for machine parsing
   - Ship logs to centralized logging system (ELK, Splunk, etc.)
   - Retain logs for compliance and audit requirements

4. **Regular Review**
   - Weekly review of dispute patterns
   - Monthly review of timeout trends
   - Quarterly review of security events

5. **Capacity Planning**
   - Monitor throughput trends
   - Alert on capacity thresholds
   - Plan scaling based on metrics

6. **Testing Alerts**
   - Regularly test alert configurations
   - Verify alert routing and escalation
   - Document alert response procedures

## Troubleshooting

### High Dispute Rate
1. Check logs for dispute reasons
2. Review arbiter vote patterns
3. Investigate specific payment IDs with repeated disputes
4. Validate escrow timeout configurations

### Signature Verification Failures
1. Verify participant public keys are correct
2. Check for signature format issues
3. Validate that signatures match the multisig configuration
4. Review logs for specific error messages

### Transaction Broadcast Failures
1. Check Bitcoin/Monero node connectivity
2. Verify node is synced
3. Check transaction fee configuration
4. Review node error logs

### Performance Degradation
1. Review metrics for throughput decline
2. Check for increased latency
3. Examine concurrent operation patterns
4. Run benchmarks to establish current performance

For additional assistance, see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
