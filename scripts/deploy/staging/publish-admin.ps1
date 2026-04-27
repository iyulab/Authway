# Thin wrapper — staging target. 실제 로직: ../_shared/lib/publish-admin.core.ps1
& (Join-Path $PSScriptRoot "..\_shared\lib\publish-admin.core.ps1") -Target "staging"
exit $LASTEXITCODE
