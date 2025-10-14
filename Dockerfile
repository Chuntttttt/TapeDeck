# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Generate templ files
RUN templ generate

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tapedeck .

# Ensure static directory exists
RUN mkdir -p static

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/tapedeck .

# Copy static assets (will be empty directory for now)
COPY --from=builder /app/static ./static

# Copy migrations directory
COPY --from=builder /app/migrations ./migrations

# Create data directory
RUN mkdir -p /data

# Expose port
EXPOSE 3001

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3001/health || exit 1

# Run as non-root user
RUN addgroup -g 1000 tapedeck && \
    adduser -D -u 1000 -G tapedeck tapedeck && \
    chown -R tapedeck:tapedeck /app /data

USER tapedeck

CMD ["./tapedeck"]
