# Authway Documentation

## Quick Links

- **[Getting Started Guide](./GETTING_STARTED.md)** - Setup, installation, and integration guide
- **[SDK Reference](./SDK_REFERENCE.md)** - Complete API documentation for both SDKs

## Feature Guides

- **[Dynamic Claims](./features/DYNAMIC_CLAIMS.md)** - Runtime user claims management
- **[Popup Login](./features/POPUP_LOGIN_GUIDE.md)** - No-redirect authentication flow
- **[OAuth & JWT Best Practices](./features/OAUTH_JWT_BEST_PRACTICES.md)** - Security guidelines and implementation patterns

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
