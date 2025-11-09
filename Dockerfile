# Authway Central API - Multi-stage Production Dockerfile
# This Dockerfile builds the Central API which is part of the main authway module

# Build stage
FROM golang:1.23-alpine AS builder

# Install dependencies for building with CGO
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY apps/central/ ./apps/central/

# Build the application with optimizations
# Central API is part of the authway module, so we build from the module root
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s -X main.version=$(date +%Y%m%d-%H%M%S)" \
    -o authway-api authway/apps/central/api/cmd

# Production stage
FROM alpine:latest AS production

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl

# Create app user
RUN adduser -D -s /bin/sh authway

# Set working directory
WORKDIR /home/authway

# Copy binary from builder stage
COPY --from=builder /app/authway-api ./app

# Create logs directory
RUN mkdir -p logs && chown authway:authway logs

# Switch to app user
USER authway

# Expose port (can be overridden at runtime)
EXPOSE 8080

# Health check (can be overridden at runtime)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:${PORT:-8080}/health || exit 1

# Run the application
CMD ["./app"]