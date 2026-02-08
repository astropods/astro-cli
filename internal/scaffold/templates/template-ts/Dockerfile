# Build stage
FROM oven/bun:1 AS builder

WORKDIR /app

# NPM private registry configuration
ARG NPM_SCOPE
ARG NPM_REGISTRY

# Install dependencies
COPY package.json bun.lock* ./
RUN --mount=type=secret,id=npm_token \
    if [ -f /run/secrets/npm_token ]; then \
      echo "${NPM_SCOPE}:registry=${NPM_REGISTRY}" >> ~/.npmrc; \
      echo "//npm.pkg.github.com/:_authToken=$(cat /run/secrets/npm_token)" >> ~/.npmrc; \
    fi && \
    bun install --frozen-lockfile && \
    rm -f ~/.npmrc

# Copy source
COPY . .

# Runtime stage
FROM oven/bun:1-slim

WORKDIR /app

# Copy from builder
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/agent ./agent
COPY --from=builder /app/package.json ./

# Use non-root user already present in oven/bun image (bun:1000)
RUN chown -R bun:bun /app
USER bun

# Run the agent
CMD ["bun", "run", "agent/index.ts"]
