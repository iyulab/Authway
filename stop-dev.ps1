# Authway Development Stop Script
# Stops all development services

Write-Host "🛑 Stopping Authway Development Environment..." -ForegroundColor Cyan
Write-Host ""

# Stop Authway backend and frontend processes
Write-Host "🔧 Stopping Authway backend and frontend processes..." -ForegroundColor Yellow

# Kill Authway processes on port 3000, 3001, 3002, 8080
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
    Write-Host "✓ Stopped $killedProcesses Authway process(es)" -ForegroundColor Green
} else {
    Write-Host "✓ No Authway backend/frontend processes running" -ForegroundColor Green
}

Write-Host ""

# Stop Docker infrastructure
Write-Host "📦 Stopping Docker services..." -ForegroundColor Yellow
docker-compose -f docker-compose.dev.yml down

if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Docker services stopped" -ForegroundColor Green
} else {
    Write-Host "⚠️  Some Docker services may still be running" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "✅ Development environment stopped" -ForegroundColor Green
Write-Host ""
