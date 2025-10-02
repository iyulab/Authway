# Authway Deployment Guide

Complete guide for deploying Authway authentication system from development to production.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Prerequisites](#prerequisites)
3. [Environment Configuration](#environment-configuration)
4. [Development Setup](#development-setup)
5. [Production Deployment](#production-deployment)
6. [Monitoring Setup](#monitoring-setup)
7. [Security Configuration](#security-configuration)
8. [Troubleshooting](#troubleshooting)

## Quick Start

### Local Development (5 minutes)

```bash
# Clone repository
git clone https://github.com/your-org/authway.git
cd authway

# Start development environment
docker-compose up -d postgres redis hydra
cd packages/web/login-ui && npm run dev &
cd packages/web/admin-dashboard && npm run dev &
go run src/server/cmd/main.go

# Access applications
# Login UI: http://localhost:3000
# Admin Dashboard: http://localhost:3001
# Backend API: http://localhost:8080
```

### Production Deployment (10 minutes)

```bash
# Configure environment
cp .env.example .env
# Edit .env with production values

# Generate security keys
./scripts/generate-keys.sh

# Deploy to Kubernetes
kubectl apply -f k8s/production/

# Verify deployment
kubectl get pods -n authway-production
```

## Prerequisites

### Development Requirements

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.21+ | Backend development |
| Node.js | 18+ | Frontend development |
| Docker | 20+ | Container runtime |
| Docker Compose | 2.0+ | Local orchestration |
| PostgreSQL | 15+ | Database |
| Redis | 7+ | Cache/sessions |

### Production Requirements

| Tool | Version | Purpose |
|------|---------|---------|
| Kubernetes | 1.28+ | Container orchestration |
| kubectl | 1.28+ | Kubernetes CLI |
| Helm | 3.10+ | Package manager (optional) |
| cert-manager | 1.13+ | TLS certificates |
| nginx-ingress | 1.8+ | Load balancer |

### Infrastructure Requirements

#### Minimum Resources (Development)
- **CPU**: 4 cores
- **Memory**: 8GB RAM
- **Storage**: 50GB SSD
- **Network**: 100Mbps

#### Production Resources (3-node cluster)
- **CPU**: 16 cores per node
- **Memory**: 32GB RAM per node
- **Storage**: 500GB SSD per node
- **Network**: 1Gbps
- **Load Balancer**: External
- **DNS**: Managed DNS service

## Environment Configuration

### Required Environment Variables

```bash
# Database Configuration
DATABASE_HOST=postgres
DATABASE_PORT=5432
DATABASE_NAME=authway
DATABASE_USER=authway
DATABASE_PASSWORD=<secure-random-password>
DATABASE_SSL_MODE=require

# Redis Configuration
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=<secure-random-password>
REDIS_DB=0

# Hydra Configuration
HYDRA_ADMIN_URL=http://hydra:4445
HYDRA_PUBLIC_URL=http://hydra:4444
HYDRA_CLIENT_ID=authway-backend
HYDRA_CLIENT_SECRET=<secure-random-secret>

# JWT Configuration
JWT_PRIVATE_KEY_PATH=/app/keys/jwt_private.pem
JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem
JWT_ISSUER=authway
JWT_AUDIENCE=authway-users
JWT_ACCESS_TOKEN_DURATION=15m
JWT_REFRESH_TOKEN_DURATION=720h

# OAuth Providers
GOOGLE_CLIENT_ID=<google-oauth-client-id>
GOOGLE_CLIENT_SECRET=<google-oauth-client-secret>
GOOGLE_REDIRECT_URL=https://your-domain.com/auth/google/callback

# Security Configuration
BCRYPT_COST=12
RATE_LIMIT_REQUESTS_PER_MINUTE=60
RATE_LIMIT_BURST_SIZE=10

# CORS Configuration
CORS_ALLOWED_ORIGINS=https://your-domain.com,https://admin.your-domain.com
CORS_ALLOWED_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOWED_HEADERS=Content-Type,Authorization,X-Requested-With
CORS_ALLOW_CREDENTIALS=true

# Email Configuration (optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=noreply@your-domain.com
SMTP_PASSWORD=<app-specific-password>
SMTP_FROM_ADDRESS=noreply@your-domain.com
SMTP_FROM_NAME=Authway
```

### Secrets Management

#### Development (docker-compose)
```bash
# Create .env file
cat > .env << 'EOF'
DATABASE_PASSWORD=dev_password_123
REDIS_PASSWORD=dev_redis_123
HYDRA_CLIENT_SECRET=dev_hydra_secret_123
GOOGLE_CLIENT_ID=your-dev-google-id
GOOGLE_CLIENT_SECRET=your-dev-google-secret
EOF
```

#### Production (Kubernetes)
```bash
# Create database secret
kubectl create secret generic authway-secrets \
  --from-literal=database-username=authway \
  --from-literal=database-password=<secure-password> \
  --from-literal=redis-password=<secure-password> \
  --from-literal=hydra-client-secret=<secure-secret> \
  --namespace=authway-production

# Create OAuth secrets
kubectl create secret generic oauth-secrets \
  --from-literal=google-client-id=<google-client-id> \
  --from-literal=google-client-secret=<google-client-secret> \
  --namespace=authway-production

# Create JWT keys
kubectl create secret generic jwt-keys \
  --from-file=private-key=keys/jwt_private.pem \
  --from-file=public-key=keys/jwt_public.pem \
  --namespace=authway-production
```

## Development Setup

### 1. Clone and Setup

```bash
# Clone repository
git clone https://github.com/your-org/authway.git
cd authway

# Install backend dependencies
go mod download

# Install frontend dependencies
cd packages/web/login-ui
npm install

cd ../admin-dashboard
npm install

cd ../../..
```

### 2. Start Infrastructure

```bash
# Start PostgreSQL, Redis, and Hydra
docker-compose up -d postgres redis hydra

# Wait for services to be ready
sleep 30

# Run database migrations
go run src/server/cmd/main.go migrate

# Create initial admin user (optional)
go run src/server/cmd/main.go create-admin --email admin@example.com --password admin123
```

### 3. Start Applications

```bash
# Terminal 1: Backend API
export ENVIRONMENT=development
go run src/server/cmd/main.go

# Terminal 2: Login UI
cd packages/web/login-ui
npm run dev

# Terminal 3: Admin Dashboard
cd packages/web/admin-dashboard
npm run dev

# Terminal 4: Watch logs
docker-compose logs -f
```

### 4. Verify Development Setup

```bash
# Check health endpoints
curl http://localhost:8080/health
curl http://localhost:3000
curl http://localhost:3001

# Test authentication flow
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"admin123"}'
```

### Development Tools

```bash
# Run tests
npm run test:coverage     # Frontend tests
go test ./... -v -race    # Backend tests

# Code formatting
npm run format           # Frontend formatting
gofmt -s -w .           # Backend formatting

# Linting
npm run lint            # Frontend linting
golangci-lint run       # Backend linting

# Build for production
npm run build           # Frontend build
go build -o authway src/server/cmd/main.go  # Backend build
```

## Production Deployment

### 1. Pre-deployment Checklist

- [ ] Kubernetes cluster ready and accessible
- [ ] DNS records configured
- [ ] TLS certificates available
- [ ] Container registry access configured
- [ ] Environment secrets created
- [ ] Backup strategy implemented
- [ ] Monitoring stack ready

### 2. Container Registry Setup

```bash
# Build and push images
docker build -t ghcr.io/your-org/authway-backend:v1.0.0 .
docker build -t ghcr.io/your-org/authway-login-ui:v1.0.0 packages/web/login-ui/
docker build -t ghcr.io/your-org/authway-admin-dashboard:v1.0.0 packages/web/admin-dashboard/

docker push ghcr.io/your-org/authway-backend:v1.0.0
docker push ghcr.io/your-org/authway-login-ui:v1.0.0
docker push ghcr.io/your-org/authway-admin-dashboard:v1.0.0
```

### 3. Kubernetes Deployment

```bash
# Create namespace and configure RBAC
kubectl apply -f k8s/production/namespace.yaml

# Deploy infrastructure (PostgreSQL, Redis)
kubectl apply -f k8s/production/postgres-deployment.yaml
kubectl apply -f k8s/production/redis-deployment.yaml

# Wait for infrastructure
kubectl wait --for=condition=ready pod -l app=postgres -n authway-production --timeout=300s
kubectl wait --for=condition=ready pod -l app=redis -n authway-production --timeout=300s

# Deploy Hydra
kubectl apply -f k8s/production/hydra-deployment.yaml
kubectl wait --for=condition=ready pod -l app=hydra -n authway-production --timeout=300s

# Deploy application
kubectl apply -f k8s/production/backend-deployment.yaml
kubectl apply -f k8s/production/frontend-deployments.yaml

# Deploy ingress and networking
kubectl apply -f k8s/production/ingress.yaml
kubectl apply -f k8s/production/network-policies.yaml

# Verify deployment
kubectl get all -n authway-production
kubectl get ingress -n authway-production
```

### 4. Database Initialization

```bash
# Run migrations
kubectl exec -it deployment/authway-backend -n authway-production -- /app/authway migrate

# Create initial admin user
kubectl exec -it deployment/authway-backend -n authway-production -- \
  /app/authway create-admin --email admin@your-domain.com --password <secure-password>

# Verify database
kubectl exec -it postgres-0 -n authway-production -- \
  psql -U authway -d authway -c "SELECT count(*) FROM users;"
```

### 5. SSL/TLS Configuration

```bash
# Install cert-manager (if not already installed)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Create ClusterIssuer for Let's Encrypt
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@your-domain.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
EOF

# Update ingress with TLS
kubectl annotate ingress authway-ingress cert-manager.io/cluster-issuer=letsencrypt-prod -n authway-production
```

### 6. Post-deployment Verification

```bash
# Health checks
curl -f https://your-domain.com/health
curl -f https://your-domain.com/login
curl -f https://admin.your-domain.com

# SSL verification
curl -I https://your-domain.com
openssl s_client -connect your-domain.com:443 -servername your-domain.com

# Load testing (optional)
./scripts/performance-benchmark.sh

# Security scan
./scripts/security-audit.sh
```

## Monitoring Setup

### 1. Deploy Monitoring Stack

```bash
# Deploy Prometheus, Grafana, AlertManager
kubectl apply -f k8s/production/monitoring-stack.yaml

# Deploy Loki and Promtail for logging
kubectl apply -f k8s/production/logging-stack.yaml

# Wait for services
kubectl wait --for=condition=ready pod -l app=prometheus -n authway-production --timeout=300s
kubectl wait --for=condition=ready pod -l app=grafana -n authway-production --timeout=300s
```

### 2. Configure Grafana

```bash
# Get Grafana admin password
kubectl get secret grafana-secrets -n authway-production -o jsonpath='{.data.admin-password}' | base64 -d

# Access Grafana
kubectl port-forward svc/grafana-service 3000:3000 -n authway-production

# Import dashboards (manual step)
# - Go to http://localhost:3000
# - Login with admin credentials
# - Import dashboards from grafana-dashboards ConfigMap
```

### 3. Configure Alerts

```bash
# Verify AlertManager configuration
kubectl get configmap alertmanager-config -n authway-production -o yaml

# Test alert routing
kubectl exec -it deployment/alertmanager -n authway-production -- \
  amtool config routes test --config.file=/etc/alertmanager/alertmanager.yml

# Send test alert
kubectl exec -it deployment/prometheus -n authway-production -- \
  promtool query instant 'up{job="authway-backend"}'
```

## Security Configuration

### 1. Network Security

```bash
# Apply network policies
kubectl apply -f k8s/production/network-policies.yaml

# Verify policies
kubectl get networkpolicies -n authway-production
kubectl describe networkpolicy authway-backend-netpol -n authway-production
```

### 2. Pod Security

```bash
# Apply pod security policies
kubectl apply -f k8s/production/pod-security-policies.yaml

# Verify security contexts
kubectl get pods -n authway-production -o yaml | grep -A 10 securityContext
```

### 3. Secrets Rotation

```bash
# Rotate JWT keys (monthly)
./scripts/generate-keys.sh
kubectl create secret generic jwt-keys \
  --from-file=private-key=keys/jwt_private.pem \
  --from-file=public-key=keys/jwt_public.pem \
  --namespace=authway-production \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart backend to pick up new keys
kubectl rollout restart deployment/authway-backend -n authway-production
```

### 4. Security Scanning

```bash
# Scan container images
trivy image ghcr.io/your-org/authway-backend:latest

# Scan Kubernetes manifests
kubesec scan k8s/production/backend-deployment.yaml

# Run security audit
./scripts/security-audit.sh
```

## Troubleshooting

### Common Issues

#### 1. Pods Not Starting

```bash
# Check pod status
kubectl get pods -n authway-production

# Describe pod issues
kubectl describe pod <pod-name> -n authway-production

# Check logs
kubectl logs <pod-name> -n authway-production

# Common fixes
kubectl delete pod <pod-name> -n authway-production  # Restart pod
kubectl rollout restart deployment/<deployment> -n authway-production  # Restart deployment
```

#### 2. Database Connection Issues

```bash
# Check PostgreSQL status
kubectl exec -it postgres-0 -n authway-production -- pg_isready

# Test connection from backend
kubectl exec -it deployment/authway-backend -n authway-production -- \
  psql "postgresql://authway:password@postgres-service:5432/authway" -c "SELECT 1;"

# Check connection pool
kubectl logs -l app=authway-backend -n authway-production | grep -i "connection\|pool"
```

#### 3. TLS Certificate Issues

```bash
# Check certificate status
kubectl get certificate -n authway-production
kubectl describe certificate authway-tls -n authway-production

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager

# Manual certificate renewal
kubectl delete certificate authway-tls -n authway-production
kubectl apply -f k8s/production/ingress.yaml
```

#### 4. High Memory Usage

```bash
# Check resource usage
kubectl top pods -n authway-production --sort-by=memory

# Check memory limits
kubectl describe pod -l app=authway-backend -n authway-production | grep -A 10 Resources

# Increase memory limits
kubectl patch deployment authway-backend -n authway-production -p '
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "authway-backend",
            "resources": {
              "limits": {
                "memory": "1Gi"
              }
            }
          }
        ]
      }
    }
  }
}'
```

### Getting Help

1. **Check logs**: Always start with application and infrastructure logs
2. **Verify configuration**: Ensure environment variables and secrets are correct
3. **Test connectivity**: Verify network connectivity between components
4. **Check resources**: Monitor CPU, memory, and storage usage
5. **Review monitoring**: Check Grafana dashboards for anomalies

### Support Channels

- **Documentation**: https://docs.your-domain.com/authway
- **Issues**: https://github.com/your-org/authway/issues
- **Slack**: #authway-support
- **Email**: support@your-domain.com

---

## Deployment Checklist

### Pre-deployment
- [ ] Requirements validated
- [ ] Environment configured
- [ ] Secrets created
- [ ] Container images built and pushed
- [ ] Database backup taken

### Deployment
- [ ] Infrastructure deployed
- [ ] Applications deployed
- [ ] Database migrated
- [ ] Ingress configured
- [ ] TLS certificates issued

### Post-deployment
- [ ] Health checks passing
- [ ] Monitoring active
- [ ] Alerts configured
- [ ] Security scan completed
- [ ] Performance baseline established
- [ ] Documentation updated

### Go-live
- [ ] DNS updated
- [ ] Load balancer configured
- [ ] Monitoring validated
- [ ] Team notified
- [ ] Rollback plan ready