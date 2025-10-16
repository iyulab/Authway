# ============================================================
# Authway Hydra 배포 스크립트
# ============================================================
# Nginx 프록시 이미지를 빌드하고 Hydra Container App에 배포합니다.
# ============================================================

param(
    [string]$ResourceGroup = "authway",
    [string]$ContainerApp = "authway-hydra",
    [string]$Registry = "authwayacr",
    [string]$ImageTag = "latest",
    [switch]$SkipBuild,
    [switch]$UseAzureBuild,
    [switch]$UpdateEnvOnly
)

Write-Host "🚀 Authway Hydra 배포 시작..." -ForegroundColor Cyan
Write-Host ""

# 스크립트 디렉토리
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

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

# 환경 변수만 업데이트하는 경우
if ($UpdateEnvOnly) {
    Write-Host ""
    Write-Host "🔧 환경 변수만 업데이트합니다..." -ForegroundColor Yellow

    Write-Host ""
    Write-Host "📝 Hydra 환경 변수 설정 중..." -ForegroundColor Yellow

    Write-Host ""
    Write-Host "  설정할 환경 변수:" -ForegroundColor Gray
    Write-Host "    - URLS_SELF_ISSUER=https://authway.iyulab.com" -ForegroundColor White
    Write-Host "    - URLS_SELF_PUBLIC=https://authway.iyulab.com" -ForegroundColor White
    Write-Host "    - URLS_LOGIN=https://auth.iyulab.com/login" -ForegroundColor White
    Write-Host "    - URLS_CONSENT=https://auth.iyulab.com/consent" -ForegroundColor White
    Write-Host "    - URLS_ERROR=https://auth.iyulab.com/error" -ForegroundColor White
    Write-Host "    - SERVE_COOKIES_SAME_SITE_MODE=Lax" -ForegroundColor White

    # 각 환경 변수를 개별적으로 설정
    Write-Host ""
    Write-Host "  URLS_SELF_ISSUER 설정 중..." -ForegroundColor Gray
    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --container-name "authway-hydra" `
        --set-env-vars "URLS_SELF_ISSUER=https://authway.iyulab.com"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ URLS_SELF_ISSUER 설정 실패" -ForegroundColor Red
        exit 1
    }

    Write-Host "  URLS_SELF_PUBLIC 설정 중..." -ForegroundColor Gray
    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --container-name "authway-hydra" `
        --set-env-vars "URLS_SELF_PUBLIC=https://authway.iyulab.com"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ URLS_SELF_PUBLIC 설정 실패" -ForegroundColor Red
        exit 1
    }

    Write-Host "  URLS_LOGIN 설정 중..." -ForegroundColor Gray
    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --container-name "authway-hydra" `
        --set-env-vars "URLS_LOGIN=https://auth.iyulab.com/login"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ URLS_LOGIN 설정 실패" -ForegroundColor Red
        exit 1
    }

    Write-Host "  URLS_CONSENT 설정 중..." -ForegroundColor Gray
    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --container-name "authway-hydra" `
        --set-env-vars "URLS_CONSENT=https://auth.iyulab.com/consent"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ URLS_CONSENT 설정 실패" -ForegroundColor Red
        exit 1
    }

    Write-Host "  URLS_ERROR 설정 중..." -ForegroundColor Gray
    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --container-name "authway-hydra" `
        --set-env-vars "URLS_ERROR=https://auth.iyulab.com/error"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ URLS_ERROR 설정 실패" -ForegroundColor Red
        exit 1
    }

    Write-Host "  SERVE_COOKIES_SAME_SITE_MODE 설정 중..." -ForegroundColor Gray
    az containerapp update `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --container-name "authway-hydra" `
        --set-env-vars "SERVE_COOKIES_SAME_SITE_MODE=Lax"

    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ SERVE_COOKIES_SAME_SITE_MODE 설정 실패" -ForegroundColor Red
        exit 1
    }

    Write-Host ""
    Write-Host "✅ 환경 변수 업데이트 완료!" -ForegroundColor Green
    Write-Host ""

    # Get latest revision and restart
    Write-Host "🔄 Container App 재시작 중..." -ForegroundColor Yellow
    $latestRevision = az containerapp revision list `
        --name $ContainerApp `
        --resource-group $ResourceGroup `
        --query "[0].name" `
        -o tsv

    if ($latestRevision) {
        Write-Host "  Revision: $latestRevision" -ForegroundColor Gray

        az containerapp revision restart `
            --name $ContainerApp `
            --resource-group $ResourceGroup `
            --revision $latestRevision

        if ($LASTEXITCODE -eq 0) {
            Write-Host ""
            Write-Host "✅ Container App 재시작 완료!" -ForegroundColor Green
            Write-Host ""
            Write-Host "⏳ 변경사항이 적용될 때까지 약 30-60초 소요됩니다." -ForegroundColor Yellow
        } else {
            Write-Host ""
            Write-Host "⚠️  자동 재시작 실패. 수동으로 재시작하세요:" -ForegroundColor Yellow
            Write-Host "   az containerapp revision restart --name $ContainerApp --resource-group $ResourceGroup --revision $latestRevision" -ForegroundColor Gray
        }
    } else {
        Write-Host "⚠️  Revision을 찾을 수 없습니다. Azure Portal에서 수동으로 재시작하세요." -ForegroundColor Yellow
    }

    Write-Host ""
    exit 0
}

# scripts 디렉토리로 이동 (nginx 설정 파일이 여기 있음)
Push-Location $ScriptDir

try {
    if (-not $SkipBuild) {
        # Nginx 이미지 이름
        $ImageName = "$Registry.azurecr.io/authway-nginx:$ImageTag"

        # nginx 설정 파일 확인
        if (-not (Test-Path "hydra-nginx.conf")) {
            throw "hydra-nginx.conf 파일을 찾을 수 없습니다."
        }

        if (-not (Test-Path "Dockerfile.nginx")) {
            throw "Dockerfile.nginx 파일을 찾을 수 없습니다."
        }

        if ($UseAzureBuild) {
            # Azure Container Registry에서 빌드
            Write-Host ""
            Write-Host "🔨 Azure Container Registry에서 Nginx 이미지 빌드 중..." -ForegroundColor Yellow
            Write-Host "  이미지: $ImageName" -ForegroundColor Gray

            az acr build `
                --registry $Registry `
                --resource-group $ResourceGroup `
                --image "authway-nginx:$ImageTag" `
                --file Dockerfile.nginx `
                .

            if ($LASTEXITCODE -ne 0) {
                throw "Azure ACR 빌드 실패"
            }
        } else {
            # 로컬에서 Docker 빌드
            Write-Host ""
            Write-Host "🔨 Nginx Docker 이미지 빌드 중..." -ForegroundColor Yellow
            Write-Host "  이미지: $ImageName" -ForegroundColor Gray

            docker build -t "authway-nginx:$ImageTag" -f Dockerfile.nginx .
            if ($LASTEXITCODE -ne 0) {
                throw "Docker 빌드 실패"
            }

            # 이미지 태깅
            Write-Host ""
            Write-Host "🏷️  이미지 태깅 중..." -ForegroundColor Yellow
            docker tag "authway-nginx:$ImageTag" $ImageName
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

        Write-Host "✓ Nginx 이미지 빌드 및 푸시 완료" -ForegroundColor Green
    } else {
        Write-Host "⏭️  이미지 빌드 건너뜀 (--SkipBuild)" -ForegroundColor Yellow
    }

    # Container App 환경 변수 및 이미지 업데이트
    Write-Host ""
    Write-Host "📦 Container App 업데이트 중..." -ForegroundColor Yellow
    Write-Host "  Container App: $ContainerApp" -ForegroundColor Gray

    if (-not $SkipBuild) {
        Write-Host "  Nginx 이미지: $ImageName" -ForegroundColor Gray
    }

    # Hydra 필수 환경 변수
    Write-Host ""
    Write-Host "📝 Hydra 환경 변수 설정 중..." -ForegroundColor Yellow

    $envVars = @(
        "URLS_SELF_ISSUER=https://authway.iyulab.com",
        "URLS_SELF_PUBLIC=https://authway.iyulab.com",
        "URLS_LOGIN=https://auth.iyulab.com/login",
        "URLS_CONSENT=https://auth.iyulab.com/consent",
        "URLS_ERROR=https://auth.iyulab.com/error",
        "SERVE_COOKIES_SAME_SITE_MODE=Lax"
    )

    Write-Host "  설정할 환경 변수:" -ForegroundColor Gray
    foreach ($env in $envVars) {
        Write-Host "    - $env" -ForegroundColor White
    }

    $envString = $envVars -join " "

    if (-not $SkipBuild) {
        # 이미지와 환경 변수 모두 업데이트
        az containerapp update `
            --name $ContainerApp `
            --resource-group $ResourceGroup `
            --container-name "authway-hydra" `
            --set-env-vars $envString

        # 참고: nginx 이미지는 별도로 업데이트해야 함 (sidecar 컨테이너)
        # Container App에서 수동으로 설정하거나 ARM 템플릿 사용 필요
    } else {
        # 환경 변수만 업데이트
        az containerapp update `
            --name $ContainerApp `
            --resource-group $ResourceGroup `
            --container-name "authway-hydra" `
            --set-env-vars $envString
    }

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
    Write-Host "✅ Hydra 배포 완료!" -ForegroundColor Green
    Write-Host ""
    Write-Host "📌 배포 정보:" -ForegroundColor Cyan
    Write-Host "   상태: $($appInfo.status)" -ForegroundColor White
    Write-Host "   리비전: $($appInfo.revision)" -ForegroundColor White
    Write-Host "   URL: https://$($appInfo.fqdn)" -ForegroundColor White
    Write-Host "   커스텀 도메인: https://authway.iyulab.com" -ForegroundColor White
    Write-Host ""
    Write-Host "🔍 Health Check 테스트:" -ForegroundColor Cyan
    Write-Host "   curl https://authway.iyulab.com/health/ready" -ForegroundColor Gray
    Write-Host ""
    Write-Host "🧪 Admin API 테스트 (내부에서만 접근 가능):" -ForegroundColor Cyan
    Write-Host "   curl https://authway.iyulab.com/admin/health/ready" -ForegroundColor Gray
    Write-Host ""
    Write-Host "📊 로그 확인:" -ForegroundColor Cyan
    Write-Host "   az containerapp logs show --name $ContainerApp --resource-group $ResourceGroup --follow" -ForegroundColor Gray
    Write-Host ""

    if (-not $SkipBuild) {
        Write-Host "⚠️  참고: Nginx 사이드카 컨테이너는 Azure Portal에서 수동 업데이트 필요" -ForegroundColor Yellow
        Write-Host "   Portal → Container Apps → $ContainerApp → Containers → authway-nginx" -ForegroundColor Gray
        Write-Host "   이미지: $ImageName" -ForegroundColor Gray
        Write-Host ""
    }

} catch {
    Write-Host ""
    Write-Host "❌ 배포 실패: $_" -ForegroundColor Red
    Write-Host ""
    exit 1
} finally {
    # 원래 디렉토리로 복귀
    Pop-Location
}
