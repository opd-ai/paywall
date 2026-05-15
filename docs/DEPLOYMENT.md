# Production Deployment Guide

This guide covers deploying the paywall in production environments.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Deployment Architecture](#deployment-architecture)
- [Systemd Service](#systemd-service)
- [Nginx Reverse Proxy](#nginx-reverse-proxy)
- [Log Rotation](#log-rotation)
- [Monitoring Setup](#monitoring-setup)
- [Security Hardening](#security-hardening)
- [Backup and Recovery](#backup-and-recovery)
- [Scaling Considerations](#scaling-considerations)

---

## Prerequisites

### Server Requirements

**Minimum**:
- 1 CPU core
- 1 GB RAM
- 10 GB disk space
- Ubuntu 20.04+ or equivalent Linux distribution
- Go 1.23.2+ (if building from source)

**Recommended**:
- 2+ CPU cores
- 2-4 GB RAM
- 50 GB SSD disk
- TLS certificate (Let's Encrypt)
- Domain name pointing to server

### Software Dependencies

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install Go
wget https://go.dev/dl/go1.23.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# Install Nginx (reverse proxy)
sudo apt install nginx -y

# Install certbot (TLS certificates)
sudo apt install certbot python3-certbot-nginx -y

# Install supervisor or systemd (process management)
# Systemd is included by default on Ubuntu
```

### Blockchain Infrastructure

**Bitcoin**:
- Option 1: Run full Bitcoin node (requires 500+ GB disk, 2+ GB RAM)
- Option 2: Use trusted RPC provider (Blockstream, BlockCypher)
- Option 3: Use SPV wallet (lighter but less secure)

```bash
# Option 1: Install Bitcoin Core
sudo add-apt-repository ppa:bitcoin/bitcoin
sudo apt update
sudo apt install bitcoind -y

# Configure Bitcoin Core
mkdir -p ~/.bitcoin
cat > ~/.bitcoin/bitcoin.conf <<EOF
server=1
rpcuser=bitcoinrpc
rpcpassword=$(openssl rand -hex 32)
testnet=0  # Set to 1 for testnet
txindex=1
EOF

# Start Bitcoin daemon
bitcoind -daemon
```

**Monero** (optional):
```bash
# Download and install Monero
wget https://downloads.getmonero.org/cli/linux64
tar -xjf linux64
sudo mv monero-x86_64-linux-gnu-v0.18.3.1/* /usr/local/bin/

# Create Monero wallet
monero-wallet-cli --generate-new-wallet ~/monero/mywallet

# Start wallet RPC
monero-wallet-rpc \
  --rpc-bind-port 18081 \
  --wallet-file ~/monero/mywallet \
  --password "$(openssl rand -hex 16)" \
  --daemon-address node.moneroworld.com:18089 \
  --rpc-login user:$(openssl rand -hex 16) \
  --detach
```

---

## Deployment Architecture

```
┌─────────────┐
│   Internet  │
└─────┬───────┘
      │ HTTPS (443)
      ▼
┌─────────────────────┐
│   Nginx (Reverse    │
│   Proxy + TLS)      │
└─────────┬───────────┘
          │ HTTP (8000)
          ▼
    ┌─────────────────┐
    │   Paywall App   │
    │   (systemd)     │
    └─────┬───────────┘
          │
    ┌─────┴────────┬──────────┐
    ▼              ▼          ▼
┌────────┐   ┌──────────┐  ┌──────────┐
│Bitcoin │   │ Monero   │  │ Storage  │
│  RPC   │   │   RPC    │  │ (Files)  │
└────────┘   └──────────┘  └──────────┘
```

**Components**:
1. **Nginx**: TLS termination, rate limiting, request forwarding
2. **Paywall App**: Main application (managed by systemd)
3. **Blockchain RPCs**: Payment verification backends
4. **Storage**: Encrypted payment and wallet data

---

## Systemd Service

Create a systemd service for process management and automatic restart.

### 1. Build the Application

```bash
# Clone repository
cd /opt
sudo git clone https://github.com/opd-ai/paywall.git
cd paywall

# Build binary
go build -o /opt/paywall/bin/paywall-server ./example/basic-server

# Create directory structure
sudo mkdir -p /opt/paywall/{bin,config,data,logs}
sudo chown -R paywall:paywall /opt/paywall
```

### 2. Create Application User

```bash
# Create dedicated user (no login)
sudo useradd -r -s /bin/false -d /opt/paywall paywall
sudo chown -R paywall:paywall /opt/paywall
```

### 3. Create Configuration File

```bash
# Create config
sudo tee /opt/paywall/config/config.json <<EOF
{
  "price_btc": 0.001,
  "price_xmr": 0.01,
  "testnet": false,
  "min_confirmations": 3,
  "payment_timeout_hours": 24,
  "data_dir": "/opt/paywall/data",
  "listen_addr": "127.0.0.1:8000",
  "xmr_rpc": "http://localhost:18081",
  "xmr_user": "user",
  "xmr_password": "password"
}
EOF

sudo chmod 600 /opt/paywall/config/config.json
sudo chown paywall:paywall /opt/paywall/config/config.json
```

### 4. Create Systemd Unit File

```bash
sudo tee /etc/systemd/system/paywall.service <<EOF
[Unit]
Description=Bitcoin/Monero Paywall Service
After=network.target bitcoind.service monero-wallet-rpc.service
Wants=bitcoind.service monero-wallet-rpc.service

[Service]
Type=simple
User=paywall
Group=paywall
WorkingDirectory=/opt/paywall
Environment="PAYWALL_CONFIG=/opt/paywall/config/config.json"
Environment="XMR_WALLET_USER=user"
Environment="XMR_WALLET_PASS=password"

ExecStart=/opt/paywall/bin/paywall-server -config \${PAYWALL_CONFIG}

# Restart policy
Restart=on-failure
RestartSec=10s

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/paywall/data /opt/paywall/logs

# Limits
LimitNOFILE=65536
LimitNPROC=4096

# Logging
StandardOutput=append:/opt/paywall/logs/paywall.log
StandardError=append:/opt/paywall/logs/paywall-error.log

[Install]
WantedBy=multi-user.target
EOF
```

### 5. Enable and Start Service

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service (start on boot)
sudo systemctl enable paywall

# Start service
sudo systemctl start paywall

# Check status
sudo systemctl status paywall

# View logs
sudo journalctl -u paywall -f
```

### 6. Service Management Commands

```bash
# Start service
sudo systemctl start paywall

# Stop service
sudo systemctl stop paywall

# Restart service
sudo systemctl restart paywall

# Reload configuration (if supported)
sudo systemctl reload paywall

# View status
sudo systemctl status paywall

# View logs (last 100 lines)
sudo journalctl -u paywall -n 100

# Follow logs in real-time
sudo journalctl -u paywall -f

# View errors only
sudo journalctl -u paywall -p err
```

---

## Nginx Reverse Proxy

Configure Nginx for TLS termination, rate limiting, and request forwarding.

### 1. Install and Configure TLS Certificate

```bash
# Get Let's Encrypt certificate
sudo certbot --nginx -d yourdomain.com -d www.yourdomain.com

# Certificate will be automatically renewed
# Test renewal
sudo certbot renew --dry-run
```

### 2. Configure Nginx

```bash
sudo tee /etc/nginx/sites-available/paywall <<'EOF'
# Rate limiting zones
limit_req_zone $binary_remote_addr zone=paywall_general:10m rate=10r/s;
limit_req_zone $binary_remote_addr zone=paywall_payment:10m rate=2r/s;

# Upstream backend
upstream paywall_backend {
    server 127.0.0.1:8000 fail_timeout=10s max_fails=3;
    keepalive 32;
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    listen [::]:80;
    server_name yourdomain.com www.yourdomain.com;
    
    location / {
        return 301 https://$server_name$request_uri;
    }
}

# HTTPS server
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name yourdomain.com www.yourdomain.com;

    # TLS configuration
    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256';
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;
    ssl_stapling on;
    ssl_stapling_verify on;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "no-referrer-when-downgrade" always;

    # Logging
    access_log /var/log/nginx/paywall-access.log combined;
    error_log /var/log/nginx/paywall-error.log warn;

    # Root location (general rate limit)
    location / {
        limit_req zone=paywall_general burst=20 nodelay;
        
        proxy_pass http://paywall_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Payment endpoints (stricter rate limit)
    location ~ ^/(payment|create-payment|verify-payment) {
        limit_req zone=paywall_payment burst=5 nodelay;
        
        proxy_pass http://paywall_backend;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
    }

    # Health check endpoint (no rate limit)
    location /health {
        proxy_pass http://paywall_backend;
        access_log off;
    }

    # Static files (if serving via paywall)
    location /static/ {
        alias /opt/paywall/static/;
        expires 1h;
        add_header Cache-Control "public, immutable";
    }
}
EOF
```

### 3. Enable and Test Configuration

```bash
# Enable site
sudo ln -s /etc/nginx/sites-available/paywall /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload Nginx
sudo systemctl reload nginx

# Check status
sudo systemctl status nginx
```

---

## Log Rotation

Configure logrotate to prevent disk space exhaustion.

### 1. Configure Logrotate for Application Logs

```bash
sudo tee /etc/logrotate.d/paywall <<EOF
/opt/paywall/logs/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 paywall paywall
    sharedscripts
    postrotate
        systemctl reload paywall > /dev/null 2>&1 || true
    endscript
}
EOF
```

### 2. Configure Logrotate for Nginx Logs

```bash
sudo tee /etc/logrotate.d/nginx-paywall <<EOF
/var/log/nginx/paywall-*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 www-data adm
    sharedscripts
    postrotate
        systemctl reload nginx > /dev/null 2>&1 || true
    endscript
}
EOF
```

### 3. Test Log Rotation

```bash
# Test configuration
sudo logrotate -d /etc/logrotate.d/paywall

# Force rotation (for testing)
sudo logrotate -f /etc/logrotate.d/paywall
```

---

## Monitoring Setup

### 1. Application Health Check Endpoint

Add a health check endpoint to your application:

```go
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    // Check critical dependencies
    status := map[string]string{
        "status": "healthy",
        "timestamp": time.Now().Format(time.RFC3339),
    }
    
    // Check storage
    if _, err := paywall.Store.ListPendingPayments(); err != nil {
        status["status"] = "unhealthy"
        status["storage_error"] = err.Error()
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    
    json.NewEncoder(w).Encode(status)
})
```

### 2. Prometheus Metrics (Optional)

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    paymentsCreated = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "paywall_payments_created_total",
        Help: "Total number of payments created",
    })
    
    paymentsConfirmed = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "paywall_payments_confirmed_total",
        Help: "Total number of payments confirmed",
    })
)

func init() {
    prometheus.MustRegister(paymentsCreated, paymentsConfirmed)
}

// Expose metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

### 3. Simple Monitoring Script

```bash
#!/bin/bash
# /usr/local/bin/check-paywall.sh

HEALTH_URL="https://yourdomain.com/health"
ALERT_EMAIL="admin@yourdomain.com"

response=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_URL")

if [ "$response" != "200" ]; then
    echo "Paywall health check failed (HTTP $response)" | \
        mail -s "Paywall Service Alert" "$ALERT_EMAIL"
    exit 1
fi

exit 0
```

Add to crontab:
```bash
# Run health check every 5 minutes
*/5 * * * * /usr/local/bin/check-paywall.sh
```

### 4. Disk Space Monitoring

```bash
#!/bin/bash
# /usr/local/bin/check-disk-space.sh

THRESHOLD=80
PARTITION="/opt/paywall"
ALERT_EMAIL="admin@yourdomain.com"

usage=$(df -h "$PARTITION" | awk 'NR==2 {print $(NF-1)}' | sed 's/%//')

if [ "$usage" -gt "$THRESHOLD" ]; then
    echo "Disk usage on $PARTITION is ${usage}% (threshold: ${THRESHOLD}%)" | \
        mail -s "Disk Space Alert" "$ALERT_EMAIL"
fi
```

Add to crontab:
```bash
# Check disk space every hour
0 * * * * /usr/local/bin/check-disk-space.sh
```

---

## Security Hardening

### 1. Key Management

**Wallet Encryption Keys**:

```bash
# Generate strong encryption key
openssl rand -hex 32 > /opt/paywall/config/wallet-encryption.key
chmod 400 /opt/paywall/config/wallet-encryption.key
chown paywall:paywall /opt/paywall/config/wallet-encryption.key

# Use in application
export WALLET_ENCRYPTION_KEY=$(cat /opt/paywall/config/wallet-encryption.key)
```

**Mnemonic Phrase Storage**:

```bash
# Store mnemonic in secure location
echo "word1 word2 ... word24" > /root/.paywall-mnemonic
chmod 400 /root/.paywall-mnemonic
# Consider encrypted USB drive or hardware security module
```

**RPC Credentials**:

```bash
# Generate random credentials
XMR_USER="user_$(openssl rand -hex 8)"
XMR_PASS=$(openssl rand -hex 32)

# Store in environment file
echo "XMR_WALLET_USER=$XMR_USER" >> /opt/paywall/config/env
echo "XMR_WALLET_PASS=$XMR_PASS" >> /opt/paywall/config/env
chmod 400 /opt/paywall/config/env
chown paywall:paywall /opt/paywall/config/env

# Source in systemd service
EnvironmentFile=/opt/paywall/config/env
```

### 2. Network Isolation

**Firewall Rules (ufw)**:

```bash
# Default: deny all incoming
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Allow SSH (change from default 22 if possible)
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow RPC only from localhost (if running locally)
# No external access needed

# Enable firewall
sudo ufw enable
```

**iptables** (alternative to ufw):

```bash
# Flush existing rules
sudo iptables -F

# Default policies
sudo iptables -P INPUT DROP
sudo iptables -P FORWARD DROP
sudo iptables -P OUTPUT ACCEPT

# Allow loopback
sudo iptables -A INPUT -i lo -j ACCEPT

# Allow established connections
sudo iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Allow SSH
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT

# Allow HTTP/HTTPS
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Save rules
sudo iptables-save > /etc/iptables/rules.v4
```

### 3. File System Permissions

```bash
# Restrict data directory
sudo chmod 700 /opt/paywall/data
sudo chown -R paywall:paywall /opt/paywall/data

# Restrict config directory
sudo chmod 700 /opt/paywall/config
sudo chown -R paywall:paywall /opt/paywall/config

# Restrict logs (readable by admin group)
sudo chmod 750 /opt/paywall/logs
sudo chown paywall:adm /opt/paywall/logs
```

### 4. Fail2Ban Integration (DDoS Protection)

```bash
# Install fail2ban
sudo apt install fail2ban -y

# Create filter
sudo tee /etc/fail2ban/filter.d/paywall.conf <<EOF
[Definition]
failregex = ^<HOST>.*"POST /payment.*429 Too Many Requests
            ^<HOST>.*"POST /create-payment.*429 Too Many Requests
ignoreregex =
EOF

# Create jail
sudo tee /etc/fail2ban/jail.d/paywall.conf <<EOF
[paywall]
enabled = true
port = http,https
logpath = /var/log/nginx/paywall-access.log
filter = paywall
maxretry = 10
bantime = 3600
findtime = 300
EOF

# Restart fail2ban
sudo systemctl restart fail2ban
```

---

## Backup and Recovery

### 1. Backup Strategy

**Daily Backups**:

```bash
#!/bin/bash
# /usr/local/bin/backup-paywall.sh

BACKUP_DIR="/backup/paywall"
DATE=$(date +%Y%m%d-%H%M%S)
RETENTION_DAYS=30

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Backup payment data
tar -czf "$BACKUP_DIR/payments-$DATE.tar.gz" /opt/paywall/data

# Backup wallet data (encrypted)
tar -czf "$BACKUP_DIR/wallets-$DATE.tar.gz" /opt/paywall/data/*.dat

# Backup configuration
tar -czf "$BACKUP_DIR/config-$DATE.tar.gz" /opt/paywall/config

# Remove old backups
find "$BACKUP_DIR" -name "*.tar.gz" -mtime +$RETENTION_DAYS -delete

echo "Backup completed: $DATE"
```

Schedule daily at 2 AM:
```bash
0 2 * * * /usr/local/bin/backup-paywall.sh >> /var/log/paywall-backup.log 2>&1
```

### 2. Off-Site Backup

```bash
# Sync to remote server via rsync
rsync -avz --delete \
    /backup/paywall/ \
    backup-user@backup-server:/backups/paywall/

# Or use cloud storage (AWS S3, Backblaze B2)
aws s3 sync /backup/paywall/ s3://my-bucket/paywall-backups/
```

### 3. Recovery Procedure

```bash
# Stop service
sudo systemctl stop paywall

# Restore from backup
cd /opt/paywall
sudo tar -xzf /backup/paywall/payments-YYYYMMDD.tar.gz
sudo tar -xzf /backup/paywall/wallets-YYYYMMDD.tar.gz
sudo tar -xzf /backup/paywall/config-YYYYMMDD.tar.gz

# Fix permissions
sudo chown -R paywall:paywall /opt/paywall/data
sudo chmod 700 /opt/paywall/data

# Start service
sudo systemctl start paywall

# Verify
sudo systemctl status paywall
```

---

## Scaling Considerations

### Vertical Scaling

**Increase Resources**:
- CPU: Add more cores for concurrent payment verification
- RAM: Increase to 4-8 GB for larger payment databases
- Disk: Use SSD for faster I/O

**Optimize Configuration**:

```go
// Increase worker pool size
config := paywall.Config{
    // ... existing config
}

// Use efficient payment store
store := paywall.NewFileStoreWithConfig(paywall.FileStoreConfig{
    DataDir: "/opt/paywall/data",
    // Enable caching if implemented
})
```

### Horizontal Scaling

**Multiple Instances** (requires shared storage):

```bash
# Instance 1
/opt/paywall/bin/paywall-server -listen :8001 -data /shared/nfs/paywall

# Instance 2
/opt/paywall/bin/paywall-server -listen :8002 -data /shared/nfs/paywall
```

**Load Balancer** (Nginx):

```nginx
upstream paywall_cluster {
    least_conn;
    server 127.0.0.1:8001 max_fails=3 fail_timeout=30s;
    server 127.0.0.1:8002 max_fails=3 fail_timeout=30s;
}

server {
    # ... TLS config ...
    
    location / {
        proxy_pass http://paywall_cluster;
    }
}
```

**Database Backend** (for multi-instance):

Replace FileStore with PostgreSQL or Redis for shared state:

```go
// Example: PostgreSQL-backed store
type PostgreSQLStore struct {
    db *sql.DB
}

func NewPostgreSQLStore(dsn string) (*PostgreSQLStore, error) {
    db, err := sql.Open("postgres", dsn)
    if err != nil {
        return nil, err
    }
    return &PostgreSQLStore{db: db}, nil
}

// Implement PaymentStore interface
func (s *PostgreSQLStore) CreatePayment(p *Payment) error {
    // INSERT INTO payments ...
}
```

---

## Production Checklist

- [ ] Server provisioned with minimum requirements
- [ ] Go 1.23.2+ installed
- [ ] Paywall application built and deployed
- [ ] Bitcoin/Monero RPC configured and tested
- [ ] Systemd service created and enabled
- [ ] Nginx reverse proxy configured with TLS
- [ ] Log rotation configured
- [ ] Health check endpoint responding
- [ ] Monitoring alerts configured
- [ ] Firewall rules applied
- [ ] Wallet encryption keys generated and secured
- [ ] Mnemonic phrases backed up offline
- [ ] Daily backup cron job configured
- [ ] Off-site backup configured
- [ ] Recovery procedure tested
- [ ] Performance tested under load
- [ ] Security audit completed

---

## Troubleshooting Production Issues

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues and solutions.

**Common production-specific issues**:

1. **Service won't start**: Check `journalctl -u paywall -n 50`
2. **High CPU**: Check payment verification frequency
3. **Out of memory**: Increase swap or RAM
4. **Disk full**: Check log rotation and backup retention
5. **Connection refused**: Check firewall rules and service status

---

## Support

For deployment assistance:
- GitHub Issues: https://github.com/opd-ai/paywall/issues
- Documentation: https://github.com/opd-ai/paywall/tree/main/docs
