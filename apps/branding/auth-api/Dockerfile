# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY apps/branding/auth-api/go.mod apps/branding/auth-api/go.sum ./
RUN go mod download

# Copy source code
COPY apps/branding/auth-api/ ./

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

# Copy static files
COPY apps/branding/auth-api/static/ ./static/

EXPOSE 8080

CMD ["./auth-backend"]
