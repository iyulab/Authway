# Authway Documentation

**Latest Version**: 0.2.0 (2025-11-17)

## 🆕 What's New in v0.2.0

- ✅ **Intelligent Auto-Migration System** - Fast detection (1-2s) with PostgreSQL advisory locks
- ✅ **Consolidated Documentation** - Reorganized into 7 core documents for easier navigation
- ✅ **Enhanced Logout Policies** - Flexible redirect validation with whitelist and custom policies
- 📝 **Updated Guides** - Comprehensive setup, features, deployment, and backend integration guides

See [CHANGELOG.md](../CHANGELOG.md) for complete release notes.

---

## Core Documentation

### Getting Started
- **[Setup Guide](./SETUP.md)** - Complete installation, configuration, and SDK integration
- **[SDK Reference](./SDK_REFERENCE.md)** - Full API documentation for React and Vanilla JS SDKs

### Features & Integration
- **[Features Guide](./FEATURES.md)** - Dynamic Claims, Popup Login, Logout Policies, OAuth/JWT Best Practices
- **[Backend Integration](./BACKEND_INTEGRATION.md)** - Protect your APIs with JWT validation

### Operations
- **[Deployment Guide](./DEPLOYMENT.md)** - Azure, Docker, CORS, production checklist
- **[Database Guide](./DATABASE.md)** - Schema, migrations, auto-migration system

## Samples

Example applications in `../samples/`:

- **[React SDK Sample](../samples/react-sdk-sample/)** - Full-featured React demo
- **[ASP.NET SPA Sample](../samples/asp-spa/)** - Backend + Frontend (React & Vanilla JS)
  - React with @authway/react SDK
  - Vanilla JS with oauth4webapi (learning OAuth2 internals)

## Architecture

```
Your App → Auth Backend (8081) → Central API (8080) → Hydra (4444) → PostgreSQL
```

**Key Concepts**:
- **Auto-Discovery**: Apps only need Auth Backend URL
- **Config Endpoint**: `GET /.well-known/authway-config`
- **Internal API**: Central API never exposed directly
- **OAuth Server**: Hydra handles OAuth 2.0 protocol

## Support

- **GitHub Issues**: [github.com/iyulab/authway/issues](https://github.com/iyulab/authway/issues)
- **Main README**: [../README.md](../README.md)
