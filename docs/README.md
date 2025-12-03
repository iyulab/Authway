# Authway Documentation

**Latest Version**: 0.2.0 (2025-12-03)

## 🆕 What's New in v0.2.0

- ✅ **i18n (Internationalization)** - Multi-language support for Auth UI (Korean, English)
- ✅ **Language Switcher** - User-selectable language with automatic browser detection
- ✅ **Auto-executing Popup Callback** - `@authway/client/popup-callback` module for seamless popup flow
- ✅ **Enhanced Logout** - OIDC logout with `post_logout_redirect_uri` support
- ✅ **Next.js Sample** - Complete integration example with `@authway/react`
- ✅ **Intelligent Auto-Migration System** - Fast detection (1-2s) with PostgreSQL advisory locks

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
- **[Next.js Sample](../samples/nextjs-sample/)** - Next.js App Router with `@authway/react`
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
