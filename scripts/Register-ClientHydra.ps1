param(
    [Parameter(Mandatory=$true)]
    [string]$AdminUrl,

    [Parameter(Mandatory=$true)]
    [string]$ClientId,

    [Parameter(Mandatory=$true)]
    [string]$ClientSecret
)

Write-Host "Registering OAuth client in Hydra..." -ForegroundColor Cyan
Write-Host "Admin URL: $AdminUrl" -ForegroundColor Gray
Write-Host "Client ID: $ClientId" -ForegroundColor Gray

$body = @{
    client_id = $ClientId
    client_secret = $ClientSecret
    client_name = "ASP.NET Sample"
    redirect_uris = @(
        "https://localhost:5001/signin-oidc",
        "http://localhost:5000/signin-oidc"
    )
    post_logout_redirect_uris = @(
        "https://localhost:5001/signout-callback-oidc",
        "http://localhost:5000/signout-callback-oidc"
    )
    grant_types = @("authorization_code", "refresh_token")
    response_types = @("code")
    scope = "openid profile email"
    token_endpoint_auth_method = "client_secret_post"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod `
        -Uri "$AdminUrl/admin/clients" `
        -Method Post `
        -Body $body `
        -ContentType "application/json" `
        -ErrorAction Stop

    Write-Host "`n✅ Client registered successfully!" -ForegroundColor Green
    Write-Host "Client ID: $($response.client_id)" -ForegroundColor White
    Write-Host "Client Name: $($response.client_name)" -ForegroundColor White
}
catch {
    Write-Host "`n❌ Failed to register client" -ForegroundColor Red
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red

    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $responseBody = $reader.ReadToEnd()
        Write-Host "Response: $responseBody" -ForegroundColor Yellow
    }

    exit 1
}
