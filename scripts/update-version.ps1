# Authway Version Update Script
# Usage: .\scripts\update-version.ps1 -Version "0.1.0"

param(
    [Parameter(Mandatory=$true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"

Write-Host "Updating Authway version to $Version..." -ForegroundColor Cyan

# Validate version format
if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    Write-Host "Invalid version format. Use semantic versioning (e.g., 0.1.0)" -ForegroundColor Red
    exit 1
}

$files = @(
    "packages\web\admin-dashboard\package.json",
    "packages\web\login-ui\package.json",
    ".env",
    ".env.example",
    ".env.production.example",
    "docker-compose.dev.yml",
    "docker-compose.prod.yml",
    "src\server\internal\config\config.go",
    "configs\config.production.yaml",
    "configs\production.yaml"
)

$updatedCount = 0

foreach ($file in $files) {
    if (Test-Path $file) {
        Write-Host "Updating: $file" -ForegroundColor Green
        $content = Get-Content $file -Raw -Encoding UTF8

        # Update different patterns based on file type
        if ($file -match "\.json$") {
            $content = $content -replace '"version":\s*"[\d\.]+"', """version"": ""$Version"""
        }
        elseif ($file -match "\.env|\.yml$|\.yaml$") {
            $content = $content -replace 'AUTHWAY_APP_VERSION[=:]?\s*[\d\.]+', "AUTHWAY_APP_VERSION=$Version"
            $content = $content -replace 'version:\s*[\d\.]+', "version: $Version"
        }
        elseif ($file -match "\.go$") {
            $content = $content -replace 'viper\.SetDefault\("app\.version",\s*"[\d\.]+"\)', "viper.SetDefault(""app.version"", ""$Version"")"
        }

        Set-Content $file $content -NoNewline -Encoding UTF8
        $updatedCount++
    }
}

Write-Host ""
Write-Host "Version update complete!" -ForegroundColor Green
Write-Host "Updated $updatedCount files to version $Version" -ForegroundColor Cyan
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. Review changes: git diff"
Write-Host "  2. Update package-lock.json: cd packages/web/admin-dashboard && npm install"
Write-Host "  3. Update package-lock.json: cd packages/web/login-ui && npm install"
Write-Host "  4. Commit: git add . && git commit -m 'chore: bump version to $Version'"
Write-Host "  5. Tag: git tag v$Version && git push --tags"
