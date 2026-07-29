# Backend Makefile for Team Health Check Go API
# Follows DDD architecture and TDD practices with Ginkgo/Gomega

.PHONY: help install run build test test-verbose test-coverage clean lint fmt vet tidy
.PHONY: otel-start otel-stop otel-status otel-logs run-with-otel

# Default target
.DEFAULT_GOAL := help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet
BINARY_NAME=team360-api
BINARY_PATH=bin/$(BINARY_NAME)
MAIN_PATH=./cmd/api

# Ginkgo parameters
GINKGO=ginkgo
GINKGO_FLAGS=-v --race --randomize-all --randomize-suites --fail-on-pending --trace

help: ## Display this help message
	@echo "Team Health Check Backend - Go API with Gin, DDD, and TDD"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

install: ## Install dependencies and tools
	@echo "Installing Go dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Installing Ginkgo CLI..."
	$(GOCMD) install github.com/onsi/ginkgo/v2/ginkgo@latest
	@echo "Installing development tools..."
	$(GOCMD) install github.com/air-verse/air@latest
	@echo "Dependencies installed successfully!"

tidy: ## Tidy go.mod and go.sum
	@echo "Tidying Go modules..."
	$(GOMOD) tidy

run: ## Run the API server (development mode)
	@echo "Starting Team Health Check API server..."
	$(GOCMD) run $(MAIN_PATH)/main.go

dev: ## Run the API server with hot reload (requires air)
	@echo "Starting Team Health Check API server with hot reload..."
	air

build: ## Build the API binary
	@echo "Building Team Health Check API..."
	@mkdir -p bin
	$(GOBUILD) -o $(BINARY_PATH) $(MAIN_PATH)/main.go
	@echo "Build complete: $(BINARY_PATH)"

test: ## Run all tests with Ginkgo
	@echo "Running tests with Ginkgo..."
	$(GINKGO) -r ./...

test-verbose: ## Run tests with verbose output
	@echo "Running tests with verbose output..."
	$(GINKGO) $(GINKGO_FLAGS) -r ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GINKGO) -r --cover --coverprofile=coverage.out ./...
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-acceptance: ## Run acceptance tests only
	@echo "Running acceptance tests..."
	$(GINKGO) $(GINKGO_FLAGS) ./tests/acceptance/...

test-watch: ## Run tests in watch mode
	@echo "Running tests in watch mode..."
	$(GINKGO) watch -r ./...

fmt: ## Format Go code
	@echo "Formatting Go code..."
	$(GOFMT) ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

lint: fmt vet ## Run linters (fmt + vet)
	@echo "Linting complete!"

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	rm -rf bin/
	rm -rf coverage.out coverage.html
	@echo "Clean complete!"

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t team360-api:latest .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 8080:8080 team360-api:latest

# =============================================================================
# Observability Stack (Jaeger, Prometheus, Grafana, OTel Collector)
# =============================================================================

otel-start: ## Start observability stack (Jaeger, Prometheus, Grafana)
	@echo "Starting observability stack..."
	@docker compose -f deploy/docker-compose.observability.yaml up -d
	@echo ""
	@echo "Observability stack started! Access at:"
	@echo "  Jaeger UI:    http://localhost:16686"
	@echo "  Prometheus:   http://localhost:9090"
	@echo "  Grafana:      http://localhost:3001 (admin/admin)"
	@echo ""
	@echo "Run 'make run-with-otel' to start the API with telemetry enabled"

otel-stop: ## Stop observability stack
	@echo "Stopping observability stack..."
	@docker compose -f deploy/docker-compose.observability.yaml down
	@echo "Observability stack stopped"

otel-status: ## Check observability stack status
	@docker compose -f deploy/docker-compose.observability.yaml ps

otel-logs: ## View OTel collector logs
	@docker compose -f deploy/docker-compose.observability.yaml logs -f otel-collector

run-with-otel: ## Run API server with telemetry enabled
	@echo "Starting Team Health Check API with OpenTelemetry enabled..."
	@echo "Make sure observability stack is running (make otel-start)"
	@echo ""
	OTEL_ENABLED=true $(GOCMD) run $(MAIN_PATH)/main.go

.PHONY: all
all: clean install lint test build ## Run all tasks (clean, install, lint, test, build)
