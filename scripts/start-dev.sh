#!/bin/bash

# Authway Development Environment Startup Script
# Usage: ./start-dev.sh [options]

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  Authway Development Environment${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        echo "Visit: https://docs.docker.com/get-docker/"
        exit 1
    fi

    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        print_error "Docker Compose is not installed."
        exit 1
    fi

    if ! docker info &> /dev/null; then
        print_error "Docker daemon is not running. Please start Docker."
        exit 1
    fi

    print_success "Docker is ready"
}

show_menu() {
    echo ""
    echo "Select startup mode:"
    echo "1) Essential services only (Backend + Frontend + Database)"
    echo "2) Full stack (All services including Admin Dashboard)"
    echo "3) Stop all services"
    echo "4) View logs"
    echo "5) Reset all data (WARNING: Deletes all data)"
    echo "6) Exit"
    echo ""
    read -p "Enter your choice [1-6]: " choice
}

start_essential() {
    print_header
    echo "Starting essential services..."
    echo ""

    docker-compose -f docker-compose.dev.yml up -d postgres redis mailhog authway-api login-ui

    echo ""
    print_success "Essential services started!"
    show_urls
}

start_full() {
    print_header
    echo "Starting all services..."
    echo ""

    docker-compose -f docker-compose.dev.yml --profile full up -d

    echo ""
    print_success "All services started!"
    show_urls
}

stop_services() {
    print_header
    echo "Stopping all services..."
    echo ""

    docker-compose -f docker-compose.dev.yml down

    echo ""
    print_success "All services stopped!"
}

view_logs() {
    print_header
    echo "Available services:"
    echo "1) Backend API (authway-api)"
    echo "2) Login UI (login-ui)"
    echo "3) Admin Dashboard (admin-dashboard)"
    echo "4) PostgreSQL (postgres)"
    echo "5) All services"
    echo ""
    read -p "Select service [1-5]: " log_choice

    case $log_choice in
        1)
            docker-compose -f docker-compose.dev.yml logs -f authway-api
            ;;
        2)
            docker-compose -f docker-compose.dev.yml logs -f login-ui
            ;;
        3)
            docker-compose -f docker-compose.dev.yml logs -f admin-dashboard
            ;;
        4)
            docker-compose -f docker-compose.dev.yml logs -f postgres
            ;;
        5)
            docker-compose -f docker-compose.dev.yml logs -f
            ;;
        *)
            print_error "Invalid choice"
            ;;
    esac
}

reset_data() {
    print_header
    print_warning "WARNING: This will delete ALL data including database and cache!"
    read -p "Are you sure? (yes/no): " confirm

    if [ "$confirm" = "yes" ]; then
        echo ""
        echo "Stopping services and removing volumes..."
        docker-compose -f docker-compose.dev.yml down -v

        echo ""
        print_success "All data has been reset!"
        echo ""
        read -p "Start services again? (yes/no): " start_again

        if [ "$start_again" = "yes" ]; then
            start_essential
        fi
    else
        print_warning "Reset cancelled"
    fi
}

show_urls() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  Service URLs${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    echo -e "🎨 Login UI:          ${GREEN}http://localhost:3001${NC}"
    echo -e "🖥️  Admin Dashboard:   ${GREEN}http://localhost:3000${NC}"
    echo -e "🚀 Backend API:       ${GREEN}http://localhost:8080${NC}"
    echo -e "📧 MailHog:           ${GREEN}http://localhost:8025${NC}"
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo ""
    echo "Wait 10-30 seconds for all services to be ready..."
    echo ""
    echo "Test Backend API:"
    echo "  curl http://localhost:8080/health"
    echo ""
    echo "View logs:"
    echo "  docker-compose -f docker-compose.dev.yml logs -f"
    echo ""
}

wait_for_services() {
    echo "Waiting for services to be ready..."
    sleep 5

    # Wait for backend API
    max_attempts=30
    attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if curl -s http://localhost:8080/health > /dev/null 2>&1; then
            print_success "Backend API is ready!"
            return 0
        fi

        attempt=$((attempt + 1))
        echo -n "."
        sleep 1
    done

    print_warning "Backend API is taking longer than expected. Check logs:"
    echo "  docker-compose -f docker-compose.dev.yml logs authway-api"
}

# Main script
print_header
check_docker

# Parse arguments
if [ "$1" = "start" ]; then
    start_essential
    wait_for_services
    exit 0
elif [ "$1" = "full" ]; then
    start_full
    wait_for_services
    exit 0
elif [ "$1" = "stop" ]; then
    stop_services
    exit 0
elif [ "$1" = "reset" ]; then
    reset_data
    exit 0
fi

# Interactive menu
while true; do
    show_menu

    case $choice in
        1)
            start_essential
            wait_for_services
            ;;
        2)
            start_full
            wait_for_services
            ;;
        3)
            stop_services
            ;;
        4)
            view_logs
            ;;
        5)
            reset_data
            ;;
        6)
            echo "Goodbye!"
            exit 0
            ;;
        *)
            print_error "Invalid choice. Please select 1-6."
            ;;
    esac

    echo ""
    read -p "Press Enter to continue..."
done
