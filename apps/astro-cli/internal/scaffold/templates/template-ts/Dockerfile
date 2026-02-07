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

# Create non-root user
RUN addgroup --gid 1000 astro && \
    adduser --uid 1000 --gid 1000 --disabled-password --gecos "" astro

# Copy from builder
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/agent ./agent
COPY --from=builder /app/package.json ./

# Set ownership and switch to non-root user
RUN chown -R astro:astro /app
USER astro

# Run the agent
CMD ["bun", "run", "agent/index.ts"]
