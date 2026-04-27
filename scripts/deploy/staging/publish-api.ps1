# Thin wrapper — staging target. 실제 로직: ../_shared/lib/publish-api.core.ps1
param(
    [string]$ImageTag = "",
    [switch]$SkipBuild
)
& (Join-Path $PSScriptRoot "..\_shared\lib\publish-api.core.ps1") `
    -Target "staging" -ImageTag $ImageTag -SkipBuild:$SkipBuild
exit $LASTEXITCODE
