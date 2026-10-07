# Thin wrapper — staging target. 실제 로직: ../_shared/lib/deploy-all.core.ps1
param(
    [switch]$SkipBuild,
    [switch]$SkipHealthCheck,
    [string[]]$Services = @("hydra", "api", "admin", "auth-ui")
)
& (Join-Path $PSScriptRoot "..\_shared\lib\deploy-all.core.ps1") `
    -Target "staging" `
    -SkipBuild:$SkipBuild `
    -SkipHealthCheck:$SkipHealthCheck `
    -Services $Services
exit $LASTEXITCODE
