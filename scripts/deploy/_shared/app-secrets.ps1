# ============================================================
# Container App secrets — values the app definition must not show
# ============================================================
# A plain environment value is part of the app definition: anyone who can read
# the app (Reader on the resource group) sees it, and every revision keeps a
# copy. A Container App secret is shown only to those who can write the app.
# So a password, key or connection string goes in as a secret and the
# environment only names it (NAME=secretref:<secret>).
#
# Assert-NoPlaintextSecretEnv is the check that keeps it that way: after an
# update it fails the deploy if a variable whose name says it holds a secret
# carries its value in the clear.
# ============================================================

# Names that hold a secret: *_PASSWORD, *_KEY, *_SECRET, and the
# authorization server's DSN (it embeds the database password) and
# SECRETS_* (its system and cookie secrets).
$script:SecretEnvNamePattern = '(_PASSWORD|_KEY|_SECRET)$|^DSN$|^SECRETS_'

function Set-ContainerAppSecrets {
    param(
        [Parameter(Mandatory = $true)][string]$App,
        [Parameter(Mandatory = $true)][string]$ResourceGroup,
        # secret name -> value
        [Parameter(Mandatory = $true)][System.Collections.IDictionary]$Secrets
    )
    $pairs = @()
    foreach ($name in $Secrets.Keys) {
        if (-not $Secrets[$name]) { throw "secret '$name' has no value" }
        $pairs += "$name=$($Secrets[$name])"
    }
    Invoke-AzRetry -What "Container App secrets ($($Secrets.Keys -join ', '))" -Command {
        az containerapp secret set `
            --name $App `
            --resource-group $ResourceGroup `
            --secrets $pairs | Out-Null
    }
}

# Removes secrets the app no longer references. A secret still referenced by
# the environment cannot be removed, so this runs after the update that
# stopped referencing it.
function Remove-StaleContainerAppSecrets {
    param(
        [Parameter(Mandatory = $true)][string]$App,
        [Parameter(Mandatory = $true)][string]$ResourceGroup,
        [Parameter(Mandatory = $true)][string[]]$Names
    )
    $present = @(az containerapp secret list --name $App --resource-group $ResourceGroup --query "[].name" -o tsv)
    if ($LASTEXITCODE -ne 0) { throw "could not list secrets of $App" }
    $toRemove = @($Names | Where-Object { $present -contains $_ })
    if ($toRemove.Count -eq 0) { return }
    Write-Host "🧹 더 이상 참조하지 않는 비밀 제거: $($toRemove -join ', ')" -ForegroundColor Yellow
    Invoke-AzRetry -What "stale secret removal" -Command {
        az containerapp secret remove --name $App --resource-group $ResourceGroup --secret-names $toRemove | Out-Null
    }
}

function Assert-NoPlaintextSecretEnv {
    param(
        [Parameter(Mandatory = $true)][string]$App,
        [Parameter(Mandatory = $true)][string]$ResourceGroup
    )
    # Only names and whether a value is inline — never the values.
    # A variable without a secretRef carries its value inline. (Not "value is
    # set": a secretref entry can come back with value "" after an update.)
    # Flattened across containers: a nested list comes out of tsv as one
    # tab-joined line per container, which no name pattern would match.
    $rows = az containerapp show --name $App --resource-group $ResourceGroup `
        --query "properties.template.containers[].env[] | [?secretRef==null].name" -o tsv
    if ($LASTEXITCODE -ne 0) { throw "could not read the environment of $App" }
    $names = @($rows | ForEach-Object { "$_" -split '\s+' } | Where-Object { $_ })
    if ($names.Count -eq 0) { throw "read no environment variables of $App — refusing to report it clean" }
    $plain = @($names | Where-Object { $_ -match $script:SecretEnvNamePattern })
    if ($plain.Count -gt 0) {
        Write-Host "  ❌ 평문 값으로 들어 있는 비밀: $($plain -join ', ')" -ForegroundColor Red
        Write-Host "     Container App 비밀로 넣고 secretref 로 참조해야 합니다(scripts/deploy/_shared/app-secrets.ps1)." -ForegroundColor Yellow
        throw "plaintext secret in the environment of ${App}: $($plain -join ', ')"
    }
    Write-Host "  ✓ $App — 비밀은 전부 secretref" -ForegroundColor Green
}
