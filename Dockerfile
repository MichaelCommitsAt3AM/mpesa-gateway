# Stage 1: Builder
FROM golang:1.22-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/api \
    ./cmd/api/main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/worker \
    ./cmd/worker/main.go

# Stage 2: Runtime
FROM alpine:latest

# Install ca-certificates for SSL verification
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy binaries from builder
COPY --from=builder /app/bin/api .
COPY --from=builder /app/bin/worker .

# Expose port
EXPOSE 8080

# Default command (can be overridden)
CMD ["./api"]
