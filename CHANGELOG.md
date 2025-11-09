# Changelog

All notable changes to Authway will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.1] - 2025-10-29

### Fixed

#### 🔒 CORS Configuration for OAuth Authentication
- **Fixed CORS wildcard issue**: Changed `AUTHWAY_CORS_ALLOWED_ORIGINS` from wildcard `*` to specific allowed origins
- **Resolved OAuth failures**: Fixed browser security policy violation that blocked all OAuth authentication flows (Google OAuth, social login providers)
- **Production domains**: `https://auth.iyulab.com`, `https://authway-admin.iyulab.com`
- **Development support**: Included `http://localhost:3000`, `http://localhost:5173` for local development
- **Impact**: Resolves 100% OAuth authentication failures in production
- **Reference**: Issue #001 - CORS Wildcard with Credentials

---

## [0.1.0] - 2025-10-20

### Added

#### 🆕 Dynamic Claims Management
- **Real-time Claims Updates**: Update user claims (workspace, role, permissions) without requiring logout/login
- **Automatic Token Refresh**: Silent re-authentication flow for seamless token updates
- **Claims API**: New REST endpoints for claims management
  - `GET /api/v1/claims` - Retrieve current user claims
  - `POST /api/v1/claims/update` - Update claims and trigger re-authentication
  - `DELETE /api/v1/claims/:claim_key` - Delete specific claim
- **Dual Storage Strategy**:
  - Redis for pending/session claims (5-minute TTL)
  - PostgreSQL for permanent claims (persistent across sessions)
- **Multi-tenant Support**: Claims isolated per tenant for security
- **Comprehensive Documentation**:
  - Complete feature guide with use cases and architecture
  - Implementation examples for JavaScript, Python, Go, and .NET
  - Integration guides with code samples
  - Testing instructions and best practices

#### 🔄 Database Migration System
- **Automatic Schema Management**: Embedded SQL migrations with version tracking
- **Migration Runner**: Internal package for automated database updates
- **Version Control**: Migration history tracked in `schema_migrations` table
- **Startup Automation**: Migrations run automatically on server startup
- **Schema Versioning**: Support for incremental schema updates

#### ✏️ OAuth Client Management Enhancements
- **Edit Functionality**: Update existing OAuth clients in Admin Dashboard
  - Client name, description, and website
  - Redirect URIs configuration
  - Grant types and scopes management
  - Client secret remains view-once only (security best practice)
- **Improved UX**: Inline edit forms with validation
- **React Query Integration**: Optimistic updates and error handling

#### 📚 Documentation Improvements
- **Comprehensive API Documentation**: New `API_INTRODUCTION.md` with complete endpoint reference
- **Dynamic Claims Guide**: Detailed feature documentation at `docs/features/DYNAMIC_CLAIMS.md`
- **Language-Specific Quickstarts**:
  - JavaScript/Node.js integration guide
  - Python/Flask integration guide
  - .NET/ASP.NET Core integration guide with ClaimsService example
  - Go integration guide
- **Integration Guide Updates**: Added Dynamic Claims Integration section
- **Quick Start Guide**: Updated with links to new features and documentation
- **README Updates**:
  - Added Dynamic Claims and Database Migration features
  - Updated project structure to reflect new packages
  - Added comprehensive API & Integration section
- **Documentation Index**: Central navigation hub for all project documentation

### Changed

#### 🧹 Code Quality Improvements
- **Production-Ready Logging**: Cleaned up verbose debug logging in claims service
  - Removed temporary diagnostic markers (🔍 DEBUG)
  - Retained essential production-level logging
  - Improved observability for production debugging
- **Error Handling**: Enhanced error messages and logging context

#### 📁 Project Structure
- **New Packages**:
  - `src/server/pkg/claims/` - Dynamic claims management service
  - `src/server/internal/database/` - Migration runner and database utilities
  - `src/server/internal/middleware/` - JWT authentication middleware
- **Migration Organization**: SQL migrations in `src/server/internal/database/migrations/`

### Fixed

- **Claims Integration Issues**: Resolved missing claims after workspace switching
  - Implemented proper UserInfo parameter passing
  - Fixed consent flow to include base user claims
  - Added fallback mechanisms for Redis and database claims
- **API Endpoint Consistency**: Corrected endpoint paths in API documentation
  - POST endpoint is `/api/v1/claims/update` (not `/api/v1/claims`)
  - DELETE endpoint parameter is `:claim_key` (standardized)

### Security

- **Claims Validation**: Server-side validation of all claim updates
- **Token Lifecycle Management**:
  - Access tokens: 15-minute lifetime (recommended)
  - Pending claims: 5-minute Redis TTL
  - Login claims: 10-minute Redis TTL
- **CSRF Protection**: State parameter validation in OAuth flows
- **Multi-tenant Isolation**: Claims scoped to authenticated user and tenant

### Documentation

All documentation files updated to version 0.1.0 with last updated date of 2025-10-18 or 2025-10-20.

### Infrastructure

- **Azure Application Insights**: Telemetry and distributed tracing
- **Database Schema**: New `user_claims` table with JSONB support
- **Redis Integration**: Claims caching and session management

---

## [0.0.1] - 2025-10-01 (Initial Release)

### Added

#### Core Features
- **Multi-tenant Architecture**: Complete tenant isolation with central management
- **OAuth 2.0 / OpenID Connect**: Standard-compliant authentication flows
- **JWT Token Management**: Secure access and refresh tokens
- **Email Authentication**: User registration with email verification
- **Password Reset**: Secure password recovery workflow
- **Social Login**: Google OAuth integration
- **Admin Console**: React-based dashboard for managing:
  - OAuth clients
  - Users and tenants
  - System configuration
- **Login UI**: Modern authentication interface with Vite and React
- **Session Management**: Redis-based session storage

#### Backend (Go)
- **Fiber Framework**: High-performance web server
- **PostgreSQL Integration**: Primary data storage with GORM
- **Ory Hydra Integration**: OAuth 2.0 server backend
- **Structured Logging**: zap logger with context
- **CORS Configuration**: Configurable cross-origin request handling

#### Frontend
- **React 18**: Modern UI library
- **TypeScript**: Type-safe development
- **TailwindCSS**: Utility-first styling
- **TanStack Query**: Efficient data fetching and caching
- **Vite**: Fast build tooling

#### Security
- **SSL/TLS Support**: Production PostgreSQL encryption
- **Azure Key Vault**: Secret management integration
- **Password Hashing**: Secure credential storage
- **Token Validation**: JWT signature verification

#### Deployment
- **Azure Container Apps**: Backend hosting
- **Azure Static Web Apps**: Frontend hosting
- **Azure Database for PostgreSQL**: Managed database
- **Docker Support**: Containerized deployment
- **PowerShell Scripts**: Automated deployment workflows

#### Documentation
- **Quick Start Guide**: 5-minute local setup
- **Docker Guide**: Complete containerization instructions
- **Configuration Guide**: Comprehensive environment variable documentation
- **Architecture Documentation**: Multi-tenancy design and system architecture
- **Deployment Guides**: Azure-specific production deployment
- **Testing Guide**: Test suite documentation

---

## Release Notes

### Version 0.1.0 Highlights

This release brings **Dynamic Claims Management**, a powerful feature that enables real-time user context switching without requiring logout/login cycles. This is particularly valuable for:

- **SaaS Applications**: Users can switch between workspaces or organizations seamlessly
- **Enterprise Systems**: Administrators can update permissions instantly
- **Feature Flags**: Enable/disable features dynamically per user
- **Support Systems**: Grant temporary elevated access for troubleshooting

Additionally, the **Database Migration System** ensures smooth schema updates across deployments, and the **OAuth Client Edit** functionality provides better administrative control.

### Breaking Changes

None. This is a minor version update that is fully backward compatible with 0.0.1.

### Upgrade Instructions

1. **Database Migration**: Migrations run automatically on server startup
2. **Environment Variables**: No new required variables (optional Application Insights connection string)
3. **API Changes**: New endpoints are additive only, existing endpoints unchanged
4. **Dependencies**: Update Go modules with `go mod download`
5. **Frontend**: Update npm packages with `npm install` in admin-dashboard and login-ui

### Known Issues

- Rate limiting not yet implemented
- Postman collection and OpenAPI spec coming in future release
- Language SDKs (JavaScript, Python, Go, .NET) planned for future releases

---

## Links

- **Repository**: https://github.com/iyulab/authway
- **Documentation**: [docs/](docs/)
- **Issues**: https://github.com/iyulab/authway/issues
- **Discussions**: https://github.com/iyulab/authway/discussions

---

**Maintained by**: Authway Team
**License**: MIT
