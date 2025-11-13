# Azure Functions Email Service Migration Guide

**Version**: 0.1.4+
**Date**: 2025-01-11
**Status**: Production Ready

---

## 📋 Overview

Authway has been updated to use **Azure Functions Email Service** for production deployments, replacing the traditional SMTP email system. This provides:

- ✅ **Enhanced Features**: Attachments, CC/BCC, priority settings, custom headers
- ✅ **Better Security**: Built-in validation, sanitization, and Function Key authentication
- ✅ **Improved Monitoring**: Application Insights integration with request tracking
- ✅ **Cost Efficiency**: Serverless pricing (~$0 for most workloads)
- ✅ **Reliability**: Azure-managed infrastructure with automatic scaling

---

## 🎯 What Changed

### For Local Development
**No changes required** - Local development continues to use SMTP (MailHog or Gmail).

### For Production Deployment
Production now uses Azure Functions Email Service by default:

| Aspect | Before | After |
|--------|--------|-------|
| **Email Provider** | Direct SMTP (Gmail/Office365) | Azure Functions Email Service |
| **Configuration** | SMTP credentials required | Azure Function Key required |
| **Features** | Basic email only | Full email features (attachments, CC/BCC, etc.) |
| **Monitoring** | Basic SMTP logs | Application Insights integration |

---

## 🚀 Migration Steps

### Step 1: Update Environment Variables

**Production (.env.production)**:
```bash
# Enable Azure Email Service
EMAIL_USE_AZURE=true
EMAIL_AZURE_BASE_URL=your-function-app.azurewebsites.net
EMAIL_AZURE_FUNCTION_KEY=your-azure-function-key-here

# Common email settings
EMAIL_FROM_EMAIL=noreply@your-domain.com
EMAIL_FROM_NAME=Authway

# Optional: SMTP as fallback (not required if using Azure)
# EMAIL_SMTP_HOST=smtp.gmail.com
# EMAIL_SMTP_PORT=587
# EMAIL_SMTP_USER=your-email@gmail.com
# EMAIL_SMTP_PASSWORD=your-app-password
```

**Development (.env or docker-compose.dev.yml)**:
```bash
# Keep using SMTP for local development
EMAIL_USE_AZURE=false
EMAIL_SMTP_HOST=mailhog  # or localhost:1025
EMAIL_SMTP_PORT=1025
EMAIL_FROM_EMAIL=test@authway.local
EMAIL_FROM_NAME=Authway Dev
```

### Step 2: Deploy Updated Configuration

**Using Docker Compose**:
```bash
# Update production environment
cp .env.production.example .env.production
# Edit .env.production with your values

# Deploy
docker-compose -f docker-compose.prod.yml up -d
```

**Using Azure Container Apps/App Service**:
```bash
# Set environment variables via Azure Portal or CLI
az webapp config appsettings set \
  --name authway-api \
  --resource-group authway-rg \
  --settings \
    AUTHWAY_EMAIL_USE_AZURE=true \
    AUTHWAY_EMAIL_AZURE_BASE_URL=your-function-app.azurewebsites.net \
    AUTHWAY_EMAIL_AZURE_FUNCTION_KEY=your-function-key
```

### Step 3: Verify Email Service

**Check logs**:
```bash
# Look for initialization message
docker logs authway-api | grep "Email Service"

# Expected output:
# Using Azure Functions Email Service baseURL=iyulab-sendemail.azurewebsites.net
```

**Test email sending**:
1. Register a new user account
2. Check that verification email is sent
3. Verify email arrives with proper formatting

---

## 🔄 Rollback Plan

If you need to rollback to SMTP:

```bash
# Set environment variable
EMAIL_USE_AZURE=false

# Ensure SMTP credentials are set
EMAIL_SMTP_HOST=smtp.gmail.com
EMAIL_SMTP_PORT=587
EMAIL_SMTP_USER=your-email@gmail.com
EMAIL_SMTP_PASSWORD=your-app-password

# Restart service
docker-compose -f docker-compose.prod.yml restart authway-api
```

---

## 📊 Feature Comparison

| Feature | SMTP (Old) | Azure Functions (New) |
|---------|------------|---------------------|
| HTML Emails | ✅ | ✅ |
| Plain Text | ❌ | ✅ |
| Attachments | ❌ | ✅ (25MB, multiple formats) |
| CC/BCC | ❌ | ✅ |
| Priority Settings | ❌ | ✅ (High/Normal/Low) |
| Custom Headers | ❌ | ✅ |
| Read Receipts | ❌ | ✅ |
| Delivery Receipts | ❌ | ✅ |
| Application Insights | ❌ | ✅ |
| Request Tracking | ❌ | ✅ (Unique RequestID) |
| HTML Sanitization | ❌ | ✅ (XSS prevention) |
| Email Validation | Basic | RFC 5322 compliant |

---

## 💰 Cost Analysis

### Azure Functions Email Service
- **Consumption Plan**: First 1M requests free, then $0.20 per million
- **Expected Usage**: ~10,000 emails/month
- **Estimated Cost**: **$0/month** (within free tier)

### Traditional SMTP
- **Gmail/Office 365**: $6-12/user/month
- **Server Resources**: SMTP connection overhead
- **Maintenance**: Manual monitoring and management

**Savings**: ~$6-12/month per service account + reduced operational overhead

---

## 🔒 Security Considerations

### Azure Functions Benefits
1. **Function Key Authentication**: API access control via secure keys
2. **Built-in Validation**: Email address, attachment size/type validation
3. **HTML Sanitization**: XSS prevention and header injection protection
4. **Rate Limiting**: Built-in request throttling
5. **Azure Security**: Managed by Microsoft's security infrastructure

### SMTP Considerations
1. **Credential Management**: SMTP passwords must be securely stored
2. **Limited Validation**: Basic email format checking only
3. **Manual Security**: Application-level security implementation required

---

## 📝 Configuration Reference

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `EMAIL_USE_AZURE` | No | `true` | Enable Azure Functions email service |
| `EMAIL_AZURE_BASE_URL` | Yes* | `iyulab-sendemail.azurewebsites.net` | Azure Functions base URL |
| `EMAIL_AZURE_FUNCTION_KEY` | Yes* | - | Azure Function authentication key |
| `EMAIL_SMTP_HOST` | No** | `smtp.gmail.com` | SMTP server host (fallback) |
| `EMAIL_SMTP_PORT` | No** | `587` | SMTP server port (fallback) |
| `EMAIL_SMTP_USER` | No** | - | SMTP username (fallback) |
| `EMAIL_SMTP_PASSWORD` | No** | - | SMTP password (fallback) |
| `EMAIL_FROM_EMAIL` | Yes | `noreply@authway.in` | Sender email address |
| `EMAIL_FROM_NAME` | Yes | `Authway` | Sender display name |

\* Required when `EMAIL_USE_AZURE=true`
\** Required when `EMAIL_USE_AZURE=false`

---

## 🐛 Troubleshooting

### Email not sending

**Check 1: Verify Azure Functions configuration**
```bash
# Check if Azure service is enabled
docker logs authway-api | grep "Email Service"

# Expected: "Using Azure Functions Email Service"
# If not: Check EMAIL_USE_AZURE environment variable
```

**Check 2: Test Azure Functions endpoint**
```bash
curl -X POST "https://iyulab-sendemail.azurewebsites.net/api/email/send?code=YOUR_FUNCTION_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "test@example.com",
    "subject": "Test Email",
    "textBody": "This is a test email"
  }'
```

**Check 3: Verify Function Key**
- Ensure `EMAIL_AZURE_FUNCTION_KEY` is set correctly
- Check for typos or extra whitespace
- Verify key hasn't expired (if using time-limited keys)

### Emails arrive but formatting is wrong

This should not happen with Azure Functions as it uses the same templates as SMTP. If it does:

1. Check `EMAIL_FROM_NAME` and `EMAIL_FROM_EMAIL` values
2. Verify frontend URL is correct (`APP_BASE_URL`)
3. Check Application Insights logs for any errors

---

## 📞 Support

### Azure Functions Email Service Issues
- **Repository**: https://github.com/iyulab/azure-functions-send-email
- **Documentation**: See `README.md` in Azure Functions repository

### Authway Integration Issues
- **GitHub Issues**: https://github.com/iyulab/authway/issues
- **Email**: support@iyulab.com

---

## 🎓 Additional Resources

- [Azure Functions Email Service README](https://github.com/iyulab/azure-functions-send-email/blob/main/README.md)
- [SMTP Providers Guide](https://github.com/iyulab/azure-functions-send-email/blob/main/docs/SMTP_PROVIDERS.md)
- [Authway Configuration Guide](./CONFIGURATION.md)
- [Production Deployment Guide](./PRODUCTION_DEPLOYMENT.md)

---

**Migration Complete!** 🎉

Your Authway production deployment now uses Azure Functions Email Service for enhanced features, better security, and improved reliability.
