# Authway Development Startup Script
# Starts backend and frontend in separate terminals

Write-Host "🚀 Starting Authway Development Environment..." -ForegroundColor Cyan
Write-Host ""

# Clean up any existing Authway processes on required ports
Write-Host "🧹 Checking for existing Authway processes..." -ForegroundColor Yellow
$ports = @(3000, 3001, 3002, 8080)
$killedProcesses = 0
$projectPath = $PSScriptRoot

foreach ($port in $ports) {
    try {
        $connections = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
        if ($connections) {
            foreach ($conn in $connections) {
                $processId = $conn.OwningProcess
                $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
                if ($process) {
                    # Check if this is an Authway process
                    $isAuthwayProcess = $false

                    try {
                        $processInfo = Get-CimInstance Win32_Process -Filter "ProcessId = $processId" -ErrorAction SilentlyContinue
                        if ($processInfo -and $processInfo.CommandLine) {
                            $commandLine = $processInfo.CommandLine
                            # Check if command line contains Authway project path
                            if ($commandLine -match [regex]::Escape($projectPath) -or
                                $commandLine -match "authway" -or
                                $commandLine -match "admin-dashboard") {
                                $isAuthwayProcess = $true
                            }
                        }
                    } catch {
                        # If we can't get command line, check process name and ask user
                        if ($process.ProcessName -eq "node" -or $process.ProcessName -eq "go" -or $process.ProcessName -match "main") {
                            Write-Host "  ⚠️  Found $($process.ProcessName) (PID: $processId) on port $port" -ForegroundColor Yellow
                            Write-Host "     Cannot verify if this is an Authway process." -ForegroundColor Yellow
                            $response = Read-Host "     Stop this process? (y/N)"
                            if ($response -eq "y" -or $response -eq "Y") {
                                $isAuthwayProcess = $true
                            }
                        }
                    }

                    if ($isAuthwayProcess) {
                        Write-Host "  Stopping Authway $($process.ProcessName) (PID: $processId) on port $port" -ForegroundColor Gray
                        Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
                        $killedProcesses++
                    }
                }
            }
        }
    } catch {
        # Silently continue if port not in use
    }
}

if ($killedProcesses -gt 0) {
    Write-Host "✓ Cleaned up $killedProcesses Authway process(es)" -ForegroundColor Green
    Start-Sleep -Seconds 2
} else {
    Write-Host "✓ No existing Authway processes to clean up" -ForegroundColor Green
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

# Start infrastructure (PostgreSQL, Redis, MailHog)
Write-Host "🐘 Starting infrastructure (PostgreSQL, Redis, MailHog)..." -ForegroundColor Yellow
docker-compose -f docker-compose.dev.yml up -d postgres redis mailhog

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
$backendPath = Join-Path $PSScriptRoot "src\server"
Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$backendPath'; Write-Host '🔧 Authway Backend Server' -ForegroundColor Cyan; Write-Host ''; go run cmd/main.go"

Start-Sleep -Seconds 2

# Start frontend in new terminal
Write-Host "🎨 Starting frontend in new terminal..." -ForegroundColor Yellow
$frontendPath = Join-Path $PSScriptRoot "packages\web\admin-dashboard"

# Check if node_modules exists
if (-not (Test-Path "$frontendPath\node_modules")) {
    Write-Host "📦 Installing frontend dependencies (first time)..." -ForegroundColor Yellow
    Push-Location $frontendPath
    npm install
    Pop-Location
}

Start-Process powershell -ArgumentList "-NoExit", "-Command", "cd '$frontendPath'; Write-Host '🎨 Authway Admin Dashboard' -ForegroundColor Cyan; Write-Host ''; npm run dev"

Write-Host ""
Write-Host "✅ Development environment started successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "📌 Access URLs:" -ForegroundColor Cyan
Write-Host "   Admin Dashboard:  http://localhost:3000" -ForegroundColor White
Write-Host "   Backend API:      http://localhost:8080" -ForegroundColor White
Write-Host "   MailHog UI:       http://localhost:8025" -ForegroundColor White
Write-Host ""
Write-Host "📝 Services:" -ForegroundColor Cyan
Write-Host "   PostgreSQL:       localhost:5432" -ForegroundColor White
Write-Host "   Redis:            localhost:6379" -ForegroundColor White
Write-Host ""
Write-Host "💡 Tip: Use stop-dev.ps1 to stop all services" -ForegroundColor Yellow
Write-Host ""
