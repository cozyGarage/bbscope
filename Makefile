# bbscope Makefile
# Build automation for development and releases

# Build variables
BINARY_NAME := bbscope
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_VERSION := $(shell go version | cut -d' ' -f3)

# Go build flags
LDFLAGS := -ldflags "-s -w \
	-X 'github.com/sw33tLie/bbscope/v2/cmd.Version=$(VERSION)' \
	-X 'github.com/sw33tLie/bbscope/v2/cmd.Commit=$(COMMIT)' \
	-X 'github.com/sw33tLie/bbscope/v2/cmd.BuildDate=$(BUILD_DATE)'"

# Directories
BUILD_DIR := build
DIST_DIR := dist

# Default target
.DEFAULT_GOAL := build

.PHONY: all build build-all clean test lint fmt vet run install help version docker

## help: Show this help message
help:
	@echo "bbscope - Bug Bounty Scope Aggregator"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## version: Show version information
version:
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $(GO_VERSION)"

## build: Build the binary for current platform
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) .

## build-linux: Build for Linux amd64
build-linux:
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .

## build-linux-arm64: Build for Linux arm64
build-linux-arm64:
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .

## build-darwin: Build for macOS amd64
build-darwin:
	@echo "Building for macOS amd64..."
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .

## build-darwin-arm64: Build for macOS arm64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building for macOS arm64..."
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .

## build-windows: Build for Windows amd64
build-windows:
	@echo "Building for Windows amd64..."
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

## build-all: Build for all platforms
build-all: clean
	@mkdir -p $(BUILD_DIR)
	$(MAKE) build-linux
	$(MAKE) build-linux-arm64
	$(MAKE) build-darwin
	$(MAKE) build-darwin-arm64
	$(MAKE) build-windows
	@echo "All builds complete. Binaries in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

## install: Install to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) .

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(BINARY_NAME).exe
	rm -rf $(BUILD_DIR) $(DIST_DIR)
	go clean

## test: Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

## test-short: Run tests without race detector
test-short:
	@echo "Running short tests..."
	go test -v -cover ./...

## coverage: Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run linters
lint:
	@echo "Running linters..."
	@which golangci-lint > /dev/null || (echo "Install golangci-lint: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@which goimports > /dev/null && goimports -w . || echo "goimports not found, skipping"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## tidy: Tidy go modules
tidy:
	@echo "Tidying modules..."
	go mod tidy

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download

## update-deps: Update dependencies
update-deps:
	@echo "Updating dependencies..."
	go get -u ./...
	go mod tidy

## security: Run security checks
security:
	@echo "Running security checks..."
	@which gosec > /dev/null || (echo "Install gosec: go install github.com/securego/gosec/v2/cmd/gosec@latest" && exit 1)
	gosec -quiet ./...
	@which govulncheck > /dev/null && govulncheck ./... || echo "govulncheck not found, skipping"

## run: Build and run with default args
run: build
	./$(BINARY_NAME)

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) -t $(BINARY_NAME):latest .

## docker-run: Run Docker container
docker-run:
	docker run --rm -it $(BINARY_NAME):latest

## release: Create release archives
release: build-all
	@mkdir -p $(DIST_DIR)
	@echo "Creating release archives..."
	@cd $(BUILD_DIR) && for f in $(BINARY_NAME)-*; do \
		if [ -f "$$f" ]; then \
			if echo "$$f" | grep -q ".exe"; then \
				zip -q ../$(DIST_DIR)/$${f%.exe}.zip $$f; \
			else \
				tar -czf ../$(DIST_DIR)/$$f.tar.gz $$f; \
			fi \
		fi \
	done
	@echo "Release archives in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/

## release-snapshot: Create a snapshot release with GoReleaser
release-snapshot:
	@echo "Creating snapshot release..."
	@which goreleaser > /dev/null || (echo "Install goreleaser: https://goreleaser.com/install/" && exit 1)
	goreleaser release --snapshot --clean

## release-dry-run: Dry run of GoReleaser
release-dry-run:
	@echo "Dry run of release..."
	@which goreleaser > /dev/null || (echo "Install goreleaser: https://goreleaser.com/install/" && exit 1)
	goreleaser release --snapshot --skip=publish --clean

## all: Run fmt, vet, lint, test, and build
all: fmt vet lint test build
