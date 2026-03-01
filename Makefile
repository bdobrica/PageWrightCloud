.PHONY: help docker-up docker-down docker-logs docker-build docker-clean \
        test-all test-gateway test-manager test-storage test-worker test-serving \
        build-all build-gateway build-manager build-storage build-worker build-serving \
	clean coverage docker-verify-local-domain docker-verify-local-domain-strict

TEST_FQDN ?= demo.pagewright.io
TEST_EMAIL ?= local-domain-test@pagewright.io
TEST_PASSWORD ?= TestPass123!

# Default target
help:
	@echo "PageWrightCloud - Makefile Commands"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make docker-up           - Start all services (infrastructure + apps)"
	@echo "  make docker-up-local-domain - Start all services with pagewright.io UI domain config"
	@echo "  make docker-up-infra     - Start only infrastructure (postgres, redis, nfs)"
	@echo "  make docker-up-worker    - Start all services including worker"
	@echo "  make docker-down         - Stop all services"
	@echo "  make docker-down-local-domain - Stop services started with local-domain overlay"
	@echo "  make docker-verify-local-domain - Verify local-domain routing and health checks"
	@echo "  make docker-verify-local-domain-strict - Verify end-to-end site setup and expect HTTP 200"
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
	@echo "  make test-compiler       - Run compiler tests"
	@echo "  make coverage            - Generate coverage reports for all services"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build-all           - Build all service binaries"
	@echo "  make build-gateway       - Build gateway binary"
	@echo "  make build-manager       - Build manager binary"
	@echo "  make build-storage       - Build storage binary"
	@echo "  make build-worker        - Build worker binary"
	@echo "  make build-serving       - Build serving binary"
	@echo "  make build-compiler      - Build compiler binary"
	@echo ""
	@echo "Cleanup Commands:"
	@echo "  make clean               - Clean all build artifacts"

# =============================================================================
# Docker Commands
# =============================================================================

docker-up:
	@echo "Starting all services..."
	docker compose up -d
	@echo "Services started. Check health with: make docker-ps"

docker-up-local-domain:
	@echo "Starting all services in local-domain mode (pagewright.io)..."
	docker compose -f docker-compose.yaml -f docker-compose.local-domain.yaml up -d --build
	@echo "Services started in local-domain mode."

docker-up-infra:
	@echo "Starting infrastructure only..."
	docker compose up -d postgres redis nfs-server
	@echo "Infrastructure started."

docker-up-worker:
	@echo "Starting all services including worker..."
	docker compose --profile worker up -d
	@echo "All services started including worker."

docker-down:
	@echo "Stopping all services..."
	docker compose down
	@echo "Services stopped."

docker-down-local-domain:
	@echo "Stopping local-domain mode services..."
	docker compose -f docker-compose.yaml -f docker-compose.local-domain.yaml down
	@echo "Services stopped."

docker-verify-local-domain:
	@echo "Verifying local-domain mode (health + host-based routing)..."
	@set -e; \
	echo "[1/4] Service status"; \
	docker compose ps; \
	echo "[2/4] Core health endpoints"; \
	curl --fail --silent --show-error http://localhost:8085/health >/dev/null; \
	curl --fail --silent --show-error http://localhost:8081/health >/dev/null; \
	curl --fail --silent --show-error http://localhost:8080/health >/dev/null; \
	curl --fail --silent --show-error http://localhost:8083/health >/dev/null; \
	echo "[3/4] UI host-header routing"; \
	ui_status=$$(curl --silent --output /dev/null --write-out "%{http_code}" -H "Host: pagewright.io" http://localhost:3000/); \
	if [ "$$ui_status" != "200" ]; then \
		echo "UI routing check failed (expected 200, got $$ui_status)"; \
		exit 1; \
	fi; \
	echo "[4/4] Serving host-header routing for $(TEST_FQDN)"; \
	serving_status=$$(curl --silent --output /dev/null --write-out "%{http_code}" -H "Host: $(TEST_FQDN)" http://localhost:8084/); \
	case "$$serving_status" in \
		200|404|503) ;; \
		*) echo "Serving routing check failed (unexpected status $$serving_status)"; exit 1 ;; \
	esac; \
	echo "Verification passed: local-domain mode looks healthy."

docker-verify-local-domain-strict:
	@echo "Running strict local-domain verification (auth + site create + serving 200)..."
	@set -e; \
	echo "[1/7] Core health endpoints"; \
	curl --fail --silent --show-error http://localhost:8085/health >/dev/null; \
	curl --fail --silent --show-error http://localhost:8083/health >/dev/null; \
	echo "[2/7] Ensure test user exists (register if needed)"; \
	register_code=$$(curl --silent --output /dev/null --write-out "%{http_code}" \
	  -H "Content-Type: application/json" \
	  -d '{"email":"$(TEST_EMAIL)","password":"$(TEST_PASSWORD)"}' \
	  http://localhost:8085/auth/register); \
	case "$$register_code" in 200|201|400|409) ;; *) echo "Register failed: $$register_code"; exit 1 ;; esac; \
	echo "[3/7] Login and get token"; \
	login_resp=$$(curl --silent --show-error \
	  -H "Content-Type: application/json" \
	  -d '{"email":"$(TEST_EMAIL)","password":"$(TEST_PASSWORD)"}' \
	  http://localhost:8085/auth/login); \
	token=$$(printf "%s" "$$login_resp" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'); \
	if [ -z "$$token" ]; then \
	  echo "Login failed: unable to extract token"; \
	  echo "Response: $$login_resp"; \
	  exit 1; \
	fi; \
	echo "[4/7] Create site $(TEST_FQDN)"; \
	create_code=$$(curl --silent --output /dev/null --write-out "%{http_code}" \
	  -H "Authorization: Bearer $$token" \
	  -H "Content-Type: application/json" \
	  -d '{"fqdn":"$(TEST_FQDN)","template_id":"starter"}' \
	  http://localhost:8085/sites); \
	case "$$create_code" in 200|201|400|409|500) ;; *) echo "Create site failed: $$create_code"; exit 1 ;; esac; \
	echo "[5/7] Seed placeholder index for deterministic 200"; \
	test_domain=$$(printf '%s' '$(TEST_FQDN)' | cut -d. -f2-); \
	docker exec pagewright-serving sh -lc 'mkdir -p "/var/www/$(TEST_FQDN)/public" \
	  && mkdir -p "/var/www/'"$$test_domain"'/$(TEST_FQDN)/public" \
	  && printf "<html><body><h1>$(TEST_FQDN)</h1></body></html>" > "/var/www/$(TEST_FQDN)/public/index.html" \
	  && printf "<html><body><h1>$(TEST_FQDN)</h1></body></html>" > "/var/www/'"$$test_domain"'/$(TEST_FQDN)/public/index.html"'; \
	echo "[6/7] Enable site via Gateway"; \
	enable_code=$$(curl --silent --output /dev/null --write-out "%{http_code}" \
	  -X POST \
	  -H "Authorization: Bearer $$token" \
	  http://localhost:8085/sites/$(TEST_FQDN)/enable); \
	if [ "$$enable_code" != "200" ]; then \
	  echo "Enable site failed: $$enable_code"; \
	  exit 1; \
	fi; \
	echo "[6.5/7] Reload nginx container"; \
	docker exec pagewright-nginx nginx -s reload >/dev/null; \
	echo "[7/7] Validate host-based serving returns HTTP 200"; \
	serving_code=$$(curl --silent --output /dev/null --write-out "%{http_code}" -H "Host: $(TEST_FQDN)" http://localhost:8084/); \
	if [ "$$serving_code" != "200" ]; then \
	  echo "Strict verification failed: expected 200, got $$serving_code"; \
	  exit 1; \
	fi; \
	echo "Strict verification passed: $(TEST_FQDN) is served with HTTP 200."

docker-logs:
	docker compose logs -f

docker-logs-gateway:
	docker compose logs -f gateway

docker-logs-manager:
	docker compose logs -f manager

docker-logs-storage:
	docker compose logs -f storage

docker-logs-worker:
	docker compose logs -f worker

docker-logs-serving:
	docker compose logs -f serving

docker-logs-ui:
	docker compose logs -f ui

docker-ps:
	docker compose ps

docker-build:
	@echo "Building all service images..."
	docker compose build
	@echo "All images built."

docker-build-gateway:
	docker compose build gateway

docker-build-manager:
	docker compose build manager

docker-build-storage:
	docker compose build storage

docker-build-worker:
	docker compose build worker

docker-build-serving:
	docker compose build serving

docker-build-themes:
	docker compose build themes

docker-build-ui:
	docker compose build ui

docker-clean:
	@echo "Cleaning up Docker resources..."
	docker compose down -v --rmi all
	@echo "Cleanup complete."

# =============================================================================
# Testing Commands
# =============================================================================

test-all: test-gateway test-manager test-storage test-worker test-serving test-compiler
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

test-compiler:
	@echo "Running compiler tests..."
	@cd pagewright/compiler && $(MAKE) test

# Integration tests (require docker compose up)
test-integration: docker-up-infra
	@echo "Running integration tests..."
	@cd pagewright/gateway && $(MAKE) test-integration
	@cd pagewright/storage && $(MAKE) test-integration
	@echo "Integration tests completed!"

# =============================================================================
# Build Commands
# =============================================================================

build-all: build-gateway build-manager build-storage build-worker build-serving build-compiler
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

build-compiler:
	@echo "Building compiler..."
	@cd pagewright/compiler && $(MAKE) build

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
	@cd pagewright/compiler && $(MAKE) coverage
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
	@cd pagewright/compiler && $(MAKE) clean
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
	@cd pagewright/compiler && go fmt ./...
	@echo "Code formatted."

vet:
	@echo "Running go vet..."
	@cd pagewright/gateway && go vet ./...
	@cd pagewright/manager && go vet ./...
	@cd pagewright/storage && go vet ./...
	@cd pagewright/worker && go vet ./...
	@cd pagewright/serving && go vet ./...
	@cd pagewright/compiler && go vet ./...
	@echo "Vet completed."

lint: fmt vet
	@echo "Linting completed!"
