.PHONY: help build test test-unit test-integration run coverage clean docker-build fmt vet

# Default target
help:
	@echo "Storage Service - Makefile"
	@echo ""
	@echo "  build              - Build the storage service binary"
	@echo "  run                - Run the service locally"
	@echo "  test               - Run all tests"
	@echo "  test-unit          - Run unit tests"
	@echo "  test-integration   - Run integration tests"
	@echo "  coverage           - Generate coverage report"
	@echo "  docker-build       - Build Docker image"
	@echo "  clean              - Clean build artifacts"
	@echo "  fmt                - Format Go code"
	@echo "  vet                - Run go vet"

# Build the binary
build:
	@echo "Building storage service..."
	go build -o storage-server ./cmd/server

# Run all tests
test: test-unit test-integration

# Run unit tests
test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires NFS server running (make docker-up-infra from root)"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

# Run the service locally
run:
	@echo "Running storage service..."
	@echo "Note: Requires NFS server running (make docker-up-infra from root)"
	PAGEWRIGHT_PORT=8080 PAGEWRIGHT_STORAGE_BACKEND=nfs PAGEWRIGHT_NFS_BASE_PATH=/tmp/nfs-test go run ./cmd/server/main.go

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t pagewright-storage:latest .

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f storage-server
	rm -f coverage.txt coverage.out coverage.html
	rm -rf /tmp/nfs-test
	go clean

# Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	@echo "Running end-to-end tests..."
	@sleep 5
	$(MAKE) test-integration
	@echo "E2E tests complete"
	$(MAKE) docker-down
