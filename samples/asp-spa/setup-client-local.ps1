# ============================================================
# Authway SPA Sample - Local Development Client Registration
# ============================================================

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🔐 SPA Sample - Local Client Registration" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Configuration for local development
$AUTH_BACKEND = "http://localhost:8081"
$CENTRAL_API = "http://localhost:8080"
$HYDRA_ADMIN = "http://localhost:4445"

# Client configuration
$CLIENT_ID = "authway_spa_sample_local"
$CLIENT_NAME = "Authway SPA Sample (Local Development)"
$REDIRECT_URIS = @(
    "http://localhost:5173",
    "http://localhost:5173/",
    "http://localhost:5173/callback.html",
    "http://localhost:5174",
    "http://localhost:5174/",
    "http://localhost:5174/callback.html",
    "http://localhost:4173",
    "http://localhost:4173/"
)

# Service Health Check
Write-Host "📡 Checking required services..." -ForegroundColor Yellow
Write-Host ""

$servicesHealthy = $true

Write-Host "  🔍 Auth Backend ($AUTH_BACKEND)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$AUTH_BACKEND/health" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

Write-Host "  🔍 Central API ($CENTRAL_API)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$CENTRAL_API/health" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

Write-Host "  🔍 Hydra Admin ($HYDRA_ADMIN)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$HYDRA_ADMIN/health/ready" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

Write-Host ""

if (-not $servicesHealthy) {
    Write-Host "❌ Services not running! Start with: .\start-dev.ps1" -ForegroundColor Red
    exit 1
}

# Fetch Tenants
Write-Host "🔍 Fetching tenants..." -ForegroundColor Yellow
try {
    $tenantsResponse = Invoke-WebRequest -Uri "$CENTRAL_API/api/v1/tenants" -Method GET -UseBasicParsing -ErrorAction Stop
    $tenantsData = $tenantsResponse.Content | ConvertFrom-Json
    if ($tenantsData.Count -eq 0) {
        Write-Host "❌ No tenant found" -ForegroundColor Red
        exit 1
    }
    $tenant = $tenantsData[0]
    Write-Host "✓ Using tenant: $($tenant.name)" -ForegroundColor Green
} catch {
    Write-Host "❌ Failed to fetch tenants" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Register in Central API
Write-Host "📝 Registering client in Central API..." -ForegroundColor Yellow
$clientData = @{
    tenant_id = $tenant.id
    client_id = $CLIENT_ID
    client_secret = "not-used-for-public-client"  # Required for fixed client_id, but not used in OAuth flow
    name = $CLIENT_NAME
    description = "Local development SPA sample"
    redirect_uris = $REDIRECT_URIS
    grant_types = @("authorization_code", "refresh_token")
    scopes = @("openid", "profile", "email")
    public = $true
}

try {
    $response = Invoke-WebRequest -Uri "$CENTRAL_API/api/v1/clients" -Method POST -ContentType "application/json" -Body ($clientData | ConvertTo-Json) -UseBasicParsing -ErrorAction Stop
    Write-Host "✓ Client created" -ForegroundColor Green
} catch {
    if ($_.Exception.Message -match "duplicate|409") {
        Write-Host "✓ Client already exists" -ForegroundColor Green
    } else {
        Write-Host "❌ Failed: $($_.Exception.Message)" -ForegroundColor Red
        exit 1
    }
}

Write-Host ""

# Register in Hydra
Write-Host "📝 Registering client in Hydra..." -ForegroundColor Yellow
try { Invoke-WebRequest -Uri "$HYDRA_ADMIN/admin/clients/$CLIENT_ID" -Method DELETE -UseBasicParsing -ErrorAction SilentlyContinue | Out-Null } catch {}

$hydraData = @{
    client_id = $CLIENT_ID
    client_name = $CLIENT_NAME
    grant_types = @("authorization_code", "refresh_token")
    response_types = @("code")
    redirect_uris = $REDIRECT_URIS
    post_logout_redirect_uris = $REDIRECT_URIS  # Allow logout redirect to same URIs
    scope = "openid profile email offline_access"
    audience = @("api")  # Whitelist 'api' audience for backend
    token_endpoint_auth_method = "none"
    access_token_strategy = "jwt"
    skip_consent = $true
    skip_logout_consent = $true
}

try {
    $response = Invoke-WebRequest -Uri "$HYDRA_ADMIN/admin/clients" -Method POST -ContentType "application/json" -Body ($hydraData | ConvertTo-Json) -UseBasicParsing -ErrorAction Stop
    Write-Host "✓ Client registered" -ForegroundColor Green
} catch {
    Write-Host "❌ Failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "✅ Registration complete!" -ForegroundColor Green
Write-Host ""
Write-Host "Client ID: $CLIENT_ID" -ForegroundColor Cyan
Write-Host "Redirect:  http://localhost:5173" -ForegroundColor Cyan
Write-Host ""

# Explicitly exit with success code
exit 0
