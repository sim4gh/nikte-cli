.PHONY: build clean test test-integration test-all install

VERSION ?= 1.0.0
BINARY_NAME = nk
BUILD_DIR = build
LDFLAGS = -ldflags "-s -w -X github.com/sim4gh/nikte-cli/internal/cli.Version=$(VERSION)"

# Build for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/nk
	@echo "Built: $(BINARY_NAME)"

# Build for all platforms
build-all: clean
	@mkdir -p $(BUILD_DIR)
	@echo "Building for macOS (arm64)..."
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/nk
	@echo "Building for macOS (amd64)..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/nk
	@echo "Building for Linux (amd64)..."
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/nk
	@echo "Building for Linux (arm64)..."
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/nk
	@echo "Building for Windows (amd64)..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/nk
	@echo "Done! Binaries in $(BUILD_DIR)/"

# Install locally
install: build
	@echo "Installing to /usr/local/bin/$(BINARY_NAME)..."
	@sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installed!"

# Clean build artifacts
clean:
	@rm -f $(BINARY_NAME)
	@rm -rf $(BUILD_DIR)

# Run tests
test:
	@go test -v ./...

# Run integration tests (requires auth)
test-integration:
	@go test -v -tags=integration -timeout=5m -count=1 ./test/integration/

# Run all tests
test-all: test test-integration

# Format code
fmt:
	@go fmt ./...

# Run go mod tidy
tidy:
	@go mod tidy

# Development build with race detector
dev:
	@go build -race -o $(BINARY_NAME) ./cmd/nk

# Check for issues
lint:
	@golangci-lint run

# Show help
help:
	@echo "OIO CLI Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build      Build for current platform"
	@echo "  make build-all  Build for all platforms"
	@echo "  make install    Build and install to /usr/local/bin"
	@echo "  make clean      Remove build artifacts"
	@echo "  make test       Run unit tests"
	@echo "  make test-integration  Run integration tests (requires auth)"
	@echo "  make test-all   Run all tests"
	@echo "  make fmt        Format code"
	@echo "  make tidy       Run go mod tidy"
	@echo "  make dev        Build with race detector"
	@echo "  make lint       Run linter"
	@echo "  make help       Show this help"
