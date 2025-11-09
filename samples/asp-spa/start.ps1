# ============================================================
# Authway SPA Sample - Local Development Startup Script
# ============================================================

param(
    [switch]$SkipSetup,
    [switch]$SkipBackend,
    [switch]$SkipFrontend
)

Write-Host ""
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  🚀 SPA Sample - Local Development" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

$ScriptDir = $PSScriptRoot

# Configuration
$AUTH_BACKEND = "http://localhost:8081"
$CENTRAL_API = "http://localhost:8080"
$HYDRA_PUBLIC = "http://localhost:4444"

# ============================================================
# 1. Service Health Check
# ============================================================

Write-Host "📡 Checking Authway services..." -ForegroundColor Yellow
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

Write-Host "  🔍 Hydra Public ($HYDRA_PUBLIC)... " -NoNewline
try {
    $response = Invoke-WebRequest -Uri "$HYDRA_PUBLIC/.well-known/openid-configuration" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓" -ForegroundColor Green
} catch {
    Write-Host "❌" -ForegroundColor Red
    $servicesHealthy = $false
}

Write-Host ""

if (-not $servicesHealthy) {
    Write-Host "❌ Authway services not running!" -ForegroundColor Red
    Write-Host ""
    Write-Host "Start services with:" -ForegroundColor Yellow
    Write-Host "  cd D:\data\Authway" -ForegroundColor Gray
    Write-Host "  .\start-dev.ps1" -ForegroundColor Gray
    Write-Host ""
    exit 1
}

Write-Host "✅ All Authway services running!" -ForegroundColor Green
Write-Host ""

# ============================================================
# 2. Client Setup
# ============================================================

if (-not $SkipSetup) {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
    Write-Host "  🔐 OAuth Client Setup" -ForegroundColor Yellow
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Yellow
    Write-Host ""

    $setupScript = Join-Path $ScriptDir "setup-client-local.ps1"
    if (Test-Path $setupScript) {
        & $setupScript
        if ($LASTEXITCODE -ne 0) {
            Write-Host "❌ Client setup failed!" -ForegroundColor Red
            exit 1
        }
    } else {
        Write-Host "⚠️  Setup script not found, skipping..." -ForegroundColor Yellow
    }
    Write-Host ""
} else {
    Write-Host "ℹ️  Skipping client setup (-SkipSetup)" -ForegroundColor Gray
    Write-Host ""
}

# ============================================================
# 3. Start Backend
# ============================================================

if (-not $SkipBackend) {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  🔧 Starting ASP.NET Backend" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    # Check if backend is already running
    Write-Host "  🔍 Checking if backend is already running... " -NoNewline
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:5222/health" -Method GET -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
        Write-Host "✓" -ForegroundColor Green
        Write-Host "  ℹ️  Backend already running on http://localhost:5222" -ForegroundColor Yellow
        Write-Host ""
    } catch {
        Write-Host "❌" -ForegroundColor Red
        Write-Host "  Starting new backend instance..." -ForegroundColor Cyan
        Write-Host ""

        $backendPath = Join-Path $ScriptDir "asp-backend\AuthwaySpaBackend"
        if (Test-Path $backendPath) {
            Write-Host "  Starting backend at http://localhost:5222..." -ForegroundColor Cyan

            Start-Process pwsh -ArgumentList "-NoExit", "-Command", "cd '$backendPath'; dotnet run" -WorkingDirectory $backendPath

            Write-Host "  ✓ Backend starting in new window..." -ForegroundColor Green
            Write-Host ""
            Start-Sleep -Seconds 2
        } else {
            Write-Host "  ❌ Backend not found at: $backendPath" -ForegroundColor Red
            exit 1
        }
    }
} else {
    Write-Host "ℹ️  Skipping backend (-SkipBackend)" -ForegroundColor Gray
    Write-Host ""
}

# ============================================================
# 4. Start Frontend
# ============================================================

if (-not $SkipFrontend) {
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host "  ⚛️  Starting React Frontend" -ForegroundColor Cyan
    Write-Host "═══════════════════════════════════════════" -ForegroundColor Cyan
    Write-Host ""

    $frontendPath = Join-Path $ScriptDir "asp-frontend"
    if (Test-Path $frontendPath) {
        Write-Host "Starting frontend at http://localhost:5173..." -ForegroundColor Cyan

        # Check if node_modules exists
        $nodeModulesPath = Join-Path $frontendPath "node_modules"
        if (-not (Test-Path $nodeModulesPath)) {
            Write-Host "Installing dependencies..." -ForegroundColor Yellow
            Push-Location $frontendPath
            pnpm install
            Pop-Location
        }

        Start-Process pwsh -ArgumentList "-NoExit", "-Command", "cd '$frontendPath'; pnpm dev" -WorkingDirectory $frontendPath

        Write-Host "✓ Frontend starting in new window..." -ForegroundColor Green
        Write-Host ""
    } else {
        Write-Host "❌ Frontend not found at: $frontendPath" -ForegroundColor Red
        exit 1
    }
} else {
    Write-Host "ℹ️  Skipping frontend (-SkipFrontend)" -ForegroundColor Gray
    Write-Host ""
}

# ============================================================
# 5. Summary
# ============================================================

Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
Write-Host "  ✅ SPA Sample Started!" -ForegroundColor Green
Write-Host "═══════════════════════════════════════════" -ForegroundColor Green
Write-Host ""
Write-Host "📌 Application URLs:" -ForegroundColor Cyan
Write-Host "  Frontend:     http://localhost:5173" -ForegroundColor White
Write-Host "  Backend API:  http://localhost:5222" -ForegroundColor White
Write-Host ""
Write-Host "📌 Authway Services:" -ForegroundColor Cyan
Write-Host "  Auth Backend: http://localhost:8081" -ForegroundColor Gray
Write-Host "  Central API:  http://localhost:8080" -ForegroundColor Gray
Write-Host "  Hydra OAuth:  http://localhost:4444" -ForegroundColor Gray
Write-Host "  Admin UI:     http://localhost:3000" -ForegroundColor Gray
Write-Host "  Login UI:     http://localhost:3001" -ForegroundColor Gray
Write-Host ""
Write-Host "💡 Usage:" -ForegroundColor Yellow
Write-Host "  1. Open http://localhost:5173 in browser" -ForegroundColor White
Write-Host "  2. Try 'Popup Login' 🪟 or 'Redirect Login' 🔄" -ForegroundColor White
Write-Host "  3. Explore Dynamic Claims 🎭 and Token Viewer 🎫" -ForegroundColor White
Write-Host "  4. Test protected API calls 🔌" -ForegroundColor White
Write-Host ""
Write-Host "💡 Flags:" -ForegroundColor Yellow
Write-Host "  -SkipSetup    : Skip OAuth client registration" -ForegroundColor Gray
Write-Host "  -SkipBackend  : Only start frontend" -ForegroundColor Gray
Write-Host "  -SkipFrontend : Only start backend" -ForegroundColor Gray
Write-Host ""
