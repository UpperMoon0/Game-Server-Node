# Build stage
FROM golang:1.24-bookworm AS builder

# Install git for go mod operations
RUN apt-get update && apt-get install -y git && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy source code and go.mod
COPY . .

# Tidy dependencies to generate proper go.sum
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o node ./cmd/node

# Production stage
FROM debian:bookworm-slim

# Install ca-certificates, bash, curl, screen
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    bash \
    curl \
    screen \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/node .
COPY --from=builder /app/config.yaml .

# Create directories
RUN mkdir -p /app/servers /app/backups /app/logs

# Expose ports
EXPOSE 50051

# Environment variables
ENV CONFIG_PATH=/app/config.yaml

# Entry point
ENTRYPOINT ["./node"]
