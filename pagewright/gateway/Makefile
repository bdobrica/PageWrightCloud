.PHONY: help build build-cli run test test-unit test-integration coverage docker-build clean

help:
	@echo "Gateway Service - Makefile"
	@echo ""
	@echo "  build              - Build the gateway binary"
	@echo "  build-cli          - Build the users CLI tool"
	@echo "  run                - Run the service locally"
	@echo "  test               - Run all tests"
	@echo "  test-unit          - Run unit tests"
	@echo "  test-integration   - Run integration tests"
	@echo "  coverage           - Generate coverage report"
	@echo "  docker-build       - Build Docker image"
	@echo "  clean              - Clean build artifacts"

build:
	@echo "Building gateway..."
	go build -o bin/gateway cmd/gateway/main.go

build-cli:
	@echo "Building users CLI..."
	go build -o bin/users cmd/users/main.go

run:
	@echo "Running gateway service..."
	@echo "Note: Requires PostgreSQL running (make docker-up-infra from root)"
	go run cmd/gateway/main.go

test: test-unit test-integration

test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.unit.out ./internal/...

test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires PostgreSQL running"
	go test -v -race -tags=integration ./test/integration/...

coverage:
	@echo "Generating coverage report..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

docker-build:
	docker build -t pagewright-gateway:latest .

clean:
	rm -rf bin/
	rm -f coverage.*.out coverage.out coverage.html
