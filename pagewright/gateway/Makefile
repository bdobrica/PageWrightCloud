.PHONY: help build test test-unit test-integration docker-build docker-up docker-down clean

help:
	@echo "Available targets:"
	@echo "  build              - Build the BFF binary"
	@echo "  test               - Run all tests"
	@echo "  test-unit          - Run unit tests"
	@echo "  test-integration   - Run integration tests"
	@echo "  docker-build       - Build Docker image"
	@echo "  docker-up          - Start services with docker-compose"
	@echo "  docker-down        - Stop services"
	@echo "  clean              - Clean build artifacts"

build:
	go build -o bin/gateway cmd/gateway/main.go

test: test-unit test-integration

test-unit:
	go test -v -race -coverprofile=coverage.unit.out ./internal/...

test-integration:
	go test -v -race -tags=integration ./test/integration/...

docker-build:
	docker build -t pagewright/bff:latest .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

clean:
	rm -rf bin/
	rm -f coverage.*.out
