# Thin wrapper — prod target. 실제 로직: ../_shared/lib/publish-auth-ui.core.ps1
& (Join-Path $PSScriptRoot "..\_shared\lib\publish-auth-ui.core.ps1") -Target "prod"
exit $LASTEXITCODE
