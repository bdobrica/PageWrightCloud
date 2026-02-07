.PHONY: build test clean install run-test

# Build the compiler
build:
	go build -o pagewrightc ./cmd/pagewrightc

# Build with optimizations for production
build-prod:
	go build -ldflags="-s -w" -o pagewrightc ./cmd/pagewrightc

# Run tests
test:
	go test ./... -v

# Run tests with coverage
coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -f pagewrightc
	rm -rf test-site/dist
	rm -f coverage.out coverage.html

# Install dependencies
deps:
	go mod download
	go mod tidy

# Run the test site build
run-test: build
	./pagewrightc build \
		--theme ../themes/starter \
		--content ./test-site/content \
		--out ./test-site/dist

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Show help
help:
	@echo "Pagewright Compiler - Makefile Commands"
	@echo ""
	@echo "  make build         - Build the compiler"
	@echo "  make build-prod    - Build optimized binary"
	@echo "  make test          - Run tests"
	@echo "  make coverage      - Run tests with coverage report"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make deps          - Download and tidy dependencies"
	@echo "  make run-test      - Build and run test site"
	@echo "  make fmt           - Format code"
	@echo "  make lint          - Lint code"
	@echo "  make help          - Show this help message"
