# Authway Documentation

See [CHANGELOG.md](../CHANGELOG.md) for the current release and full history.

## Recent additions

Since the feature set below was last written up, these have shipped —
covered in the CHANGELOG rather than a dedicated guide for now:

- **Invitation-only onboarding** - no public sign-up; every account starts
  from an invitation
- **Magic-link sign-in** - passwordless email links, gated by the same
  invitation policy as password and social accounts
- **MFA (TOTP)** - encrypted secrets, backup codes
- **Admin impersonation** - audited "sign in as this user" for support
- **Audit logging** - auth, admin and webhook actions per tenant
- **Webhooks** - subscribe to account and session lifecycle events

---

## Core Documentation

### Getting Started
- **[Setup Guide](./SETUP.md)** - Installation, first-user invitation flow, client registration, SDK integration
- **[SDK Reference](./SDK_REFERENCE.md)** - Full API documentation for React and Vanilla JS SDKs

### Features & Integration
- **[Features Guide](./FEATURES.md)** - Dynamic Claims, Popup Login, Logout Policies, OAuth/JWT Best Practices
- **[Backend Integration](./BACKEND_INTEGRATION.md)** - Protect your APIs with JWT validation
- **[Client Management API](./api/client-management.md)** - OAuth client config, Hydra sync semantics, application-type matrix (ASP.NET / SPA / M2M)

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
