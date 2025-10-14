# ============================================================
# Authway Backend API 배포 스크립트
# ============================================================
# Docker 이미지를 빌드하고 Azure Container Apps에 배포합니다.
# ============================================================

param(
    [string]$ResourceGroup = "authway",
    [string]$ContainerApp = "authway-api",
    [string]$Registry = "authwayacr",
    [string]$ImageTag = "latest",
    [switch]$SkipBuild,
    [switch]$UseAzureBuild
)

Write-Host "🚀 Authway Backend API 배포 시작..." -ForegroundColor Cyan
Write-Host ""

# 스크립트 루트 디렉토리
$ScriptRoot = Split-Path -Parent $PSScriptRoot

# Azure CLI 로그인 확인
Write-Host "🔑 Azure 인증 확인 중..." -ForegroundColor Yellow
try {
    $account = az account show 2>&1 | ConvertFrom-Json
    Write-Host "✓ Azure 로그인됨: $($account.user.name)" -ForegroundColor Green
} catch {
    Write-Host "❌ Azure에 로그인되지 않았습니다." -ForegroundColor Red
    Write-Host "  az login을 실행하세요." -ForegroundColor Gray
    exit 1
}

# 프로젝트 루트로 이동
Push-Location $ScriptRoot

try {
    if (-not $SkipBuild) {
        # Docker 이미지 이름
        $ImageName = "$Registry.azurecr.io/authway-backend:$ImageTag"

        if ($UseAzureBuild) {
            # Azure Container Registry에서 빌드
            Write-Host ""
            Write-Host "🔨 Azure Container Registry에서 빌드 중..." -ForegroundColor Yellow
            Write-Host "  이미지: $ImageName" -ForegroundColor Gray

            az acr build `
                --registry $Registry `
                --resource-group $ResourceGroup `
                --image "authway-backend:$ImageTag" `
                --file Dockerfile `
                .

            if ($LASTEXITCODE -ne 0) {
                throw "Azure ACR 빌드 실패"
            }
        } else {
            # 로컬에서 Docker 빌드
            Write-Host ""
            Write-Host "🔨 Docker 이미지 빌드 중..." -ForegroundColor Yellow
            Write-Host "  이미지: $ImageName" -ForegroundColor Gray

            docker build -t "authway-backend:$ImageTag" -f Dockerfile .
            if ($LASTEXITCODE -ne 0) {
                throw "Docker 빌드 실패"
            }

            # 이미지 태깅
            Write-Host ""
            Write-Host "🏷️  이미지 태깅 중..." -ForegroundColor Yellow
            docker tag "authway-backend:$ImageTag" $ImageName
            if ($LASTEXITCODE -ne 0) {
                throw "Docker 태깅 실패"
            }

            # ACR 로그인
            Write-Host ""
            Write-Host "🔐 Azure Container Registry 로그인 중..." -ForegroundColor Yellow
            az acr login --name $Registry
            if ($LASTEXITCODE -ne 0) {
                throw "ACR 로그인 실패"
            }

            # ACR에 푸시
            Write-Host ""
            Write-Host "☁️  Azure Container Registry에 푸시 중..." -ForegroundColor Yellow
            docker push $ImageName
            if ($LASTEXITCODE -ne 0) {
                throw "Docker 푸시 실패"
            }
        }

        Write-Host "✓ 이미지 빌드 및 푸시 완료" -ForegroundColor Green
    } else {
        Write-Host "⏭️  이미지 빌드 건너뜀 (--SkipBuild)" -ForegroundColor Yellow
        $ImageName = "$Registry.azurecr.io/authway-backend:$ImageTag"
    }

    # Container App 업데이트
    Write-Host ""
    Write-Host "📦 Container App 업데이트 중..." -ForegroundColor Yellow
    Write-Host "  Container App: $ContainerApp" -ForegroundColor Gray
    Write-Host "  이미지: $ImageName" -ForegroundColor Gray

    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --image $ImageName

    if ($LASTEXITCODE -ne 0) {
        throw "Container App 업데이트 실패"
    }

    # 배포 완료 확인
    Write-Host ""
    Write-Host "⏳ 배포 상태 확인 중..." -ForegroundColor Yellow
    Start-Sleep -Seconds 5

    $appInfo = az containerapp show `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --query "{status:properties.runningStatus, revision:properties.latestRevisionName, fqdn:properties.configuration.ingress.fqdn}" `
        -o json | ConvertFrom-Json

    Write-Host ""
    Write-Host "✅ Backend API 배포 완료!" -ForegroundColor Green
    Write-Host ""
    Write-Host "📌 배포 정보:" -ForegroundColor Cyan
    Write-Host "   상태: $($appInfo.status)" -ForegroundColor White
    Write-Host "   리비전: $($appInfo.revision)" -ForegroundColor White
    Write-Host "   URL: https://$($appInfo.fqdn)" -ForegroundColor White
    Write-Host ""
    Write-Host "🔍 Health Check 테스트:" -ForegroundColor Cyan
    Write-Host "   curl https://$($appInfo.fqdn)/health" -ForegroundColor Gray
    Write-Host ""
    Write-Host "📊 로그 확인:" -ForegroundColor Cyan
    Write-Host "   az containerapp logs show --name $ContainerApp --resource-group $ResourceGroup --follow" -ForegroundColor Gray
    Write-Host ""

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
} finally {
    # 원래 디렉토리로 복귀
    Pop-Location
}
