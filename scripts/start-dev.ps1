# Authway Development Environment Startup Script (PowerShell)
# Usage: .\start-dev.ps1 [start|full|stop|reset]

param(
    [Parameter(Position=0)]
    [ValidateSet('start', 'full', 'stop', 'reset', '')]
    [string]$Command = ''
)

# Functions
function Print-Header {
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host "  Authway Development Environment" -ForegroundColor Blue
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host ""
}

function Print-Success {
    param([string]$Message)
    Write-Host "✓ $Message" -ForegroundColor Green
}

function Print-Warning {
    param([string]$Message)
    Write-Host "⚠ $Message" -ForegroundColor Yellow
}

function Print-Error {
    param([string]$Message)
    Write-Host "✗ $Message" -ForegroundColor Red
}

function Check-Docker {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        Print-Error "Docker is not installed. Please install Docker Desktop first."
        Write-Host "Visit: https://docs.docker.com/desktop/install/windows-install/"
        exit 1
    }

    try {
        docker info 2>&1 | Out-Null
        Print-Success "Docker is ready"
    }
    catch {
        Print-Error "Docker daemon is not running. Please start Docker Desktop."
        exit 1
    }
}

function Show-Menu {
    Write-Host ""
    Write-Host "Select startup mode:"
    Write-Host "1) Essential services only (Backend + Frontend + Database)"
    Write-Host "2) Full stack (All services including Admin Dashboard)"
    Write-Host "3) Stop all services"
    Write-Host "4) View logs"
    Write-Host "5) Reset all data (WARNING: Deletes all data)"
    Write-Host "6) Exit"
    Write-Host ""
    $choice = Read-Host "Enter your choice [1-6]"
    return $choice
}

function Start-Essential {
    Print-Header
    Write-Host "Starting essential services..." -ForegroundColor Cyan
    Write-Host ""

    docker-compose -f docker-compose.dev.yml up -d postgres redis mailhog authway-api login-ui

    Write-Host ""
    Print-Success "Essential services started!"
    Show-URLs
}

function Start-Full {
    Print-Header
    Write-Host "Starting all services..." -ForegroundColor Cyan
    Write-Host ""

    docker-compose -f docker-compose.dev.yml --profile full up -d

    Write-Host ""
    Print-Success "All services started!"
    Show-URLs
}

function Stop-Services {
    Print-Header
    Write-Host "Stopping all services..." -ForegroundColor Cyan
    Write-Host ""

    docker-compose -f docker-compose.dev.yml down

    Write-Host ""
    Print-Success "All services stopped!"
}

function View-Logs {
    Print-Header
    Write-Host "Available services:"
    Write-Host "1) Backend API (authway-api)"
    Write-Host "2) Login UI (login-ui)"
    Write-Host "3) Admin Dashboard (admin-dashboard)"
    Write-Host "4) PostgreSQL (postgres)"
    Write-Host "5) All services"
    Write-Host ""
    $logChoice = Read-Host "Select service [1-5]"

    switch ($logChoice) {
        '1' { docker-compose -f docker-compose.dev.yml logs -f authway-api }
        '2' { docker-compose -f docker-compose.dev.yml logs -f login-ui }
        '3' { docker-compose -f docker-compose.dev.yml logs -f admin-dashboard }
        '4' { docker-compose -f docker-compose.dev.yml logs -f postgres }
        '5' { docker-compose -f docker-compose.dev.yml logs -f }
        default { Print-Error "Invalid choice" }
    }
}

function Reset-Data {
    Print-Header
    Print-Warning "WARNING: This will delete ALL data including database and cache!"
    $confirm = Read-Host "Are you sure? (yes/no)"

    if ($confirm -eq "yes") {
        Write-Host ""
        Write-Host "Stopping services and removing volumes..."
        docker-compose -f docker-compose.dev.yml down -v

        Write-Host ""
        Print-Success "All data has been reset!"
        Write-Host ""
        $startAgain = Read-Host "Start services again? (yes/no)"

        if ($startAgain -eq "yes") {
            Start-Essential
        }
    }
    else {
        Print-Warning "Reset cancelled"
    }
}

function Show-URLs {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host "  Service URLs" -ForegroundColor Blue
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host ""
    Write-Host "🎨 Login UI:          " -NoNewline
    Write-Host "http://localhost:3001" -ForegroundColor Green
    Write-Host "🖥️  Admin Dashboard:   " -NoNewline
    Write-Host "http://localhost:3000" -ForegroundColor Green
    Write-Host "🚀 Backend API:       " -NoNewline
    Write-Host "http://localhost:8080" -ForegroundColor Green
    Write-Host "📧 MailHog:           " -NoNewline
    Write-Host "http://localhost:8025" -ForegroundColor Green
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Blue
    Write-Host ""
    Write-Host "Wait 10-30 seconds for all services to be ready..."
    Write-Host ""
    Write-Host "Test Backend API:"
    Write-Host "  curl http://localhost:8080/health"
    Write-Host ""
    Write-Host "View logs:"
    Write-Host "  docker-compose -f docker-compose.dev.yml logs -f"
    Write-Host ""
}

function Wait-ForServices {
    Write-Host "Waiting for services to be ready..."
    Start-Sleep -Seconds 5

    $maxAttempts = 30
    $attempt = 0

    while ($attempt -lt $maxAttempts) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:8080/health" -TimeoutSec 1 -ErrorAction SilentlyContinue
            if ($response.StatusCode -eq 200) {
                Print-Success "Backend API is ready!"
                return
            }
        }
        catch {
            # Continue waiting
        }

        $attempt++
        Write-Host -NoNewline "."
        Start-Sleep -Seconds 1
    }

    Write-Host ""
    Print-Warning "Backend API is taking longer than expected. Check logs:"
    Write-Host "  docker-compose -f docker-compose.dev.yml logs authway-api"
}

# Main script
Print-Header
Check-Docker

# Handle command line arguments
switch ($Command) {
    'start' {
        Start-Essential
        Wait-ForServices
        exit 0
    }
    'full' {
        Start-Full
        Wait-ForServices
        exit 0
    }
    'stop' {
        Stop-Services
        exit 0
    }
    'reset' {
        Reset-Data
        exit 0
    }
}

# Interactive menu
while ($true) {
    $choice = Show-Menu

    switch ($choice) {
        '1' {
            Start-Essential
            Wait-ForServices
        }
        '2' {
            Start-Full
            Wait-ForServices
        }
        '3' {
            Stop-Services
        }
        '4' {
            View-Logs
        }
        '5' {
            Reset-Data
        }
        '6' {
            Write-Host "Goodbye!" -ForegroundColor Cyan
            exit 0
        }
        default {
            Print-Error "Invalid choice. Please select 1-6."
        }
    }

    Write-Host ""
    Read-Host "Press Enter to continue..."
}
