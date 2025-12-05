.PHONY: build run test clean docker-build docker-up docker-down docker-logs

# Binary name
BINARY=caddyshack

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean

# Build the binary
build:
	$(GOBUILD) -o $(BINARY) ./cmd/caddyshack

# Run the application locally
run:
	$(GORUN) ./cmd/caddyshack

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -cover -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY)
	rm -f coverage.out coverage.html

# Build Docker image
docker-build:
	docker build -t $(BINARY) .

# Start development environment
docker-up:
	docker compose -f docker-compose.dev.yml up -d

# Stop development environment
docker-down:
	docker compose -f docker-compose.dev.yml down

# View logs from development environment
docker-logs:
	docker compose -f docker-compose.dev.yml logs -f

# Rebuild and restart development environment
docker-rebuild:
	docker compose -f docker-compose.dev.yml down
	docker compose -f docker-compose.dev.yml build
	docker compose -f docker-compose.dev.yml up -d

# Format code
fmt:
	$(GOCMD) fmt ./...

# Run linter
lint:
	golangci-lint run

# Install development dependencies
deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy
