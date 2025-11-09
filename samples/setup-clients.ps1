# Authway Sample Services - Client Registration Script
# This script registers the sample service OAuth clients with Authway

Write-Host "🔐 Authway Sample Services - Client Registration" -ForegroundColor Cyan
Write-Host ""

# Configuration
$AUTHWAY_API = "http://localhost:8080"

# Fetch tenants and map by slug
Write-Host "🔍 Fetching tenants..." -ForegroundColor Yellow
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

    # Find Fruits and Sweets tenants
    $fruitsTenant = $tenantsData | Where-Object { $_.slug -eq "fruits" }
    $sweetsTenant = $tenantsData | Where-Object { $_.slug -eq "sweets" }

    if (-not $fruitsTenant)
    {
        Write-Host "❌ Fruits tenant not found. Please run setup-clients.ps1 to create tenants." -ForegroundColor Red
        exit 1
    }
    if (-not $sweetsTenant)
    {
        Write-Host "❌ Sweets tenant not found. Please run setup-clients.ps1 to create tenants." -ForegroundColor Red
        exit 1
    }

    Write-Host "✓ Found Fruits tenant: $($fruitsTenant.id)" -ForegroundColor Green
    Write-Host "✓ Found Sweets tenant: $($sweetsTenant.id)" -ForegroundColor Green
}
catch
{
    Write-Host "❌ Failed to fetch tenants" -ForegroundColor Red
    Write-Host "   Error: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Sample services configuration with tenant mapping
$services = @(
    @{
        Name = "AppleService"
        ClientID = "apple-service-client"
        ClientSecret = "apple-service-secret"
        RedirectURI = "http://localhost:9001/callback"
        Icon = "🍎"
        Color = "Red"
        TenantID = $fruitsTenant.id
    },
    @{
        Name = "BananaService"
        ClientID = "banana-service-client"
        ClientSecret = "banana-service-secret"
        RedirectURI = "http://localhost:9002/callback"
        Icon = "🍌"
        Color = "Yellow"
        TenantID = $fruitsTenant.id
    },
    @{
        Name = "ChocolateService"
        ClientID = "chocolate-service-client"
        ClientSecret = "chocolate-service-secret"
        RedirectURI = "http://localhost:9003/callback"
        Icon = "🍫"
        Color = "DarkYellow"
        TenantID = $sweetsTenant.id
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
        tenant_id = $service.TenantID
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

    # Check if client already exists in Authway
    $clientExists = $false
    try
    {
        $checkResponse = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/clients" -Method GET -UseBasicParsing -ErrorAction Stop
        $existingClients = $checkResponse.Content | ConvertFrom-Json
        $existingClient = $existingClients.clients | Where-Object { $_.client_id -eq $service.ClientID }
        if ($existingClient)
        {
            $clientExists = $true
            Write-Host "  ℹ Client already exists in Authway" -ForegroundColor Gray
        }
    }
    catch
    {
        # Failed to check, will try to create
    }

    # Register in Authway database (only if doesn't exist)
    if (-not $clientExists)
    {
        try
        {
            $response = Invoke-WebRequest -Uri "$AUTHWAY_API/api/v1/clients" -Method POST -ContentType "application/json" -Body $jsonData -UseBasicParsing -ErrorAction Stop
            $client = $response.Content | ConvertFrom-Json
            Write-Host "  ✓ Client created in Authway" -ForegroundColor Green
        }
        catch
        {
            $errorMessage = $_.Exception.Message
            if ($errorMessage -match "duplicate" -or $errorMessage -match "409" -or $errorMessage -match "already exists")
            {
                Write-Host "  ℹ Client already exists in Authway (duplicate key)" -ForegroundColor Yellow
                $clientExists = $true
            }
            else
            {
                Write-Host "  ❌ Failed to register client in Authway" -ForegroundColor Red
                Write-Host "  Error: $errorMessage" -ForegroundColor Gray
            }
        }
    }

    # Register in Hydra (delete first if exists, then create)
    $hydraData = @{
        client_id = $service.ClientID
        client_secret = $service.ClientSecret
        client_name = $service.Name
        grant_types = @("authorization_code", "refresh_token")
        response_types = @("code")
        redirect_uris = @($service.RedirectURI)
        scope = "openid profile email"
        token_endpoint_auth_method = "client_secret_post"  # Support client_secret_post method
        access_token_strategy = "jwt"  # Issue JWT access tokens instead of opaque tokens
        skip_consent = $true  # Skip consent screen for trusted first-party clients
        skip_logout_consent = $true  # Skip consent on logout
    }
    $hydraJson = $hydraData | ConvertTo-Json

    # Try to delete existing client first
    try
    {
        $deleteResponse = Invoke-WebRequest -Uri "http://localhost:4445/admin/clients/$($service.ClientID)" -Method DELETE -UseBasicParsing -ErrorAction Stop
        Write-Host "  ℹ Deleted existing Hydra client" -ForegroundColor Gray
    }
    catch
    {
        # Client doesn't exist or delete failed, continue
    }

    # Register new client
    try
    {
        $hydraResponse = Invoke-WebRequest -Uri "http://localhost:4445/admin/clients" -Method POST -ContentType "application/json" -Body $hydraJson -UseBasicParsing -ErrorAction Stop
        Write-Host "  ✓ Client registered in Hydra" -ForegroundColor Green
    }
    catch
    {
        $hydraError = $_.Exception.Message
        Write-Host "  ⚠ Warning: Failed to register in Hydra" -ForegroundColor Yellow
        Write-Host "  Error: $hydraError" -ForegroundColor Gray
    }

    Write-Host "  ✓ Redirect URI: $($service.RedirectURI)" -ForegroundColor Gray
    Write-Host ""
}

Write-Host "✅ Client registration complete!" -ForegroundColor Green
Write-Host ""
Write-Host "📊 Multi-Tenant Configuration:" -ForegroundColor Cyan
Write-Host "  🏢 Fruits Company (fruits):" -ForegroundColor White
Write-Host "     ├─ 🍎 AppleService   (http://localhost:9001)" -ForegroundColor Gray
Write-Host "     └─ 🍌 BananaService  (http://localhost:9002)" -ForegroundColor Gray
Write-Host ""
Write-Host "  🏢 Sweets Company (sweets):" -ForegroundColor White
Write-Host "     └─ 🍫 ChocolateService (http://localhost:9003)" -ForegroundColor Gray
Write-Host ""
Write-Host "📝 Next steps:" -ForegroundColor Cyan
Write-Host "  1. Start the sample services:" -ForegroundColor White
Write-Host "     cd samples/AppleService; go run main.go" -ForegroundColor Gray
Write-Host "     cd samples/BananaService; go run main.go" -ForegroundColor Gray
Write-Host "     cd samples/ChocolateService; go run main.go" -ForegroundColor Gray
Write-Host ""
Write-Host "  2. Test multi-tenancy:" -ForegroundColor White
Write-Host "     - Apple and Banana share SSO (same Fruits tenant)" -ForegroundColor Gray
Write-Host "     - Chocolate has separate authentication (different Sweets tenant)" -ForegroundColor Gray
Write-Host ""
