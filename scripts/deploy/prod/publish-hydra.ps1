# Thin wrapper — prod target. 실제 로직: ../_shared/lib/publish-hydra.core.ps1
param(
    [switch]$UpdateEnvOnly
)
& (Join-Path $PSScriptRoot "..\_shared\lib\publish-hydra.core.ps1") `
    -Target "prod" -UpdateEnvOnly:$UpdateEnvOnly
exit $LASTEXITCODE
