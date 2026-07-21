# Authway

Modern OAuth 2.0 / OpenID Connect authentication system built on Ory Hydra with JavaScript/TypeScript SDKs.

[![Version](https://img.shields.io/badge/version-0.2.0-blue.svg)](https://github.com/iyulab/authway)
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
- **Popup Login** - No-redirect authentication flow with Google OAuth support
- **i18n Support** - Multi-language UI (Korean, English)
- **TypeScript SDKs** - `@authway/client` and `@authway/react`

### What's New in v0.2.0

- ✅ **i18n (Internationalization)** - Multi-language support for Auth UI (Korean, English)
- ✅ **Language Switcher** - User-selectable language with browser detection
- ✅ **Auto-executing Popup Callback** - `@authway/client/popup-callback` module for seamless popup flow
- ✅ **Enhanced Logout** - OIDC logout with `post_logout_redirect_uri` support
- ✅ **Next.js Sample** - Complete integration example with `@authway/react`

See [CHANGELOG.md](./CHANGELOG.md) for complete release notes.

## Quick Start

### Prerequisites

- Node.js 18+, pnpm 9+
- Go 1.25+
- Docker (for the backing services)

### Installation

```bash
# Clone repository
git clone https://github.com/iyulab/authway.git
cd authway

# Install dependencies
pnpm install

# Each Go service reads a .env next to itself; the defaults match compose
cp apps/central/api/.env.example apps/central/api/.env
cp apps/branding/auth-api/.env.example apps/branding/auth-api/.env
# Fill in GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET — the Auth Backend requires them

# Start the backing services: Postgres, Redis, MailHog, Hydra
# (Postgres is published on 5433 and Redis on 6380 — the .env files point there)
docker compose up -d
```

On Windows, `.\start-dev.ps1` starts everything below in one step. Otherwise run
each in its own shell:

```bash
# Central API (port 8080) — applies database migrations on startup
cd apps/central/api && go run ./cmd/

# Auth Backend (port 8081)
cd apps/branding/auth-api && go run ./cmd/

# Login UI (port 3001) — Hydra redirects here for login and consent
cd apps/branding/auth-ui && npm run dev

# Admin console (port 3000)
cd apps/central/admin && npm run dev

# Sample app (port 9004)
cd samples/react-sdk-sample && pnpm dev
```

Access: http://localhost:9004

The APIs and UIs deliberately run natively rather than in containers — see the
comment at the top of `docker-compose.yml`.

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
  - [Next.js Sample](./samples/nextjs-sample/) - Next.js App Router integration
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
│   ├── nextjs-sample/        # Next.js App Router demo
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
