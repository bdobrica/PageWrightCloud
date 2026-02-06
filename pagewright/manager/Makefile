.PHONY: help build build-worker test test-unit test-integration run clean docker-build docker-up docker-down docker-logs fmt vet

# Default target
help:
	@echo "PageWright Manager Service - Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  make build              - Build the manager service binary"
	@echo "  make build-worker       - Build the worker binary"
	@echo "  make test               - Run all tests (unit + integration)"
	@echo "  make test-unit          - Run unit tests only"
	@echo "  make test-integration   - Run integration tests (requires docker-up)"
	@echo "  make run                - Run the service locally"
	@echo "  make docker-build       - Build Docker images"
	@echo "  make docker-up          - Start services with docker-compose"
	@echo "  make docker-down        - Stop services with docker-compose"
	@echo "  make docker-logs        - View docker-compose logs"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make fmt                - Format Go code"
	@echo "  make vet                - Run go vet"

# Build the manager binary
build:
	@echo "Building manager service..."
	go build -o manager-server ./cmd/server

# Build the worker binary
build-worker:
	@echo "Building worker..."
	go build -o worker ./cmd/worker

# Run all tests
test: test-unit test-integration

# Run unit tests
test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires docker-compose services to be running (make docker-up)"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

# Run the service locally
run:
	@echo "Running manager service..."
	PAGEWRIGHT_PORT=8081 \
	PAGEWRIGHT_QUEUE_BACKEND=redis \
	PAGEWRIGHT_WORKER_SPAWNER=docker \
	PAGEWRIGHT_REDIS_ADDR=localhost:6379 \
	PAGEWRIGHT_WORKER_IMAGE=pagewright-worker:latest \
	go run ./cmd/server/main.go

# Build Docker images
docker-build:
	@echo "Building Docker images..."
	docker build -t pagewright-manager:latest -f Dockerfile .
	docker build -t pagewright-worker:latest -f Dockerfile.worker .

# Start services with docker-compose
docker-up:
	@echo "Starting services..."
	docker-compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@echo "Services started. Manager API: http://localhost:8081"

# Stop services
docker-down:
	@echo "Stopping services..."
	docker-compose down

# View logs
docker-logs:
	docker-compose logs -f manager-service

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f manager-server
	rm -f worker
	rm -f coverage.txt
	rm -f coverage.html
	go clean

# Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Install dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Generate test coverage report
coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run end-to-end test suite
e2e: docker-down docker-build docker-up
	@echo "Running end-to-end tests..."
	@sleep 5
	$(MAKE) test-integration
	@echo "E2E tests complete"
	$(MAKE) docker-down
