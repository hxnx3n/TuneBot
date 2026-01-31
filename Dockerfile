# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o tunebot ./cmd/tunebot

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    ffmpeg \
    python3 \
    py3-pip \
    && pip3 install --no-cache-dir --break-system-packages yt-dlp \
    && rm -rf /var/cache/apk/*

# Create non-root user for security
RUN addgroup -g 1000 tunebot && \
    adduser -u 1000 -G tunebot -s /bin/sh -D tunebot

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/tunebot .

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Change ownership
RUN chown -R tunebot:tunebot /app

# Switch to non-root user
USER tunebot

# Set environment variables
ENV TZ=UTC

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep tunebot || exit 1

# Run the bot
ENTRYPOINT ["./tunebot"]
