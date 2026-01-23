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

# Install dependencies
RUN pnpm install

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
RUN pnpm install --prod

# Setup agent
WORKDIR /app/astro-agents/engineering-assistant
COPY astro-agents/engineering-assistant/package*.json ./

# Install production dependencies only
RUN pnpm install --prod

# Copy built application
COPY --from=builder /app/astro-agents/engineering-assistant/dist ./dist

# Set default gRPC server address (can be overridden by astro CLI)
ENV GRPC_SERVER_ADDR=astro-messaging:9090

# Run the agent
CMD ["node", "dist/index.js"]
