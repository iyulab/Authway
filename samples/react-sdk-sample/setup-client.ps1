# React SDK Sample - OAuth Client Registration
# Registers the React SDK Sample client with Authway

Write-Host "🚀 React SDK Sample - Client Registration" -ForegroundColor Cyan
Write-Host ""

# Configuration
$AUTHWAY_API = "http://localhost:8080"
$CLIENT_ID = "react-sdk-sample-client"
$CLIENT_SECRET = "react-sdk-sample-secret-change-in-production"
$REDIRECT_URIS = @(
    "http://localhost:5173",              # Default Vite port
    "http://localhost:5173/callback.html", # Popup callback
    "http://localhost:9004",              # Alternative port 1
    "http://localhost:9004/callback.html", # Popup callback
    "http://localhost:9005",              # Alternative port 2
    "http://localhost:9005/callback.html", # Popup callback
    "http://localhost:9006",              # Alternative port 3
    "http://localhost:9006/callback.html", # Popup callback
    "http://localhost:9007",              # Alternative port 4
    "http://localhost:9007/callback.html", # Popup callback
    "http://localhost:9008",              # Alternative port 5
    "http://localhost:9008/callback.html", # Popup callback
    "http://localhost:9009",              # Alternative port 6
    "http://localhost:9009/callback.html"  # Popup callback
)
$SERVICE_NAME = "React SDK Sample"

# Check if Authway is running
Write-Host "📡 Checking Authway API..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$AUTHWAY_API/health" -Method GET -UseBasicParsing -ErrorAction Stop
    $health = $response.Content | ConvertFrom-Json
    Write-Host "✓ Authway is running (version: $($health.version))" -ForegroundColor Green
} catch {
    Write-Host "❌ Authway API is not accessible at $AUTHWAY_API" -ForegroundColor Red
    Write-Host "   Please make sure Authway is running first." -ForegroundColor Red
    Write-Host "   Run: .\start-dev.ps1" -ForegroundColor Yellow
    exit 1
}

Write-Host ""

# Fetch tenants
Write-Host "🔍 Fetching tenants..." -ForegroundColor Yellow
try {
    $tenantsResponse = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/tenants" -Method GET -UseBasicParsing -ErrorAction Stop
    $tenants = $tenantsResponse.Content | ConvertFrom-Json

    if ($tenants.Count -eq 0) {
        Write-Host "❌ No tenants found. Creating default tenant..." -ForegroundColor Yellow
        # You might want to create a default tenant here
    }

    # Use first tenant (or you can specify a tenant slug)
    $tenant = $tenants[0]
    $TENANT_ID = $tenant.id

    Write-Host "✓ Using tenant: $($tenant.name) ($($tenant.slug))" -ForegroundColor Green
    Write-Host "  Tenant ID: $TENANT_ID" -ForegroundColor Gray
} catch {
    Write-Host "❌ Failed to fetch tenants" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Register client in Authway database
Write-Host "🔐 Registering client in Authway..." -ForegroundColor Yellow

$clientData = @{
    tenant_id = $TENANT_ID
    client_id = $CLIENT_ID
    client_secret = $CLIENT_SECRET
    name = $SERVICE_NAME
    description = "React SDK Sample Application - demonstrates @authway/client and @authway/react usage"
    redirect_uris = $REDIRECT_URIS
    grant_types = @("authorization_code", "refresh_token")
    scopes = @("openid", "profile", "email")
    public = $true  # SPA must be public client (no client_secret in browser)
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/clients" -Method POST -ContentType "application/json" -Body $clientData -UseBasicParsing -ErrorAction Stop
    Write-Host "✓ Client registered in Authway database" -ForegroundColor Green
} catch {
    $errorMessage = $_.Exception.Message
    if ($errorMessage -match "duplicate" -or $errorMessage -match "409" -or $errorMessage -match "already exists") {
        Write-Host "ℹ Client already exists in Authway" -ForegroundColor Yellow
    } else {
        Write-Host "❌ Failed to register client in Authway" -ForegroundColor Red
        Write-Host "   Error: $errorMessage" -ForegroundColor Gray
    }
}

# Register client in Hydra
Write-Host "🔐 Registering client in Ory Hydra..." -ForegroundColor Yellow

$hydraData = @{
    client_id = $CLIENT_ID
    client_name = $SERVICE_NAME
    grant_types = @("authorization_code", "refresh_token")
    response_types = @("code")
    redirect_uris = $REDIRECT_URIS
    post_logout_redirect_uris = $REDIRECT_URIS  # Allow logout to same origins
    scope = "openid profile email"
    token_endpoint_auth_method = "none"  # Public client - no authentication
    access_token_strategy = "jwt"
    skip_consent = $true
    skip_logout_consent = $true
} | ConvertTo-Json

# Delete existing client first
try {
    $null = Invoke-WebRequest -Uri "http://localhost:4445/admin/clients/$CLIENT_ID" -Method DELETE -UseBasicParsing -ErrorAction Stop
    Write-Host "ℹ Deleted existing Hydra client" -ForegroundColor Gray
} catch {
    # Client doesn't exist, continue
}

# Register new client
try {
    $null = Invoke-WebRequest -Uri "http://localhost:4445/admin/clients" -Method POST -ContentType "application/json" -Body $hydraData -UseBasicParsing -ErrorAction Stop
    Write-Host "✓ Client registered in Ory Hydra" -ForegroundColor Green
} catch {
    Write-Host "⚠ Warning: Failed to register in Hydra" -ForegroundColor Yellow
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Gray
}

Write-Host ""
Write-Host "✅ Client registration complete!" -ForegroundColor Green
Write-Host ""
Write-Host "📝 Client Configuration:" -ForegroundColor Cyan
Write-Host "  Client ID:     $CLIENT_ID" -ForegroundColor White
Write-Host "  Redirect URIs:" -ForegroundColor White
foreach ($uri in $REDIRECT_URIS) {
    Write-Host "    - $uri" -ForegroundColor Gray
}
Write-Host "  Tenant:        $($tenant.name) ($TENANT_ID)" -ForegroundColor White
Write-Host ""
Write-Host "🚀 Next steps:" -ForegroundColor Cyan
Write-Host "  1. Build the SDK packages:" -ForegroundColor White
Write-Host "     cd packages\client && pnpm build" -ForegroundColor Gray
Write-Host "     cd ..\react && pnpm build" -ForegroundColor Gray
Write-Host ""
Write-Host "  2. Start the sample app:" -ForegroundColor White
Write-Host "     cd samples\react-sdk-sample" -ForegroundColor Gray
Write-Host "     pnpm install" -ForegroundColor Gray
Write-Host "     pnpm dev" -ForegroundColor Gray
Write-Host ""
Write-Host "  3. Open http://localhost:5173 (or any port Vite assigns)" -ForegroundColor White
Write-Host ""

# Exit with success code
exit 0
