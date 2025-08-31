# Makefile for KongTask

.PHONY: help build test test-integration test-perf clean lint fmt vet mod-tidy

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build the application
	@echo "Building KongTask..."
	@go build -v ./...

build-cmd: ## Build command-line tools
	@echo "Building command-line tools..."
	@go build -o bin/kongtask ./cmd/api
	@go build -o bin/kongtask-migrate ./cmd/migrate

# Test targets
test: ## Run unit tests
	@echo "Running unit tests..."
	@go test -race -coverprofile=coverage.out -covermode=atomic ./...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v ./test/integration/...

test-perf: ## Run performance tests
	@echo "Running performance tests..."
	@go test -v ./perftest/...

test-all: test test-integration test-perf ## Run all tests

# Coverage
coverage: test ## Generate test coverage report
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Code quality
lint: ## Run linters
	@echo "Running golangci-lint..."
	@golangci-lint run

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@gofumpt -l -w .

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

# Security
security: ## Run security checks
	@echo "Running security checks..."
	@gosec ./...

# Dependencies
mod-download: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download

mod-tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	@go mod tidy

mod-verify: ## Verify dependencies
	@echo "Verifying dependencies..."
	@go mod verify

# Development
install-tools: ## Install development tools
	@echo "Installing development tools..."
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go install golang.org/x/lint/golint@latest
	@go install mvdan.cc/gofumpt@latest
	@go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin

# Clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@go clean ./...

# Database
db-setup: ## Set up test database (requires Docker)
	@echo "Setting up test database..."
	@docker run --name kongtask-postgres-test -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=kongtask_test -p 5432:5432 -d postgres:16-alpine

db-teardown: ## Tear down test database
	@echo "Tearing down test database..."
	@docker stop kongtask-postgres-test || true
	@docker rm kongtask-postgres-test || true

# CI simulation
ci-test: lint vet test test-integration ## Run CI tests locally

# Docker
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t kongtask:latest .

docker-test: ## Run tests in Docker
	@echo "Running tests in Docker..."
	@docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
