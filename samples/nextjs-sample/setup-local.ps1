# Next.js Sample - Local Development Setup Script
# This script registers the OAuth client and starts the development server

Write-Host "⚡ Next.js Sample - Local Development Setup" -ForegroundColor Cyan
Write-Host ""

# Configuration
$AUTHWAY_API = "http://localhost:8080"
$CLIENT_ID = "nextjs-sample-client"
$SERVICE_NAME = "Next.js Sample"
$REDIRECT_URIS = @(
    "http://localhost:3100",              # Default port
    "http://localhost:3100/callback"      # OAuth callback route (handles both redirect & popup)
)

# Check if Authway is running
Write-Host "📡 Checking Authway API..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "$AUTHWAY_API/health" -Method GET -UseBasicParsing -ErrorAction Stop
    $health = $response.Content | ConvertFrom-Json
    Write-Host "✓ Authway is running (version: $($health.version))" -ForegroundColor Green
} catch {
    Write-Host "❌ Authway API is not accessible at $AUTHWAY_API" -ForegroundColor Red
    Write-Host "   Please make sure Authway is running first." -ForegroundColor Red
    Write-Host "   Run: .\start-dev.ps1 (from project root)" -ForegroundColor Yellow
    exit 1
}

Write-Host ""

# Fetch tenants
Write-Host "🔍 Fetching tenants..." -ForegroundColor Yellow
try {
    $tenantsResponse = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/tenants" -Method GET -UseBasicParsing -ErrorAction Stop
    $tenants = $tenantsResponse.Content | ConvertFrom-Json

    if ($tenants.Count -eq 0) {
        Write-Host "❌ No tenants found. Please create a tenant first." -ForegroundColor Red
        exit 1
    }

    # Use first tenant
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
    name = $SERVICE_NAME
    description = "Next.js App Router sample application - demonstrates @authway/client usage with Next.js 15"
    redirect_uris = $REDIRECT_URIS
    grant_types = @("authorization_code", "refresh_token")
    scopes = @("openid", "profile", "email")
    public = $true  # SPA - public client
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
    post_logout_redirect_uris = $REDIRECT_URIS
    scope = "openid profile email"
    token_endpoint_auth_method = "none"  # Public client
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

# Check if dependencies are installed
$packageJsonPath = Join-Path $PSScriptRoot "package.json"
$nodeModulesPath = Join-Path $PSScriptRoot "node_modules"

if (-not (Test-Path $nodeModulesPath)) {
    Write-Host "📦 Installing dependencies..." -ForegroundColor Yellow
    Push-Location $PSScriptRoot
    pnpm install
    Pop-Location
    Write-Host "✓ Dependencies installed" -ForegroundColor Green
    Write-Host ""
}

Write-Host "🚀 Starting Next.js development server..." -ForegroundColor Cyan
Write-Host "   URL: http://localhost:3100" -ForegroundColor White
Write-Host ""
Write-Host "Press Ctrl+C to stop the server" -ForegroundColor Gray
Write-Host ""

# Start the development server
Push-Location $PSScriptRoot
pnpm dev
Pop-Location
