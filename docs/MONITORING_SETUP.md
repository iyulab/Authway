# Authway Monitoring Setup Guide

## Overview
This guide covers setting up comprehensive monitoring for Authway using Prometheus, Grafana, and AlertManager.

## Quick Start

### 1. Start Monitoring Stack
```bash
# Start the monitoring stack
docker-compose -f docker-compose.monitoring.yml up -d

# Verify services are running
docker-compose -f docker-compose.monitoring.yml ps
```

### 2. Access Monitoring Services
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin123)
- **AlertManager**: http://localhost:9093

### 3. Configure Grafana Dashboards
1. Login to Grafana (admin/admin123)
2. Add Prometheus data source: http://prometheus:9090
3. Import recommended dashboards (see below)

## Monitoring Components

### Prometheus (Metrics Collection)
- **Purpose**: Collects metrics from Authway and system components
- **Port**: 9090
- **Config**: `configs/prometheus.yml`
- **Data Retention**: 200 hours (configurable)

### Grafana (Visualization)
- **Purpose**: Visualizes metrics through dashboards
- **Port**: 3000
- **Default Login**: admin/admin123
- **Data Source**: Prometheus

### AlertManager (Alerting)
- **Purpose**: Handles alerts from Prometheus
- **Port**: 9093
- **Config**: `configs/alertmanager.yml`
- **Notifications**: Email, Slack

### Node Exporter (System Metrics)
- **Purpose**: Collects system-level metrics
- **Port**: 9100
- **Metrics**: CPU, Memory, Disk, Network

## Key Metrics to Monitor

### Application Metrics
```prometheus
# HTTP Request metrics
http_requests_total
http_request_duration_seconds

# Authentication metrics
failed_login_attempts_total
jwt_validation_failures_total
oauth_authorization_failures_total

# Database metrics
database_connections_active
database_connections_max
database_query_duration_seconds

# Redis metrics (if applicable)
redis_connected_clients
redis_commands_processed_total
```

### System Metrics
```prometheus
# CPU Usage
100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)

# Memory Usage
(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100

# Disk Usage
(1 - (node_filesystem_avail_bytes / node_filesystem_size_bytes)) * 100

# Network I/O
rate(node_network_receive_bytes_total[5m])
rate(node_network_transmit_bytes_total[5m])
```

## Alert Rules

### Critical Alerts
- **Service Down**: Authway service is unavailable
- **High Error Rate**: >10% of requests failing
- **Database Connectivity**: Connection pool exhausted

### Warning Alerts
- **High Response Time**: 95th percentile >2 seconds
- **High CPU Usage**: >80% for 5 minutes
- **High Memory Usage**: >80% for 5 minutes
- **Low Disk Space**: >85% used
- **Failed Login Attempts**: >10 per second

### Security Alerts
- **JWT Validation Failures**: >5 per second
- **OAuth Authorization Failures**: >5 per second
- **Suspicious Activity**: Multiple failed logins from same IP

## Grafana Dashboard Configuration

### Recommended Dashboards

#### 1. Authway Application Dashboard
```json
{
  "dashboard": {
    "title": "Authway Application Metrics",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(http_requests_total[5m])",
            "legendFormat": "{{method}} {{status}}"
          }
        ]
      },
      {
        "title": "Response Time",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          }
        ]
      },
      {
        "title": "Error Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(http_requests_total{status=~\"5..\"}[5m]) / rate(http_requests_total[5m])",
            "legendFormat": "Error Rate"
          }
        ]
      }
    ]
  }
}
```

#### 2. System Resource Dashboard
```json
{
  "dashboard": {
    "title": "System Resources",
    "panels": [
      {
        "title": "CPU Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "100 - (avg by(instance) (rate(node_cpu_seconds_total{mode=\"idle\"}[5m])) * 100)",
            "legendFormat": "CPU Usage %"
          }
        ]
      },
      {
        "title": "Memory Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100",
            "legendFormat": "Memory Usage %"
          }
        ]
      }
    ]
  }
}
```

#### 3. Security Dashboard
```json
{
  "dashboard": {
    "title": "Security Metrics",
    "panels": [
      {
        "title": "Failed Login Attempts",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(failed_login_attempts_total[5m])",
            "legendFormat": "Failed Logins/sec"
          }
        ]
      },
      {
        "title": "JWT Validation Failures",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(jwt_validation_failures_total[5m])",
            "legendFormat": "JWT Failures/sec"
          }
        ]
      }
    ]
  }
}
```

## Alert Configuration

### Email Alerts
```yaml
# configs/alertmanager.yml
receivers:
  - name: 'email-alerts'
    email_configs:
      - to: 'admin@yourdomain.com'
        from: 'alerts@yourdomain.com'
        smarthost: 'smtp.yourdomain.com:587'
        auth_username: 'alerts@yourdomain.com'
        auth_password: 'your-smtp-password'
        subject: 'Authway Alert: {{ .GroupLabels.alertname }}'
        body: |
          Alert: {{ range .Alerts }}{{ .Annotations.summary }}{{ end }}
          Description: {{ range .Alerts }}{{ .Annotations.description }}{{ end }}
          Time: {{ .StartsAt }}
```

### Slack Alerts
```yaml
# configs/alertmanager.yml
receivers:
  - name: 'slack-alerts'
    slack_configs:
      - api_url: 'https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK'
        channel: '#authway-alerts'
        title: 'Authway Alert'
        text: |
          *Alert:* {{ range .Alerts }}{{ .Annotations.summary }}{{ end }}
          *Description:* {{ range .Alerts }}{{ .Annotations.description }}{{ end }}
          *Severity:* {{ range .Alerts }}{{ .Labels.severity }}{{ end }}
```

## Health Checks

### Application Health Check
```bash
# Simple health check
curl http://localhost:8080/health

# Expected response
{
  "status": "healthy",
  "timestamp": "2024-01-01T00:00:00Z",
  "version": "1.0.0",
  "checks": {
    "database": "healthy",
    "redis": "healthy"
  }
}
```

### Monitoring Stack Health
```bash
# Check Prometheus targets
curl http://localhost:9090/api/v1/targets

# Check AlertManager status
curl http://localhost:9093/api/v1/status

# Check Grafana health
curl http://localhost:3000/api/health
```

## Production Considerations

### Security
- Change default Grafana credentials
- Use HTTPS for all monitoring services
- Restrict access to monitoring ports
- Enable authentication for Prometheus
- Use secure communication between services

### Performance
- Configure appropriate data retention policies
- Set up proper resource limits for containers
- Monitor monitoring system resource usage
- Implement log rotation for monitoring logs

### Backup
- Backup Grafana dashboards and configuration
- Backup Prometheus data (if long-term retention needed)
- Document custom alert rules and configurations

## Troubleshooting

### Common Issues

#### Prometheus Can't Scrape Targets
```bash
# Check if Authway metrics endpoint is accessible
curl http://localhost:8080/metrics

# Check Prometheus configuration
docker exec authway-prometheus promtool check config /etc/prometheus/prometheus.yml

# Check Prometheus logs
docker logs authway-prometheus
```

#### Grafana Dashboard Not Showing Data
```bash
# Verify Prometheus data source configuration
# Check if metrics are available in Prometheus UI
# Verify query syntax in Grafana panel
# Check time range settings
```

#### Alerts Not Firing
```bash
# Check AlertManager configuration
docker exec authway-alertmanager amtool check-config /etc/alertmanager/alertmanager.yml

# Check alert rules syntax
docker exec authway-prometheus promtool check rules /etc/prometheus/alerting_rules.yml

# Check AlertManager logs
docker logs authway-alertmanager
```

### Performance Tuning

#### Prometheus Optimization
```yaml
# prometheus.yml optimizations
global:
  scrape_interval: 15s      # Adjust based on needs
  evaluation_interval: 15s  # Adjust based on needs

storage:
  retention.time: 200h      # Adjust retention period
  retention.size: 10GB      # Set maximum storage size
```

#### Grafana Optimization
```yaml
# grafana.ini optimizations
[database]
max_open_conns = 10
max_idle_conns = 2

[session]
session_life_time = 86400

[dashboards]
default_home_dashboard_path = /var/lib/grafana/dashboards/home.json
```

## Monitoring Best Practices

### Metrics Collection
- Focus on business-critical metrics
- Avoid collecting too many high-cardinality metrics
- Use appropriate metric types (counter, gauge, histogram)
- Include relevant labels for filtering and grouping

### Alerting Strategy
- Set up alerts for symptoms, not just causes
- Avoid alert fatigue with proper thresholds
- Use alert grouping and inhibition rules
- Test alerts regularly

### Dashboard Design
- Keep dashboards focused and readable
- Use consistent color schemes and units
- Include context and drill-down capabilities
- Document dashboard purpose and usage

---

**Note**: Adjust configurations based on your specific environment and requirements. Regular review and updates of monitoring setup are recommended.