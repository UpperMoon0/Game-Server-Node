# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

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
