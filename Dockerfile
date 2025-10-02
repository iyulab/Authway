# Authway Backend - Multi-stage Production Dockerfile

# Build stage
FROM golang:1.21-alpine AS builder

# Install dependencies for building with CGO (required for SQLite tests)
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY src/ ./src/

# Build the application with optimizations
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s -X main.version=$(date +%Y%m%d-%H%M%S)" \
    -o authway ./src/server/cmd/main.go

# Production stage
FROM alpine:latest AS production

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl

# Create app user
RUN adduser -D -s /bin/sh authway

# Set working directory
WORKDIR /home/authway

# Copy binary from builder stage
COPY --from=builder /app/authway .

# Copy config files if needed
COPY --chown=authway:authway configs/ ./configs/

# Create logs directory
RUN mkdir -p logs && chown authway:authway logs

# Switch to app user
USER authway

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the application
CMD ["./authway"]