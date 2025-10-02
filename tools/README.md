# Tools Directory

This directory contains utility tools and scripts for development and testing.

## Files

### test-server.go
A simple test server to validate OAuth2 integration with Hydra.

**Usage:**
```bash
cd tools
go run test-server.go
```

**Features:**
- Complete OAuth2 Authorization Code flow testing
- Login and consent flow handlers
- Simple callback endpoint for testing
- Test credentials: admin@test.com / password

**Endpoints:**
- `GET /login` - Login form (handles login_challenge)
- `POST /login` - Process login
- `GET /consent` - Consent form (handles consent_challenge)
- `POST /consent` - Process consent
- `GET /callback` - OAuth callback endpoint
- `GET /health` - Health check

This tool is particularly useful for:
- Testing Hydra integration
- Validating OAuth flows
- Development debugging
- Manual testing of authentication workflows