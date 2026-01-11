.PHONY: build build-api build-worker run test docker-build docker-up docker-down clean

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
	go test  ./...

# Build Docker image
docker-build:
	docker build -t mpesa-gateway:latest .

# Start all services with docker-compose
docker-up:
	docker-compose up -d

# Stop all services
docker-down:
	docker-compose down

# View logs
logs:
	docker-compose logs -f api worker

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
