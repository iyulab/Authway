# Thin wrapper — staging target. 실제 로직: ../_shared/lib/check-migration-status.core.ps1
& (Join-Path $PSScriptRoot "..\_shared\lib\check-migration-status.core.ps1") -Target "staging"
exit $LASTEXITCODE
