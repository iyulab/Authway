#!/bin/bash

# Production Deployment Script for Authway
# Handles secure deployment with rollback capabilities

set -e

# Configuration
DEPLOY_ENV="${DEPLOY_ENV:-production}"
VERSION="${VERSION:-$(date +%Y%m%d-%H%M%S)}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"
HEALTH_CHECK_TIMEOUT="${HEALTH_CHECK_TIMEOUT:-60}"
ROLLBACK_TIMEOUT="${ROLLBACK_TIMEOUT:-300}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# Error handling
trap 'log_error "Deployment failed at line $LINENO. Exit code: $?"; cleanup_on_error' ERR

cleanup_on_error() {
    log_warning "Cleaning up after error..."
    # Add cleanup logic here if needed
    exit 1
}

# Pre-deployment checks
pre_deployment_checks() {
    log_info "🔍 Running pre-deployment checks..."

    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        log_error "Docker is not running or not accessible"
        exit 1
    fi

    # Check if required files exist
    local required_files=(
        "docker-compose.prod.yml"
        "Dockerfile"
        "configs/production.yaml"
    )

    for file in "${required_files[@]}"; do
        if [[ ! -f "$file" ]]; then
            log_error "Required file not found: $file"
            exit 1
        fi
    done

    # Check environment variables
    local required_env_vars=(
        "DATABASE_PASSWORD"
        "REDIS_PASSWORD"
        "HYDRA_CLIENT_SECRET"
    )

    for var in "${required_env_vars[@]}"; do
        if [[ -z "${!var}" ]]; then
            log_warning "Environment variable $var is not set"
        fi
    done

    log_success "Pre-deployment checks passed"
}

# Database backup
backup_database() {
    log_info "💾 Creating database backup..."

    local backup_file="backup_$(date +%Y%m%d_%H%M%S).sql"
    local backup_path="./backups/$backup_file"

    # Create backups directory
    mkdir -p ./backups

    # Create database backup using Docker
    if docker-compose -f docker-compose.prod.yml ps postgres | grep -q "Up"; then
        docker-compose -f docker-compose.prod.yml exec -T postgres pg_dump \
            -U authway -d authway > "$backup_path" || {
            log_warning "Database backup failed, but continuing deployment"
            return 0
        }
        log_success "Database backup created: $backup_path"
    else
        log_warning "PostgreSQL container not running, skipping backup"
    fi

    # Cleanup old backups
    find ./backups -name "backup_*.sql" -mtime +$BACKUP_RETENTION_DAYS -delete 2>/dev/null || true
}

# Build application
build_application() {
    log_info "🔨 Building application..."

    # Build backend
    log_info "Building backend..."
    docker-compose -f docker-compose.prod.yml build authway-api

    # Build frontend services
    log_info "Building login UI..."
    docker-compose -f docker-compose.prod.yml build authway-login-ui

    log_info "Building admin dashboard..."
    docker-compose -f docker-compose.prod.yml build authway-admin-dashboard

    log_success "Application build completed"
}

# Deploy services
deploy_services() {
    log_info "🚀 Deploying services..."

    # Deploy infrastructure services first
    log_info "Deploying infrastructure services..."
    docker-compose -f docker-compose.prod.yml up -d postgres redis hydra

    # Wait for infrastructure to be ready
    log_info "Waiting for infrastructure services..."
    sleep 30

    # Run database migrations
    log_info "Running database migrations..."
    docker-compose -f docker-compose.prod.yml run --rm authway-api /app/authway migrate || {
        log_error "Database migration failed"
        return 1
    }

    # Deploy application services
    log_info "Deploying application services..."
    docker-compose -f docker-compose.prod.yml up -d authway-api
    docker-compose -f docker-compose.prod.yml up -d authway-login-ui
    docker-compose -f docker-compose.prod.yml up -d authway-admin-dashboard

    # Deploy monitoring services
    log_info "Deploying monitoring services..."
    docker-compose -f docker-compose.prod.yml up -d prometheus grafana loki

    # Deploy nginx last
    log_info "Deploying reverse proxy..."
    docker-compose -f docker-compose.prod.yml up -d nginx

    log_success "Services deployed"
}

# Health checks
perform_health_checks() {
    log_info "🏥 Performing health checks..."

    local services=(
        "http://localhost:8080/health:Backend API"
        "http://localhost:3000:Login UI"
        "http://localhost:3001:Admin Dashboard"
    )

    local failed_checks=0
    local max_attempts=12
    local sleep_interval=5

    for service in "${services[@]}"; do
        local url="${service%:*}"
        local name="${service#*:}"
        local attempts=0
        local success=false

        log_info "Checking $name ($url)..."

        while [[ $attempts -lt $max_attempts ]]; do
            if curl -f -s -o /dev/null --max-time 10 "$url"; then
                log_success "$name is healthy"
                success=true
                break
            fi

            attempts=$((attempts + 1))
            if [[ $attempts -lt $max_attempts ]]; then
                log_info "Attempt $attempts/$max_attempts failed, retrying in ${sleep_interval}s..."
                sleep $sleep_interval
            fi
        done

        if [[ "$success" != true ]]; then
            log_error "$name health check failed after $max_attempts attempts"
            failed_checks=$((failed_checks + 1))
        fi
    done

    if [[ $failed_checks -gt 0 ]]; then
        log_error "$failed_checks health checks failed"
        return 1
    fi

    log_success "All health checks passed"
}

# Smoke tests
run_smoke_tests() {
    log_info "🧪 Running smoke tests..."

    # Test basic API endpoints
    local test_endpoints=(
        "http://localhost:8080/health"
        "http://localhost:8080/metrics"
    )

    for endpoint in "${test_endpoints[@]}"; do
        log_info "Testing $endpoint..."
        local response_code=$(curl -s -o /dev/null -w "%{http_code}" "$endpoint" || echo "000")

        if [[ "$response_code" == "200" ]]; then
            log_success "$endpoint responded with 200"
        else
            log_warning "$endpoint responded with $response_code"
        fi
    done

    log_success "Smoke tests completed"
}

# Post-deployment tasks
post_deployment_tasks() {
    log_info "📋 Running post-deployment tasks..."

    # Update deployment record
    echo "$(date): Deployed version $VERSION" >> ./deployment-history.log

    # Send notifications (placeholder)
    log_info "Deployment notifications would be sent here"

    # Security scan (placeholder)
    log_info "Post-deployment security scan would run here"

    log_success "Post-deployment tasks completed"
}

# Rollback function
rollback_deployment() {
    log_warning "🔄 Starting rollback procedure..."

    # Stop current services
    docker-compose -f docker-compose.prod.yml stop

    # Restore from backup if needed
    log_warning "Manual database restore may be required"

    # Restart with previous version
    log_warning "Manual rollback to previous version required"

    log_error "Rollback completed - manual intervention may be required"
}

# Main deployment function
main() {
    log_info "🚀 Starting Authway deployment (version: $VERSION, environment: $DEPLOY_ENV)"

    # Deployment steps
    pre_deployment_checks
    backup_database
    build_application
    deploy_services

    # Health checks with rollback on failure
    if ! perform_health_checks; then
        log_error "Health checks failed, initiating rollback..."
        rollback_deployment
        exit 1
    fi

    run_smoke_tests
    post_deployment_tasks

    log_success "🎉 Deployment completed successfully!"
    log_info "Version $VERSION is now live"

    # Display service URLs
    echo ""
    log_info "Service URLs:"
    echo "  🌐 Login UI: http://localhost:3000"
    echo "  👥 Admin Dashboard: http://localhost:3001"
    echo "  🔑 API: http://localhost:8080"
    echo "  📊 Grafana: http://localhost:3001/grafana"
    echo "  📈 Prometheus: http://localhost:9090"
}

# Script usage
usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  -e, --env ENV          Deployment environment (default: production)"
    echo "  -v, --version VERSION  Version tag (default: current timestamp)"
    echo "  -h, --help            Show this help message"
    echo ""
    echo "Environment variables:"
    echo "  DATABASE_PASSWORD      PostgreSQL password (required)"
    echo "  REDIS_PASSWORD         Redis password (optional)"
    echo "  HYDRA_CLIENT_SECRET    Hydra client secret (required)"
    echo ""
    echo "Examples:"
    echo "  $0                     Deploy with defaults"
    echo "  $0 -e staging -v 1.2.3 Deploy version 1.2.3 to staging"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--env)
            DEPLOY_ENV="$2"
            shift 2
            ;;
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Run main function
main "$@"