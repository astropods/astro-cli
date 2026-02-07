FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o injection-worker ./cmd/worker

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates su-exec

WORKDIR /app

COPY --from=builder /build/injection-worker .

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Run as non-root user
RUN adduser -D -u 1000 astro && \
    mkdir -p /app/state && \
    chown -R astro:astro /app/state

USER astro

ENTRYPOINT ["/app/entrypoint.sh"]
