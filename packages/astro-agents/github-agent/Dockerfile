# Dockerfile for GitHub Agent

FROM oven/bun:latest

WORKDIR /app

# Copy package files
COPY package.json bun.lockb* ./

# Install dependencies
RUN bun install

# Copy source code
COPY src ./src

# Expose agent port
EXPOSE 8080

# Run the GitHub agent server
CMD ["bun", "run", "src/index.ts"]
