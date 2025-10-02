# Google OAuth Setup Guide

This guide helps you configure Google OAuth for Authway authentication.

## 1. Google Cloud Console Setup

### Step 1: Create a Google Cloud Project
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select existing one
3. Enable the **Google+ API** and **OAuth2 API**

### Step 2: Configure OAuth Consent Screen
1. Navigate to **APIs & Services** > **OAuth consent screen**
2. Choose **External** user type (or Internal for G Suite)
3. Fill in required fields:
   - App name: `Authway Authentication`
   - User support email: Your email
   - Developer contact email: Your email
4. Add scopes: `email`, `profile`, `openid`
5. Add test users if in testing mode

### Step 3: Create OAuth Credentials
1. Go to **APIs & Services** > **Credentials**
2. Click **Create Credentials** > **OAuth 2.0 Client ID**
3. Choose **Web application**
4. Configure:
   - Name: `Authway Web Client`
   - Authorized JavaScript origins: `https://auth.yourdomain.com`
   - Authorized redirect URIs: `https://auth.yourdomain.com/auth/google/callback`
5. Save the **Client ID** and **Client Secret**

## 2. Authway Configuration

### Step 1: Update Environment Variables
Edit `.env.production` or set environment variables:

```bash
# Google OAuth Configuration
GOOGLE_CLIENT_ID=your-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret
```

### Step 2: Update Config File
Edit `configs/config.production.yaml`:

```yaml
google:
  enabled: true
  client_id: "${GOOGLE_CLIENT_ID}"
  client_secret: "${GOOGLE_CLIENT_SECRET}"
  redirect_url: "https://auth.yourdomain.com/auth/google/callback"
```

### Step 3: Update CORS Settings
Ensure your frontend domains are allowed:

```yaml
cors:
  allowed_origins:
    - "https://yourdomain.com"
    - "https://app.yourdomain.com"
```

## 3. Testing Google OAuth

### Local Development URLs
For local testing, add these to Google OAuth credentials:
- Authorized origins: `http://localhost:3000`, `http://localhost:8080`
- Redirect URIs: `http://localhost:8080/auth/google/callback`

### Test Flow
1. Start Authway server with Google OAuth enabled
2. Navigate to login page with `?login_challenge=test`
3. Click "Google로 로그인" button
4. Complete Google OAuth flow
5. Verify user creation in database

## 4. Security Considerations

### Production Requirements
- Use HTTPS for all URLs
- Store secrets in environment variables, never in code
- Regularly rotate client secrets
- Monitor OAuth usage in Google Console
- Implement rate limiting for OAuth endpoints

### Hydra Integration
Google OAuth integrates with Ory Hydra:
1. User clicks Google login
2. Redirected to Google OAuth
3. Google callback creates/updates user
4. Hydra login request accepted
5. User redirected to consent flow
6. Final redirect to client application

## 5. Troubleshooting

### Common Issues
- **Redirect URI mismatch**: Check exact URL match in Google Console
- **Invalid client**: Verify Client ID and Secret
- **Consent screen**: Ensure OAuth consent screen is configured
- **CORS errors**: Add frontend domains to CORS configuration

### Debug Endpoints
- Test OAuth URL: `GET /auth/google/url`
- Health check: `GET /health`
- User profile: `GET /api/v1/profile/:id`

### Logs
Check server logs for OAuth flow debugging:
```bash
docker logs authway-server
```