.PHONY: help build run test test-unit test-integration coverage docker-build clean

help:
	@echo "Worker Service - Makefile"
	@echo ""
	@echo "  build              - Build runner binary"
	@echo "  run                - Run the service locally"
	@echo "  test               - Run all tests"
	@echo "  test-unit          - Run unit tests"
	@echo "  test-integration   - Run integration tests"
	@echo "  coverage           - Generate coverage report"
	@echo "  docker-build       - Build Docker image"
	@echo "  clean              - Clean build artifacts"

build:
	@echo "Building runner..."
	go build -o runner ./cmd/runner

run:
	@echo "Running worker service..."
	@echo "Note: Worker requires PAGEWRIGHT_JOB env variable with job JSON"
	@echo "Example: PAGEWRIGHT_JOB='{\"job_id\":\"test\"}' make run"
	go run ./cmd/runner/main.go

test: test-unit

test-unit:
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-integration:
	@echo "Running integration tests..."
	@echo "Note: Requires storage and manager services"
	INTEGRATION_TEST=true go test -v -tags=integration ./test/integration/...

coverage:
	@echo "Generating coverage report..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

docker-build:
	@echo "Building Docker image..."
	docker build -t pagewright-worker:latest .

clean:
	@echo "Cleaning..."
	rm -f runner coverage.txt coverage.out coverage.html
	rm -rf /tmp/work-*
