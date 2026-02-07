# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o astro-messaging ./cmd/sidecar

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 astro && \
    adduser -u 1000 -G astro -s /bin/sh -D astro

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/astro-messaging .

# Set ownership and switch to non-root user
RUN chown -R astro:astro /app
USER astro

# Run the service
CMD ["./astro-messaging"]
