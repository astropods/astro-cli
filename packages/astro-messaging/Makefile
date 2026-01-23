# Makefile for Astro Messaging Service
# Builds and publishes Docker images to GitHub Container Registry

# Load environment variables from root .env file
ifneq (,$(wildcard ../../.env))
    include ../../.env
    export
endif

# Version management
VERSION := $(shell cat VERSION 2>/dev/null || echo "0.1.0")
COMMIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Docker image configuration
IMAGE_NAME := astro-messaging
REGISTRY := $(GITHUB_REGISTRY)
USERNAME := $(GITHUB_USERNAME)
FULL_IMAGE := $(REGISTRY)/$(USERNAME)/$(IMAGE_NAME)

# Build configuration
DOCKERFILE := deployments/Dockerfile
BUILD_CONTEXT := .

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help
help: ## Show this help message
	@echo "$(BLUE)Astro Messaging Service - Docker Build & Publish$(NC)"
	@echo ""
	@echo "$(GREEN)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_-]+:.*##/ {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
	@echo ""
	@echo "$(GREEN)Configuration:$(NC)"
	@echo "  Registry:    $(REGISTRY)"
	@echo "  Username:    $(USERNAME)"
	@echo "  Image:       $(FULL_IMAGE)"
	@echo "  Version:     $(VERSION)"
	@echo "  Commit:      $(COMMIT_SHA)"

.PHONY: check-env
check-env: ## Check if required environment variables are set
	@echo "$(BLUE)Checking environment variables...$(NC)"
	@if [ -z "$(GITHUB_USERNAME)" ]; then \
		echo "$(RED)ERROR: GITHUB_USERNAME is not set$(NC)"; \
		echo "Please create a .env file in the root directory with:"; \
		echo "  GITHUB_USERNAME=your-username"; \
		echo "  GITHUB_TOKEN=your-token"; \
		echo "  GITHUB_REGISTRY=ghcr.io"; \
		exit 1; \
	fi
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "$(RED)ERROR: GITHUB_TOKEN is not set$(NC)"; \
		echo "Please set GITHUB_TOKEN in the root .env file"; \
		exit 1; \
	fi
	@if [ -z "$(GITHUB_REGISTRY)" ]; then \
		echo "$(RED)ERROR: GITHUB_REGISTRY is not set$(NC)"; \
		echo "Please set GITHUB_REGISTRY in the root .env file (e.g., ghcr.io)"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Environment variables are set$(NC)"

.PHONY: build
build: check-env ## Build the Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build \
		-f $(DOCKERFILE) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT_SHA) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(FULL_IMAGE):$(VERSION) \
		-t $(FULL_IMAGE):$(COMMIT_SHA) \
		-t $(FULL_IMAGE):latest \
		$(BUILD_CONTEXT)
	@echo "$(GREEN)✓ Build complete$(NC)"
	@echo "  Tagged as:"
	@echo "    - $(FULL_IMAGE):$(VERSION)"
	@echo "    - $(FULL_IMAGE):$(COMMIT_SHA)"
	@echo "    - $(FULL_IMAGE):latest"

.PHONY: login
login: check-env ## Login to GitHub Container Registry
	@echo "$(BLUE)Logging in to $(REGISTRY)...$(NC)"
	@echo "$(GITHUB_TOKEN)" | docker login $(REGISTRY) --username $(USERNAME) --password-stdin || \
	(echo "$(RED)✗ Login failed$(NC)" && \
	 echo "$(YELLOW)Try logging in manually with:$(NC)" && \
	 echo "  echo \"\$$GITHUB_TOKEN\" | docker login $(REGISTRY) --username $(USERNAME) --password-stdin" && \
	 exit 1)
	@echo "$(GREEN)✓ Login successful$(NC)"

.PHONY: push
push: check-env ## Push the Docker image to registry (requires login)
	@echo "$(BLUE)Pushing images to registry...$(NC)"
	docker push $(FULL_IMAGE):$(VERSION)
	docker push $(FULL_IMAGE):$(COMMIT_SHA)
	docker push $(FULL_IMAGE):latest
	@echo "$(GREEN)✓ Push complete$(NC)"
	@echo "  Images available at:"
	@echo "    - $(FULL_IMAGE):$(VERSION)"
	@echo "    - $(FULL_IMAGE):$(COMMIT_SHA)"
	@echo "    - $(FULL_IMAGE):latest"

.PHONY: publish
publish: check-env login build push ## Complete workflow: login, build, and push
	@echo "$(GREEN)✓ Publish complete!$(NC)"
	@echo ""
	@echo "$(BLUE)Pull your image with:$(NC)"
	@echo "  docker pull $(FULL_IMAGE):$(VERSION)"
	@echo "  docker pull $(FULL_IMAGE):latest"

.PHONY: run
run: ## Run the container locally
	@echo "$(BLUE)Running container locally...$(NC)"
	@if [ ! -f ../../.env ]; then \
		echo "$(YELLOW)WARNING: No .env file found in root directory$(NC)"; \
		echo "The container may not start without proper configuration."; \
	fi
	docker run --rm -it \
		--env-file ../../.env \
		-p 8081:8081 \
		$(FULL_IMAGE):latest

.PHONY: test
test: ## Run container in test mode (with health check)
	@echo "$(BLUE)Testing container...$(NC)"
	@docker run -d --name astro-messaging-test \
		-e STORAGE_TYPE=memory \
		-e LISTEN_ADDR=:8081 \
		-e AGENT_URL=http://localhost:8080 \
		-p 8081:8081 \
		$(FULL_IMAGE):latest
	@echo "Waiting for container to start..."
	@sleep 3
	@echo "Checking health endpoint..."
	@if curl -s http://localhost:8081/health > /dev/null; then \
		echo "$(GREEN)✓ Container is healthy$(NC)"; \
		docker logs astro-messaging-test; \
		docker stop astro-messaging-test; \
		docker rm astro-messaging-test; \
	else \
		echo "$(RED)✗ Container health check failed$(NC)"; \
		docker logs astro-messaging-test; \
		docker stop astro-messaging-test; \
		docker rm astro-messaging-test; \
		exit 1; \
	fi

.PHONY: clean
clean: ## Remove local Docker images
	@echo "$(BLUE)Cleaning up local images...$(NC)"
	-docker rmi $(FULL_IMAGE):$(VERSION) 2>/dev/null
	-docker rmi $(FULL_IMAGE):$(COMMIT_SHA) 2>/dev/null
	-docker rmi $(FULL_IMAGE):latest 2>/dev/null
	@echo "$(GREEN)✓ Cleanup complete$(NC)"

.PHONY: info
info: ## Show build information
	@echo "$(BLUE)Build Information:$(NC)"
	@echo "  Image Name:     $(IMAGE_NAME)"
	@echo "  Full Image:     $(FULL_IMAGE)"
	@echo "  Version:        $(VERSION)"
	@echo "  Commit SHA:     $(COMMIT_SHA)"
	@echo "  Build Date:     $(BUILD_DATE)"
	@echo "  Registry:       $(REGISTRY)"
	@echo "  Username:       $(USERNAME)"
	@echo "  Dockerfile:     $(DOCKERFILE)"
	@echo ""
	@echo "$(BLUE)Tags that will be created:$(NC)"
	@echo "  - $(FULL_IMAGE):$(VERSION)"
	@echo "  - $(FULL_IMAGE):$(COMMIT_SHA)"
	@echo "  - $(FULL_IMAGE):latest"

.PHONY: version
version: ## Show current version
	@echo "$(VERSION)"

.PHONY: version-bump-patch
version-bump-patch: ## Bump patch version (0.1.0 -> 0.1.1)
	@echo "$(BLUE)Bumping patch version...$(NC)"
	@CURRENT=$$(cat VERSION); \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1); \
	MINOR=$$(echo $$CURRENT | cut -d. -f2); \
	PATCH=$$(echo $$CURRENT | cut -d. -f3); \
	NEW_PATCH=$$(($$PATCH + 1)); \
	NEW_VERSION="$$MAJOR.$$MINOR.$$NEW_PATCH"; \
	echo $$NEW_VERSION > VERSION; \
	echo "$(GREEN)✓ Version bumped: $$CURRENT -> $$NEW_VERSION$(NC)"

.PHONY: version-bump-minor
version-bump-minor: ## Bump minor version (0.1.0 -> 0.2.0)
	@echo "$(BLUE)Bumping minor version...$(NC)"
	@CURRENT=$$(cat VERSION); \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1); \
	MINOR=$$(echo $$CURRENT | cut -d. -f2); \
	NEW_MINOR=$$(($$MINOR + 1)); \
	NEW_VERSION="$$MAJOR.$$NEW_MINOR.0"; \
	echo $$NEW_VERSION > VERSION; \
	echo "$(GREEN)✓ Version bumped: $$CURRENT -> $$NEW_VERSION$(NC)"

.PHONY: version-bump-major
version-bump-major: ## Bump major version (0.1.0 -> 1.0.0)
	@echo "$(BLUE)Bumping major version...$(NC)"
	@CURRENT=$$(cat VERSION); \
	MAJOR=$$(echo $$CURRENT | cut -d. -f1); \
	NEW_MAJOR=$$(($$MAJOR + 1)); \
	NEW_VERSION="$$NEW_MAJOR.0.0"; \
	echo $$NEW_VERSION > VERSION; \
	echo "$(GREEN)✓ Version bumped: $$CURRENT -> $$NEW_VERSION$(NC)"

.DEFAULT_GOAL := help
