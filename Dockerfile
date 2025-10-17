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

# Build-time arguments for version information
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Build binary with version information injected
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
    -X github.com/Chuntttttt/tapedeck/internal/version.Version=${VERSION} \
    -X github.com/Chuntttttt/tapedeck/internal/version.GitCommit=${GIT_COMMIT} \
    -X github.com/Chuntttttt/tapedeck/internal/version.BuildDate=${BUILD_DATE}" \
    -o tapedeck .

# Ensure static directory exists
RUN mkdir -p static

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS and su-exec for user switching
RUN apk --no-cache add ca-certificates tzdata su-exec

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/tapedeck .

# Copy static assets (will be empty directory for now)
COPY --from=builder /app/static ./static

# Copy migrations directory
COPY --from=builder /app/migrations ./migrations

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Create data directory
RUN mkdir -p /data

# Expose port
EXPOSE 3001

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:3001/health || exit 1

# Environment variables for user/group ID
ENV PUID=1000 \
    PGID=1000

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./tapedeck"]
