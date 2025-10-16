# Register ASP.NET OAuth Client in Azure Hydra

$clientData = @{
    client_id = "asp-sample-azure"
    client_secret = "azure-secret-change-in-production"
    grant_types = @("authorization_code", "refresh_token")
    response_types = @("code")
    redirect_uris = @(
        "https://localhost:5001/signin-oidc",
        "http://localhost:5000/signin-oidc"
    )
    post_logout_redirect_uris = @(
        "https://localhost:5001/signout-callback-oidc",
        "http://localhost:5000/signout-callback-oidc"
    )
    scope = "openid profile email"
    token_endpoint_auth_method = "client_secret_post"
} | ConvertTo-Json

Write-Host "Registering OAuth client in Azure Hydra..." -ForegroundColor Cyan

try {
    $response = Invoke-RestMethod `
        -Uri "https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io/admin/clients" `
        -Method Post `
        -Body $clientData `
        -ContentType "application/json"

    Write-Host "✅ Client registered successfully!" -ForegroundColor Green
    Write-Host "Client ID: asp-sample-azure" -ForegroundColor Yellow
    $response | ConvertTo-Json -Depth 10
}
catch {
    Write-Host "❌ Failed to register client" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red

    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response: $responseBody" -ForegroundColor Yellow
    }
}
