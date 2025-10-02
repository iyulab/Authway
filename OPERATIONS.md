# Authway Operations Guide

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Deployment](#deployment)
4. [Monitoring & Alerting](#monitoring--alerting)
5. [Security](#security)
6. [Troubleshooting](#troubleshooting)
7. [Maintenance](#maintenance)
8. [Emergency Procedures](#emergency-procedures)
9. [Performance Optimization](#performance-optimization)
10. [Backup & Recovery](#backup--recovery)

## Overview

Authway is a production-ready authentication system built with Go backend and React frontend components. This guide covers operational procedures for managing the system in production environments.

### System Components

| Component | Technology | Purpose |
|-----------|------------|---------|
| Backend API | Go 1.21 | Core authentication services |
| Login UI | React 18 | User authentication interface |
| Admin Dashboard | React 18 | Administrative interface |
| PostgreSQL | 15.x | Primary database |
| Redis | 7.x | Session cache and rate limiting |
| Ory Hydra | Latest | OAuth2/OpenID Connect provider |
| Prometheus | 2.47.x | Metrics collection |
| Grafana | 10.1.x | Monitoring dashboard |
| Loki | 2.9.x | Log aggregation |

## Architecture

### Production Architecture

```
[Load Balancer]
    ↓
[Nginx Reverse Proxy]
    ↓
[Kubernetes Cluster]
    ├── Authway Backend (3 replicas)
    ├── Login UI (2 replicas)
    ├── Admin Dashboard (2 replicas)
    ├── PostgreSQL (Primary/Replica)
    ├── Redis (Master/Replica)
    └── Monitoring Stack
```

### Network Flow

1. **User Authentication**: User → Load Balancer → Nginx → Login UI → Backend API → Hydra
2. **Admin Operations**: Admin → Load Balancer → Nginx → Admin Dashboard → Backend API
3. **Monitoring**: Prometheus ← Backend API (metrics) → Grafana (visualization)
4. **Logging**: All components → Promtail → Loki → Grafana

## Deployment

### Prerequisites

- Kubernetes cluster (v1.28+)
- kubectl configured
- Docker registry access
- Required secrets configured

### Environment Variables

#### Required Secrets
```bash
# Database
DATABASE_PASSWORD=<secure-password>

# Redis
REDIS_PASSWORD=<secure-password>

# OAuth
HYDRA_CLIENT_SECRET=<hydra-secret>
GOOGLE_CLIENT_ID=<google-oauth-id>
GOOGLE_CLIENT_SECRET=<google-oauth-secret>

# JWT Keys (generated via scripts/generate-keys.sh)
JWT_PRIVATE_KEY=<base64-encoded-private-key>
JWT_PUBLIC_KEY=<base64-encoded-public-key>
```

### Deployment Commands

```bash
# Generate JWT keys
./scripts/generate-keys.sh

# Deploy to production
kubectl apply -f k8s/production/

# Verify deployment
kubectl get pods -n authway-production
kubectl get services -n authway-production

# Check health
curl -f https://your-domain.com/health
```

### Rolling Updates

```bash
# Update backend image
kubectl set image deployment/authway-backend authway-backend=ghcr.io/your-org/authway-backend:v2.0.0 -n authway-production

# Wait for rollout
kubectl rollout status deployment/authway-backend -n authway-production

# Verify health
kubectl get pods -n authway-production
```

### Rollback Procedures

```bash
# Rollback to previous version
kubectl rollout undo deployment/authway-backend -n authway-production

# Rollback to specific revision
kubectl rollout undo deployment/authway-backend --to-revision=2 -n authway-production

# Check rollout history
kubectl rollout history deployment/authway-backend -n authway-production
```

## Monitoring & Alerting

### Key Metrics

#### Application Metrics
- **Request Rate**: `rate(http_requests_total[5m])`
- **Error Rate**: `rate(http_requests_total{code=~"5.."}[5m])`
- **Response Time**: `histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))`
- **Active Users**: `active_sessions_total`

#### Infrastructure Metrics
- **CPU Usage**: `rate(container_cpu_usage_seconds_total[5m])`
- **Memory Usage**: `container_memory_usage_bytes / container_spec_memory_limit_bytes`
- **Database Connections**: `postgres_stat_database_numbackends`
- **Cache Hit Rate**: `redis_keyspace_hits_total / (redis_keyspace_hits_total + redis_keyspace_misses_total)`

### Alert Thresholds

| Alert | Threshold | Severity |
|-------|-----------|----------|
| Service Down | up == 0 for 1m | Critical |
| High Error Rate | >5% for 5m | Critical |
| High Response Time | >2s p95 for 5m | Warning |
| Memory Usage | >80% for 5m | Warning |
| Database Connections | >20 for 5m | Warning |

### Grafana Dashboards

1. **Authway Overview**: System-wide metrics and health
2. **Application Performance**: Request metrics and response times
3. **Infrastructure**: CPU, memory, disk, network
4. **Database**: Query performance and connection metrics
5. **Alerts**: Active alerts and incidents

### Accessing Monitoring

- **Grafana**: `https://your-domain.com/grafana`
- **Prometheus**: `https://your-domain.com/prometheus` (internal)
- **Alertmanager**: `https://your-domain.com/alertmanager` (internal)

## Security

### Security Checklist

#### Application Security
- [ ] JWT keys rotated monthly
- [ ] Strong password policies enforced
- [ ] Rate limiting active
- [ ] HTTPS only (TLS 1.2+)
- [ ] CORS properly configured
- [ ] SQL injection protection
- [ ] XSS protection headers

#### Infrastructure Security
- [ ] Network policies applied
- [ ] Pod security policies enforced
- [ ] Secrets encrypted at rest
- [ ] Regular security scans
- [ ] Access controls reviewed
- [ ] Audit logs enabled

### Security Monitoring

```bash
# Run security audit
./scripts/security-audit.sh

# Check for vulnerabilities
kubectl exec -it deployment/authway-backend -n authway-production -- trivy fs /app

# Review audit logs
kubectl logs -l app=authway-backend -n authway-production | grep "audit"
```

### Key Rotation

```bash
# Generate new JWT keys
./scripts/generate-keys.sh

# Update Kubernetes secrets
kubectl create secret generic jwt-keys \
  --from-file=private-key=keys/jwt_private.pem \
  --from-file=public-key=keys/jwt_public.pem \
  --namespace=authway-production \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to pick up new keys
kubectl rollout restart deployment/authway-backend -n authway-production
```

## Troubleshooting

### Common Issues

#### High CPU Usage
```bash
# Check resource usage
kubectl top pods -n authway-production

# Check for CPU-intensive queries
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "SELECT query, calls, total_time, mean_time FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10;"

# Scale up if needed
kubectl scale deployment/authway-backend --replicas=5 -n authway-production
```

#### Database Connection Issues
```bash
# Check database status
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "SELECT * FROM pg_stat_activity;"

# Check connection pool settings
kubectl logs -l app=authway-backend -n authway-production | grep -i "connection"

# Restart backend if needed
kubectl rollout restart deployment/authway-backend -n authway-production
```

#### Memory Leaks
```bash
# Check memory usage trends
kubectl top pods -n authway-production --sort-by memory

# Check Go memory stats
curl -s http://localhost:9090/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Restart affected pods
kubectl delete pod -l app=authway-backend -n authway-production
```

#### Authentication Failures
```bash
# Check Hydra status
kubectl exec -it hydra-0 -n authway-production -- hydra introspect token <token>

# Check logs for auth errors
kubectl logs -l app=authway-backend -n authway-production | grep -i "auth\|error"

# Verify configuration
kubectl get configmap authway-config -n authway-production -o yaml
```

### Log Analysis

```bash
# View recent logs
kubectl logs -l app=authway-backend -n authway-production --tail=100

# Follow logs in real-time
kubectl logs -l app=authway-backend -n authway-production -f

# Search for errors
kubectl logs -l app=authway-backend -n authway-production | grep -i error

# Export logs for analysis
kubectl logs -l app=authway-backend -n authway-production --since=1h > authway-logs.txt
```

### Performance Analysis

```bash
# Run performance benchmark
./scripts/performance-benchmark.sh

# Check database performance
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "SELECT schemaname,tablename,attname,n_distinct,correlation FROM pg_stats WHERE tablename = 'users';"

# Analyze slow queries
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "SELECT query, mean_time, calls FROM pg_stat_statements WHERE mean_time > 100 ORDER BY mean_time DESC;"
```

## Maintenance

### Regular Maintenance Tasks

#### Daily
- [ ] Check system health dashboards
- [ ] Review alert notifications
- [ ] Verify backup completion
- [ ] Monitor resource utilization

#### Weekly
- [ ] Update dependency versions
- [ ] Review security scan results
- [ ] Analyze performance trends
- [ ] Clean up old logs/metrics

#### Monthly
- [ ] Rotate JWT keys
- [ ] Update OS/container images
- [ ] Conduct security audit
- [ ] Review and update documentation
- [ ] Performance capacity planning

### Maintenance Windows

```bash
# Schedule maintenance
kubectl annotate deployment/authway-backend maintenance.authway.com/scheduled="$(date -u +%Y-%m-%dT%H:%M:%SZ)" -n authway-production

# Drain nodes for updates
kubectl drain node-1 --ignore-daemonsets --delete-emptydir-data

# Update and uncordon
kubectl uncordon node-1

# Verify system health post-maintenance
./scripts/health-check.sh
```

### Database Maintenance

```bash
# Vacuum and analyze
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "VACUUM ANALYZE;"

# Check index usage
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "SELECT schemaname,tablename,indexname,idx_scan,idx_tup_read,idx_tup_fetch FROM pg_stat_user_indexes ORDER BY idx_scan ASC;"

# Update statistics
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "ANALYZE;"
```

## Emergency Procedures

### Incident Response

#### 1. Assessment (0-5 minutes)
- Check monitoring dashboards
- Verify alert notifications
- Assess system availability
- Determine impact scope

#### 2. Communication (5-10 minutes)
- Notify stakeholders
- Update status page
- Create incident channel
- Assign roles and responsibilities

#### 3. Mitigation (10-30 minutes)
- Implement immediate fixes
- Scale resources if needed
- Rollback if necessary
- Monitor system recovery

#### 4. Resolution (30+ minutes)
- Apply permanent fixes
- Verify full recovery
- Update monitoring/alerts
- Document lessons learned

### Emergency Contacts

| Role | Contact | Phone | Email |
|------|---------|--------|-------|
| On-Call Engineer | Primary | +1-XXX-XXX-XXXX | oncall@your-domain.com |
| Platform Lead | Secondary | +1-XXX-XXX-XXXX | platform@your-domain.com |
| Security Team | Security | +1-XXX-XXX-XXXX | security@your-domain.com |

### Service Recovery

```bash
# Emergency scale-up
kubectl scale deployment/authway-backend --replicas=10 -n authway-production

# Emergency rollback
kubectl rollout undo deployment/authway-backend -n authway-production

# Force restart all pods
kubectl delete pods -l app=authway-backend -n authway-production

# Emergency database failover
kubectl patch postgresql postgres-cluster -n authway-production --type merge -p '{"spec":{"postgresql":{"synchronous_mode": false}}}'
```

## Performance Optimization

### Optimization Strategies

#### Application Level
- Connection pool tuning
- Query optimization
- Caching strategies
- Async processing
- Resource limits

#### Database Level
- Index optimization
- Query plan analysis
- Connection pooling
- Read replicas
- Partitioning

#### Infrastructure Level
- Horizontal pod autoscaling
- Resource limits/requests
- Node affinity rules
- Network optimization
- Storage optimization

### Performance Monitoring

```bash
# Monitor resource usage
kubectl top pods -n authway-production --sort-by=cpu
kubectl top nodes --sort-by=memory

# Database performance
kubectl exec -it postgres-0 -n authway-production -- psql -U authway -d authway -c "SELECT * FROM pg_stat_user_tables WHERE relname = 'users';"

# Application metrics
curl -s http://prometheus-service:9090/api/v1/query?query=rate\(http_requests_total\[5m\]\)
```

## Backup & Recovery

### Backup Strategy

#### Database Backups
- **Full backup**: Daily at 2 AM UTC
- **Incremental backup**: Every 4 hours
- **Point-in-time recovery**: 7 days
- **Long-term retention**: 30 days

#### Configuration Backups
- Kubernetes manifests
- Environment variables
- JWT keys (encrypted)
- SSL certificates

### Backup Procedures

```bash
# Manual database backup
kubectl exec postgres-0 -n authway-production -- pg_dump -U authway authway > backup-$(date +%Y%m%d).sql

# Backup Kubernetes resources
kubectl get all,configmap,secret -n authway-production -o yaml > k8s-backup-$(date +%Y%m%d).yaml

# Verify backup integrity
kubectl exec postgres-0 -n authway-production -- pg_restore --list backup-$(date +%Y%m%d).sql
```

### Recovery Procedures

```bash
# Database recovery
kubectl exec -i postgres-0 -n authway-production -- psql -U authway -d authway < backup-20240101.sql

# Kubernetes resource recovery
kubectl apply -f k8s-backup-20240101.yaml

# Verify recovery
kubectl get pods -n authway-production
curl -f https://your-domain.com/health
```

### Disaster Recovery

#### RTO/RPO Targets
- **Recovery Time Objective (RTO)**: 30 minutes
- **Recovery Point Objective (RPO)**: 4 hours
- **Mean Time to Recovery (MTTR)**: 15 minutes

#### DR Checklist
- [ ] Backup systems verified
- [ ] DR site ready
- [ ] Network connectivity confirmed
- [ ] DNS updated
- [ ] SSL certificates valid
- [ ] Monitoring active
- [ ] Stakeholders notified

---

## Contact Information

**Operations Team**: ops@your-domain.com
**Security Team**: security@your-domain.com
**Development Team**: dev@your-domain.com

**Documentation**: https://docs.your-domain.com/authway
**Status Page**: https://status.your-domain.com
**Monitoring**: https://grafana.your-domain.com