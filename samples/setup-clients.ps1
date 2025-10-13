# Authway Sample Services - Client Registration Script
# This script registers the sample service OAuth clients with Authway

Write-Host "🔐 Authway Sample Services - Client Registration" -ForegroundColor Cyan
Write-Host ""

# Configuration
$AUTHWAY_API = "http://localhost:8080"

# Fetch tenant ID dynamically
Write-Host "🔍 Fetching tenant ID..." -ForegroundColor Yellow
try
{
    $tenantsResponse = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/tenants" -Method GET -UseBasicParsing -ErrorAction Stop
    $tenantsData = $tenantsResponse.Content | ConvertFrom-Json

    # API returns array directly, not wrapped in {data: [...]}
    if ($tenantsData.Count -eq 0)
    {
        Write-Host "❌ No tenant found. Please create a tenant first." -ForegroundColor Red
        exit 1
    }

    $TENANT_ID = $tenantsData[0].id
    Write-Host "✓ Using tenant ID: $TENANT_ID" -ForegroundColor Green
}
catch
{
    Write-Host "❌ Failed to fetch tenant ID" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Sample services configuration
$services = @(
    @{
        Name = "AppleService"
        ClientID = "apple-service-client"
        ClientSecret = "apple-service-secret"
        RedirectURI = "http://localhost:9001/callback"
        Icon = "🍎"
        Color = "Red"
    },
    @{
        Name = "BananaService"
        ClientID = "banana-service-client"
        ClientSecret = "banana-service-secret"
        RedirectURI = "http://localhost:9002/callback"
        Icon = "🍌"
        Color = "Yellow"
    },
    @{
        Name = "ChocolateService"
        ClientID = "chocolate-service-client"
        ClientSecret = "chocolate-service-secret"
        RedirectURI = "http://localhost:9003/callback"
        Icon = "🍫"
        Color = "DarkYellow"
    }
)

# Check if Authway is running
Write-Host "📡 Checking Authway API..." -ForegroundColor Yellow
try
{
    $response = Invoke-WebRequest -Uri "$AUTHWAY_API/health" -Method GET -UseBasicParsing -ErrorAction Stop
    $health = $response.Content | ConvertFrom-Json
    Write-Host "✓ Authway is running (version: $($health.version))" -ForegroundColor Green
}
catch
{
    Write-Host "❌ Authway API is not accessible at $AUTHWAY_API" -ForegroundColor Red
    Write-Host "   Please make sure Authway is running first." -ForegroundColor Red
    exit 1
}

Write-Host ""

# Register each service
foreach ($service in $services)
{
    Write-Host "$($service.Icon) Registering $($service.Name)..." -ForegroundColor $service.Color

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

    # Register in Authway database
    try
    {
        $response = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/clients" -Method POST -ContentType "application/json" -Body $jsonData -UseBasicParsing -ErrorAction Stop
        $client = $response.Content | ConvertFrom-Json
        Write-Host "  ✓ Client created in Authway" -ForegroundColor Green
    }
    catch
    {
        $errorMessage = $_.Exception.Message
        if ($errorMessage -match "409" -or $errorMessage -match "already exists")
        {
            Write-Host "  ℹ Client already exists in Authway, skipping..." -ForegroundColor Yellow
        }
        else
        {
            Write-Host "  ❌ Failed to register client in Authway" -ForegroundColor Red
            Write-Host "  Error: $errorMessage" -ForegroundColor Red
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

    try
    {
        $hydraResponse = Invoke-WebRequest -Uri "http://localhost:4445/admin/clients" -Method POST -ContentType "application/json" -Body $hydraJson -UseBasicParsing -ErrorAction Stop
        Write-Host "  ✓ Client registered in Hydra" -ForegroundColor Green
    }
    catch
    {
        $hydraError = $_.Exception.Message
        if ($hydraError -match "already exists")
        {
            Write-Host "  ℹ Client already exists in Hydra, skipping..." -ForegroundColor Yellow
        }
        else
        {
            Write-Host "  ⚠ Warning: Failed to register in Hydra" -ForegroundColor Yellow
            Write-Host "  You may need to register manually in Hydra" -ForegroundColor Yellow
        }
    }

    Write-Host "  ✓ Redirect URI: $($service.RedirectURI)" -ForegroundColor Gray
    Write-Host ""
}

Write-Host "✅ Client registration complete!" -ForegroundColor Green
Write-Host ""
Write-Host "📝 Next steps:" -ForegroundColor Cyan
Write-Host "  1. Make sure Authway is running on port 8080"
Write-Host "  2. Start the sample services:"
Write-Host "     cd samples/AppleService && go run main.go"
Write-Host "     cd samples/BananaService && go run main.go"
Write-Host "     cd samples/ChocolateService && go run main.go"
Write-Host ""
Write-Host "  3. Open the services in your browser:"
Write-Host "     🍎 Apple Service:     http://localhost:9001"
Write-Host "     🍌 Banana Service:    http://localhost:9002"
Write-Host "     🍫 Chocolate Service: http://localhost:9003"
Write-Host ""
Write-Host "  4. Test SSO by logging into one service, then accessing another"
Write-Host ""
