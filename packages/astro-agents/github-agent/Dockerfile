# Dockerfile for GitHub Agent

FROM oven/bun:latest

WORKDIR /app

# Copy package files
COPY package.json bun.lockb* ./

# Install dependencies
RUN bun install

# Copy source code
COPY src ./src

# Use non-root user already present in oven/bun image (bun:1000)
RUN chown -R bun:bun /app
USER bun

# Expose agent port
EXPOSE 8080

# Run the GitHub agent server
CMD ["bun", "run", "src/index.ts"]
