.PHONY: help test test-unit test-integration build docker-build clean

help:
	@echo "PageWright Worker - Available targets:"
	@echo "  test             - Run all tests"
	@echo "  test-unit        - Run unit tests only"
	@echo "  test-integration - Run integration tests"
	@echo "  build            - Build runner binary"
	@echo "  docker-build     - Build Docker image"
	@echo "  clean            - Clean build artifacts"

test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-integration:
	@echo "Running integration tests..."
	@echo "Note: Integration tests require storage and manager services running"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

build:
	@echo "Building runner..."
	go build -o runner ./cmd/runner

docker-build:
	@echo "Building Docker image..."
	docker build -t pagewright-worker:latest .

clean:
	@echo "Cleaning..."
	rm -f runner coverage.txt
	rm -rf /tmp/work-*
