.PHONY: build run clean test deps help

# Binary name
BINARY_NAME=astro-server

# Build directory
BUILD_DIR=.

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download dependencies
	go mod download
	go mod tidy

build: ## Build the application
	go build -o $(BUILD_DIR)/$(BINARY_NAME) main.go

run: ## Run the application
	go run main.go

dev: ## Run with hot reload (requires air)
	air

test: ## Run tests
	go test -v ./...

test-cover: ## Run tests with coverage
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

clean: ## Remove build artifacts
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f coverage.out
	go clean

fmt: ## Format code
	go fmt ./...

lint: ## Run linter (requires golangci-lint)
	golangci-lint run

vet: ## Run go vet
	go vet ./...

install: ## Install the binary
	go install

all: clean deps fmt vet build ## Run all build steps
