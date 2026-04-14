# Changelog

All notable changes to Authway will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.3.0] - 2026-04-14

### Added

- **`sync_status` in client mutation responses**. `PUT/DELETE /api/v1/clients/:id`
  and `POST /api/v1/clients/:id/regenerate-secret` now include a
  `sync_status: { state, error }` object describing whether the change was
  replicated to Hydra. Previously a Hydra failure was logged as a warning and
  the response remained 200 OK — silently producing drift between Authway DB
  and Hydra. (Issue: `hydra-sync-silent-failure`.)
- **Strict-sync mode**. Pass `?strict_sync=true` on a mutation to receive
  `502 Bad Gateway` (with `sync_status` and `hint`) when the upstream sync
  fails, instead of the default best-effort 200. Useful for CI/migration
  scripts that must not proceed past silent drift.
- **Client config validation**. Create/Update now reject internally
  inconsistent OAuth client configurations with a structured 400 response
  (`code`, `field`, `message`, `hint`). Catches the ASP.NET-on-PKCE foot-gun
  at registration instead of at first login. (Issue:
  `aspnet-confidential-guidance-gap`.)
  - `public_client_with_secret` — public clients must not have a `client_secret`
  - `public_client_with_client_credentials` / `public_client_with_password_grant` — wrong client type for grant
  - `public_client_missing_allowed_origins` — SPA needs CORS allow-list
  - `confidential_client_unsupported_grants` — confidential clients need credential-bearing grants

---

## [0.2.1] - 2026-04-14

### Security

- **Critical: Authentication added to `/api/v1/clients/*` CRUD endpoints**. Before
  this release, Create/Read/Update/Delete/RegenerateSecret/SyncHydra/GoogleOAuth
  endpoints accepted unauthenticated requests, allowing any network-reachable
  caller to enumerate OAuth client configurations, modify `redirect_uris`
  (enabling authorization-code hijacking), and delete clients across tenants.
  All write endpoints and sensitive reads now require `AUTHWAY_ADMIN_API_KEY`
  via `Authorization: Bearer <key>`. Public-config (`/clients/:client_id/config`)
  and internal lookup (`/clients/by-client-id/:client_id`) remain unchanged.
- **Critical: `AdminAuth` middleware is now fail-closed**. Previously, when the
  configured API key was empty the middleware called `c.Next()` — meaning a
  deployment missing `AUTHWAY_ADMIN_API_KEY` silently exposed every protected
  endpoint. It now returns `503 Service Unavailable` on empty configuration.
  Production config validation enforces the key is set; development deployments
  auto-generate a random key at startup and log it.
- **PII: `/api/v1/profile/:id` now requires JWT**. The endpoint previously
  returned `email`, `name`, and `email_verified` for any UUID without auth,
  enabling user-enumeration. Use `/profile/me` for the authenticated user's
  own profile and `/users/:id` (admin) for administrative access.

### Changed

- **Unified admin auth**. `/api/v1/clients/*`, `/api/v1/users/*`, and
  `/api/v1/tenants/*` now accept either the long-lived `AUTHWAY_ADMIN_API_KEY`
  (programmatic callers) **or** an admin session token issued by `/admin/login`
  (Admin Console UI). Previously only the API key was accepted by the new
  client routes, which would have broken the Admin Console.
- **`GetAdminConsoleAuth` is fail-closed**. The same silent-bypass pattern that
  affected `AdminAuth` was present here for `/api/v1/webhooks/*`,
  `/api/v1/audit/*`, `/api/v1/invitations/*`, and `/api/v1/admin/impersonate/*`.
  Empty `AUTHWAY_ADMIN_API_KEY` now returns 503 instead of granting access.

### Migration

Operators running Authway without `AUTHWAY_ADMIN_API_KEY` set in production
**must** set it before upgrading — otherwise the server will refuse to start
(`configuration validation failed`). Existing clients of the admin API must
send `Authorization: Bearer $AUTHWAY_ADMIN_API_KEY` on all `/clients/*` and
`/profile/:id` requests. The Admin Console UI continues to work via session
tokens issued by `/admin/login`.

---

## [0.2.0] - 2025-12-03

### Added
- **i18n (Internationalization)**: Multi-language support for Auth UI
  - Korean (ko) and English (en) languages
  - Automatic browser language detection
  - URL query parameter support (`?lang=ko`)
  - Language preference persistence via localStorage
- **LanguageSwitcher Component**: User-selectable language switching in Auth UI
- **Popup Callback Module**: `@authway/client/popup-callback` for auto-executing callback handling
  - Detects popup/iframe context automatically
  - Extracts OAuth code and sends to parent via postMessage
  - Closes popup after completion
- **Next.js Sample**: Complete integration example with `@authway/react`
- **OIDC Logout Enhancement**: `post_logout_redirect_uri` support for controlled logout redirects

### Changed
- **Auth UI Pages**: All pages migrated to use i18n (`useTranslation` hook)
  - LoginPage, RegisterPage, ConsentPage, ErrorPage
  - ForgotPasswordPage, ResetPasswordPage, VerifyEmailPage
  - ResendVerificationPage, LogoutPage, PopupCallbackPage
  - GoogleLoginButton
- **Zod Validation**: Dynamic schema factories for i18n validation messages
- **Error Messages**: API error mapping to localized messages

### Technical Details
- **i18n Stack**: i18next + react-i18next + i18next-browser-languagedetector
- **Translation Namespaces**: common, auth, consent, password, errors
- **Language Detection Order**: querystring → localStorage → navigator → fallback (en)

---

## [0.1.4] - 2025-11-10

### Fixed
- **Popup Login Mode with OAuth Providers**: Popup mode now works correctly with Google OAuth and other external providers
- **Cross-Origin-Opener-Policy (COOP) Blocking**: Solved `window.opener` loss after Google OAuth redirect using sessionStorage
- **Popup Window Closure**: Popup now closes automatically after successful authentication
- **Authorization Code Extraction**: Code is extracted AFTER Hydra generates it, not before

### Changed
- **GoogleLoginButton.tsx**: Set sessionStorage flag when popup mode is detected
- **ConsentPage.tsx**: Check both window.opener AND sessionStorage for popup mode detection; added postMessage listener to receive code from iframe
- **LoginPage.tsx**: Check both window.opener AND sessionStorage for popup mode detection; added postMessage listener to receive code from iframe
- **callback.html**: Modified to actively send postMessage when running in iframe (both asp-spa and react-sdk-sample versions)
- **Popup Flow**: SessionStorage persists popup mode flag across cross-origin redirects

### Technical Details
- **Root Cause**: Google OAuth's Cross-Origin-Opener-Policy (COOP) blocks `window.opener` during cross-origin redirects
- **Secondary Issue**: redirect_to is a Hydra URL that GENERATES the code, not a callback URL that HAS the code
- **Solution**: SessionStorage + Hidden iframe + postMessage communication
  1. GoogleLoginButton detects popup mode and sets `sessionStorage.setItem('authway_popup_mode', 'true')`
  2. After Google OAuth redirect (which blocks window.opener), pages check sessionStorage
  3. Create hidden iframe in popup window (ConsentPage/LoginPage)
  4. Load Hydra redirect URL in iframe (popup stays intact)
  5. callback.html detects it's in iframe and sends postMessage with code/state to parent
  6. ConsentPage/LoginPage receives postMessage from iframe
  7. Forward code/state via postMessage to main window (using window.opener || window.parent)
  8. Clean up sessionStorage, iframe, event listeners, and close popup
  9. Fallback: URL polling every 100ms if postMessage fails (max 5 seconds)
- **Why SessionStorage**: Survives cross-origin redirects (unlike window.opener which COOP blocks)
- **Pattern**: Hybrid approach - sessionStorage for state persistence + hidden iframe for OAuth flow
- **Compatibility**: 100% backward compatible - redirect mode unchanged

### How It Works
```
Before (v0.1.3): ❌
LoginPage → Google OAuth (cross-origin) → ⚠️ COOP blocks window.opener
→ ConsentPage: window.opener === null → ❌ popup mode NOT detected

After (v0.1.4): ✅ SessionStorage + Hidden iframe
GoogleLoginButton (3001) popup:
  ↓ Detects popup mode: window.opener !== null
  ↓ Sets sessionStorage.setItem('authway_popup_mode', 'true')
  ↓ window.location.href = Google OAuth (cross-origin redirect)

Google OAuth redirect → ⚠️ COOP blocks window.opener

ConsentPage (3001) in popup:
  ↓ window.opener === null (BLOCKED by COOP)
  ↓ BUT sessionStorage.getItem('authway_popup_mode') === 'true' ✅
  ↓ Popup mode detected via sessionStorage!
  ↓ Create hidden iframe
  ↓ iframe.src = Hydra URL (4444)
  ↓ Hydra redirects iframe → callback.html (5173) with code
  ↓ Read iframe.contentWindow.location.href
  ↓ Extract code from iframe URL
  ↓ (window.opener || window.parent).postMessage({code, state})
  ↓ sessionStorage.removeItem('authway_popup_mode')
  ↓ Close popup

Main window (5173):
  ↓ Receives postMessage with code
  ✅ Exchange code for tokens
```

---

## [0.1.3] - 2025-11-10

### Fixed (Partial)
- **Popup Mode Detection**: Login UI now detects popup mode and logs flow progression
- **Developer Experience**: Added console logging for debugging popup authentication flow

### Known Issues (Fixed in v0.1.4)
- Popup mode detection worked but popup login still failed due to cross-origin redirect
- `window.opener` was lost when redirecting from localhost:3001 to localhost:5173

### Added
- **Popup Mode Detection**: Added `window.opener` detection logic to 7 redirect points
- **Flow Tracking**: Console logs show popup mode status at every navigation step

### Changed
- **GoogleLoginButton.tsx**: Added popup mode detection logging
- **ConsentPage.tsx**: Added popup mode detection logging (3 locations)
- **LoginPage.tsx**: Added popup mode detection logging (3 locations)

### Technical Details
- Detection only - did not solve the underlying cross-origin redirect problem
- Required v0.1.4 for actual popup mode functionality

---

## [0.1.2] - 2025-11-10

### Fixed
- **OAuth 2.0 Compliance**: Public clients can now register without `client_secret`, following RFC 6749 Section 2.1
- **Client Registration Logic**: Fixed silent failure when custom `client_id` was provided without `client_secret`
- **Error Messages**: Added clear validation errors for confidential clients with partial credentials

### Added
- **Comprehensive Documentation**: New [Client Registration Guide](./docs/CLIENT_REGISTRATION.md) covering Public and Confidential clients
- **Unit Tests**: Added extensive test coverage for public/confidential client creation scenarios
- **Better Logging**: Enhanced error messages with masked secrets for security

### Changed
- **API Behavior**: Public clients now correctly ignore `client_secret` field (backward compatible)
- **Confidential Clients**: Now require both `client_id` and `client_secret`, or neither (for auto-generation)
- **Code Comments**: Updated `CreateClientRequest` documentation to reflect actual behavior

### Security
- **Public Client Security**: Removed requirement to store dummy secrets for public clients
- **Secret Masking**: Added `maskSecret()` helper for safe logging of credentials

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
