# Authway Packages Publishing Script
# Publishes @authway/client and @authway/react to npm

$ErrorActionPreference = "Stop"

$clientPackagePath = ".\client"
$reactPackagePath = ".\react"

function Get-PackageVersion {
    param([string]$packagePath)

    $packageJsonPath = Join-Path $packagePath "package.json"
    if (-not (Test-Path $packageJsonPath)) {
        throw "package.json not found at: $packageJsonPath"
    }

    $packageJson = Get-Content $packageJsonPath -Raw | ConvertFrom-Json
    return $packageJson.version
}

function Set-PackageVersion {
    param(
        [string]$packagePath,
        [string]$newVersion
    )

    $packageJsonPath = Join-Path $packagePath "package.json"
    $content = Get-Content $packageJsonPath -Raw
    $packageJson = $content | ConvertFrom-Json
    $packageJson.version = $newVersion

    $content | Set-Content $packageJsonPath
    $updatedContent = $packageJson | ConvertTo-Json -Depth 10
    $updatedContent | Set-Content $packageJsonPath -Encoding UTF8
}

# Display header
Write-Host ""
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "  Authway Packages Publishing" -ForegroundColor Cyan
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host ""

# Check if we're in the packages directory
if (-not (Test-Path $clientPackagePath) -or -not (Test-Path $reactPackagePath)) {
    Write-Host "Error: Must run this script from the packages directory" -ForegroundColor Red
    Write-Host "Current directory: $(Get-Location)" -ForegroundColor Red
    exit 1
}

# Get current versions
try {
    $clientVersion = Get-PackageVersion $clientPackagePath
    $reactVersion = Get-PackageVersion $reactPackagePath
} catch {
    Write-Host "Error reading package versions: $_" -ForegroundColor Red
    exit 1
}

# Display current versions
Write-Host "Current Versions:" -ForegroundColor Yellow
Write-Host "  @authway/client: $clientVersion" -ForegroundColor White
Write-Host "  @authway/react:  $reactVersion" -ForegroundColor White
Write-Host ""

# Prompt for new version
Write-Host "Enter new version (e.g., 0.1.1, 0.2.0): " -ForegroundColor Green -NoNewline
$newVersion = Read-Host

# Validate version format
if ($newVersion -notmatch '^\d+\.\d+\.\d+(-[a-zA-Z0-9.-]+)?$') {
    Write-Host "Error: Invalid version format. Use semantic versioning (e.g., 0.1.1)" -ForegroundColor Red
    exit 1
}

# Confirm version update
Write-Host ""
Write-Host "Will update both packages to version: $newVersion" -ForegroundColor Yellow
Write-Host "Continue? (Y/N): " -ForegroundColor Green -NoNewline
$confirm = Read-Host

if ($confirm -ne 'Y' -and $confirm -ne 'y') {
    Write-Host "Publishing cancelled." -ForegroundColor Yellow
    exit 0
}

Write-Host ""
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "  Step 1: Updating Versions" -ForegroundColor Cyan
Write-Host "=====================================" -ForegroundColor Cyan

# Update versions in package.json files
try {
    Write-Host "Updating @authway/client..." -ForegroundColor Yellow
    Set-PackageVersion $clientPackagePath $newVersion
    Write-Host " @authway/client updated to $newVersion" -ForegroundColor Green

    Write-Host "Updating @authway/react..." -ForegroundColor Yellow
    Set-PackageVersion $reactPackagePath $newVersion
    Write-Host " @authway/react updated to $newVersion" -ForegroundColor Green
} catch {
    Write-Host "Error updating versions: $_" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "  Step 2: Building Packages" -ForegroundColor Cyan
Write-Host "=====================================" -ForegroundColor Cyan

# Build client package
Write-Host ""
Write-Host "Building @authway/client..." -ForegroundColor Yellow
Push-Location $clientPackagePath
try {
    pnpm build
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed for @authway/client"
    }
    Write-Host " @authway/client built successfully" -ForegroundColor Green
} catch {
    Write-Host "Error building @authway/client: $_" -ForegroundColor Red
    Pop-Location
    exit 1
}
Pop-Location

# Build react package
Write-Host ""
Write-Host "Building @authway/react..." -ForegroundColor Yellow
Push-Location $reactPackagePath
try {
    pnpm build
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed for @authway/react"
    }
    Write-Host " @authway/react built successfully" -ForegroundColor Green
} catch {
    Write-Host "Error building @authway/react: $_" -ForegroundColor Red
    Pop-Location
    exit 1
}
Pop-Location

Write-Host ""
Write-Host "=====================================" -ForegroundColor Cyan
Write-Host "  Step 3: Publishing to npm" -ForegroundColor Cyan
Write-Host "=====================================" -ForegroundColor Cyan

# Publish client package
Write-Host ""
Write-Host "Publishing @authway/client..." -ForegroundColor Yellow
Push-Location $clientPackagePath
try {
    npm publish --access public
    if ($LASTEXITCODE -ne 0) {
        throw "Publishing failed for @authway/client"
    }
    Write-Host " @authway/client@$newVersion published successfully" -ForegroundColor Green
} catch {
    Write-Host "Error publishing @authway/client: $_" -ForegroundColor Red
    Pop-Location
    exit 1
}
Pop-Location

# Publish react package
Write-Host ""
Write-Host "Publishing @authway/react..." -ForegroundColor Yellow
Push-Location $reactPackagePath
try {
    npm publish --access public
    if ($LASTEXITCODE -ne 0) {
        throw "Publishing failed for @authway/react"
    }
    Write-Host " @authway/react@$newVersion published successfully" -ForegroundColor Green
} catch {
    Write-Host "Error publishing @authway/react: $_" -ForegroundColor Red
    Pop-Location
    exit 1
}
Pop-Location

# Success summary
Write-Host ""
Write-Host "=====================================" -ForegroundColor Green
Write-Host "  Publishing Complete! " -ForegroundColor Green
Write-Host "=====================================" -ForegroundColor Green
Write-Host ""
Write-Host "Published packages:" -ForegroundColor Yellow
Write-Host "   @authway/client@$newVersion" -ForegroundColor Green
Write-Host "   @authway/react@$newVersion" -ForegroundColor Green
Write-Host ""
Write-Host "View on npm:" -ForegroundColor Yellow
Write-Host "  https://www.npmjs.com/package/@authway/client" -ForegroundColor Cyan
Write-Host "  https://www.npmjs.com/package/@authway/react" -ForegroundColor Cyan
Write-Host ""
