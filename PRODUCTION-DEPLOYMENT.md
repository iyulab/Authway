# 🚀 Authway 프로덕션 배포 가이드

Authway를 프로덕션 환경에 배포하기 위한 완전한 가이드입니다.

---

## 📋 목차

1. [사전 준비사항](#사전-준비사항)
2. [환경 설정](#환경-설정)
3. [보안 설정](#보안-설정)
4. [배포 방법](#배포-방법)
5. [모니터링](#모니터링)
6. [백업 및 복구](#백업-및-복구)
7. [문제 해결](#문제-해결)

---

## 사전 준비사항

### 시스템 요구사항

- **OS**: Ubuntu 20.04+ / Debian 11+ / CentOS 8+
- **CPU**: 최소 2 cores (권장 4+ cores)
- **메모리**: 최소 4GB RAM (권장 8GB+)
- **디스크**: 최소 20GB SSD
- **네트워크**: 고정 IP 또는 도메인

### 필수 소프트웨어

```bash
# Docker 설치
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Docker Compose 설치
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# 설치 확인
docker --version
docker-compose --version
```

### 도메인 및 DNS 설정

프로덕션 배포를 위해서는 다음 도메인이 필요합니다:

- `auth.yourdomain.com` - Authway API 서버
- `login.yourdomain.com` - 로그인 UI
- `admin.yourdomain.com` - 관리자 대시보드

**DNS A 레코드 설정:**
```
auth.yourdomain.com   -> 서버 IP
login.yourdomain.com  -> 서버 IP
admin.yourdomain.com  -> 서버 IP
```

---

## 환경 설정

### 1. 프로젝트 클론

```bash
git clone https://github.com/yourusername/authway.git
cd authway
```

### 2. 환경 변수 설정

```bash
# .env.production 파일 생성
cp .env.production.example .env.production

# 환경 변수 편집
nano .env.production
```

**필수 설정 항목:**

```bash
# 애플리케이션 URL
APP_BASE_URL=https://auth.yourdomain.com

# 강력한 비밀번호 생성 (예시)
POSTGRES_PASSWORD=$(openssl rand -base64 32)
REDIS_PASSWORD=$(openssl rand -base64 32)
JWT_ACCESS_TOKEN_SECRET=$(openssl rand -base64 32)
JWT_REFRESH_TOKEN_SECRET=$(openssl rand -base64 32)

# CORS 설정
CORS_ALLOWED_ORIGINS=https://login.yourdomain.com,https://admin.yourdomain.com

# 이메일 설정 (SendGrid 예시)
EMAIL_SMTP_HOST=smtp.sendgrid.net
EMAIL_SMTP_PORT=587
EMAIL_SMTP_USER=apikey
EMAIL_SMTP_PASSWORD=YOUR_SENDGRID_API_KEY
EMAIL_FROM_EMAIL=noreply@yourdomain.com
```

### 3. SSL/TLS 인증서 설정

**Let's Encrypt를 사용한 무료 SSL 인증서:**

```bash
# Certbot 설치
sudo apt-get update
sudo apt-get install certbot

# 인증서 발급 (도메인별로 실행)
sudo certbot certonly --standalone -d auth.yourdomain.com
sudo certbot certonly --standalone -d login.yourdomain.com
sudo certbot certonly --standalone -d admin.yourdomain.com

# 인증서 위치 확인
# /etc/letsencrypt/live/auth.yourdomain.com/fullchain.pem
# /etc/letsencrypt/live/auth.yourdomain.com/privkey.pem
```

**Nginx 설정 파일 생성:**

```bash
mkdir -p nginx
nano nginx/nginx.conf
```

**기본 Nginx 설정 (nginx/nginx.conf):**

```nginx
events {
    worker_connections 1024;
}

http {
    upstream authway_api {
        server authway-api:8080;
    }

    upstream login_ui {
        server login-ui:80;
    }

    upstream admin_dashboard {
        server admin-dashboard:80;
    }

    # API 서버
    server {
        listen 80;
        server_name auth.yourdomain.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name auth.yourdomain.com;

        ssl_certificate /etc/letsencrypt/live/auth.yourdomain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/auth.yourdomain.com/privkey.pem;

        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;

        location / {
            proxy_pass http://authway_api;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }

    # Login UI
    server {
        listen 80;
        server_name login.yourdomain.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name login.yourdomain.com;

        ssl_certificate /etc/letsencrypt/live/login.yourdomain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/login.yourdomain.com/privkey.pem;

        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;

        location / {
            proxy_pass http://login_ui;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }

    # Admin Dashboard
    server {
        listen 80;
        server_name admin.yourdomain.com;
        return 301 https://$server_name$request_uri;
    }

    server {
        listen 443 ssl http2;
        server_name admin.yourdomain.com;

        ssl_certificate /etc/letsencrypt/live/admin.yourdomain.com/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/admin.yourdomain.com/privkey.pem;

        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;

        location / {
            proxy_pass http://admin_dashboard;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
    }
}
```

---

## 보안 설정

### 1. 방화벽 설정

```bash
# UFW 방화벽 설정 (Ubuntu/Debian)
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw enable

# 상태 확인
sudo ufw status
```

### 2. 비밀번호 및 시크릿 키 생성

```bash
# 강력한 비밀번호 생성 스크립트
cat > generate-secrets.sh << 'EOF'
#!/bin/bash
echo "# Generated Secrets ($(date))"
echo ""
echo "POSTGRES_PASSWORD=$(openssl rand -base64 32)"
echo "REDIS_PASSWORD=$(openssl rand -base64 32)"
echo "JWT_ACCESS_TOKEN_SECRET=$(openssl rand -base64 32)"
echo "JWT_REFRESH_TOKEN_SECRET=$(openssl rand -base64 32)"
EOF

chmod +x generate-secrets.sh
./generate-secrets.sh
```

### 3. 환경 변수 파일 보호

```bash
# .env.production 파일 권한 설정
chmod 600 .env.production

# root만 읽을 수 있도록 설정
sudo chown root:root .env.production
```

---

## 배포 방법

### 기본 배포 (Nginx 없이)

```bash
# 이미지 빌드
docker-compose -f docker-compose.prod.yml build

# 서비스 시작
docker-compose -f docker-compose.prod.yml up -d

# 로그 확인
docker-compose -f docker-compose.prod.yml logs -f
```

### Nginx와 함께 배포

```bash
# SSL 인증서 볼륨 마운트 설정
mkdir -p nginx/ssl
sudo cp /etc/letsencrypt/live/*/fullchain.pem nginx/ssl/
sudo cp /etc/letsencrypt/live/*/privkey.pem nginx/ssl/

# Nginx 프로필과 함께 시작
docker-compose -f docker-compose.prod.yml --profile with-nginx up -d

# 상태 확인
docker-compose -f docker-compose.prod.yml ps
```

### 헬스 체크

```bash
# API 서버 확인
curl https://auth.yourdomain.com/health

# Login UI 확인
curl https://login.yourdomain.com

# Admin Dashboard 확인
curl https://admin.yourdomain.com
```

---

## 모니터링

### 로그 관리

```bash
# 모든 서비스 로그
docker-compose -f docker-compose.prod.yml logs -f

# 특정 서비스 로그
docker-compose -f docker-compose.prod.yml logs -f authway-api

# 최근 100줄만 보기
docker-compose -f docker-compose.prod.yml logs --tail=100
```

### 리소스 모니터링

```bash
# 컨테이너 리소스 사용량
docker stats

# 디스크 사용량
df -h

# 메모리 사용량
free -h
```

### 애플리케이션 모니터링 (Optional)

Prometheus + Grafana 스택을 사용한 고급 모니터링:

```bash
# docker-compose.monitoring.yml 생성 필요
docker-compose -f docker-compose.monitoring.yml up -d
```

---

## 백업 및 복구

### 데이터베이스 백업

```bash
# 수동 백업
docker exec authway-postgres pg_dump -U authway authway > backup_$(date +%Y%m%d_%H%M%S).sql

# 자동 백업 스크립트 (cron)
cat > /usr/local/bin/authway-backup.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/var/backups/authway"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# PostgreSQL 백업
docker exec authway-postgres pg_dump -U authway authway > $BACKUP_DIR/postgres_$DATE.sql

# Redis 백업
docker exec authway-redis redis-cli BGSAVE
docker cp authway-redis:/data/dump.rdb $BACKUP_DIR/redis_$DATE.rdb

# 7일 이상 된 백업 삭제
find $BACKUP_DIR -name "*.sql" -mtime +7 -delete
find $BACKUP_DIR -name "*.rdb" -mtime +7 -delete
EOF

chmod +x /usr/local/bin/authway-backup.sh

# 매일 새벽 2시에 백업
echo "0 2 * * * /usr/local/bin/authway-backup.sh" | sudo crontab -
```

### 데이터 복구

```bash
# PostgreSQL 복구
cat backup_20240101_120000.sql | docker exec -i authway-postgres psql -U authway authway

# Redis 복구
docker cp backup_20240101_120000.rdb authway-redis:/data/dump.rdb
docker-compose -f docker-compose.prod.yml restart redis
```

---

## 업데이트 및 유지보수

### 무중단 업데이트

```bash
# 1. 새 버전 이미지 빌드
docker-compose -f docker-compose.prod.yml build

# 2. 서비스별 순차 업데이트
docker-compose -f docker-compose.prod.yml up -d --no-deps authway-api
docker-compose -f docker-compose.prod.yml up -d --no-deps login-ui
docker-compose -f docker-compose.prod.yml up -d --no-deps admin-dashboard
```

### SSL 인증서 갱신

```bash
# Let's Encrypt 인증서 자동 갱신
sudo certbot renew

# Nginx 재시작
docker-compose -f docker-compose.prod.yml restart nginx
```

---

## 문제 해결

### 서비스가 시작되지 않음

```bash
# 로그 확인
docker-compose -f docker-compose.prod.yml logs

# 컨테이너 상태 확인
docker-compose -f docker-compose.prod.yml ps

# 특정 서비스 재시작
docker-compose -f docker-compose.prod.yml restart authway-api
```

### 데이터베이스 연결 실패

```bash
# PostgreSQL 컨테이너 접속
docker exec -it authway-postgres psql -U authway -d authway

# 연결 테스트
docker exec authway-postgres pg_isready -U authway -d authway
```

### 메모리 부족

```bash
# 사용하지 않는 컨테이너 정리
docker system prune -a

# 볼륨 정리
docker volume prune
```

### 완전 재시작

```bash
# 모든 서비스 중지 및 데이터 유지
docker-compose -f docker-compose.prod.yml down

# 재시작
docker-compose -f docker-compose.prod.yml up -d
```

---

## 보안 체크리스트

- [ ] 모든 비밀번호와 시크릿 키를 강력하게 설정
- [ ] SSL/TLS 인증서 설정 완료
- [ ] 방화벽 규칙 적용
- [ ] `.env.production` 파일 권한 설정 (600)
- [ ] CORS 설정 확인
- [ ] 데이터베이스 백업 자동화
- [ ] 로그 모니터링 설정
- [ ] 정기적인 보안 업데이트

---

## 성능 최적화

### 데이터베이스 최적화

```sql
-- PostgreSQL 성능 튜닝
ALTER SYSTEM SET shared_buffers = '256MB';
ALTER SYSTEM SET effective_cache_size = '1GB';
ALTER SYSTEM SET maintenance_work_mem = '64MB';
```

### Redis 최적화

```bash
# redis.conf 설정
maxmemory 256mb
maxmemory-policy allkeys-lru
```

### Nginx 최적화

```nginx
# nginx.conf에 추가
worker_processes auto;
worker_connections 2048;
keepalive_timeout 65;
gzip on;
gzip_types text/plain text/css application/json application/javascript;
```

---

## 지원 및 문의

- 🐛 [GitHub Issues](https://github.com/authway/authway/issues)
- 💬 [Discord](https://discord.gg/authway)
- 📧 [Email](mailto:hello@authway.dev)
- 📚 [문서](https://docs.authway.dev)

---

**Authway 프로덕션 배포 완료! 🎉**
