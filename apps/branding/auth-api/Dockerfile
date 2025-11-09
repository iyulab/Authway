# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY apps/auth-backend/go.mod apps/auth-backend/go.sum ./
RUN go mod download

# Copy source code
COPY apps/auth-backend/ ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o auth-backend ./cmd/main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/auth-backend .

# Create static directory
RUN mkdir -p /root/static

# Copy static files if they exist (will be empty initially)
COPY apps/auth-backend/static/ ./static/ 2>/dev/null || :

EXPOSE 8080

CMD ["./auth-backend"]
