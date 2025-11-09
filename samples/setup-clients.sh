#!/bin/bash
# Authway Sample Services - Client Registration Script
# This script registers the sample service OAuth clients with Authway

set -e

echo "🔐 Authway Sample Services - Client Registration"
echo ""

# Configuration
AUTHWAY_API="http://localhost:8080"
SAMPLE_PORTS=(9001 9002 9003)

# Function to kill process on specific port
kill_port_process() {
    local port=$1
    local service_name=$2

    # For Windows (Git Bash/MSYS)
    if command -v netstat.exe &> /dev/null; then
        local pid=$(netstat.exe -ano | grep ":$port " | grep LISTENING | awk '{print $5}' | head -1)
        if [ -n "$pid" ]; then
            echo "  Killing process on port $port (PID: $pid) for $service_name"
            taskkill.exe //PID $pid //F > /dev/null 2>&1 || true
            return 0
        fi
    # For Linux/macOS
    elif command -v lsof &> /dev/null; then
        local pid=$(lsof -ti:$port 2>/dev/null)
        if [ -n "$pid" ]; then
            echo "  Killing process on port $port (PID: $pid) for $service_name"
            kill -9 $pid 2>/dev/null || true
            return 0
        fi
    fi
    return 1
}

# Function to ensure port is free
ensure_port_free() {
    local port=$1
    local service_name=$2
    local max_attempts=3
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        # Check if port is free
        if command -v netstat.exe &> /dev/null; then
            if ! netstat.exe -ano | grep ":$port " | grep -q LISTENING; then
                echo "✓ Port $port is free for $service_name"
                return 0
            fi
        elif command -v lsof &> /dev/null; then
            if ! lsof -ti:$port &>/dev/null; then
                echo "✓ Port $port is free for $service_name"
                return 0
            fi
        fi

        echo "⚠️  Port $port is in use, attempting to free it... (Attempt $((attempt + 1))/$max_attempts)"
        kill_port_process $port "$service_name"
        sleep 1
        attempt=$((attempt + 1))
    done

    echo "❌ Failed to free port $port for $service_name"
    return 1
}

# Clean up sample service ports
echo "🧹 Cleaning up sample service ports..."
for port in "${SAMPLE_PORTS[@]}"; do
    kill_port_process $port "Sample Service" || true
done

if [ ${#SAMPLE_PORTS[@]} -gt 0 ]; then
    echo "⏳ Waiting for ports to be released..."
    sleep 2
fi
echo ""

# Check if Authway is running
echo "📡 Checking Authway API..."
if ! curl -s "$AUTHWAY_API/health" > /dev/null; then
    echo "❌ Authway API is not accessible at $AUTHWAY_API"
    echo "   Please make sure Authway is running first."
    exit 1
fi
echo "✓ Authway is running"
echo ""

# Get tenant ID dynamically
echo "🔍 Fetching tenant ID..."
TENANT_RESPONSE=$(curl -s "$AUTHWAY_API/api/v1/tenants")
TENANT_ID=$(echo "$TENANT_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$TENANT_ID" ]; then
    echo "❌ No tenant found. Please create a tenant first."
    exit 1
fi
echo "✓ Using tenant ID: $TENANT_ID"
echo ""

# Function to register a client
register_client() {
    local name=$1
    local client_id=$2
    local client_secret=$3
    local redirect_uri=$4
    local icon=$5

    echo "$icon Registering $name..."

    # Register in Authway database
    response=$(curl -s -w "\n%{http_code}" -X POST "$AUTHWAY_API/api/v1/clients" \
        -H "Content-Type: application/json" \
        -d "{
            \"tenant_id\": \"$TENANT_ID\",
            \"client_id\": \"$client_id\",
            \"client_secret\": \"$client_secret\",
            \"name\": \"$name\",
            \"description\": \"Sample service for testing Authway OAuth 2.0 integration\",
            \"redirect_uris\": [\"$redirect_uri\"],
            \"grant_types\": [\"authorization_code\", \"refresh_token\"],
            \"scopes\": [\"openid\", \"profile\", \"email\"],
            \"public\": false
        }")

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" -eq 200 ] || [ "$http_code" -eq 201 ]; then
        echo "  ✓ Client created in Authway"
    elif [ "$http_code" -eq 409 ]; then
        echo "  ℹ️  Client already exists in Authway, skipping..."
    elif [ "$http_code" -eq 500 ] && echo "$body" | grep -q "duplicate key"; then
        echo "  ℹ️  Client already exists in Authway, skipping..."
    else
        echo "  ❌ Failed to register client in Authway (HTTP $http_code)"
        echo "  Response: $body"
    fi

    # Register in Hydra
    hydra_response=$(curl -s -w "\n%{http_code}" -X POST "http://localhost:4445/admin/clients" \
        -H "Content-Type: application/json" \
        -d "{
            \"client_id\": \"$client_id\",
            \"client_secret\": \"$client_secret\",
            \"client_name\": \"$name\",
            \"grant_types\": [\"authorization_code\", \"refresh_token\"],
            \"response_types\": [\"code\"],
            \"redirect_uris\": [\"$redirect_uri\"],
            \"scope\": \"openid profile email\"
        }")

    hydra_http_code=$(echo "$hydra_response" | tail -n1)
    hydra_body=$(echo "$hydra_response" | sed '$d')

    if [ "$hydra_http_code" -eq 200 ] || [ "$hydra_http_code" -eq 201 ]; then
        echo "  ✓ Client registered in Hydra"
    elif [ "$hydra_http_code" -eq 409 ]; then
        echo "  ℹ️  Client already exists in Hydra, skipping..."
    elif echo "$hydra_body" | grep -q "already exists"; then
        echo "  ℹ️  Client already exists in Hydra, skipping..."
    else
        echo "  ⚠️  Warning: Failed to register in Hydra (HTTP $hydra_http_code)"
        echo "  You may need to register manually in Hydra"
    fi

    echo "  ✓ Redirect URI: $redirect_uri"
    echo ""
}

# Register services
register_client "AppleService" "apple-service-client" "apple-service-secret" "http://localhost:9001/callback" "🍎"
register_client "BananaService" "banana-service-client" "banana-service-secret" "http://localhost:9002/callback" "🍌"
register_client "ChocolateService" "chocolate-service-client" "chocolate-service-secret" "http://localhost:9003/callback" "🍫"

echo "✅ Client registration complete!"
echo ""
echo "📝 To start sample services, run in separate terminals:"
echo "  cd samples/AppleService && go run main.go"
echo "  cd samples/BananaService && go run main.go"
echo "  cd samples/ChocolateService && go run main.go"
echo ""
echo "📌 Service URLs:"
echo "   🍎 Apple Service:     http://localhost:9001"
echo "   🍌 Banana Service:    http://localhost:9002"
echo "   🍫 Chocolate Service: http://localhost:9003"
echo ""
echo "💡 Test SSO by logging into one service, then accessing another"
echo ""
