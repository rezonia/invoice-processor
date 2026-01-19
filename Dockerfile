# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /app/invoice-processor \
    ./cmd/invoice-processor

# Final stage - minimal image
FROM alpine:3.21

# Install runtime dependencies
# - ca-certificates: for HTTPS/TLS connections
# - tzdata: for timezone support
# - poppler-utils: provides pdfsig for PDF signature verification
RUN apk add --no-cache ca-certificates tzdata poppler-utils

# Create non-root user for security
RUN adduser -D -u 1000 appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/invoice-processor /app/invoice-processor

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose default HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command - run the server
ENTRYPOINT ["/app/invoice-processor"]
CMD ["serve", "--address", ":8080"]
