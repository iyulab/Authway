# ============================================================
# Start All Sample Services
# ============================================================
# AppleService, BananaService, ChocolateService를 자동으로 시작합니다.
# ============================================================

param(
    [switch]$StopOnly
)

$ErrorActionPreference = "Stop"

# 포트 정의
$Services = @{
    "AppleService" = @{
        Port = 9001
        Path = "AppleService"
        Icon = "🍎"
        Color = "Red"
    }
    "BananaService" = @{
        Port = 9002
        Path = "BananaService"
        Icon = "🍌"
        Color = "Yellow"
    }
    "ChocolateService" = @{
        Port = 9003
        Path = "ChocolateService"
        Icon = "🍫"
        Color = "DarkYellow"
    }
}

# Kill process on port
function Kill-PortProcess {
    param([int]$Port)
    
    $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    if ($connections) {
        foreach ($conn in $connections) {
            $processId = $conn.OwningProcess
            $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
            if ($process) {
                Write-Host "  Stopping process on port $Port (PID: $processId)" -ForegroundColor Gray
                Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
            }
        }
    }
}

# Stop all services
function Stop-AllServices {
    Write-Host "🧹 Stopping all sample services..." -ForegroundColor Yellow
    
    foreach ($service in $Services.Keys) {
        $port = $Services[$service].Port
        Kill-PortProcess -Port $port
    }
    
    Write-Host "✓ All services stopped" -ForegroundColor Green
}

if ($StopOnly) {
    Stop-AllServices
    exit 0
}

# Stop existing instances
Stop-AllServices
Start-Sleep -Seconds 2

Write-Host ""
Write-Host "🚀 Starting all sample services..." -ForegroundColor Cyan
Write-Host ""

$SamplesRoot = $PSScriptRoot

# Start each service
foreach ($serviceName in $Services.Keys) {
    $service = $Services[$serviceName]
    $servicePath = Join-Path $SamplesRoot $service.Path
    
    if (-not (Test-Path $servicePath)) {
        Write-Host "$($service.Icon) ⚠️  $serviceName not found at $servicePath" -ForegroundColor Yellow
        continue
    }
    
    Write-Host "$($service.Icon) Starting $serviceName (port $($service.Port))..." -ForegroundColor $service.Color
    
    # Check if go.mod exists
    if (-not (Test-Path "$servicePath\go.mod")) {
        Write-Host "  ❌ go.mod not found" -ForegroundColor Red
        continue
    }
    
    # Start in new terminal
    $command = @"
cd '$servicePath'
Write-Host '$($service.Icon) $serviceName (port $($service.Port))' -ForegroundColor $($service.Color)
Write-Host 'URL: http://localhost:$($service.Port)' -ForegroundColor Gray
Write-Host ''
go run main.go
"@
    
    Start-Process powershell -ArgumentList "-NoExit", "-Command", $command
    Start-Sleep -Seconds 1
}

Write-Host ""
Write-Host "✅ All sample services started!" -ForegroundColor Green
Write-Host ""
Write-Host "📌 Service URLs:" -ForegroundColor Cyan
Write-Host "   🍎 AppleService:     http://localhost:9001" -ForegroundColor Red
Write-Host "   🍌 BananaService:    http://localhost:9002" -ForegroundColor Yellow
Write-Host "   🍫 ChocolateService: http://localhost:9003" -ForegroundColor DarkYellow
Write-Host ""
Write-Host "🏢 Multi-Tenancy:" -ForegroundColor Cyan
Write-Host "   Fruits tenant (SSO): 🍎 Apple + 🍌 Banana" -ForegroundColor White
Write-Host "   Sweets tenant:       🍫 Chocolate" -ForegroundColor White
Write-Host ""
Write-Host "💡 Tip: .\samples\start-all-services.ps1 -StopOnly to stop" -ForegroundColor Yellow
Write-Host ""
