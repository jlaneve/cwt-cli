# CWT Makefile

.PHONY: test build clean lint install run help

# Default target
help:
	@echo "Available targets:"
	@echo "  test     - Run all tests"
	@echo "  build    - Build the binary"
	@echo "  clean    - Clean build artifacts"
	@echo "  lint     - Run linters (if available)"
	@echo "  install  - Install the binary"
	@echo "  run      - Run with arguments (use ARGS=...)"
	@echo "  help     - Show this help"

# Run tests
test:
	go test ./internal/... -v

# Run tests with coverage
test-cover:
	go test ./internal/... -v -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Build the binary
build:
	go build -o cwt ./cmd/cwt

# Clean build artifacts
clean:
	rm -f cwt coverage.out coverage.html

# Run linters (requires golangci-lint)
lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Install the binary to GOPATH/bin
install:
	go install ./cmd/cwt

# Run with arguments (make run ARGS="new test-session")
run: build
	./cwt $(ARGS)

# Development: watch for changes and run tests
watch:
	@if command -v fswatch > /dev/null; then \
		fswatch -o . -e ".*" -i "\\.go$$" | xargs -n1 -I{} make test; \
	else \
		echo "fswatch not found. Install with: brew install fswatch (macOS)"; \
	fi