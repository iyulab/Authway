# Authway - Session Context

**Last Updated**: 2025-10-02
**Session ID**: cleanup-and-load-001
**Project Status**: Phase 1.5 Complete - Production Ready

## 📋 Project Overview

**Authway** - Modern OAuth2/OIDC authentication platform built on Ory Hydra
Open-source alternative to Auth0 with enterprise-grade features and performance.

### Core Architecture (3-Layer)
```
┌─────────────────────────────────────────┐
│  Layer 3: Developer Experience         │  ← Admin Dashboard + Login UI (React)
├─────────────────────────────────────────┤
│  Layer 2: User Management               │  ← Authway API (Go/Fiber)
├─────────────────────────────────────────┤
│  Layer 1: OAuth2 Core                   │  ← Ory Hydra v2.2.0
└─────────────────────────────────────────┘
```

### Key Features
- **OAuth 2.0/OIDC**: Certified implementation via Ory Hydra
- **Hybrid OAuth**: Client-specific + centralized Google OAuth modes
- **User Management**: Complete CRUD with secure authentication
- **SSO**: Single Sign-On across multiple services
- **Performance**: <50ms response time, 1000+ req/s throughput

## 🏗️ Technical Stack

### Backend (Go 1.21+)
```yaml
Framework: Fiber v2.52.0
OAuth Core: Ory Hydra v2.2.0
ORM: GORM (PostgreSQL + SQLite support)
Cache: Redis v9.3.1
Config: Viper v1.18.2
Logging: Uber Zap v1.26.0
Validation: go-playground/validator v10.16.0
Security: golang.org/x/crypto (bcrypt)
```

### Frontend (React 18 + TypeScript)
```yaml
Build: Vite 4.5.0
UI: Tailwind CSS + shadcn/ui components
State: Zustand 5.0.8
API: TanStack Query v5.8.4 + Axios
Forms: React Hook Form v7.48.2 + Zod v3.22.4
Router: React Router v6.20.1
Testing: Vitest + Testing Library + MSW
```

### Infrastructure
```yaml
Database: PostgreSQL 15-alpine
Cache: Redis 7-alpine
Containers: Docker + Docker Compose
OAuth: Ory Hydra v2.2.0 (certified)
Monitoring: Prometheus + Grafana (configured)
```

## 📁 Project Structure

```
authway/
├── src/server/                 # Go backend
│   ├── cmd/main.go            # Application entry point
│   ├── internal/              # Private application code
│   │   ├── config/            # Configuration management
│   │   ├── database/          # DB (Postgres, Redis)
│   │   ├── handler/           # HTTP handlers (auth, user, client, social)
│   │   ├── hydra/             # Hydra client integration
│   │   ├── middleware/        # Auth & error middleware
│   │   └── service/           # Business logic (social/google)
│   └── pkg/                   # Public libraries
│       ├── auth/              # Auth service + tests
│       ├── client/            # OAuth client service + tests
│       └── user/              # User service + tests
│
├── packages/web/              # Frontend applications
│   ├── admin-dashboard/       # Admin UI (React/TS/Vite)
│   │   ├── src/
│   │   │   ├── components/    # Layout components
│   │   │   ├── pages/         # Dashboard, Clients, Settings
│   │   │   └── lib/           # Utilities
│   │   └── package.json       # Dependencies
│   │
│   └── login-ui/              # Authentication UI (React/TS/Vite)
│       ├── src/
│       │   ├── components/    # GoogleLoginButton
│       │   └── pages/         # RegisterPage, etc.
│       └── package.json       # Dependencies
│
├── configs/                   # Configuration files
│   ├── hydra.yml             # Ory Hydra config
│   └── config.production.yaml # Production settings
│
├── scripts/                   # Database & setup scripts
│   ├── init-db.sql           # DB initialization
│   └── setup-google-oauth.md # Google OAuth guide
│
├── tools/                     # Development utilities
│   ├── test-server.go        # Hydra integration testing
│   └── simple-server.go      # Mock API server
│
├── k8s/                      # Kubernetes manifests (if needed)
├── docs/                     # Additional documentation
├── .air.toml                 # Hot reload config
├── docker-compose.yml        # Development environment
├── Dockerfile                # Production container
└── go.mod                    # Go dependencies
```

## 🔑 Core Dependencies

### Go Modules (go.mod)
- **Web**: gofiber/fiber/v2 v2.52.0
- **Database**: gorm.io/gorm v1.30.0, lib/pq v1.10.9
- **Cache**: redis/go-redis/v9 v9.3.1
- **Auth**: golang.org/x/crypto v0.17.0
- **Config**: spf13/viper v1.18.2
- **Logging**: go.uber.org/zap v1.26.0
- **Testing**: stretchr/testify v1.8.4

### Frontend (package.json)
- **Core**: react v18.2.0, typescript v5.2.2
- **State**: zustand v5.0.8 (admin) / v4.4.7 (login)
- **API**: @tanstack/react-query v5.8.4, axios v1.6.2+
- **Forms**: react-hook-form v7.48.2, zod v3.22.4
- **UI**: tailwind v3.3.5, lucide-react v0.294.0
- **Testing**: vitest v1.0.4, @testing-library/react v14.1.2, msw v2.0.8

## 🚀 Development Workflow

### Environment Setup
```bash
# Start full stack with Docker Compose
docker-compose up -d

# Services running:
# - postgres:5432 (PostgreSQL)
# - redis:6379 (Redis)
# - hydra:4444 (Public) / 4445 (Admin)
# - authway-api:8080 (Go API)
# - admin-dashboard:3000 (React)
# - login-ui:3001 (React)
```

### Local Development
```bash
# Go backend (with hot reload)
air -c .air.toml

# Admin Dashboard
cd packages/web/admin-dashboard && npm run dev

# Login UI
cd packages/web/login-ui && npm run dev
```

### Build Commands
```bash
# Go build
go build -o authway ./src/server/cmd

# Frontend build
npm run build  # in respective package directories
```

### Testing
```bash
# Go tests
go test ./...

# Frontend tests
npm test              # Unit tests
npm run test:coverage # Coverage report
```

## 🔐 Security Configuration

### Authentication Flow
1. User clicks login → Redirected to Authway
2. Authway displays login UI (email/password or Google)
3. On success → Hydra consent flow (if needed)
4. Hydra issues authorization code
5. Client exchanges code for tokens
6. Access token grants API access

### Hybrid Google OAuth
- **Client-Specific**: Each OAuth client can configure own Google app (branding)
- **Central Fallback**: Default Authway Google OAuth if client has no config
- **Automatic**: System selects appropriate OAuth app based on client_id

### Security Features
- **Password**: bcrypt hashing
- **Tokens**: JWT with RSA-256 signing
- **Sessions**: Redis-backed with secure cookies
- **CORS**: Configurable allowed origins
- **CSRF**: State parameter validation
- **SQL Injection**: GORM prepared statements

## 📊 Performance Metrics

### Current State (Phase 1.5)
```yaml
Response Time: <50ms (token issuance)
Throughput: 1,000+ req/s (single instance)
Memory: ~50MB (idle), <100MB (loaded)
Docker Image: <20MB (Go binary)
Startup: <3 seconds
```

### Scalability
- Horizontal scaling via load balancer
- Database connection pooling (GORM)
- Redis caching for sessions
- Stateless JWT for distributed auth

## 🔄 Current Development Phase

### ✅ Phase 1 Complete
- OAuth 2.0/OIDC with Ory Hydra
- User registration/login
- JWT token issuance/validation
- Basic Admin Dashboard
- Docker deployment

### ✅ Phase 1.5 Complete (Production Ready)
- Consent Flow UI
- User registration forms
- Admin authentication system
- Production security configuration
- Monitoring stack (Prometheus/Grafana)
- Hybrid Google OAuth

### 🚧 Phase 2 In Progress (Next)
- Additional social logins (GitHub, Kakao, Naver)
- Email verification system
- Password reset flow
- React/Vue/Next.js SDKs
- Advanced token management

### 📅 Future Phases
- **Phase 3**: 2FA (TOTP), WebAuthn, RBAC, Audit logs
- **Phase 4**: Multi-tenancy, High availability, ML-based threat detection

## 🐛 Known Issues

### TypeScript Test Files (login-ui)
**Files with unused variables**:
- `src/pages/ConsentPage.test.tsx` (14 instances of unused `req`, `ctx` in MSW handlers)
- `src/pages/RegisterPage.test.tsx` (7 instances of unused `req`, `ctx` in MSW handlers)

**Fix**: Prefix with underscore (`_req`, `_ctx`) or remove if truly unused

### Go Test Issue
- `src/server/internal/handler/auth_test.go:143` - MockHydraClient type mismatch
- Needs investigation for proper mock interface implementation

## 📝 Recent Session Activity

### Cleanup Completed (2025-10-02)
1. ✅ Moved `simple-server.go` → `tools/simple-server.go`
2. ✅ Added `build/` directory to `.gitignore`
3. ✅ Removed build artifacts (`build/authway`, `build/authway.exe`)
4. ✅ Identified unused imports/variables in test files
5. ✅ Go code formatted with `go fmt`

### Project Status
- **Codebase**: Clean and organized
- **Build System**: Validated and functional
- **Dependencies**: All up to date (Go 1.21+, React 18)
- **Docker**: Multi-service orchestration ready
- **Documentation**: Comprehensive guides available

## 🎯 Next Development Steps

### Immediate Priorities (Phase 2)
1. Fix TypeScript test file warnings (unused variables)
2. Implement GitHub OAuth integration
3. Add email verification system
4. Create React SDK (@authway/react)
5. Implement password reset flow

### Medium Term (Phase 3)
1. 2FA with TOTP (Google Authenticator)
2. WebAuthn (biometric auth)
3. Audit logging system
4. RBAC implementation
5. Webhook system

### Long Term (Phase 4)
1. Multi-tenancy support
2. High availability architecture
3. Advanced analytics dashboard
4. Enterprise SSO (SAML 2.0)
5. Machine learning threat detection

## 🔗 Key Resources

### Documentation
- `README.md` - Project overview and quick start
- `GETTING_STARTED.md` - Onboarding guide
- `DEPLOYMENT-GUIDE.md` - Production deployment
- `DEPLOYMENT_READY.md` - Deployment readiness report
- `OPERATIONS.md` - Operational procedures
- `TESTING.md` - Testing strategies
- `TASKS.md` - Development roadmap

### Configuration
- `configs/hydra.yml` - Ory Hydra configuration
- `configs/config.production.yaml` - Production settings
- `docker-compose.yml` - Development environment
- `.env.example` - Environment variables template

### Key Endpoints
```
Hydra Public:  http://localhost:4444
Hydra Admin:   http://localhost:4445
Authway API:   http://localhost:8080
Admin UI:      http://localhost:3000
Login UI:      http://localhost:3001
PostgreSQL:    localhost:5432
Redis:         localhost:6379
```

## 💡 Development Notes

### Architecture Decisions
- **Ory Hydra**: Chose certified OAuth2 implementation over custom
- **Go + Fiber**: Performance-critical auth server needs efficiency
- **React 18**: Modern UI with Vite for fast development
- **PostgreSQL**: Relational data with ACID guarantees
- **Redis**: Session storage and caching layer

### Design Patterns
- **3-Layer Architecture**: Clear separation of concerns
- **Repository Pattern**: Data access abstraction (GORM)
- **Service Layer**: Business logic isolation
- **Dependency Injection**: Testable, maintainable code

### Performance Optimizations
- Database connection pooling
- Redis caching for hot paths
- Stateless JWT for horizontal scaling
- Efficient Go concurrency (goroutines)
- Frontend code splitting (Vite)

---

**Session Context Loaded** ✅
Ready for development tasks, feature implementation, or deployment operations.
