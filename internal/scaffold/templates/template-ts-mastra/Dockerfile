# Build stage
FROM oven/bun:1 AS builder

WORKDIR /app

# No lockfile in a freshly scaffolded project, so resolve fresh here. Commit
# bun.lock and switch to `bun install --frozen-lockfile` for reproducible builds.
COPY package.json ./
RUN bun install

# Runtime stage
FROM oven/bun:1-slim

WORKDIR /app
ENV NODE_ENV=production
# Read by src/mastra/agents/agent.ts to drop the file:// output convention, which
# points at a container the user cannot browse. Deliberately not ASTRO_RUNTIME:
# adapter-core reserves that name and switches transport when it is "agentcore".
ENV ASTROPODS_CONTAINER=1

COPY --from=builder /app/node_modules ./node_modules
COPY package.json tsconfig.json ./
COPY src ./src

# Writable scratch space for the workspace filesystem and sandbox tools; the rest
# of the container filesystem is read-only in production.
RUN mkdir -p /tmp/workspace && chown -R bun:bun /app /tmp/workspace
USER bun

# Bun runs TypeScript directly, so there is no build step.
CMD ["bun", "run", "src/astro.ts"]
