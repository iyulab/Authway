# Authway Development Stop Script (All Docker)
# Stops all Docker services

Write-Host "🛑 Stopping Authway Development Environment (Full Docker)..." -ForegroundColor Cyan
Write-Host ""

# Stop any local Authway backend/frontend processes first
Write-Host "🔧 Stopping any local Authway backend/frontend processes..." -ForegroundColor Yellow

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
    Write-Host "✓ Stopped $killedProcesses Authway local process(es)" -ForegroundColor Green
} else {
    Write-Host "✓ No Authway local processes running" -ForegroundColor Green
}

Write-Host ""

# Stop all Docker services
Write-Host "📦 Stopping Docker services..." -ForegroundColor Yellow
docker-compose -f docker-compose.dev.yml --profile full down

if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ All services stopped" -ForegroundColor Green
} else {
    Write-Host "⚠️  Some services may still be running" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "✅ Development environment stopped" -ForegroundColor Green
Write-Host ""
Write-Host "💡 Tip: Use 'docker-compose -f docker-compose.dev.yml down -v' to remove volumes too" -ForegroundColor Yellow
Write-Host ""
