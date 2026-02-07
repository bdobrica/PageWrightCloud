.PHONY: help build build-worker test test-unit test-integration run coverage clean docker-build fmt vet

# Default target
help:
	@echo "Manager Service - Makefile"
	@echo ""
	@echo "  build              - Build the manager service binary"
	@echo "  build-worker       - Build the worker binary"
	@echo "  run                - Run the service locally"
	@echo "  test               - Run all tests"
	@echo "  test-unit          - Run unit tests"
	@echo "  test-integration   - Run integration tests"
	@echo "  coverage           - Generate coverage report"
	@echo "  docker-build       - Build Docker images"
	@echo "  clean              - Clean build artifacts"
	@echo "  fmt                - Format Go code"
	@echo "  vet                - Run go vet"

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
	@echo "Note: Requires Redis running (make docker-up-infra from root)"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run the service locally
run:
	@echo "Running manager service..."
	@echo "Note: Requires Redis running (make docker-up-infra from root)"
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

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f manager-server worker coverage.txt coverage.out coverage.html
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
