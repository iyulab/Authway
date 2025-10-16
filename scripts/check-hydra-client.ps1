# Check if client exists in Hydra

param(
    [Parameter(Mandatory=$true)]
    [string]$ClientId
)

$hydraAdminUrl = "https://authway.iyulab.com"

Write-Host "Checking if client exists in Hydra..." -ForegroundColor Cyan
Write-Host "Admin URL: $hydraAdminUrl" -ForegroundColor Gray
Write-Host "Client ID: $ClientId" -ForegroundColor Gray
Write-Host ""

try {
    $response = Invoke-RestMethod `
        -Uri "$hydraAdminUrl/admin/clients/$ClientId" `
        -Method Get `
        -ContentType "application/json" `
        -ErrorAction Stop

    Write-Host "[SUCCESS] Client EXISTS in Hydra!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Client Details:" -ForegroundColor Yellow
    Write-Host "  Client ID: $($response.client_id)" -ForegroundColor White
    Write-Host "  Client Name: $($response.client_name)" -ForegroundColor White
    Write-Host "  Grant Types: $($response.grant_types -join ', ')" -ForegroundColor White
    Write-Host "  Redirect URIs:" -ForegroundColor White
    foreach ($uri in $response.redirect_uris) {
        Write-Host "    - $uri" -ForegroundColor Gray
    }
    Write-Host "  Scopes: $($response.scope)" -ForegroundColor White
    Write-Host ""
    Write-Host "[SUCCESS] Backend API successfully registered this client in Hydra!" -ForegroundColor Green
}
catch {
    if ($_.Exception.Response.StatusCode -eq 404) {
        Write-Host "[ERROR] Client NOT FOUND in Hydra!" -ForegroundColor Red
        Write-Host ""
        Write-Host "This means Backend API failed to register the client in Hydra." -ForegroundColor Yellow
        Write-Host ""
        Write-Host "Possible causes:" -ForegroundColor Yellow
        Write-Host "  1. Backend API deployment failed" -ForegroundColor White
        Write-Host "  2. Backend code with Hydra integration not deployed" -ForegroundColor White
        Write-Host "  3. Network issue between Backend API and Hydra" -ForegroundColor White
        Write-Host ""
        Write-Host "Check Backend API logs for errors containing:" -ForegroundColor Yellow
        Write-Host "  'Failed to register client in Hydra'" -ForegroundColor Gray
    }
    else {
        Write-Host "[ERROR] Error checking Hydra:" -ForegroundColor Red
        Write-Host "Status Code: $($_.Exception.Response.StatusCode)" -ForegroundColor Yellow
        Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
    }
    exit 1
}
