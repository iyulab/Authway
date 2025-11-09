# 🍌 Banana Service

Sample OAuth 2.0 client application for testing Authway authentication.

## Quick Start

```bash
# Install dependencies
go mod download

# Run the service
go run main.go
```

Service will start on: **http://localhost:9002**

## Configuration

- **Port**: 9002
- **Client ID**: `banana-service-client`
- **Client Secret**: `banana-service-secret`
- **Redirect URI**: `http://localhost:9002/callback`
- **Color Theme**: Yellow (#FFD93D)

## Endpoints

- `/` - Home page
- `/login` - Initiate OAuth login
- `/callback` - OAuth callback handler
- `/profile` - User profile page (authenticated)
- `/logout` - Logout
- `/api/session` - Session status API

## Testing

1. Make sure Authway is running on `http://localhost:8080`
2. Register this client using `../setup-clients.ps1`
3. Start this service: `go run main.go`
4. Open: http://localhost:9002
5. Click "Login with Authway"

## See Also

- [Main README](../README.md) - Complete testing guide
- [Shared OAuth Package](../shared/) - OAuth utilities
