.PHONY: build install clean test help

# Version information
VERSION ?= 0.1.0
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'github.com/postman/astro/apps/astro-cli/cmd.version=$(VERSION)' \
           -X 'github.com/postman/astro/apps/astro-cli/cmd.commit=$(COMMIT)' \
           -X 'github.com/postman/astro/apps/astro-cli/cmd.date=$(DATE)'

# Binary name
BINARY_NAME := astro

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME) v$(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .
	@echo "Build complete: ./$(BINARY_NAME)"

install: ## Install the binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME) v$(VERSION)..."
	go install -ldflags "$(LDFLAGS)" .
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

clean: ## Remove built binaries
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	@echo "Clean complete"

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

run: build ## Build and run the binary
	./$(BINARY_NAME)

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies updated"

fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...
	@echo "Format complete"

lint: ## Run linter
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run
	@echo "Lint complete"

.DEFAULT_GOAL := help
