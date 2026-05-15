# Docker Deployment Guide

This guide covers deploying the paywall using Docker and Docker Compose.

## Table of Contents

- [Quick Start](#quick-start)
- [Dockerfile](#dockerfile)
- [Docker Compose](#docker-compose)
- [Environment Variables](#environment-variables)
- [Volumes and Persistence](#volumes-and-persistence)
- [Networking](#networking)
- [Security Considerations](#security-considerations)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Production Best Practices](#production-best-practices)

---

## Quick Start

### Prerequisites

```bash
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Install Docker Compose
sudo apt install docker-compose -y

# Verify installation
docker --version
docker-compose --version
```

### Run with Docker Compose

```bash
# Clone repository
git clone https://github.com/opd-ai/paywall.git
cd paywall

# Create environment file
cp .env.example .env
# Edit .env with your configuration

# Start services
docker-compose up -d

# Check logs
docker-compose logs -f paywall

# Check status
docker-compose ps
```

---

## Dockerfile

### Production Dockerfile

Create `Dockerfile` in repository root:

```dockerfile
# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.23.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o /build/paywall-server \
    ./example/basic-server

# Runtime stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 paywall && \
    adduser -D -u 1000 -G paywall paywall

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/paywall-server /app/paywall-server

# Copy static files and templates (if needed)
COPY --from=builder /build/static /app/static
COPY --from=builder /build/templates /app/templates

# Create data directory
RUN mkdir -p /app/data && chown -R paywall:paywall /app

# Switch to non-root user
USER paywall

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8000/health || exit 1

# Set entrypoint
ENTRYPOINT ["/app/paywall-server"]
```

### Development Dockerfile

For development with hot-reload:

```dockerfile
FROM golang:1.23.2-alpine

RUN apk add --no-cache git make

# Install air for hot-reload
RUN go install github.com/cosmtrek/air@latest

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Expose port
EXPOSE 8000

CMD ["air", "-c", ".air.toml"]
```

### Build Image

```bash
# Production build
docker build -t paywall:latest .

# Development build
docker build -f Dockerfile.dev -t paywall:dev .

# Multi-architecture build (arm64, amd64)
docker buildx build --platform linux/amd64,linux/arm64 -t paywall:latest .
```

---

## Docker Compose

### Production Stack

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  # Paywall application
  paywall:
    image: paywall:latest
    container_name: paywall
    restart: unless-stopped
    ports:
      - "127.0.0.1:8000:8000"
    environment:
      - PAYWALL_PRICE_BTC=${PAYWALL_PRICE_BTC:-0.001}
      - PAYWALL_PRICE_XMR=${PAYWALL_PRICE_XMR:-0.01}
      - PAYWALL_TESTNET=${PAYWALL_TESTNET:-false}
      - PAYWALL_MIN_CONFIRMATIONS=${PAYWALL_MIN_CONFIRMATIONS:-3}
      - PAYWALL_PAYMENT_TIMEOUT=${PAYWALL_PAYMENT_TIMEOUT:-24h}
      - XMR_WALLET_USER=${XMR_WALLET_USER}
      - XMR_WALLET_PASS=${XMR_WALLET_PASS}
      - XMR_RPC_URL=http://monero-wallet-rpc:18081
    volumes:
      - paywall-data:/app/data:rw
      - paywall-logs:/app/logs:rw
    depends_on:
      - monero-wallet-rpc
    networks:
      - paywall-network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8000/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s

  # Monero wallet RPC (optional, for XMR support)
  monero-wallet-rpc:
    image: sethsimmons/simple-monero-wallet-rpc:latest
    container_name: monero-wallet-rpc
    restart: unless-stopped
    environment:
      - DAEMON_HOST=${MONERO_DAEMON_HOST:-node.moneroworld.com}
      - DAEMON_PORT=${MONERO_DAEMON_PORT:-18089}
      - RPC_BIND_PORT=18081
      - RPC_USER=${XMR_WALLET_USER}
      - RPC_PASS=${XMR_WALLET_PASS}
      - WALLET_NAME=${MONERO_WALLET_NAME:-mywallet}
      - WALLET_PASS=${MONERO_WALLET_PASS}
      - TESTNET=${PAYWALL_TESTNET:-false}
    volumes:
      - monero-data:/wallet:rw
    networks:
      - paywall-network
    expose:
      - "18081"

  # Nginx reverse proxy (TLS termination)
  nginx:
    image: nginx:alpine
    container_name: nginx-proxy
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
      - nginx-logs:/var/log/nginx:rw
    depends_on:
      - paywall
    networks:
      - paywall-network

volumes:
  paywall-data:
    driver: local
  paywall-logs:
    driver: local
  monero-data:
    driver: local
  nginx-logs:
    driver: local

networks:
  paywall-network:
    driver: bridge
```

### Development Stack

Create `docker-compose.dev.yml`:

```yaml
version: '3.8'

services:
  paywall-dev:
    build:
      context: .
      dockerfile: Dockerfile.dev
    container_name: paywall-dev
    ports:
      - "8000:8000"
    environment:
      - PAYWALL_TESTNET=true
      - PAYWALL_PRICE_BTC=0.00001
      - PAYWALL_MIN_CONFIRMATIONS=1
    volumes:
      - .:/app:rw
      - /app/tmp  # Exclude tmp directory
    networks:
      - paywall-dev-network

networks:
  paywall-dev-network:
    driver: bridge
```

---

## Environment Variables

### Required Variables

Create `.env` file:

```bash
# Application settings
PAYWALL_PRICE_BTC=0.001
PAYWALL_PRICE_XMR=0.01
PAYWALL_TESTNET=false
PAYWALL_MIN_CONFIRMATIONS=3
PAYWALL_PAYMENT_TIMEOUT=24h

# Monero RPC credentials
XMR_WALLET_USER=user
XMR_WALLET_PASS=securepassword
MONERO_WALLET_NAME=mywallet
MONERO_WALLET_PASS=walletpassword

# Monero daemon (public node or your own)
MONERO_DAEMON_HOST=node.moneroworld.com
MONERO_DAEMON_PORT=18089

# Nginx settings
DOMAIN=yourdomain.com
```

### Example `.env` File

```bash
# .env.example - Copy to .env and customize

# === Application Configuration ===
# Price in Bitcoin (mainnet: minimum 0.00001 BTC)
PAYWALL_PRICE_BTC=0.001

# Price in Monero (optional, omit for Bitcoin-only)
PAYWALL_PRICE_XMR=0.01

# Use testnet (true) or mainnet (false)
PAYWALL_TESTNET=false

# Blockchain confirmations required
PAYWALL_MIN_CONFIRMATIONS=3

# Payment expiration (e.g., 24h, 1h, 30m)
PAYWALL_PAYMENT_TIMEOUT=24h

# === Monero Configuration (optional) ===
# RPC authentication
XMR_WALLET_USER=user
XMR_WALLET_PASS=change_me_to_secure_password

# Wallet name and password
MONERO_WALLET_NAME=mywallet
MONERO_WALLET_PASS=change_me_to_wallet_password

# Monero daemon endpoint
# Public nodes: node.moneroworld.com:18089 (mainnet), stagenet.xmr-tw.org:38081 (testnet)
MONERO_DAEMON_HOST=node.moneroworld.com
MONERO_DAEMON_PORT=18089

# === Web Server Configuration ===
DOMAIN=yourdomain.com
EMAIL=admin@yourdomain.com

# === Security ===
# Generate with: openssl rand -hex 32
WALLET_ENCRYPTION_KEY=

# === Optional: Bitcoin RPC (if running own node) ===
# BITCOIN_RPC_HOST=localhost
# BITCOIN_RPC_PORT=8332
# BITCOIN_RPC_USER=bitcoinrpc
# BITCOIN_RPC_PASS=rpcpassword
```

---

## Volumes and Persistence

### Data Volumes

**Paywall Data**:
- Location: `/app/data`
- Contains: Payment database, wallet files (encrypted)
- Backup: Daily recommended

**Logs**:
- Location: `/app/logs`
- Contains: Application logs, error logs
- Rotation: Configure with Docker logging driver

**Monero Wallet**:
- Location: `/wallet`
- Contains: Monero wallet files
- Backup: Critical - contains wallet keys

### Backup Volumes

```bash
# Backup paywall data
docker run --rm \
    -v paywall-data:/data:ro \
    -v $(pwd)/backup:/backup \
    alpine tar czf /backup/paywall-data-$(date +%Y%m%d).tar.gz -C /data .

# Backup Monero wallet
docker run --rm \
    -v monero-data:/wallet:ro \
    -v $(pwd)/backup:/backup \
    alpine tar czf /backup/monero-wallet-$(date +%Y%m%d).tar.gz -C /wallet .
```

### Restore Volumes

```bash
# Stop services
docker-compose down

# Restore paywall data
docker run --rm \
    -v paywall-data:/data:rw \
    -v $(pwd)/backup:/backup \
    alpine sh -c "cd /data && tar xzf /backup/paywall-data-YYYYMMDD.tar.gz"

# Restore Monero wallet
docker run --rm \
    -v monero-data:/wallet:rw \
    -v $(pwd)/backup:/backup \
    alpine sh -c "cd /wallet && tar xzf /backup/monero-wallet-YYYYMMDD.tar.gz"

# Start services
docker-compose up -d
```

---

## Networking

### Internal Network

Services communicate via Docker's internal network:

```yaml
networks:
  paywall-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.28.0.0/16
```

### Port Mapping

**Production** (via reverse proxy):
```yaml
services:
  nginx:
    ports:
      - "80:80"
      - "443:443"
  
  paywall:
    ports:
      - "127.0.0.1:8000:8000"  # Only accessible from localhost
```

**Development** (direct access):
```yaml
services:
  paywall-dev:
    ports:
      - "8000:8000"  # Accessible from host
```

### Custom Network

```bash
# Create external network
docker network create paywall-external

# Use in compose
networks:
  default:
    external:
      name: paywall-external
```

---

## Security Considerations

### 1. Non-Root User

Always run as non-root user:

```dockerfile
RUN adduser -D -u 1000 paywall
USER paywall
```

### 2. Read-Only Root Filesystem

```yaml
services:
  paywall:
    read_only: true
    tmpfs:
      - /tmp:noexec,nosuid,size=100m
```

### 3. Resource Limits

```yaml
services:
  paywall:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 512M
```

### 4. Secrets Management

Use Docker secrets instead of environment variables:

```yaml
services:
  paywall:
    secrets:
      - xmr_wallet_pass
      - wallet_encryption_key

secrets:
  xmr_wallet_pass:
    file: ./secrets/xmr_wallet_pass.txt
  wallet_encryption_key:
    file: ./secrets/wallet_encryption_key.txt
```

Access in application:
```go
pass, _ := ioutil.ReadFile("/run/secrets/xmr_wallet_pass")
key, _ := ioutil.ReadFile("/run/secrets/wallet_encryption_key")
```

### 5. Security Scanning

```bash
# Scan image for vulnerabilities
docker scan paywall:latest

# Use Trivy
trivy image paywall:latest

# Use Snyk
snyk test --docker paywall:latest
```

---

## Kubernetes Deployment

### Deployment Manifest

Create `k8s/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: paywall
  namespace: paywall
spec:
  replicas: 2
  selector:
    matchLabels:
      app: paywall
  template:
    metadata:
      labels:
        app: paywall
    spec:
      containers:
      - name: paywall
        image: paywall:latest
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8000
          name: http
        env:
        - name: PAYWALL_PRICE_BTC
          valueFrom:
            configMapKeyRef:
              name: paywall-config
              key: price_btc
        - name: PAYWALL_TESTNET
          valueFrom:
            configMapKeyRef:
              name: paywall-config
              key: testnet
        - name: XMR_WALLET_USER
          valueFrom:
            secretKeyRef:
              name: paywall-secrets
              key: xmr_user
        - name: XMR_WALLET_PASS
          valueFrom:
            secretKeyRef:
              name: paywall-secrets
              key: xmr_pass
        volumeMounts:
        - name: data
          mountPath: /app/data
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 10
          periodSeconds: 5
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: paywall-data-pvc
```

### Service Manifest

Create `k8s/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: paywall
  namespace: paywall
spec:
  type: ClusterIP
  ports:
  - port: 8000
    targetPort: 8000
    protocol: TCP
    name: http
  selector:
    app: paywall
```

### Ingress Manifest

Create `k8s/ingress.yaml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: paywall
  namespace: paywall
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/rate-limit: "10"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - yourdomain.com
    secretName: paywall-tls
  rules:
  - host: yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: paywall
            port:
              number: 8000
```

### ConfigMap

Create `k8s/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: paywall-config
  namespace: paywall
data:
  price_btc: "0.001"
  price_xmr: "0.01"
  testnet: "false"
  min_confirmations: "3"
  payment_timeout: "24h"
```

### Secrets

Create `k8s/secrets.yaml`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: paywall-secrets
  namespace: paywall
type: Opaque
stringData:
  xmr_user: "user"
  xmr_pass: "securepassword"
  wallet_encryption_key: "0123456789abcdef0123456789abcdef"
```

### Persistent Volume Claim

Create `k8s/pvc.yaml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: paywall-data-pvc
  namespace: paywall
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard
```

### Deploy to Kubernetes

```bash
# Create namespace
kubectl create namespace paywall

# Apply manifests
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secrets.yaml
kubectl apply -f k8s/pvc.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/ingress.yaml

# Check status
kubectl get pods -n paywall
kubectl get svc -n paywall
kubectl get ingress -n paywall

# View logs
kubectl logs -f deployment/paywall -n paywall

# Scale deployment
kubectl scale deployment/paywall --replicas=3 -n paywall
```

---

## Production Best Practices

### 1. Multi-Stage Builds

Use multi-stage Dockerfile to reduce image size:

```dockerfile
# Build stage: ~500 MB
FROM golang:1.23.2 AS builder
# ... build steps ...

# Runtime stage: ~15 MB
FROM alpine:3.18
COPY --from=builder /build/paywall-server /app/
```

### 2. Health Checks

Always include health checks:

```yaml
healthcheck:
  test: ["CMD", "wget", "--quiet", "--spider", "http://localhost:8000/health"]
  interval: 30s
  timeout: 10s
  retries: 3
```

### 3. Logging

Use Docker logging drivers:

```yaml
services:
  paywall:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

Or centralized logging (ELK, Loki):

```yaml
logging:
  driver: "fluentd"
  options:
    fluentd-address: "fluentd:24224"
    tag: "paywall"
```

### 4. Monitoring

Add Prometheus exporter:

```yaml
services:
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
```

### 5. Auto-Restart Policies

```yaml
services:
  paywall:
    restart: unless-stopped  # or "always"
```

### 6. Use .dockerignore

Create `.dockerignore`:

```
# Version control
.git
.gitignore

# Dependencies
vendor/

# Build artifacts
*.test
*.out
coverage.out

# Documentation
docs/
*.md

# Development files
.env
.env.local
docker-compose.dev.yml

# IDE
.vscode/
.idea/

# Temporary files
tmp/
*.log
```

---

## Docker Commands Reference

```bash
# Build
docker build -t paywall:latest .

# Run
docker run -d --name paywall -p 8000:8000 paywall:latest

# Stop
docker stop paywall

# Remove
docker rm paywall

# Logs
docker logs -f paywall

# Shell access
docker exec -it paywall sh

# Stats
docker stats paywall

# Inspect
docker inspect paywall

# Compose up
docker-compose up -d

# Compose down
docker-compose down

# Compose logs
docker-compose logs -f

# Compose rebuild
docker-compose up -d --build

# Compose scale
docker-compose up -d --scale paywall=3
```

---

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs paywall

# Check health
docker inspect paywall | grep -A 10 Health
```

### Permission Denied

```bash
# Fix volume permissions
docker run --rm -v paywall-data:/data alpine chown -R 1000:1000 /data
```

### Network Issues

```bash
# Check networks
docker network ls
docker network inspect paywall-network

# Test connectivity
docker exec paywall ping monero-wallet-rpc
```

---

## Support

For Docker deployment assistance:
- GitHub Issues: https://github.com/opd-ai/paywall/issues
- Docker Hub: https://hub.docker.com/r/opdai/paywall
