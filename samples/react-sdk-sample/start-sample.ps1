# React SDK Sample - Quick Start Script
# This script builds the SDK packages and starts the sample app

Write-Host "🚀 Authway React SDK Sample - Quick Start" -ForegroundColor Cyan
Write-Host ""

# Check if Authway is running
Write-Host "📡 Checking Authway..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/health" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
    Write-Host "✓ Authway is running" -ForegroundColor Green
} catch {
    Write-Host "❌ Authway is not running!" -ForegroundColor Red
    Write-Host "   Please start Authway first: .\start-dev.ps1" -ForegroundColor Yellow
    exit 1
}

Write-Host ""

# Register OAuth client
Write-Host "🔐 Registering OAuth client..." -ForegroundColor Yellow
$setupOutput = & ".\setup-client.ps1" 2>&1
$setupExitCode = $LASTEXITCODE

if ($setupExitCode -and $setupExitCode -ne 0) {
    Write-Host "❌ Client registration failed with exit code: $setupExitCode" -ForegroundColor Red
    exit 1
}

Write-Host "✓ Client registration successful" -ForegroundColor Green

Write-Host ""

# Install SDK package dependencies
Write-Host "📦 Installing SDK dependencies..." -ForegroundColor Yellow
Write-Host ""

Write-Host "  Installing @authway/client dependencies..." -ForegroundColor Gray
Push-Location "..\..\packages\client"
pnpm install
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    Write-Host "❌ Failed to install @authway/client dependencies" -ForegroundColor Red
    exit 1
}
Pop-Location

Write-Host "  Installing @authway/react dependencies..." -ForegroundColor Gray
Push-Location "..\..\packages\react"
pnpm install
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    Write-Host "❌ Failed to install @authway/react dependencies" -ForegroundColor Red
    exit 1
}
Pop-Location

Write-Host "✓ SDK dependencies installed" -ForegroundColor Green
Write-Host ""

# Build SDK packages
Write-Host "🔨 Building SDK packages..." -ForegroundColor Yellow
Write-Host ""

Write-Host "  Building @authway/client..." -ForegroundColor Gray
Push-Location "..\..\packages\client"
pnpm build
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    Write-Host "❌ Failed to build @authway/client" -ForegroundColor Red
    exit 1
}
Pop-Location

Write-Host "  Building @authway/react..." -ForegroundColor Gray
Push-Location "..\..\packages\react"
pnpm build
if ($LASTEXITCODE -ne 0) {
    Pop-Location
    Write-Host "❌ Failed to build @authway/react" -ForegroundColor Red
    exit 1
}
Pop-Location

Write-Host "✓ SDK packages built successfully" -ForegroundColor Green
Write-Host ""

# Install sample dependencies
Write-Host "📦 Installing sample dependencies..." -ForegroundColor Yellow
Write-Host "   Running: pnpm install --force" -ForegroundColor Gray
pnpm install --force
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Failed to install dependencies" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Sample dependencies installed" -ForegroundColor Green

Write-Host ""
Write-Host "✅ Setup complete!" -ForegroundColor Green
Write-Host ""
Write-Host "🎯 Starting development server..." -ForegroundColor Cyan
Write-Host "   URL: http://localhost:9004" -ForegroundColor White
Write-Host ""
Write-Host "   Press Ctrl+C to stop" -ForegroundColor Gray
Write-Host ""

# Start development server
pnpm dev
