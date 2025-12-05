# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git nodejs npm

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy package.json and install npm dependencies for Tailwind
COPY package.json tailwind.config.js ./
RUN npm install

# Copy source code
COPY . .

# Build Tailwind CSS
RUN npx tailwindcss -i ./static/css/input.css -o ./static/css/output.css --minify

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o caddyshack ./cmd/caddyshack

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' caddyshack

# Copy binary from builder
COPY --from=builder /app/caddyshack .

# Copy static files and templates (will be embedded in binary in future)
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Set ownership
RUN chown -R caddyshack:caddyshack /app

USER caddyshack

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["./caddyshack"]
