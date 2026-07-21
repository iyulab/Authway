# Authway Development Startup Script
# Starts backend and frontend in separate terminals

Write-Host "🚀 Starting Authway Development Environment..." -ForegroundColor Cyan
Write-Host ""

# Function to kill process on specific port
function Kill-PortProcess {
    param([int]$Port)

    $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    if ($connections) {
        foreach ($conn in $connections) {
            $processId = $conn.OwningProcess
            $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
            if ($process) {
                # Don't kill Docker processes
                if ($process.ProcessName -like "*docker*" -or $process.ProcessName -like "com.docker.*") {
                    Write-Host "  ⚠️  Skipping Docker process: $($process.ProcessName) (PID: $processId) on port $Port" -ForegroundColor Yellow
                    Write-Host "     Please stop this manually if needed" -ForegroundColor Gray
                    return $false
                }

                Write-Host "  Killing $($process.ProcessName) (PID: $processId) on port $Port" -ForegroundColor Gray
                Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
                return $true
            }
        }
    }
    return $false
}

# Function to ensure port is free
function Ensure-PortFree {
    param([int]$Port, [string]$ServiceName)

    $maxAttempts = 5
    $attempt = 0

    while ($attempt -lt $maxAttempts) {
        $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
        if (-not $conn) {
            Write-Host "✓ Port $Port is free for $ServiceName" -ForegroundColor Green
            return $true
        }

        Write-Host "⚠️  Port $Port is still in use, attempting to free it... (Attempt $($attempt + 1)/$maxAttempts)" -ForegroundColor Yellow
        Kill-PortProcess -Port $Port
        Start-Sleep -Seconds 1
        $attempt++
    }

    Write-Host "❌ Failed to free port $Port for $ServiceName after $maxAttempts attempts" -ForegroundColor Red
    return $false
}

# Clean up any existing processes on required ports
Write-Host "🧹 Cleaning up ports for Authway services..." -ForegroundColor Yellow
$ports = @(3000, 3001, 8080, 8081, 9001, 9002, 9003)
$killedProcesses = 0

foreach ($port in $ports) {
    if (Kill-PortProcess -Port $port) {
        $killedProcesses++
    }
}

if ($killedProcesses -gt 0) {
    Write-Host "✓ Cleaned up $killedProcesses process(es)" -ForegroundColor Green
    Write-Host "⏳ Waiting for ports to be released..." -ForegroundColor Yellow
    Start-Sleep -Seconds 2
} else {
    Write-Host "✓ All ports are already free" -ForegroundColor Green
}

Write-Host ""

# Check if Docker is running
Write-Host "📦 Checking Docker..." -ForegroundColor Yellow
try {
    $dockerStatus = docker ps 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ Docker is not running. Please start Docker Desktop first." -ForegroundColor Red
        exit 1
    }
    Write-Host "✓ Docker is running" -ForegroundColor Green
} catch {
    Write-Host "❌ Docker is not installed or not running" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Start infrastructure services
Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "📦 Starting Infrastructure Services" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""

Write-Host "🐘 Starting PostgreSQL, Redis, and MailHog..." -ForegroundColor Yellow
docker compose up -d postgres redis mailhog

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Failed to start infrastructure services" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Infrastructure services started" -ForegroundColor Green
Write-Host ""

# Wait for PostgreSQL before starting Hydra migration
Write-Host "⏳ Waiting for PostgreSQL to initialize..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# Start Ory Hydra (migration will run first due to depends_on)
Write-Host "🌊 Starting Ory Hydra OAuth Server..." -ForegroundColor Yellow
docker compose up -d hydra-migrate hydra

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Failed to start Ory Hydra" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Ory Hydra started" -ForegroundColor Green
Write-Host ""

# Wait for PostgreSQL to be healthy
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "🔍 Database Health Check" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""
Write-Host "⏳ Waiting for PostgreSQL to be ready..." -ForegroundColor Yellow
$maxAttempts = 30
$attempt = 0
while ($attempt -lt $maxAttempts) {
    $postgresHealth = docker inspect --format='{{.State.Health.Status}}' authway-postgres 2>$null
    if ($postgresHealth -eq "healthy") {
        Write-Host "✓ PostgreSQL is healthy and ready" -ForegroundColor Green
        break
    }
    Start-Sleep -Seconds 1
    $attempt++
    if ($attempt % 5 -eq 0) {
        Write-Host "  [$attempt/$maxAttempts] PostgreSQL status: $postgresHealth" -ForegroundColor Gray
    }
}

if ($attempt -ge $maxAttempts) {
    Write-Host ""
    Write-Host "❌ PostgreSQL failed to start within $maxAttempts seconds" -ForegroundColor Red
    Write-Host "   Check Docker logs: docker logs authway-postgres" -ForegroundColor Yellow
    exit 1
}

Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "🔄 Database Migration Strategy" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""
Write-Host "💡 Authway uses automatic migrations:" -ForegroundColor Yellow
Write-Host "   1. Backend detects database schema version" -ForegroundColor White
Write-Host "   2. Applies pending migrations automatically on startup" -ForegroundColor White
Write-Host "   3. Logs migration progress to backend console" -ForegroundColor White
Write-Host ""
Write-Host "✅ No manual migration required - migrations run with backend startup" -ForegroundColor Green
Write-Host ""

# Verify all ports are free before starting
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "🔧 Verifying Ports" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""

if (-not (Ensure-PortFree -Port 8080 -ServiceName "Backend API")) {
    Write-Host "❌ Cannot start backend - port 8080 is in use" -ForegroundColor Red
    exit 1
}
if (-not (Ensure-PortFree -Port 3000 -ServiceName "Admin Dashboard")) {
    Write-Host "❌ Cannot start Admin Dashboard - port 3000 is in use" -ForegroundColor Red
    exit 1
}
if (-not (Ensure-PortFree -Port 3001 -ServiceName "Login UI")) {
    Write-Host "❌ Cannot start Login UI - port 3001 is in use" -ForegroundColor Red
    exit 1
}
if (-not (Ensure-PortFree -Port 8081 -ServiceName "Auth Backend")) {
    Write-Host "❌ Cannot start Auth Backend - port 8081 is in use" -ForegroundColor Red
    exit 1
}

# Define paths
$backendPath = Join-Path $PSScriptRoot "apps\central\api"
$frontendPath = Join-Path $PSScriptRoot "apps\central\admin"
$loginUiPath = Join-Path $PSScriptRoot "apps\branding\auth-ui"
$authBackendPath = Join-Path $PSScriptRoot "apps\branding\auth-api"

Write-Host ""

# Check and install frontend dependencies
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "📦 Checking Dependencies" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""

if (-not (Test-Path "$frontendPath\node_modules")) {
    Write-Host "📦 Installing Admin Dashboard dependencies (first time)..." -ForegroundColor Yellow
    Push-Location $frontendPath
    npm install
    Pop-Location
}

if (-not (Test-Path "$loginUiPath\node_modules")) {
    Write-Host "📦 Installing Login UI dependencies (first time)..." -ForegroundColor Yellow
    Push-Location $loginUiPath
    npm install
    Pop-Location
}

Write-Host "✓ All dependencies ready" -ForegroundColor Green
Write-Host ""

# Start all 4 services in Windows Terminal with tabs
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host "🚀 Starting Development Servers" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""
Write-Host "📂 Paths:" -ForegroundColor Gray
Write-Host "   Backend:        $backendPath" -ForegroundColor Gray
Write-Host "   Admin Dashboard: $frontendPath" -ForegroundColor Gray
Write-Host "   Login UI:       $loginUiPath" -ForegroundColor Gray
Write-Host "   Auth Backend:   $authBackendPath" -ForegroundColor Gray
Write-Host ""
Write-Host "🖥️  Launching Windows Terminal with 4 tabs..." -ForegroundColor Yellow
Write-Host ""

# Create a temporary script for each tab to avoid escaping issues
$backendScript = @"
Write-Host '🔧 Backend API - Port 8080' -ForegroundColor Cyan
Write-Host ''
go run ./cmd/
"@

$adminScript = @"
Write-Host '🎨 Admin Dashboard - Port 3000' -ForegroundColor Cyan
Write-Host ''
npm run dev
"@

$loginScript = @"
Write-Host '🔐 Login UI - Port 3001' -ForegroundColor Cyan
Write-Host ''
npm run dev
"@

$authScript = @"
Write-Host '🔑 Auth Backend - Port 8081' -ForegroundColor Cyan
Write-Host ''
go run cmd/main.go
"@

# Save scripts to temp files
$tempDir = [System.IO.Path]::GetTempPath()
$backendScriptPath = Join-Path $tempDir "authway-backend.ps1"
$adminScriptPath = Join-Path $tempDir "authway-admin.ps1"
$loginScriptPath = Join-Path $tempDir "authway-login.ps1"
$authScriptPath = Join-Path $tempDir "authway-auth.ps1"

$backendScript | Out-File -FilePath $backendScriptPath -Encoding UTF8
$adminScript | Out-File -FilePath $adminScriptPath -Encoding UTF8
$loginScript | Out-File -FilePath $loginScriptPath -Encoding UTF8
$authScript | Out-File -FilePath $authScriptPath -Encoding UTF8

# Launch Windows Terminal with 4 tabs using script files
# Using cmd /c to properly handle the semicolon separators
$wtCmd = "wt --title `"Authway Dev`" -d `"$backendPath`" powershell -NoExit -File `"$backendScriptPath`" ; new-tab --title `"Admin Dashboard`" -d `"$frontendPath`" powershell -NoExit -File `"$adminScriptPath`" ; new-tab --title `"Login UI`" -d `"$loginUiPath`" powershell -NoExit -File `"$loginScriptPath`" ; new-tab --title `"Auth Backend`" -d `"$authBackendPath`" powershell -NoExit -File `"$authScriptPath`""

cmd /c $wtCmd

# Wait for backend port to be listening
Write-Host "⏳ Waiting for backend (port 8080) to start..." -ForegroundColor Yellow
$maxAttempts = 30
$attempt = 0
$backendReady = $false

while ($attempt -lt $maxAttempts) {
    $attempt++

    # Check if port 8080 is listening
    $portCheck = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
    if ($portCheck) {
        Write-Host "✓ Backend port is listening (took ~$attempt seconds)" -ForegroundColor Green
        $backendReady = $true
        Start-Sleep -Seconds 1
        break
    }

    if ($attempt % 5 -eq 0) {
        Write-Host "  [$attempt/$maxAttempts] Waiting..." -ForegroundColor Gray
    }
    Start-Sleep -Seconds 1
}

if (-not $backendReady) {
    Write-Host ""
    Write-Host "⚠️  Backend did not start after $maxAttempts seconds" -ForegroundColor Yellow
    Write-Host "   Check the Backend API tab in Windows Terminal for errors" -ForegroundColor Gray
    Write-Host "   Skipping client registration (you can run .\samples\setup-clients.ps1 manually)" -ForegroundColor Gray
}
Write-Host ""

# Setup sample tenants and clients
if ($backendReady) {
    Write-Host "🔧 Setting up sample tenants and clients..." -ForegroundColor Yellow
    Write-Host "   Running: .\samples\setup-clients.ps1" -ForegroundColor Gray
    Write-Host ""

    # Run the setup script
    try {
        & ".\samples\setup-clients.ps1"
    } catch {
        Write-Host ""
        Write-Host "  ⚠️  Setup script failed: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "     You can manually run: .\samples\setup-clients.ps1" -ForegroundColor Gray
    }
} else {
    Write-Host "⚠️  Backend not ready, skipping setup" -ForegroundColor Yellow
    Write-Host "   You can manually run later: .\samples\setup-clients.ps1" -ForegroundColor Gray
}
Write-Host ""

Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Green
Write-Host "✨ Development Environment is Running!" -ForegroundColor Green
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Green
Write-Host ""
Write-Host "🌐 Application Endpoints:" -ForegroundColor Cyan
Write-Host "   Admin Dashboard:  http://localhost:3000" -ForegroundColor White
Write-Host "   Login UI:         http://localhost:3001" -ForegroundColor White
Write-Host "   Backend API:      http://localhost:8080" -ForegroundColor White
Write-Host "   Auth Backend:     http://localhost:8081" -ForegroundColor White
Write-Host "   MailHog UI:       http://localhost:8025 (Email Testing)" -ForegroundColor White
Write-Host ""
Write-Host "📦 Infrastructure Services:" -ForegroundColor Cyan
Write-Host "   PostgreSQL:       localhost:5433 (Database)" -ForegroundColor White
Write-Host "   Redis:            localhost:6380 (Cache/Sessions)" -ForegroundColor White
Write-Host "   Ory Hydra:        http://localhost:4444 (Public API)" -ForegroundColor White
Write-Host "                     http://localhost:4445 (Admin API)" -ForegroundColor White
Write-Host ""
Write-Host "🔄 Database Migration Status:" -ForegroundColor Cyan
Write-Host "   Automatic:        Enabled (runs on backend startup)" -ForegroundColor White
Write-Host "   Location:         Backend Server window" -ForegroundColor White
Write-Host "   Check logs:       Watch backend window for migration progress" -ForegroundColor Gray
Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""
Write-Host "📝 Management Commands:" -ForegroundColor Yellow
Write-Host "   Stop all services:    .\stop-dev.ps1" -ForegroundColor Gray
Write-Host "   View DB migrations:   Check backend server window" -ForegroundColor Gray
Write-Host "   Docker logs:          docker compose logs -f" -ForegroundColor Gray
Write-Host ""
Write-Host "💡 Tips:" -ForegroundColor Yellow
Write-Host "   - All services are running in Windows Terminal tabs" -ForegroundColor Gray
Write-Host "   - Database migrations run automatically on backend startup" -ForegroundColor Gray
Write-Host "   - Use Ctrl+Shift+W to close a tab or Ctrl+C to stop a service" -ForegroundColor Gray
Write-Host "   - Docker containers will keep running after closing Terminal" -ForegroundColor Gray
Write-Host "   - Use stop-dev.ps1 to stop all services including Docker" -ForegroundColor Gray
Write-Host ""
Write-Host "🎯 Quick Start:" -ForegroundColor Yellow
Write-Host "   1. Open browser: http://localhost:3000 (Admin Dashboard)" -ForegroundColor Gray
Write-Host "   2. Check API: http://localhost:8080/health" -ForegroundColor Gray
Write-Host "   3. View backend logs for migration status" -ForegroundColor Gray
Write-Host ""
