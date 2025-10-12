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
$ports = @(3000, 3001, 8080, 9001, 9002, 9003)
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

# Start infrastructure (PostgreSQL, Redis, MailHog, Ory Hydra)
Write-Host "🐘 Starting infrastructure (PostgreSQL, Redis, MailHog, Ory Hydra)..." -ForegroundColor Yellow
docker-compose -f docker-compose.dev.yml up -d postgres redis mailhog

# Wait for PostgreSQL before starting Hydra migration
Write-Host "⏳ Waiting for PostgreSQL before Hydra migration..." -ForegroundColor Yellow
Start-Sleep -Seconds 3

# Start Ory Hydra (migration will run first due to depends_on)
Write-Host "🌊 Starting Ory Hydra OAuth Server..." -ForegroundColor Yellow
docker-compose -f docker-compose.dev.yml up -d hydra-migrate hydra

# Wait for PostgreSQL to be healthy
Write-Host "⏳ Waiting for PostgreSQL to be ready..." -ForegroundColor Yellow
$maxAttempts = 30
$attempt = 0
while ($attempt -lt $maxAttempts) {
    $postgresHealth = docker inspect --format='{{.State.Health.Status}}' authway-postgres 2>$null
    if ($postgresHealth -eq "healthy") {
        Write-Host "✓ PostgreSQL is ready" -ForegroundColor Green
        break
    }
    Start-Sleep -Seconds 1
    $attempt++
    Write-Host "." -NoNewline
}

if ($attempt -ge $maxAttempts) {
    Write-Host ""
    Write-Host "❌ PostgreSQL failed to start" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Run database migration
Write-Host "🗄️  Running database migration..." -ForegroundColor Yellow
Push-Location scripts
go run migrate.go
$migrationStatus = $LASTEXITCODE
Pop-Location

if ($migrationStatus -ne 0) {
    Write-Host "❌ Migration failed" -ForegroundColor Red
    exit 1
}

Write-Host "✓ Migration completed" -ForegroundColor Green
Write-Host ""

# Start backend server in new terminal
Write-Host "🔧 Starting backend server in new terminal..." -ForegroundColor Yellow
if (-not (Ensure-PortFree -Port 8080 -ServiceName "Backend API")) {
    Write-Host "❌ Cannot start backend - port 8080 is in use" -ForegroundColor Red
    exit 1
}
$backendPath = Join-Path $PSScriptRoot "src\server"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$backendPath'; Write-Host '🔧 Authway Backend Server' -ForegroundColor Cyan; Write-Host ''; go run cmd/main.go"

Start-Sleep -Seconds 2

# Start frontend in new terminal
Write-Host "🎨 Starting frontend in new terminal..." -ForegroundColor Yellow
if (-not (Ensure-PortFree -Port 3000 -ServiceName "Admin Dashboard")) {
    Write-Host "❌ Cannot start Admin Dashboard - port 3000 is in use" -ForegroundColor Red
    exit 1
}
$frontendPath = Join-Path $PSScriptRoot "packages\web\admin-dashboard"

# Check if node_modules exists
if (-not (Test-Path "$frontendPath\node_modules")) {
    Write-Host "📦 Installing frontend dependencies (first time)..." -ForegroundColor Yellow
    Push-Location $frontendPath
    npm install
    Pop-Location
}

Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$frontendPath'; Write-Host '🎨 Authway Admin Dashboard' -ForegroundColor Cyan; Write-Host ''; npm run dev"

Start-Sleep -Seconds 2

# Start Login UI in new terminal
Write-Host "🔐 Starting Login UI in new terminal..." -ForegroundColor Yellow
if (-not (Ensure-PortFree -Port 3001 -ServiceName "Login UI")) {
    Write-Host "❌ Cannot start Login UI - port 3001 is in use" -ForegroundColor Red
    exit 1
}
$loginUiPath = Join-Path $PSScriptRoot "packages\web\login-ui"

# Check if node_modules exists
if (-not (Test-Path "$loginUiPath\node_modules")) {
    Write-Host "📦 Installing Login UI dependencies (first time)..." -ForegroundColor Yellow
    Push-Location $loginUiPath
    npm install
    Pop-Location
}

Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$loginUiPath'; Write-Host '🔐 Authway Login UI' -ForegroundColor Cyan; Write-Host ''; npm run dev"

Write-Host ""
Write-Host "✅ Development environment started successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "📌 Access URLs:" -ForegroundColor Cyan
Write-Host "   Admin Dashboard:  http://localhost:3000" -ForegroundColor White
Write-Host "   Login UI:         http://localhost:3001" -ForegroundColor White
Write-Host "   Backend API:      http://localhost:8080" -ForegroundColor White
Write-Host "   MailHog UI:       http://localhost:8025" -ForegroundColor White
Write-Host ""
Write-Host "📝 Services:" -ForegroundColor Cyan
Write-Host "   PostgreSQL:       localhost:5432" -ForegroundColor White
Write-Host "   Redis:            localhost:6379" -ForegroundColor White
Write-Host "   Ory Hydra:        http://localhost:4444 (Public)" -ForegroundColor White
Write-Host "                     http://localhost:4445 (Admin)" -ForegroundColor White
Write-Host ""
Write-Host "💡 Tip: Use stop-dev.ps1 to stop all services" -ForegroundColor Yellow
Write-Host ""
