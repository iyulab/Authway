# Authway Development Startup Script (All Docker)
# Starts everything in Docker including backend and frontend

Write-Host "🚀 Starting Authway Development Environment (Full Docker)..." -ForegroundColor Cyan
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

# Start all services with full profile
Write-Host "🐳 Starting all services..." -ForegroundColor Yellow
docker-compose -f docker-compose.dev.yml --profile full up -d

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Failed to start services" -ForegroundColor Red
    exit 1
}

Write-Host "✓ All services started" -ForegroundColor Green
Write-Host ""

# Wait for backend to be ready
Write-Host "⏳ Waiting for services to be ready..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

# Run database migration
Write-Host "🗄️  Running database migration..." -ForegroundColor Yellow
docker exec -it authway-api go run /app/scripts/migrate.go

if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Migration completed" -ForegroundColor Green
} else {
    Write-Host "⚠️  Migration may have failed. Check logs with: docker-compose logs authway-api" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "✅ Development environment started successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "📌 Access URLs:" -ForegroundColor Cyan
Write-Host "   Admin Dashboard:  http://localhost:3000" -ForegroundColor White
Write-Host "   Backend API:      http://localhost:8080" -ForegroundColor White
Write-Host "   Login UI:         http://localhost:3001" -ForegroundColor White
Write-Host "   MailHog UI:       http://localhost:8025" -ForegroundColor White
Write-Host ""
Write-Host "📝 View logs:" -ForegroundColor Cyan
Write-Host "   docker-compose -f docker-compose.dev.yml logs -f [service-name]" -ForegroundColor White
Write-Host ""
Write-Host "💡 Tip: Use stop-dev-all.ps1 to stop all services" -ForegroundColor Yellow
Write-Host ""
