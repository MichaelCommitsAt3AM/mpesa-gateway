.PHONY: help setup up down restart ps logs build build-api build-worker run test docker-build clean deps fmt shell-api shell-db shell-redis

# Default target - show help
help:
	@echo "M-Pesa Payment Gateway - Available Commands"
	@echo ""
	@echo "Quick Start (Docker Compose):"
	@echo "  make setup      - Copy .env.example to .env (first time only)"
	@echo "  make up         - Start all services (PostgreSQL, Redis, API, Worker)"
	@echo "  make down       - Stop all services"
	@echo "  make restart    - Restart all services"
	@echo "  make ps         - Show service status"
	@echo "  make logs       - Follow API and Worker logs"
	@echo ""
	@echo "Local Development:"
	@echo "  make build      - Build both API and Worker binaries"
	@echo "  make run        - Run API server locally (requires PostgreSQL and Redis)"
	@echo "  make test       - Run tests"
	@echo ""
	@echo "Utilities:"
	@echo "  make shell-api  - Shell access to API container"
	@echo "  make shell-db   - Shell access to PostgreSQL container"
	@echo "  make shell-redis - Shell access to Redis container"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make deps       - Download and tidy Go dependencies"
	@echo "  make fmt        - Format Go code"

# Setup - copy .env.example to .env if it doesn't exist
setup:
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "✓ Created .env file from .env.example"; \
		echo "⚠ IMPORTANT: Edit .env and add your Safaricom credentials before running 'make up'"; \
	else \
		echo "✓ .env file already exists"; \
	fi

# Start all services
up:
	@if [ ! -f .env ]; then \
		echo "⚠ WARNING: .env file not found. Run 'make setup' first."; \
		exit 1; \
	fi
	docker compose up -d
	@echo "✓ All services started. API available at http://localhost:8081"
	@echo "  Run 'make logs' to view logs"
	@echo "  Run 'make ps' to check service status"

# Stop all services
down:
	docker compose down

# Restart all services
restart:
	docker compose restart

# Show service status
ps:
	docker compose ps

# View logs
logs:
	docker compose logs -f api worker

# Build both binaries
build: build-api build-worker

# Build API server
build-api:
	go build -o bin/api cmd/api/main.go

# Build worker
build-worker:
	go build -o bin/worker cmd/worker/main.go

# Run API server (requires PostgreSQL and Redis)
run:
	go run cmd/api/main.go

# Run tests
test:
	go test ./...

# Build Docker image
docker-build:
	docker build -t mpesa-gateway:latest .

# Shell into API container
shell-api:
	docker exec -it mpesa_api sh

# Shell into PostgreSQL container
shell-db:
	docker exec -it mpesa_postgres psql -U mpesa -d mpesa_gateway

# Shell into Redis container
shell-redis:
	docker exec -it mpesa_redis redis-cli

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...
