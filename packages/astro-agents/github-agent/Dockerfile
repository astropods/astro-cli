# Dockerfile for GitHub Agent

FROM oven/bun:latest

WORKDIR /app

# Create non-root user
RUN addgroup --gid 1000 astro && \
    adduser --uid 1000 --gid 1000 --disabled-password --gecos "" astro

# Copy package files
COPY package.json bun.lockb* ./

# Install dependencies
RUN bun install

# Copy source code
COPY src ./src

# Set ownership and switch to non-root user
RUN chown -R astro:astro /app
USER astro

# Expose agent port
EXPOSE 8080

# Run the GitHub agent server
CMD ["bun", "run", "src/index.ts"]
