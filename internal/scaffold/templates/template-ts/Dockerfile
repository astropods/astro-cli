# Build stage
FROM oven/bun:1 AS builder

WORKDIR /app

# The platform sets a prerelease channel in preview; `latest` leaves the
# dependency versions below as written.
ARG ASTRO_SDK_CHANNEL=latest

COPY package.json ./
RUN if [ "$ASTRO_SDK_CHANNEL" != "latest" ]; then \
      bun -e 'const fs=require("fs");const c=process.env.ASTRO_SDK_CHANNEL;const p=JSON.parse(fs.readFileSync("package.json","utf8"));p.overrides={...p.overrides,"@astropods/messaging":c,"@astropods/adapter-core":c};for(const d of Object.keys(p.dependencies||{}))if(d.startsWith("@astropods/")){p.dependencies[d]=c;p.overrides[d]=c}fs.writeFileSync("package.json",JSON.stringify(p,null,2)+"\n")'; \
    fi
RUN bun install

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
