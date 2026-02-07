.PHONY: help build run test test-unit test-integration coverage docker-build clean

help:
	@echo "Serving Service - Makefile"
	@echo ""
	@echo "  build              - Build runner binary"
	@echo "  run                - Run the service locally"
	@echo "  test               - Run all tests"
	@echo "  test-unit          - Run unit tests"
	@echo "  test-integration   - Run integration tests"
	@echo "  coverage           - Generate coverage report"
	@echo "  docker-build       - Build Docker images"
	@echo "  clean              - Clean build artifacts"

build:
	@echo "Building serving runner..."
	go build -o serving-runner ./cmd/runner

run:
	@echo "Running serving service..."
	@echo "Note: Requires storage service and nginx"
	PAGEWRIGHT_SERVING_PORT=8083 \
	PAGEWRIGHT_STORAGE_URL=http://localhost:8080 \
	PAGEWRIGHT_WWW_ROOT=./test/var/www \
	PAGEWRIGHT_NGINX_SITES_ENABLED=./test/etc/nginx/sites-enabled \
	go run ./cmd/runner/main.go

test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires storage service and nginx"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

coverage:
	@echo "Generating coverage report..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

docker-build:
	@echo "Building Docker image..."
	docker build -t pagewright-serving:latest .

clean:
	@echo "Cleaning..."
	rm -f serving-runner coverage.txt coverage.out coverage.html
