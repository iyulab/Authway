# ============================================================
# Hydra shared settings (configs/hydra.shared.env)
# ============================================================
# The authorization-server settings every deployment shares are written once,
# in configs/hydra.shared.env, which local development and CI read directly.
# The deploy (publish-hydra) and its check (verify-hydra-env) read them from
# here, so a setting added to that file reaches every environment.
#
# The one value a deployment extends is OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS: the
# names in HYDRA_ALLOWED_TOP_LEVEL_CLAIMS (deploy .env) are appended to the
# shared list. Those are a deployment's own claim names and belong in its
# environment, not in the shared file.
# ============================================================

function Get-HydraSharedEnv {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$EnvVars
    )

    $repoRoot = Split-Path -Parent (Split-Path -Parent (Split-Path -Parent $PSScriptRoot))
    $path = Join-Path $repoRoot "configs\hydra.shared.env"
    if (-not (Test-Path $path)) {
        throw "Hydra shared settings not found: $path"
    }

    $shared = [ordered]@{}
    foreach ($line in Get-Content $path) {
        $line = $line.Trim()
        if (-not $line -or $line.StartsWith('#')) { continue }
        $eq = $line.IndexOf('=')
        if ($eq -le 0) { throw "Not KEY=value in ${path}: $line" }
        $shared[$line.Substring(0, $eq)] = $line.Substring($eq + 1)
    }
    if ($shared.Count -eq 0) { throw "No settings in $path" }

    $extra = $EnvVars['HYDRA_ALLOWED_TOP_LEVEL_CLAIMS']
    if ($extra) {
        $names = @($shared['OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS'].Split(','))
        $names += $extra.Split(',') | ForEach-Object { $_.Trim() } | Where-Object { $_ }
        $shared['OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS'] = ($names | Select-Object -Unique) -join ','
    }
    return $shared
}
