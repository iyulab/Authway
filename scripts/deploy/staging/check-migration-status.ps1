# Thin wrapper — staging target. 실제 로직: ../_shared/lib/check-migration-status.core.ps1
param(
    # The versions the deployed API ships (its /health names them); empty = this working tree.
    [string]$Shipped = ""
)
& (Join-Path $PSScriptRoot "..\_shared\lib\check-migration-status.core.ps1") -Target "staging" -Shipped $Shipped
exit $LASTEXITCODE
