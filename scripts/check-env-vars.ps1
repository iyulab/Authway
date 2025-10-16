# Authway Environment Variables Check Script
# Validates all Azure resources environment variables

param(
    [string]$ResourceGroup = "authway"
)

Write-Host "Checking Authway environment variables..." -ForegroundColor Cyan
Write-Host ""

# Check Azure CLI login
try {
    $account = az account show 2>&1 | ConvertFrom-Json
    Write-Host "[OK] Logged in as: $($account.user.name)" -ForegroundColor Green
} catch {
    Write-Host "[ERROR] Not logged into Azure. Run 'az login'" -ForegroundColor Red
    exit 1
}

Write-Host ""

# Track validation results
$issues = @()
$warnings = @()

function Check-ContainerAppEnv {
    param(
        [string]$AppName,
        [hashtable]$ExpectedVars
    )

    Write-Host "================================================" -ForegroundColor DarkGray
    Write-Host "Container App: $AppName" -ForegroundColor Yellow
    Write-Host ""

    try {
        # Get Container App environment variables
        # For authway-hydra, we need to check the 'authway-hydra' container specifically (not nginx-proxy)
        if ($AppName -eq "authway-hydra") {
            $envVars = az containerapp show `
                --name $AppName `
                --resource-group $ResourceGroup `
                --query "properties.template.containers[?name=='authway-hydra'].env | [0]" `
                -o json | ConvertFrom-Json
        } else {
            $envVars = az containerapp show `
                --name $AppName `
                --resource-group $ResourceGroup `
                --query "properties.template.containers[0].env" `
                -o json | ConvertFrom-Json
        }

        $envMap = @{}
        foreach ($env in $envVars) {
            if ($env.name -and $env.value) {
                $envMap[$env.name] = $env.value
            }
        }

        # Check expected environment variables
        foreach ($key in $ExpectedVars.Keys) {
            $expected = $ExpectedVars[$key]
            $actual = $envMap[$key]

            if ($actual) {
                if ($actual -eq $expected) {
                    Write-Host "  [OK] $key" -ForegroundColor Green
                    Write-Host "    = $actual" -ForegroundColor Gray
                } elseif ($expected -eq "*") {
                    Write-Host "  [OK] $key (value confirmed)" -ForegroundColor Green
                    Write-Host "    = $actual" -ForegroundColor Gray
                } else {
                    Write-Host "  [WARN] $key (value mismatch)" -ForegroundColor Yellow
                    Write-Host "    Expected: $expected" -ForegroundColor Gray
                    Write-Host "    Actual: $actual" -ForegroundColor Gray
                    $script:warnings += "$AppName - $key value mismatch"
                }
            } else {
                Write-Host "  [ERROR] $key (missing)" -ForegroundColor Red
                Write-Host "    Should be: $expected" -ForegroundColor Gray
                $script:issues += "$AppName - $key environment variable missing"
            }
        }

        # Show additional environment variables
        $extraVars = $envMap.Keys | Where-Object { -not $ExpectedVars.ContainsKey($_) }
        if ($extraVars) {
            Write-Host ""
            Write-Host "  Additional environment variables:" -ForegroundColor Cyan
            foreach ($key in $extraVars) {
                Write-Host "    - $key = $($envMap[$key])" -ForegroundColor Gray
            }
        }

    } catch {
        Write-Host "  [ERROR] Container App not found: $AppName" -ForegroundColor Red
        $script:issues += "$AppName - Container App not found"
    }

    Write-Host ""
}

function Check-StaticWebAppEnv {
    param(
        [string]$AppName,
        [hashtable]$ExpectedVars
    )

    Write-Host "================================================" -ForegroundColor DarkGray
    Write-Host "Static Web App: $AppName" -ForegroundColor Yellow
    Write-Host ""

    Write-Host "  [INFO] Static Web App environment variables must be checked in Azure Portal:" -ForegroundColor Cyan
    Write-Host "  Portal > Static Web Apps > $AppName > Configuration > Application settings" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  Required environment variables:" -ForegroundColor Yellow
    foreach ($key in $ExpectedVars.Keys) {
        Write-Host "    $key = $($ExpectedVars[$key])" -ForegroundColor White
    }

    Write-Host ""
}

# ============================================================
# 1. Hydra Container App
# ============================================================

$hydraExpected = @{
    "URLS_SELF_ISSUER" = "https://authway.iyulab.com"
    "URLS_SELF_PUBLIC" = "https://authway.iyulab.com"
    "URLS_LOGIN" = "https://auth.iyulab.com/login"
    "URLS_CONSENT" = "https://auth.iyulab.com/consent"
    "URLS_ERROR" = "https://auth.iyulab.com/error"
    "SERVE_COOKIES_SAME_SITE_MODE" = "Lax"
    "DSN" = "*"  # Database connection string
}

Check-ContainerAppEnv -AppName "authway-hydra" -ExpectedVars $hydraExpected

# ============================================================
# 2. Backend API Container App
# ============================================================

$backendExpected = @{
    "AUTHWAY_HYDRA_ADMIN_URL" = "https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io"
    "AUTHWAY_CORS_ALLOWED_ORIGINS" = "https://authway-admin.iyulab.com,https://auth.iyulab.com,http://localhost:5173,http://localhost:3000"
    "AUTHWAY_APP_BASE_URL" = "*"  # Already set
    "AUTHWAY_APP_ENVIRONMENT" = "*"  # Already set
    "AUTHWAY_DATABASE_HOST" = "*"  # Already set
    "AUTHWAY_REDIS_HOST" = "*"  # Already set
}

Check-ContainerAppEnv -AppName "authway-api" -ExpectedVars $backendExpected

# ============================================================
# 3. Admin UI Static Web App
# ============================================================

$adminExpected = @{
    "VITE_API_URL" = "https://authway-api.iyulab.com"
}

Check-StaticWebAppEnv -AppName "authway-admin" -ExpectedVars $adminExpected

# ============================================================
# 4. Login UI Static Web App
# ============================================================

$loginExpected = @{
    "VITE_API_URL" = "https://authway-api.iyulab.com"
    "VITE_HYDRA_PUBLIC_URL" = "https://authway.iyulab.com"
}

Check-StaticWebAppEnv -AppName "authway-login" -ExpectedVars $loginExpected

# ============================================================
# Validation Summary
# ============================================================

Write-Host "================================================" -ForegroundColor DarkGray
Write-Host "Validation Summary" -ForegroundColor Cyan
Write-Host ""

if ($issues.Count -eq 0 -and $warnings.Count -eq 0) {
    Write-Host "[SUCCESS] All environment variables are correctly set!" -ForegroundColor Green
} else {
    if ($issues.Count -gt 0) {
        Write-Host "[ERROR] Found $($issues.Count) issue(s):" -ForegroundColor Red
        foreach ($issue in $issues) {
            Write-Host "  - $issue" -ForegroundColor White
        }
        Write-Host ""
    }

    if ($warnings.Count -gt 0) {
        Write-Host "[WARN] Found $($warnings.Count) warning(s):" -ForegroundColor Yellow
        foreach ($warning in $warnings) {
            Write-Host "  - $warning" -ForegroundColor White
        }
        Write-Host ""
    }
}

# ============================================================
# How to Fix
# ============================================================

if ($issues.Count -gt 0 -or $warnings.Count -gt 0) {
    Write-Host "================================================" -ForegroundColor DarkGray
    Write-Host "How to Fix Environment Variables" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "1. Update Hydra environment variables:" -ForegroundColor Cyan
    Write-Host "   .\scripts\publish-hydra.ps1 -UpdateEnvOnly" -ForegroundColor Gray
    Write-Host ""
    Write-Host "2. Update Backend API environment variables:" -ForegroundColor Cyan
    Write-Host "   Azure Portal > Container Apps > authway-api > Environment variables" -ForegroundColor Gray
    Write-Host ""
    Write-Host "3. Update Admin/Login UI environment variables:" -ForegroundColor Cyan
    Write-Host "   Azure Portal > Static Web Apps > (authway-admin/authway-login) > Configuration" -ForegroundColor Gray
    Write-Host ""
    Write-Host "4. Redeploy after changes:" -ForegroundColor Cyan
    Write-Host "   .\scripts\publish-api.ps1        # Backend API" -ForegroundColor Gray
    Write-Host "   .\scripts\publish-admin-ui.ps1   # Admin UI" -ForegroundColor Gray
    Write-Host "   .\scripts\publish-login-ui.ps1   # Login UI" -ForegroundColor Gray
    Write-Host ""
}

Write-Host "================================================" -ForegroundColor DarkGray
