# Astro Platform Makefile

export AWS_PROFILE := astro

IMAGE_TAG ?= latest
AWS_ACCOUNT_ID := $(shell aws sts get-caller-identity --query Account --output text)
ECR_REGISTRY := $(AWS_ACCOUNT_ID).dkr.ecr.us-east-1.amazonaws.com
IMAGE := $(ECR_REGISTRY)/prod-astro-server:$(IMAGE_TAG)

.PHONY: release
release: ## Build and push Docker image, then show required env vars
	@echo "Logging into ECR..."
	@aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin $(ECR_REGISTRY) || \
		(echo ""; echo "ERROR: AWS profile 'astro' not found. Only @saswat can deploy at the moment."; exit 1)
	@echo ""
	@echo "Building $(IMAGE) for linux/amd64..."
	@docker build -t $(IMAGE) .
	@echo ""
	@echo "Pushing $(IMAGE)..."
	@docker push $(IMAGE)
	@echo ""
	@$(MAKE) --no-print-directory envs

.PHONY: envs
envs: ## List required environment variables (extracted from config.go)
	@echo "=============================================================================="
	@echo "ENVIRONMENT VARIABLES (from apps/astro-server/internal/config/config.go)"
	@echo "=============================================================================="
	@awk -F'"' '/getEnv\(/ && !/^func/ {printf "%-25s (default: %s)\n", $$2, $$4}' \
		apps/astro-server/internal/config/config.go | sort -u
	@awk -F'"' '/getEnvDuration\(/ && !/^func/ {split($$0,a,","); gsub(/\).*/,"",a[2]); printf "%-25s (default:%s)\n", $$2, a[2]}' \
		apps/astro-server/internal/config/config.go | sort -u
	@awk -F'"' '/getEnvSlice\(/ && !/^func/ {split($$0,a,","); gsub(/\).*/,"",a[2]); printf "%-25s (default:%s)\n", $$2, a[2]}' \
		apps/astro-server/internal/config/config.go | sort -u
	@echo "=============================================================================="
