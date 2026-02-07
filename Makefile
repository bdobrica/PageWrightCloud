.PHONY: help docker-up docker-down docker-logs docker-build docker-clean \
        test-all test-gateway test-manager test-storage test-worker test-serving \
        build-all build-gateway build-manager build-storage build-worker build-serving \
        clean coverage

# Default target
help:
	@echo "PageWrightCloud - Makefile Commands"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make docker-up           - Start all services (infrastructure + apps)"
	@echo "  make docker-up-infra     - Start only infrastructure (postgres, redis, nfs)"
	@echo "  make docker-up-worker    - Start all services including worker"
	@echo "  make docker-down         - Stop all services"
	@echo "  make docker-logs         - Follow logs for all services"
	@echo "  make docker-logs-<svc>   - Follow logs for specific service"
	@echo "  make docker-build        - Build all service images"
	@echo "  make docker-clean        - Remove all containers, volumes, and images"
	@echo ""
	@echo "Testing Commands:"
	@echo "  make test-all            - Run all tests"
	@echo "  make test-gateway        - Run gateway tests"
	@echo "  make test-manager        - Run manager tests"
	@echo "  make test-storage        - Run storage tests"
	@echo "  make test-worker         - Run worker tests"
	@echo "  make test-serving        - Run serving tests"
	@echo "  make coverage            - Generate coverage reports for all services"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build-all           - Build all service binaries"
	@echo "  make build-gateway       - Build gateway binary"
	@echo "  make build-manager       - Build manager binary"
	@echo "  make build-storage       - Build storage binary"
	@echo "  make build-worker        - Build worker binary"
	@echo "  make build-serving       - Build serving binary"
	@echo ""
	@echo "Cleanup Commands:"
	@echo "  make clean               - Clean all build artifacts"

# =============================================================================
# Docker Commands
# =============================================================================

docker-up:
	@echo "Starting all services..."
	docker-compose up -d
	@echo "Services started. Check health with: make docker-ps"

docker-up-infra:
	@echo "Starting infrastructure only..."
	docker-compose up -d postgres redis nfs-server
	@echo "Infrastructure started."

docker-up-worker:
	@echo "Starting all services including worker..."
	docker-compose --profile worker up -d
	@echo "All services started including worker."

docker-down:
	@echo "Stopping all services..."
	docker-compose down
	@echo "Services stopped."

docker-logs:
	docker-compose logs -f

docker-logs-gateway:
	docker-compose logs -f gateway

docker-logs-manager:
	docker-compose logs -f manager

docker-logs-storage:
	docker-compose logs -f storage

docker-logs-worker:
	docker-compose logs -f worker

docker-logs-serving:
	docker-compose logs -f serving

docker-logs-ui:
	docker-compose logs -f ui

docker-ps:
	docker-compose ps

docker-build:
	@echo "Building all service images..."
	docker-compose build
	@echo "All images built."

docker-build-gateway:
	docker-compose build gateway

docker-build-manager:
	docker-compose build manager

docker-build-storage:
	docker-compose build storage

docker-build-worker:
	docker-compose build worker

docker-build-serving:
	docker-compose build serving

docker-build-ui:
	docker-compose build ui

docker-clean:
	@echo "Cleaning up Docker resources..."
	docker-compose down -v --rmi all
	@echo "Cleanup complete."

# =============================================================================
# Testing Commands
# =============================================================================

test-all: test-gateway test-manager test-storage test-worker test-serving
	@echo "All tests completed!"

test-gateway:
	@echo "Running gateway tests..."
	@cd pagewright/gateway && $(MAKE) test

test-manager:
	@echo "Running manager tests..."
	@cd pagewright/manager && $(MAKE) test

test-storage:
	@echo "Running storage tests..."
	@cd pagewright/storage && $(MAKE) test

test-worker:
	@echo "Running worker tests..."
	@cd pagewright/worker && $(MAKE) test

test-serving:
	@echo "Running serving tests..."
	@cd pagewright/serving && $(MAKE) test

# Integration tests (require docker-compose up)
test-integration: docker-up-infra
	@echo "Running integration tests..."
	@cd pagewright/gateway && $(MAKE) test-integration
	@cd pagewright/storage && $(MAKE) test-integration
	@echo "Integration tests completed!"

# =============================================================================
# Build Commands
# =============================================================================

build-all: build-gateway build-manager build-storage build-worker build-serving
	@echo "All binaries built!"

build-gateway:
	@echo "Building gateway..."
	@cd pagewright/gateway && $(MAKE) build

build-manager:
	@echo "Building manager..."
	@cd pagewright/manager && $(MAKE) build

build-storage:
	@echo "Building storage..."
	@cd pagewright/storage && $(MAKE) build

build-worker:
	@echo "Building worker..."
	@cd pagewright/worker && $(MAKE) build

build-serving:
	@echo "Building serving..."
	@cd pagewright/serving && $(MAKE) build

# =============================================================================
# Coverage Commands
# =============================================================================

coverage:
	@echo "Generating coverage reports..."
	@cd pagewright/gateway && $(MAKE) coverage
	@cd pagewright/manager && $(MAKE) coverage
	@cd pagewright/storage && $(MAKE) coverage
	@cd pagewright/worker && $(MAKE) coverage
	@cd pagewright/serving && $(MAKE) coverage
	@echo "Coverage reports generated. Open coverage.html in each service directory."

# =============================================================================
# Cleanup Commands
# =============================================================================

clean:
	@echo "Cleaning build artifacts..."
	@cd pagewright/gateway && $(MAKE) clean
	@cd pagewright/manager && $(MAKE) clean
	@cd pagewright/storage && $(MAKE) clean
	@cd pagewright/worker && $(MAKE) clean
	@cd pagewright/serving && $(MAKE) clean
	@echo "Cleanup complete."

# =============================================================================
# Development Helpers
# =============================================================================

fmt:
	@echo "Formatting Go code..."
	@cd pagewright/gateway && go fmt ./...
	@cd pagewright/manager && go fmt ./...
	@cd pagewright/storage && go fmt ./...
	@cd pagewright/worker && go fmt ./...
	@cd pagewright/serving && go fmt ./...
	@echo "Code formatted."

vet:
	@echo "Running go vet..."
	@cd pagewright/gateway && go vet ./...
	@cd pagewright/manager && go vet ./...
	@cd pagewright/storage && go vet ./...
	@cd pagewright/worker && go vet ./...
	@cd pagewright/serving && go vet ./...
	@echo "Vet completed."

lint: fmt vet
	@echo "Linting completed!"
