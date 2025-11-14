# Authway

Modern OAuth 2.0 / OpenID Connect authentication system built on Ory Hydra with JavaScript/TypeScript SDKs.

[![Version](https://img.shields.io/badge/version-0.1.4-blue.svg)](https://github.com/iyulab/authway)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

**NPM Packages:**

[![@authway/client](https://img.shields.io/npm/v/@authway/client?label=%40authway%2Fclient&color=blue)](https://www.npmjs.com/package/@authway/client)
[![@authway/react](https://img.shields.io/npm/v/@authway/react?label=%40authway%2Freact&color=blue)](https://www.npmjs.com/package/@authway/react)

**GitHub Packages:**

[![GitHub Packages](https://img.shields.io/badge/GitHub_Packages-@authway-blue?logo=github)](https://github.com/orgs/iyulab/packages)

## Features

- **OAuth 2.0 / OIDC** - Standards-compliant authentication with PKCE support
- **Auto-Discovery** - Apps only need Auth Backend URL, rest is auto-discovered
- **Dynamic Claims** - Runtime user claims management
- **Multi-Tenancy** - Fully isolated tenant support
- **Popup Login** - No-redirect authentication flow with Google OAuth support (v0.1.4)
- **TypeScript SDKs** - `@authway/client` and `@authway/react`

### What's New in v0.1.4 (2025-11-10)

- ✅ **Popup Mode with External OAuth Providers** - Google OAuth, GitHub, and other external providers now work seamlessly in popup mode
- ✅ **COOP Compatibility** - Solved Cross-Origin-Opener-Policy blocking with sessionStorage persistence
- 🔧 **SessionStorage + Hidden Iframe Pattern** - Robust cross-origin authentication flow
- 📝 **Comprehensive Documentation** - Updated guides for popup authentication with external providers

See [CHANGELOG.md](./CHANGELOG.md) for complete release notes.

## Quick Start

### Prerequisites

- Node.js 18+, pnpm 9+
- Go 1.21+
- PostgreSQL 15+
- Docker (for Hydra)

### Installation

```bash
# Clone repository
git clone https://github.com/iyulab/authway.git
cd authway

# Install dependencies
pnpm install

# Setup environment
cp .env.example .env
# Edit .env with your configuration

# Start Hydra (Docker)
docker run -d --name hydra -p 4444:4444 -p 4445:4445 \
  oryd/hydra:v2.2.0 serve all --dev

# Start Central API (port 8080)
cd apps/central/api
go run cmd/main.go

# Start Auth Backend (port 8081)
cd apps/branding/auth-api
go run cmd/main.go

# Start sample app (port 9004)
cd samples/react-sdk-sample
pnpm dev
```

Access: http://localhost:9004

## SDK Usage

### React

```tsx
import { AuthwayProvider, useAuth } from '@authway/react'

function App() {
  return (
    <AuthwayProvider
      config={{
        domain: 'http://localhost:8081',
        clientId: 'your-client-id'
      }}
    >
      <Dashboard />
    </AuthwayProvider>
  )
}

function Dashboard() {
  const { isAuthenticated, user, loginWithPopup, logout } = useAuth()

  if (!isAuthenticated) {
    return <button onClick={() => loginWithPopup()}>Login</button>
  }

  return (
    <div>
      <h1>Welcome, {user.name}!</h1>
      <button onClick={() => logout()}>Logout</button>
    </div>
  )
}
```

### Vanilla JS/TypeScript

```typescript
import { AuthwayClient } from '@authway/client'

const client = new AuthwayClient({
  domain: 'http://localhost:8081',
  clientId: 'your-client-id'
})

await client.waitForReady()
await client.loginWithPopup()

const token = await client.getAccessToken()
const user = await client.getUser()
```

## Documentation

- **[Getting Started Guide](./docs/GETTING_STARTED.md)** - Complete setup and integration guide
- **[Backend Integration Guide](./docs/BACKEND_INTEGRATION_GUIDE.md)** - ASP.NET, Node.js, Go backend integration
- **[Client Registration](./docs/CLIENT_REGISTRATION.md)** - OAuth 2.0 client setup for Public and Confidential clients
- **[SDK Reference](./docs/SDK_REFERENCE.md)** - API documentation for both SDKs
- **[OAuth Best Practices](./docs/features/OAUTH_JWT_BEST_PRACTICES.md)** - Security guidelines and common patterns
- **[Popup Login Guide](./docs/features/POPUP_LOGIN_GUIDE.md)** - Popup authentication with external OAuth providers
- **[Samples](./samples/)** - Example applications
  - [React SDK Sample](./samples/react-sdk-sample/) - Full-featured React demo
  - [ASP.NET SPA Sample](./samples/asp-spa/) - Backend + Frontend (React & Vanilla JS)

## Architecture

```
┌─────────────────┐
│  Consumer App   │  @authway/react or @authway/client
└────────┬────────┘
         │ GET /.well-known/authway-config
         ▼
┌─────────────────┐
│  Auth Backend   │  (port 8081) - App entry point
└────────┬────────┘
         │ Proxies to Central API
         ▼
┌─────────────────┐         ┌──────────────┐
│  Central API    │◄────────┤ Ory Hydra    │
│  (port 8080)    │         │ (4444/4445)  │
└─────────┬───────┘         └──────────────┘
          │
          ▼
    ┌───────────┐
    │PostgreSQL │
    └───────────┘
```

**Key Points**:
- Apps only connect to Auth Backend (8081)
- Central API (8080) is internal - never exposed directly
- Hydra handles OAuth 2.0 protocol
- PostgreSQL stores users, tenants, and configurations

## Project Structure

```
authway/
├── apps/
│   ├── central/api/          # Core business logic (Go)
│   └── branding/auth-api/    # Auth backend (Go)
│
├── packages/
│   ├── client/               # @authway/client (TypeScript)
│   └── react/                # @authway/react
│
├── samples/
│   ├── react-sdk-sample/     # React demo
│   └── asp-spa/              # ASP.NET + React/Vanilla JS
│
└── docs/                     # Documentation
```

## Development

### Build SDKs

```bash
# Build client SDK
cd packages/client && pnpm build

# Build React SDK
cd packages/react && pnpm build
```

### Run Tests

```bash
# SDK tests
pnpm test

# Backend tests
cd apps/central/api && go test ./...
```

## Contributing

Contributions welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) first.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Links

- **Documentation**: [./docs/](./docs/)
- **Issues**: [GitHub Issues](https://github.com/iyulab/authway/issues)
- **Changelog**: [CHANGELOG.md](CHANGELOG.md)
