.PHONY: help build test lint fmt clean tidy

# Default target
help:
	@echo "Available targets:"
	@echo "  build     - Build all packages"
	@echo "  test      - Run tests"
	@echo "  lint      - Run golangci-lint"
	@echo "  fmt       - Format code with gofmt"
	@echo "  clean     - Clean build artifacts"
	@echo "  tidy      - Tidy go.mod and go.sum"
	@echo "  ci        - Run all CI checks (fmt, lint, test)"

# Build all packages
build:
	@echo "Building..."
	go build ./...

# Run tests with coverage
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run golangci-lint
lint:
	@echo "Running linter..."
	golangci-lint run --timeout 5m

# Format code
fmt:
	@echo "Formatting code..."
	gofmt -s -w .

# Check formatting
fmt-check:
	@echo "Checking code formatting..."
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "The following files are not formatted:"; \
		gofmt -s -l .; \
		exit 1; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning..."
	go clean ./...
	rm -f coverage.txt

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Run all CI checks
ci: fmt-check lint test
	@echo "All CI checks passed!"
