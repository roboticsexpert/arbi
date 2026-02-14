# Multi-stage build for smaller final image
FROM golang:1.24-alpine AS builder

ARG PROXY_URI
# Install git and ca-certificates (needed for fetching dependencies and HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN http_proxy=$PROXY_URI go mod download

# Copy all source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o arbi .

# Final stage - minimal image
FROM alpine:latest

# Install ca-certificates for HTTPS and tzdata for timezone
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /home/appuser

# Copy binary from builder
COPY --from=builder /app/arbi .
COPY .env.production .env

# Change ownership
RUN chown -R appuser:appuser /home/appuser

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Set environment variables (can be overridden at runtime)
ENV PROXY_URL=""

# Run the application
CMD ["./arbi"]
