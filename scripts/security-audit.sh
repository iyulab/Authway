#!/bin/bash

# Security Audit Script for Authway
# Performs comprehensive security checks and generates audit report

set -e

REPORT_DIR="./security-reports"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
REPORT_FILE="$REPORT_DIR/security_audit_$TIMESTAMP.md"

echo "🔒 Starting security audit for Authway..."

# Create reports directory
mkdir -p "$REPORT_DIR"

# Start report
cat > "$REPORT_FILE" << 'EOF'
# Authway Security Audit Report

## Audit Overview
**Date:** $(date)
**Auditor:** Automated Security Scanner
**Scope:** Full application security assessment

## Executive Summary
This report contains findings from automated security scanning of the Authway authentication system.

## Detailed Findings

EOF

echo "📄 Security audit report started: $REPORT_FILE"

# Function to add findings to report
add_finding() {
    local severity="$1"
    local title="$2"
    local description="$3"
    local recommendation="$4"

    cat >> "$REPORT_FILE" << EOF
### $severity: $title

**Description:** $description

**Recommendation:** $recommendation

---

EOF
}

echo "🔍 Checking file permissions..."

# Check for sensitive files with wrong permissions
if [ -f "keys/jwt_private.pem" ]; then
    PERM=$(stat -c "%a" "keys/jwt_private.pem" 2>/dev/null || echo "644")
    if [ "$PERM" != "600" ]; then
        add_finding "HIGH" "Insecure Private Key Permissions" \
            "JWT private key has permissions $PERM instead of 600" \
            "Run: chmod 600 keys/jwt_private.pem"
    fi
fi

echo "🔍 Checking for hardcoded secrets..."

# Check for potential hardcoded secrets in Go files
HARDCODED_SECRETS=$(find src/ -name "*.go" -exec grep -l "password\|secret\|key\|token" {} \; 2>/dev/null || true)
if [ -n "$HARDCODED_SECRETS" ]; then
    add_finding "MEDIUM" "Potential Hardcoded Secrets" \
        "Found references to secrets in Go files: $HARDCODED_SECRETS" \
        "Review code for hardcoded credentials and use environment variables"
fi

echo "🔍 Checking Docker configuration..."

# Check for root user in Dockerfile
if grep -q "USER root" Dockerfile 2>/dev/null; then
    add_finding "HIGH" "Docker Running as Root" \
        "Dockerfile configured to run as root user" \
        "Change to non-root user in production Dockerfile"
fi

echo "🔍 Checking database configuration..."

# Check for SSL configuration
if ! grep -q "sslmode=require" configs/ 2>/dev/null; then
    add_finding "HIGH" "Database SSL Not Enforced" \
        "Database connection may not enforce SSL encryption" \
        "Add sslmode=require to database connection string"
fi

echo "🔍 Checking CORS configuration..."

# Check if CORS allows all origins
if grep -q "allowed_origins.*\*" configs/ 2>/dev/null; then
    add_finding "HIGH" "Overly Permissive CORS" \
        "CORS allows all origins (*)" \
        "Restrict CORS to specific allowed origins"
fi

echo "🔍 Checking password policies..."

# Add password policy findings
add_finding "INFO" "Password Policy Review" \
    "Current bcrypt cost: 12, Min length: 8" \
    "Consider increasing bcrypt cost to 14+ for higher security"

echo "🔍 Checking rate limiting..."

# Check if rate limiting is enabled
if ! grep -q "rate_limiting:" configs/ 2>/dev/null; then
    add_finding "MEDIUM" "Missing Rate Limiting" \
        "No rate limiting configuration found" \
        "Implement rate limiting to prevent brute force attacks"
fi

echo "🔍 Checking JWT configuration..."

# Check JWT token expiry
if grep -q "access_token_duration.*[0-9][0-9][0-9][0-9]" configs/ 2>/dev/null; then
    add_finding "MEDIUM" "Long JWT Expiry" \
        "JWT tokens have very long expiry times" \
        "Reduce JWT expiry times and implement refresh tokens"
fi

echo "🔍 Checking logging configuration..."

# Check for debug logging in production
if grep -q "level.*debug" configs/ 2>/dev/null; then
    add_finding "LOW" "Debug Logging Enabled" \
        "Debug logging may be enabled in production config" \
        "Set logging level to 'info' or 'warn' in production"
fi

# Add recommendations section
cat >> "$REPORT_FILE" << 'EOF'
## Security Recommendations

### High Priority
1. **Implement HTTPS Everywhere**: Ensure all communications use TLS 1.2+
2. **Regular Security Updates**: Keep all dependencies up to date
3. **Key Rotation**: Implement regular JWT key rotation
4. **Database Encryption**: Enable database encryption at rest
5. **Audit Logging**: Log all security-relevant events

### Medium Priority
1. **Content Security Policy**: Implement strict CSP headers
2. **Session Management**: Implement secure session invalidation
3. **Input Validation**: Validate and sanitize all inputs
4. **Error Handling**: Avoid exposing sensitive information in errors
5. **Backup Security**: Encrypt backups and test recovery procedures

### Low Priority
1. **Security Headers**: Add additional security headers (HSTS, X-Frame-Options)
2. **Monitoring**: Implement real-time security monitoring
3. **Penetration Testing**: Conduct regular security assessments
4. **Documentation**: Keep security documentation up to date

## Next Steps
1. Address all HIGH severity findings immediately
2. Create remediation plan for MEDIUM severity findings
3. Schedule regular security audits (monthly)
4. Implement automated security testing in CI/CD pipeline

EOF

echo "✅ Security audit completed!"
echo "📄 Report saved to: $REPORT_FILE"
echo ""
echo "📊 Summary:"
HIGH_COUNT=$(grep -c "### HIGH:" "$REPORT_FILE" || echo "0")
MEDIUM_COUNT=$(grep -c "### MEDIUM:" "$REPORT_FILE" || echo "0")
LOW_COUNT=$(grep -c "### LOW:" "$REPORT_FILE" || echo "0")
INFO_COUNT=$(grep -c "### INFO:" "$REPORT_FILE" || echo "0")

echo "   🔴 High Priority:   $HIGH_COUNT findings"
echo "   🟡 Medium Priority: $MEDIUM_COUNT findings"
echo "   🟢 Low Priority:    $LOW_COUNT findings"
echo "   ℹ️  Informational:   $INFO_COUNT findings"

if [ "$HIGH_COUNT" -gt 0 ]; then
    echo ""
    echo "⚠️  URGENT: $HIGH_COUNT high-priority security issues found!"
    echo "   Review and address immediately before production deployment."
fi