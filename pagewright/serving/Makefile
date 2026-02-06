.PHONY: help test test-unit test-integration build docker-build docker-up docker-down clean

help:
	@echo "PageWright Serving - Available targets:"
	@echo "  test             - Run all tests"
	@echo "  test-unit        - Run unit tests only"
	@echo "  test-integration - Run integration tests"
	@echo "  build            - Build runner binary"
	@echo "  docker-build     - Build Docker images"
	@echo "  docker-up        - Start services"
	@echo "  docker-down      - Stop services"
	@echo "  clean            - Clean build artifacts"

test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires docker-compose services running"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

build:
	@echo "Building serving runner..."
	go build -o serving-runner ./cmd/runner

docker-build:
	@echo "Building Docker images..."
	docker-compose build

docker-up:
	@echo "Starting services..."
	docker-compose up -d

docker-down:
	@echo "Stopping services..."
	docker-compose down

clean:
	@echo "Cleaning..."
	rm -f serving-runner coverage.txt
	docker-compose down -v
