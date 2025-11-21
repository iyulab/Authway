# ASP.NET Sample - Local Development Client Registration Script
# This script registers the ASP.NET sample OAuth client with local Authway

Write-Host "🔐 ASP.NET Sample - Local Development Client Registration" -ForegroundColor Cyan
Write-Host ""

# Configuration for local development
$AUTH_BACKEND = "http://localhost:8081"
$CENTRAL_API = "http://localhost:8080"
$HYDRA_ADMIN = "http://localhost:4445"

# Client configuration
$CLIENT_ID = "asp-sample-client"
$CLIENT_SECRET = "asp-sample-secret-change-in-production"
$REDIRECT_URIS = @(
    "http://localhost:5000/signin-oidc",
    "https://localhost:5001/signin-oidc",
    "http://localhost:5000/callback.html",
    "https://localhost:5001/callback.html"
)
$CLIENT_NAME = "ASP.NET Sample Application (Local)"

Write-Host "📡 Checking services..." -ForegroundColor Yellow

# Check Auth Backend
try {
    $response = Invoke-WebRequest -Uri "$AUTH_BACKEND/health" -Method GET -UseBasicParsing -ErrorAction Stop
    Write-Host "✓ Auth Backend is running" -ForegroundColor Green
} catch {
    Write-Host "❌ Auth Backend is not accessible at $AUTH_BACKEND" -ForegroundColor Red
    Write-Host "   Please start services: .\start-dev.ps1" -ForegroundColor Yellow
    exit 1
}

# Check Central API
try {
    $response = Invoke-WebRequest -Uri "$CENTRAL_API/health" -Method GET -UseBasicParsing -ErrorAction Stop
    $health = $response.Content | ConvertFrom-Json
    Write-Host "✓ Central API is running (version: $($health.version))" -ForegroundColor Green
} catch {
    Write-Host "❌ Central API is not accessible at $CENTRAL_API" -ForegroundColor Red
    Write-Host "   Please start services: .\start-dev.ps1" -ForegroundColor Yellow
    exit 1
}

Write-Host ""

# Fetch tenants
Write-Host "🔍 Fetching tenants..." -ForegroundColor Yellow
try {
    $tenantsResponse = Invoke-WebRequest -Uri "$CENTRAL_API/api/v1/tenants" -Method GET -UseBasicParsing -ErrorAction Stop
    $tenantsData = $tenantsResponse.Content | ConvertFrom-Json

    if ($tenantsData.Count -eq 0) {
        Write-Host "❌ No tenant found. Creating default tenant..." -ForegroundColor Yellow
        Write-Host "   Please restart services to create default tenant" -ForegroundColor Yellow
        exit 1
    }

    # Use the first tenant
    $tenant = $tenantsData[0]
    Write-Host "✓ Using tenant: $($tenant.name) (ID: $($tenant.id))" -ForegroundColor Green
} catch {
    Write-Host "❌ Failed to fetch tenants" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Check if client already exists in Central API
Write-Host "🔍 Checking if client exists..." -ForegroundColor Yellow
$clientExists = $false
try {
    $checkResponse = Invoke-WebRequest -Uri "$CENTRAL_API/api/v1/clients" -Method GET -UseBasicParsing -ErrorAction Stop
    $existingClients = $checkResponse.Content | ConvertFrom-Json
    $existingClient = $existingClients.clients | Where-Object { $_.client_id -eq $CLIENT_ID }
    if ($existingClient) {
        $clientExists = $true
        Write-Host "✓ Client already exists" -ForegroundColor Green
    }
} catch {
    Write-Host "⚠ Failed to check existing clients, will attempt to create" -ForegroundColor Yellow
}

Write-Host ""

# Register in Central API database (only if doesn't exist)
if (-not $clientExists) {
    Write-Host "📝 Registering client in Central API..." -ForegroundColor Yellow

    $clientData = @{
        tenant_id = $tenant.id
        client_id = $CLIENT_ID
        client_secret = $CLIENT_SECRET
        name = $CLIENT_NAME
        description = "ASP.NET Core sample application for local development testing"
        redirect_uris = $REDIRECT_URIS
        grant_types = @("authorization_code", "refresh_token")
        scopes = @("openid", "profile", "email")
        public = $false
    }

    $jsonData = $clientData | ConvertTo-Json

    try {
        $response = Invoke-WebRequest -Uri "$CENTRAL_API/api/v1/clients" -Method POST -ContentType "application/json" -Body $jsonData -UseBasicParsing -ErrorAction Stop
        $client = $response.Content | ConvertFrom-Json
        Write-Host "✓ Client created in Central API" -ForegroundColor Green
    } catch {
        $errorMessage = $_.Exception.Message
        if ($errorMessage -match "duplicate" -or $errorMessage -match "409" -or $errorMessage -match "already exists") {
            Write-Host "✓ Client already exists (duplicate key)" -ForegroundColor Green
            $clientExists = $true
        } else {
            Write-Host "❌ Failed to register client" -ForegroundColor Red
            Write-Host "   Error: $errorMessage" -ForegroundColor Gray
            exit 1
        }
    }
} else {
    Write-Host "ℹ Skipping registration (already exists)" -ForegroundColor Gray
}

Write-Host ""

# Register in Hydra
Write-Host "📝 Registering client in Hydra..." -ForegroundColor Yellow

$hydraData = @{
    client_id = $CLIENT_ID
    client_secret = $CLIENT_SECRET
    client_name = $CLIENT_NAME
    grant_types = @("authorization_code", "refresh_token")
    response_types = @("code")
    redirect_uris = $REDIRECT_URIS
    post_logout_redirect_uris = @("http://localhost:5000", "https://localhost:5001")  # OIDC RP-Initiated Logout
    scope = "openid profile email offline_access"  # Added offline_access for refresh tokens
    token_endpoint_auth_method = "client_secret_post"  # ASP.NET uses client_secret_post
    access_token_strategy = "jwt"  # Issue JWT access tokens instead of opaque tokens
    skip_consent = $true  # Skip consent screen for trusted first-party clients
    skip_logout_consent = $true  # Skip consent on logout
}
$hydraJson = $hydraData | ConvertTo-Json

# Try to delete existing client first
try {
    $deleteResponse = Invoke-WebRequest -Uri "$HYDRA_ADMIN/admin/clients/$CLIENT_ID" -Method DELETE -UseBasicParsing -ErrorAction Stop
    Write-Host "ℹ Deleted existing Hydra client" -ForegroundColor Gray
} catch {
    # Client doesn't exist or delete failed, continue
}

# Register new client
try {
    $hydraResponse = Invoke-WebRequest -Uri "$HYDRA_ADMIN/admin/clients" -Method POST -ContentType "application/json" -Body $hydraJson -UseBasicParsing -ErrorAction Stop
    Write-Host "✓ Client registered in Hydra" -ForegroundColor Green
} catch {
    $hydraError = $_.Exception.Message
    Write-Host "❌ Failed to register client in Hydra" -ForegroundColor Red
    Write-Host "   Error: $hydraError" -ForegroundColor Gray
    Write-Host "   Please make sure Hydra is running at $HYDRA_ADMIN" -ForegroundColor Yellow
    exit 1
}

Write-Host ""
Write-Host "✅ Client registration complete!" -ForegroundColor Green
Write-Host ""
Write-Host "📊 Client Configuration:" -ForegroundColor Cyan
Write-Host "  Client ID:      $CLIENT_ID" -ForegroundColor White
Write-Host "  Redirect URIs:  http://localhost:5000/signin-oidc" -ForegroundColor White
Write-Host "                  https://localhost:5001/signin-oidc" -ForegroundColor White
Write-Host "  Tenant:         $($tenant.name)" -ForegroundColor White
Write-Host ""
Write-Host "📝 Next steps:" -ForegroundColor Cyan
Write-Host "  1. Make sure appsettings.Development.json exists with local configuration" -ForegroundColor White
Write-Host "  2. Run the ASP.NET application:" -ForegroundColor White
Write-Host "     cd samples/asp-sample" -ForegroundColor Gray
Write-Host "     dotnet run" -ForegroundColor Gray
Write-Host "  3. Navigate to http://localhost:5000" -ForegroundColor White
Write-Host "  4. Click 'Login with Authway' to test OAuth flow" -ForegroundColor White
Write-Host ""
Write-Host "🔧 Required services:" -ForegroundColor Cyan
Write-Host "  ✓ Auth Backend: $AUTH_BACKEND" -ForegroundColor Green
Write-Host "  ✓ Central API:  $CENTRAL_API" -ForegroundColor Green
Write-Host "  ✓ Hydra Admin:  $HYDRA_ADMIN" -ForegroundColor Green
Write-Host "  ✓ Hydra Public: http://localhost:4444" -ForegroundColor Green
Write-Host ""
