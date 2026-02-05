# syntax=docker/dockerfile:1
FROM node:20-alpine AS builder

# Install pnpm
RUN npm install -g pnpm

# Set CI environment for pnpm
ENV CI=true

WORKDIR /app

# Copy SDK package (includes proto files)
COPY astro-messaging ./astro-messaging
WORKDIR /app/astro-messaging
RUN pnpm install && pnpm build

# Copy and build agent
WORKDIR /app/astro-agents/engineering-assistant
COPY astro-agents/engineering-assistant/package*.json ./
COPY astro-agents/engineering-assistant/tsconfig.json ./

# Install dependencies (with optional GitHub NPM token for @saswatds private packages)
RUN --mount=type=secret,id=npm_token \
    if [ -f /run/secrets/npm_token ]; then \
      echo "@saswatds:registry=https://npm.pkg.github.com" >> ~/.npmrc; \
      echo "//npm.pkg.github.com/:_authToken=$(cat /run/secrets/npm_token)" >> ~/.npmrc; \
    fi && \
    pnpm install && \
    rm -f ~/.npmrc

# Copy source code
COPY astro-agents/engineering-assistant/src ./src

# Build TypeScript
RUN pnpm build

# Production image
FROM node:20-alpine

# Install pnpm
RUN npm install -g pnpm

# Set CI environment for pnpm
ENV CI=true

WORKDIR /app

# Copy SDK package
COPY astro-messaging ./astro-messaging
WORKDIR /app/astro-messaging
RUN --mount=type=secret,id=npm_token \
    if [ -f /run/secrets/npm_token ]; then \
      echo "@saswatds:registry=https://npm.pkg.github.com" >> ~/.npmrc; \
      echo "//npm.pkg.github.com/:_authToken=$(cat /run/secrets/npm_token)" >> ~/.npmrc; \
    fi && \
    pnpm install --prod && \
    rm -f ~/.npmrc

# Setup agent
WORKDIR /app/astro-agents/engineering-assistant
COPY astro-agents/engineering-assistant/package*.json ./

# Install production dependencies only (with optional GitHub NPM token for @saswatds private packages)
RUN --mount=type=secret,id=npm_token \
    if [ -f /run/secrets/npm_token ]; then \
      echo "@saswatds:registry=https://npm.pkg.github.com" >> ~/.npmrc; \
      echo "//npm.pkg.github.com/:_authToken=$(cat /run/secrets/npm_token)" >> ~/.npmrc; \
    fi && \
    pnpm install --prod && \
    rm -f ~/.npmrc

# Copy built application
COPY --from=builder /app/astro-agents/engineering-assistant/dist ./dist

# Set default gRPC server address (can be overridden by astro CLI)
ENV GRPC_SERVER_ADDR=astro-messaging:9090

# Run the agent
CMD ["node", "dist/index.js"]
