#!/bin/bash

# Performance Benchmark Script for Authway
# Tests API performance and generates performance reports

set -e

REPORT_DIR="./performance-reports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
REPORT_FILE="$REPORT_DIR/performance_benchmark_$TIMESTAMP.md"
BASE_URL="http://localhost:8080"

echo "⚡ Starting performance benchmark for Authway..."

# Check if required tools are installed
command -v curl >/dev/null 2>&1 || { echo "❌ curl is required but not installed."; exit 1; }
command -v ab >/dev/null 2>&1 && HAS_AB=true || HAS_AB=false

# Create reports directory
mkdir -p "$REPORT_DIR"

# Start report
cat > "$REPORT_FILE" << EOF
# Authway Performance Benchmark Report

**Date:** $(date)
**Test Environment:** Development
**Base URL:** $BASE_URL

## Test Summary

EOF

echo "📄 Performance report started: $REPORT_FILE"

# Function to add test results to report
add_test_result() {
    local test_name="$1"
    local endpoint="$2"
    local result="$3"

    cat >> "$REPORT_FILE" << EOF
### $test_name

**Endpoint:** \`$endpoint\`

\`\`\`
$result
\`\`\`

EOF
}

echo "🏥 Testing health endpoint..."

# Test health endpoint response time
HEALTH_TEST=$(curl -w "@-" -o /dev/null -s "$BASE_URL/health" << 'EOF'
     time_namelookup:  %{time_namelookup}s\n
      time_connect:    %{time_connect}s\n
   time_appconnect:    %{time_appconnect}s\n
  time_pretransfer:    %{time_pretransfer}s\n
     time_redirect:    %{time_redirect}s\n
time_starttransfer:    %{time_starttransfer}s\n
                   ----------\n
       time_total:     %{time_total}s\n
         http_code:    %{http_code}\n
EOF
)

add_test_result "Health Endpoint Response Time" "GET /health" "$HEALTH_TEST"

echo "🔐 Testing authentication endpoints..."

# Test login endpoint (should return method not allowed or error)
LOGIN_TEST=$(curl -w "@-" -o /dev/null -s "$BASE_URL/login" << 'EOF'
     time_namelookup:  %{time_namelookup}s\n
      time_connect:    %{time_connect}s\n
   time_appconnect:    %{time_appconnect}s\n
  time_pretransfer:    %{time_pretransfer}s\n
     time_redirect:    %{time_redirect}s\n
time_starttransfer:    %{time_starttransfer}s\n
                   ----------\n
       time_total:     %{time_total}s\n
         http_code:    %{http_code}\n
EOF
)

add_test_result "Login Endpoint Response Time" "GET /login" "$LOGIN_TEST"

# Test metrics endpoint if available
METRICS_TEST=$(curl -w "@-" -o /dev/null -s "$BASE_URL/metrics" 2>/dev/null << 'EOF' || echo "Metrics endpoint not available"
     time_namelookup:  %{time_namelookup}s\n
      time_connect:    %{time_connect}s\n
   time_appconnect:    %{time_appconnect}s\n
  time_pretransfer:    %{time_pretransfer}s\n
     time_redirect:    %{time_redirect}s\n
time_starttransfer:    %{time_starttransfer}s\n
                   ----------\n
       time_total:     %{time_total}s\n
         http_code:    %{http_code}\n
EOF
)

add_test_result "Metrics Endpoint Response Time" "GET /metrics" "$METRICS_TEST"

# Load testing with Apache Bench if available
if [ "$HAS_AB" = true ]; then
    echo "🚀 Running load tests with Apache Bench..."

    # Light load test on health endpoint
    AB_HEALTH_TEST=$(ab -n 100 -c 10 -q "$BASE_URL/health" 2>/dev/null | grep -E "(Requests per second|Time per request|Connection Times)" || echo "Load test failed")
    add_test_result "Health Endpoint Load Test (100 requests, 10 concurrent)" "GET /health" "$AB_HEALTH_TEST"

    # Medium load test
    AB_MEDIUM_TEST=$(ab -n 500 -c 25 -q "$BASE_URL/health" 2>/dev/null | grep -E "(Requests per second|Time per request|Connection Times)" || echo "Medium load test failed")
    add_test_result "Health Endpoint Load Test (500 requests, 25 concurrent)" "GET /health" "$AB_MEDIUM_TEST"

else
    echo "ℹ️  Apache Bench not available, skipping load tests"
    cat >> "$REPORT_FILE" << EOF
### Load Testing
Apache Bench (ab) not available. Install with:
- Ubuntu/Debian: \`apt-get install apache2-utils\`
- CentOS/RHEL: \`yum install httpd-tools\`
- macOS: \`brew install httpd\`

EOF
fi

# Memory and CPU usage (if tools available)
echo "📊 Checking system resources..."

SYSTEM_INFO=""
if command -v free >/dev/null 2>&1; then
    SYSTEM_INFO+="\n**Memory Usage:**\n\`\`\`\n$(free -h)\n\`\`\`\n"
fi

if command -v ps >/dev/null 2>&1; then
    AUTHWAY_PROCESSES=$(ps aux | grep -E "(authway|go run)" | grep -v grep || echo "No Authway processes found")
    SYSTEM_INFO+="\n**Authway Processes:**\n\`\`\`\n$AUTHWAY_PROCESSES\n\`\`\`\n"
fi

if [ -n "$SYSTEM_INFO" ]; then
    cat >> "$REPORT_FILE" << EOF
## System Resources
$SYSTEM_INFO

EOF
fi

# Add performance recommendations
cat >> "$REPORT_FILE" << 'EOF'
## Performance Analysis

### Response Time Targets
- **Health checks**: < 50ms
- **Authentication**: < 200ms
- **User operations**: < 500ms
- **Admin operations**: < 1000ms

### Recommended Optimizations

#### Database Optimizations
1. **Connection Pooling**: Optimize pool size (25 max, 5 idle)
2. **Indexing**: Ensure all queries use proper indexes
3. **Query Optimization**: Analyze slow queries with EXPLAIN
4. **Read Replicas**: Consider read replicas for high traffic

#### Application Optimizations
1. **Caching**: Implement Redis caching for frequent operations
2. **Compression**: Enable Gzip compression for responses
3. **Static Assets**: Use CDN for static files
4. **Resource Limits**: Set appropriate memory/CPU limits

#### Infrastructure Optimizations
1. **Load Balancing**: Implement load balancer for high availability
2. **Auto Scaling**: Configure horizontal pod autoscaling
3. **Resource Monitoring**: Set up detailed performance monitoring
4. **CDN**: Use content delivery network for global distribution

### Performance Metrics to Monitor
- Response time percentiles (50th, 95th, 99th)
- Requests per second (RPS)
- Error rate percentage
- Database query performance
- Memory and CPU utilization
- Disk I/O and network bandwidth

### Next Steps
1. Set up continuous performance monitoring
2. Implement automated performance testing in CI/CD
3. Establish performance baselines and alerts
4. Regular performance optimization reviews

EOF

echo "✅ Performance benchmark completed!"
echo "📄 Report saved to: $REPORT_FILE"

# Quick summary
HEALTH_TIME=$(echo "$HEALTH_TEST" | grep "time_total" | awk '{print $2}' | sed 's/s//')
if [ -n "$HEALTH_TIME" ]; then
    HEALTH_MS=$(echo "$HEALTH_TIME * 1000" | bc 2>/dev/null || echo "N/A")
    echo "📊 Health endpoint response time: ${HEALTH_MS}ms"
fi

echo ""
echo "💡 Next steps:"
echo "   1. Review performance report for bottlenecks"
echo "   2. Set up continuous monitoring"
echo "   3. Implement recommended optimizations"
echo "   4. Establish performance baselines"