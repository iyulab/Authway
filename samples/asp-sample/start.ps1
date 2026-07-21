# ============================================================
# ASP.NET Sample - Local Development Startup Script
# ============================================================
# This script starts the ASP.NET sample application in local
# development mode after verifying all required services
# ============================================================

param(
    [switch]$SkipSetup,
    [switch]$Production,
    [string]$Profile = "http"
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 ASP.NET Sample - Local Development" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Configuration
$AUTH_BACKEND = "http://localhost:8081"
$CENTRAL_API = "http://localhost:8080"
$HYDRA_PUBLIC = "http://localhost:4444"
$HYDRA_ADMIN = "http://localhost:4445"
$ADMIN_UI = "http://localhost:3000"
$LOGIN_UI = "http://localhost:3001"

# ============================================================
# 1. Service Health Check
# ============================================================

Write-Host "📡 Checking required services..." -ForegroundColor Yellow
Write-Host ""

$servicesHealthy = $true

# Check Auth Backend
Write-Host "  🔍 Auth Backend ($AUTH_BACKEND)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$AUTH_BACKEND/health" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

# Check Central API
Write-Host "  🔍 Central API ($CENTRAL_API)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$CENTRAL_API/health" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

# Check Hydra Public
Write-Host "  🔍 Hydra Public ($HYDRA_PUBLIC)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$HYDRA_PUBLIC/.well-known/openid-configuration" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

# Check Hydra Admin
Write-Host "  🔍 Hydra Admin ($HYDRA_ADMIN)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$HYDRA_ADMIN/health/ready" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

# Check Login UI (optional)
Write-Host "  🔍 Login UI ($LOGIN_UI)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri $LOGIN_UI -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "⚠️  (optional)" -ForegroundColor Yellow
}

Write-Host ""

if (-not $servicesHealthy) {
    Write-Host "❌ Some required services are not running!" -ForegroundColor Red
    Write-Host ""
    Write-Host "📝 To start all services:" -ForegroundColor Yellow
    Write-Host "   1. Navigate to Authway root: cd D:\data\Authway" -ForegroundColor Gray
    Write-Host "   2. Run: .\start-dev.ps1" -ForegroundColor Gray
    Write-Host ""
    Write-Host "   Or manually start Docker services:" -ForegroundColor Yellow
    Write-Host "   docker compose up -d" -ForegroundColor Gray
    Write-Host ""
    exit 1
}

Write-Host "✅ All required services are running!" -ForegroundColor Green
Write-Host ""

# ============================================================
# 2. Client Setup (if not skipped)
# ============================================================

if (-not $SkipSetup) {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
    Write-Host "  🔐 OAuth Client Setup" -ForegroundColor Yellow
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
    Write-Host ""

    $setupScript = Join-Path $PSScriptRoot "setup-client-local.ps1"

    if (Test-Path $setupScript) {
        Write-Host "Running client setup script..." -ForegroundColor Cyan
        & $setupScript

        if ($LASTEXITCODE -ne 0) {
            Write-Host ""
            Write-Host "❌ Client setup failed!" -ForegroundColor Red
            Write-Host "   You can skip setup with: .\start.ps1 -SkipSetup" -ForegroundColor Yellow
            Write-Host ""
            exit 1
        }

        Write-Host ""
    } else {
        Write-Host "⚠️  Setup script not found: $setupScript" -ForegroundColor Yellow
        Write-Host "   Continuing without client setup..." -ForegroundColor Gray
        Write-Host ""
    }
} else {
    Write-Host "ℹ️  Skipping client setup (use -SkipSetup:$false to enable)" -ForegroundColor Gray
    Write-Host ""
}

# ============================================================
# 3. Configuration Check
# ============================================================

Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
Write-Host "  ⚙️  Configuration Check" -ForegroundColor Yellow
Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
Write-Host ""

$configFile = if ($Production) {
    Join-Path $PSScriptRoot "appsettings.json"
} else {
    Join-Path $PSScriptRoot "appsettings.Development.json"
}

if (Test-Path $configFile) {
    Write-Host "✓ Configuration file found: $(Split-Path -Leaf $configFile)" -ForegroundColor Green

    try {
        $config = Get-Content $configFile | ConvertFrom-Json

        Write-Host ""
        Write-Host "📋 Current configuration:" -ForegroundColor Cyan
        Write-Host "  Server:       $($config.Authway.Server)" -ForegroundColor White
        Write-Host "  Domain:       $($config.Authway.Domain)" -ForegroundColor White
        Write-Host "  Client ID:    $($config.Authway.ClientId)" -ForegroundColor White
        Write-Host "  Environment:  $(if ($Production) { 'Production' } else { 'Development' })" -ForegroundColor White
        Write-Host ""
    } catch {
        Write-Host "⚠️  Could not parse configuration file" -ForegroundColor Yellow
        Write-Host ""
    }
} else {
    Write-Host "❌ Configuration file not found: $configFile" -ForegroundColor Red
    Write-Host ""
    exit 1
}

# ============================================================
# 4. Start ASP.NET Application
# ============================================================

Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 Starting ASP.NET Application" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Determine launch profile
$launchProfile = if ($Production) { "Production" } else { $Profile }

Write-Host "🔧 Launch Profile: $launchProfile" -ForegroundColor Yellow
Write-Host ""

# Build and run
Write-Host "Building application..." -ForegroundColor Cyan
dotnet build --configuration $(if ($Production) { 'Release' } else { 'Debug' }) --nologo

if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "❌ Build failed!" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
Write-Host "  ✅ Starting Application..." -ForegroundColor Green
Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
Write-Host ""
Write-Host "📌 Application URLs:" -ForegroundColor Cyan

if ($Production) {
    Write-Host "   http://localhost:5000" -ForegroundColor White
} else {
    if ($Profile -eq "https") {
        Write-Host "   https://localhost:5001" -ForegroundColor White
        Write-Host "   http://localhost:5000" -ForegroundColor White
    } else {
        Write-Host "   http://localhost:5000" -ForegroundColor White
    }
}

Write-Host ""
Write-Host "📌 Service URLs:" -ForegroundColor Cyan
Write-Host "   Admin Dashboard:  $ADMIN_UI" -ForegroundColor Gray
Write-Host "   Login UI:         $LOGIN_UI" -ForegroundColor Gray
Write-Host "   Auth Backend:     $AUTH_BACKEND" -ForegroundColor Gray
Write-Host "   Central API:      $CENTRAL_API" -ForegroundColor Gray
Write-Host "   Hydra (OAuth):    $HYDRA_PUBLIC" -ForegroundColor Gray
Write-Host ""
Write-Host "💡 Press Ctrl+C to stop" -ForegroundColor Yellow
Write-Host ""

# Run the application
dotnet run --launch-profile $launchProfile --no-build
