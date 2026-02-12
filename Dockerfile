# Build stage
FROM golang:1.21-alpine AS builder

# Install git for go mod operations
RUN apk add --no-cache git

WORKDIR /app

# Copy source code and go.mod
COPY . .

# Tidy dependencies to generate proper go.sum
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o node ./cmd/node

# Production stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates bash curl

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
