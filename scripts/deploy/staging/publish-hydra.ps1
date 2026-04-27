# Thin wrapper — staging target. 실제 로직: ../_shared/lib/publish-hydra.core.ps1
param(
    [switch]$UpdateEnvOnly
)
& (Join-Path $PSScriptRoot "..\_shared\lib\publish-hydra.core.ps1") `
    -Target "staging" -UpdateEnvOnly:$UpdateEnvOnly
exit $LASTEXITCODE
