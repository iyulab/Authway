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

# Wait for backend port to be listening
Write-Host "⏳ Waiting for backend (port 8080) to start..." -ForegroundColor Yellow
$maxAttempts = 20
$attempt = 0
$backendReady = $false

while ($attempt -lt $maxAttempts) {
    $attempt++

    # Check if port 8080 is listening
    $portCheck = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue
    if ($portCheck) {
        Write-Host "✓ Backend port is listening (took ~$attempt seconds)" -ForegroundColor Green
        $backendReady = $true
        # Give it 1 more second to fully initialize
        Start-Sleep -Seconds 1
        break
    }

    Write-Host "  [$attempt/$maxAttempts] Waiting..." -ForegroundColor Gray
    Start-Sleep -Seconds 1
}

if (-not $backendReady) {
    Write-Host ""
    Write-Host "⚠️  Backend did not start after $maxAttempts seconds" -ForegroundColor Yellow
    Write-Host "   Check the backend terminal window for errors" -ForegroundColor Gray
    Write-Host "   Skipping client registration (you can run .\samples\setup-clients.ps1 manually)" -ForegroundColor Gray
}
Write-Host ""

# Register sample OAuth clients
if ($backendReady) {
    Write-Host "🔧 Registering sample OAuth clients..." -ForegroundColor Yellow

    # Fetch tenant ID
    try {
        $tenantsResponse = Invoke-WebRequest -Uri "http://localhost:8080/api/v1/tenants" -Method GET -UseBasicParsing -ErrorAction Stop
        $tenantsData = $tenantsResponse.Content | ConvertFrom-Json

        # API returns array directly, not wrapped in {data: [...]}
        if ($tenantsData.Count -eq 0) {
            Write-Host "  ⚠️  No tenant found, skipping client registration" -ForegroundColor Yellow
        } else {
            $TENANT_ID = $tenantsData[0].id
            Write-Host "  ✓ Found tenant: $TENANT_ID" -ForegroundColor Green

            # Sample services configuration
            $services = @(
                @{
                    Name = "AppleService"
                    ClientID = "apple-service-client"
                    ClientSecret = "apple-service-secret"
                    RedirectURI = "http://localhost:9001/callback"
                    Icon = "🍎"
                },
                @{
                    Name = "BananaService"
                    ClientID = "banana-service-client"
                    ClientSecret = "banana-service-secret"
                    RedirectURI = "http://localhost:9002/callback"
                    Icon = "🍌"
                },
                @{
                    Name = "ChocolateService"
                    ClientID = "chocolate-service-client"
                    ClientSecret = "chocolate-service-secret"
                    RedirectURI = "http://localhost:9003/callback"
                    Icon = "🍫"
                }
            )

            # Register each service
            foreach ($service in $services) {
                Write-Host "  $($service.Icon) $($service.Name)..." -NoNewline

                # Register in Authway database
                $clientData = @{
                    tenant_id = $TENANT_ID
                    client_id = $service.ClientID
                    client_secret = $service.ClientSecret
                    name = $service.Name
                    description = "Sample service for testing Authway OAuth 2.0 integration"
                    redirect_uris = @($service.RedirectURI)
                    grant_types = @("authorization_code", "refresh_token")
                    scopes = @("openid", "profile", "email")
                    public = $false
                }
                $jsonData = $clientData | ConvertTo-Json

                try {
                    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/v1/clients" -Method POST -ContentType "application/json" -Body $jsonData -UseBasicParsing -ErrorAction Stop
                    Write-Host " ✓ Authway" -NoNewline -ForegroundColor Green
                } catch {
                    if ($_.Exception.Message -match "409" -or $_.Exception.Message -match "already exists") {
                        Write-Host " ✓ Authway(exists)" -NoNewline -ForegroundColor Yellow
                    } else {
                        Write-Host " ✗ Authway" -NoNewline -ForegroundColor Red
                    }
                }

                # Register in Hydra
                $hydraData = @{
                    client_id = $service.ClientID
                    client_secret = $service.ClientSecret
                    client_name = $service.Name
                    grant_types = @("authorization_code", "refresh_token")
                    response_types = @("code")
                    redirect_uris = @($service.RedirectURI)
                    scope = "openid profile email"
                }
                $hydraJson = $hydraData | ConvertTo-Json

                try {
                    $hydraResponse = Invoke-WebRequest -Uri "http://localhost:4445/admin/clients" -Method POST -ContentType "application/json" -Body $hydraJson -UseBasicParsing -ErrorAction Stop
                    Write-Host " ✓ Hydra" -ForegroundColor Green
                } catch {
                    if ($_.Exception.Message -match "already exists") {
                        Write-Host " ✓ Hydra(exists)" -ForegroundColor Yellow
                    } else {
                        Write-Host " ✗ Hydra" -ForegroundColor Red
                    }
                }
            }

            Write-Host "  ✓ Client registration complete!" -ForegroundColor Green
        }
    } catch {
        Write-Host "  ⚠️  Failed to register clients: $($_.Exception.Message)" -ForegroundColor Yellow
        Write-Host "     You can manually run: .\samples\setup-clients.ps1" -ForegroundColor Gray
    }
} else {
    Write-Host "⚠️  Backend not ready, skipping client registration" -ForegroundColor Yellow
    Write-Host "   You can manually run later: .\samples\setup-clients.ps1" -ForegroundColor Gray
}
Write-Host ""

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
