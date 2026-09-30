# Root Makefile for Team Health Check Monorepo
# Orchestrates both Frontend (Next.js/TypeScript) and Backend (Go/Gin)
#
# Quick Start: make run
# Documentation: docs/development/makefile.md

# =============================================================================
# Configuration
# =============================================================================

.DEFAULT_GOAL := help

# Colors for terminal output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m
BOLD := \033[1m

# Database configuration
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable

# PID files for server management
PID_DIR := /tmp/team360
BACKEND_PID := $(PID_DIR)/backend.pid
FRONTEND_PID := $(PID_DIR)/frontend.pid

# =============================================================================
# Phony Targets Declaration
# =============================================================================

.PHONY: help
.PHONY: install install-frontend install-backend
.PHONY: run run-frontend run-backend dev dev-backend
.PHONY: build build-frontend build-backend
.PHONY: test test-unit test-frontend test-frontend-unit test-frontend-watch test-frontend-coverage test-backend test-backend-unit test-backend-verbose test-backend-coverage test-backend-watch test-e2e
.PHONY: lint lint-frontend lint-backend fmt-backend
.PHONY: clean clean-frontend clean-backend clean-all
.PHONY: db-start db-stop db-setup db-reset db-test-setup
.PHONY: docker-build docker-run
.PHONY: docs-build
.PHONY: status info
.PHONY: all ci
.PHONY: otel-start otel-stop otel-status otel-logs run-with-otel
.PHONY: _ensure-deps _ensure-db _kill-servers _print-banner _start-frontend _start-backend _ensure-pid-dir _ensure-otel _start-frontend-otel _start-backend-otel

# =============================================================================
# Help
# =============================================================================

help: ## Show this help message
	@echo "$(BOLD)Team Health Check Application$(RESET)"
	@echo "Full-stack application with Go backend and Next.js frontend"
	@echo ""
	@echo "$(BOLD)$(CYAN)Quick Start:$(RESET) make run"
	@echo ""
	@echo "$(CYAN)Usage:$(RESET) make [target]"
	@echo ""
	@echo "$(CYAN)Main Targets:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | grep -v "\[Frontend\]\|\[Backend\]" | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-22s$(RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(CYAN)Frontend Targets:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?##.*\[Frontend\]' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-22s$(RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(CYAN)Backend Targets:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?##.*\[Backend\]' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-22s$(RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(CYAN)Documentation:$(RESET) See docs/development/makefile.md for detailed documentation"

# =============================================================================
# Installation
# =============================================================================

install: install-frontend install-backend ## Install all dependencies

install-frontend: ## [Frontend] Install npm dependencies
	@echo "$(CYAN)Installing frontend dependencies...$(RESET)"
	@cd frontend && npm install
	@echo "$(GREEN)Frontend dependencies installed!$(RESET)"

install-backend: ## [Backend] Install Go dependencies
	@echo "$(CYAN)Installing backend dependencies...$(RESET)"
	@cd backend && go mod download
	@echo "$(GREEN)Backend dependencies installed!$(RESET)"

# =============================================================================
# Running the Application
# =============================================================================

run: _ensure-deps _ensure-db _kill-servers docs-build _print-banner ## Run the full application locally
	@$(MAKE) -j2 _start-frontend _start-backend

run-frontend: _ensure-deps ## [Frontend] Run only the frontend
	@echo "$(CYAN)Starting frontend on http://localhost:3000...$(RESET)"
	@cd frontend && npm run dev

run-backend: _ensure-deps _ensure-db ## [Backend] Run only the backend
	@echo "$(CYAN)Starting backend on http://localhost:8080...$(RESET)"
	@cd backend && DATABASE_URL="$(DATABASE_URL)" go run cmd/api/main.go

dev: _ensure-deps _ensure-db _kill-servers docs-build _print-banner ## Run with hot reload (requires 'air' for backend)
	@echo "$(BOLD)$(CYAN)Development mode with hot reload$(RESET)"
	@echo "$(YELLOW)Tip:$(RESET) Install 'air' for backend hot reload: go install github.com/air-verse/air@latest"
	@echo ""
	@$(MAKE) -j2 _start-frontend dev-backend

docs-build: ## [Docs] Build MkDocs site into frontend/public/docs (served at /docs)
	@if command -v mkdocs >/dev/null 2>&1; then \
		echo "$(CYAN)Building documentation site...$(RESET)"; \
		mkdocs build -d frontend/public/docs -q; \
	elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then \
		echo "$(CYAN)Building documentation site (via Docker)...$(RESET)"; \
		docker run --rm -v "$(CURDIR)":/docs squidfunk/mkdocs-material:latest build -d /docs/frontend/public/docs -q || \
			echo "$(YELLOW)Documentation build failed; continuing without generated docs.$(RESET)"; \
	elif command -v docker >/dev/null 2>&1; then \
		echo "$(YELLOW)Docker is installed but the daemon isn't running; skipping docs build.$(RESET)"; \
	else \
		echo "$(YELLOW)Neither mkdocs nor docker found, skipping docs build.$(RESET)"; \
	fi

dev-backend: ## [Backend] Run with hot reload using 'air'
	@cd backend && if command -v air >/dev/null 2>&1; then \
		DATABASE_URL="$(DATABASE_URL)" air; \
	else \
		echo "$(YELLOW)air not installed, using go run...$(RESET)"; \
		DATABASE_URL="$(DATABASE_URL)" go run cmd/api/main.go; \
	fi

# Internal targets for running servers
_start-frontend:
	@cd frontend && npm run dev

_start-backend:
	@cd backend && DATABASE_URL="$(DATABASE_URL)" go run cmd/api/main.go

_print-banner:
	@echo ""
	@echo "$(BOLD)$(CYAN)Starting Team Health Check...$(RESET)"
	@echo ""
	@echo "  $(CYAN)Frontend:$(RESET)      http://localhost:3000"
	@echo "  $(CYAN)Backend:$(RESET)       http://localhost:8080"
	@echo "  $(CYAN)Documentation:$(RESET) http://localhost:3000/docs/"
	@echo ""
	@echo "$(BOLD)Demo Credentials:$(RESET)"
	@echo "  demo/demo      - Team Member"
	@echo "  teamlead1/demo - Team Lead"
	@echo "  manager1/demo  - Manager"
	@echo "  admin/admin    - Administrator"
	@echo ""
	@echo "$(YELLOW)Press Ctrl+C to stop$(RESET)"
	@echo ""

_kill-servers:
	@echo "$(CYAN)Stopping any running servers...$(RESET)"
	@lsof -ti:3000 | xargs kill -9 2>/dev/null || true
	@lsof -ti:8080 | xargs kill -9 2>/dev/null || true
	@sleep 1

_ensure-deps:
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "$(CYAN)Installing frontend dependencies...$(RESET)"; \
		cd frontend && npm install; \
	fi
	@if [ ! -f "backend/go.sum" ] || [ ! -s "backend/go.sum" ]; then \
		echo "$(CYAN)Installing backend dependencies...$(RESET)"; \
		cd backend && go mod download; \
	fi

_ensure-db:
	@echo "$(CYAN)Checking database...$(RESET)"
	@if command -v docker >/dev/null 2>&1; then \
		if docker ps 2>/dev/null | grep -qE "teams360-(db|test)"; then \
			echo "$(GREEN)PostgreSQL container already running.$(RESET)"; \
		elif docker ps -a 2>/dev/null | grep -qE "teams360-(db|test)"; then \
			CONTAINER=$$(docker ps -a --format '{{.Names}}' 2>/dev/null | grep -E "teams360-(db|test)" | head -1); \
			echo "$(CYAN)Starting existing PostgreSQL container ($$CONTAINER)...$(RESET)"; \
			docker start $$CONTAINER; \
			sleep 3; \
		elif ! lsof -i:5432 >/dev/null 2>&1; then \
			echo "$(CYAN)Creating PostgreSQL container...$(RESET)"; \
			docker run -d --name teams360-db -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16-alpine; \
			sleep 3; \
		else \
			echo "$(GREEN)Port 5432 already in use (existing PostgreSQL).$(RESET)"; \
		fi; \
	else \
		if lsof -i:5432 >/dev/null 2>&1; then \
			echo "$(GREEN)PostgreSQL running on port 5432.$(RESET)"; \
		else \
			echo "$(RED)Warning: Docker not available and PostgreSQL not running on port 5432.$(RESET)"; \
			echo "$(YELLOW)Please start PostgreSQL manually or install Docker.$(RESET)"; \
		fi; \
	fi
	@echo "$(GREEN)Database ready. Migrations run automatically on backend startup.$(RESET)"

_ensure-pid-dir:
	@mkdir -p $(PID_DIR)

# =============================================================================
# Database Management
# =============================================================================

db-start: ## Start the PostgreSQL Docker container
	@echo "$(CYAN)Starting PostgreSQL...$(RESET)"
	@if docker ps 2>/dev/null | grep -q teams360-db; then \
		echo "$(GREEN)PostgreSQL already running.$(RESET)"; \
	elif docker ps -a 2>/dev/null | grep -q teams360-db; then \
		docker start teams360-db; \
		echo "$(GREEN)PostgreSQL started.$(RESET)"; \
	else \
		docker run -d --name teams360-db -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16-alpine; \
		echo "$(GREEN)PostgreSQL container created and started.$(RESET)"; \
	fi

db-stop: ## Stop the PostgreSQL Docker container
	@echo "$(CYAN)Stopping PostgreSQL...$(RESET)"
	@docker stop teams360-db 2>/dev/null || echo "$(YELLOW)Container not running.$(RESET)"

db-setup: db-start ## Initialize the database with migrations and seed data
	@echo "$(CYAN)Setting up database...$(RESET)"
	@sleep 2
	@cd backend && DATABASE_URL="$(DATABASE_URL)" go run cmd/api/main.go migrate 2>/dev/null || \
		(DATABASE_URL="$(DATABASE_URL)" timeout 5 go run cmd/api/main.go & sleep 3 && pkill -f "go run cmd/api/main.go" 2>/dev/null || true)
	@echo "$(GREEN)Database setup complete!$(RESET)"

db-reset: ## Reset database (WARNING: deletes all data)
	@echo "$(RED)Resetting database - this will delete all data!$(RESET)"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	@docker exec teams360-db psql -U postgres -c "DROP DATABASE IF EXISTS teams360;" 2>/dev/null || true
	@docker exec teams360-db psql -U postgres -c "CREATE DATABASE teams360;" 2>/dev/null || true
	@$(MAKE) db-setup

db-test-setup: db-start ## Setup test database
	@echo "$(CYAN)Setting up test database...$(RESET)"
	@docker exec teams360-db psql -U postgres -c "DROP DATABASE IF EXISTS teams360_test;" 2>/dev/null || true
	@docker exec teams360-db psql -U postgres -c "CREATE DATABASE teams360_test;" 2>/dev/null || true
	@cd backend && DATABASE_URL="$(TEST_DATABASE_URL)" timeout 5 go run cmd/api/main.go 2>/dev/null & sleep 3 && pkill -f "go run cmd/api/main.go" 2>/dev/null || true
	@echo "$(GREEN)Test database setup complete!$(RESET)"

# =============================================================================
# Build
# =============================================================================

build: build-frontend build-backend ## Build both frontend and backend for production

build-frontend: docs-build ## [Frontend] Build Next.js for production
	@echo "$(CYAN)Building frontend...$(RESET)"
	@cd frontend && npm run build
	@echo "$(GREEN)Frontend build complete!$(RESET)"

build-backend: ## [Backend] Build Go API binary
	@echo "$(CYAN)Building backend...$(RESET)"
	@cd backend && go build -o bin/team360-api ./cmd/api/main.go
	@echo "$(GREEN)Backend build complete: backend/bin/team360-api$(RESET)"

# =============================================================================
# Testing
# =============================================================================

test: test-frontend test-backend test-e2e ## Run ALL tests (frontend + backend + E2E)

test-unit: test-frontend-unit test-backend-unit ## Run only unit/integration tests (no E2E)

test-frontend: test-frontend-unit ## [Frontend] Alias for test-frontend-unit

test-frontend-unit: ## [Frontend] Run frontend unit tests (Vitest)
	@echo "$(CYAN)Running frontend tests...$(RESET)"
	@cd frontend && npm test
	@echo "$(GREEN)Frontend tests complete!$(RESET)"

test-frontend-watch: ## [Frontend] Run frontend tests in watch mode
	@cd frontend && npm run test:watch

test-frontend-coverage: ## [Frontend] Run frontend tests with coverage
	@cd frontend && npm run test:coverage

test-backend: test-backend-unit ## [Backend] Alias for test-backend-unit

test-backend-unit: ## [Backend] Run backend unit and integration tests
	@echo "$(CYAN)Running backend tests...$(RESET)"
	@cd backend && ginkgo -r --skip-package=tests/acceptance ./...

test-backend-verbose: ## [Backend] Run backend tests with verbose output
	@cd backend && ginkgo -v --race -r --skip-package=tests/acceptance ./...

test-backend-coverage: ## [Backend] Run backend tests with coverage report
	@echo "$(CYAN)Running backend tests with coverage...$(RESET)"
	@cd backend && ginkgo -r --cover --coverprofile=coverage.out --skip-package=tests/acceptance ./...
	@cd backend && go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report: backend/coverage.html$(RESET)"

test-backend-watch: ## [Backend] Run backend tests in watch mode
	@cd backend && ginkgo watch -r --skip-package=tests/acceptance ./...

test-e2e: _ensure-db _ensure-pid-dir ## Run E2E acceptance tests (starts servers automatically)
	@echo "$(BOLD)$(CYAN)Running E2E Tests$(RESET)"
	@echo ""
	@# Kill any existing servers
	@pkill -f "go run cmd/api/main.go" 2>/dev/null || true
	@pkill -f "next dev" 2>/dev/null || true
	@sleep 2
	@# Start backend
	@echo "$(CYAN)Starting backend server...$(RESET)"
	@cd backend && DATABASE_URL="$(TEST_DATABASE_URL)" go run cmd/api/main.go > $(PID_DIR)/backend.log 2>&1 & echo $$! > $(BACKEND_PID)
	@sleep 3
	@# Start frontend
	@echo "$(CYAN)Starting frontend server...$(RESET)"
	@cd frontend && npm run dev > $(PID_DIR)/frontend.log 2>&1 & echo $$! > $(FRONTEND_PID)
	@sleep 5
	@# Wait for servers to be healthy
	@echo "$(CYAN)Waiting for servers...$(RESET)"
	@for i in $$(seq 1 15); do \
		if curl -s http://localhost:8080/health >/dev/null 2>&1; then \
			echo "$(GREEN)Backend ready!$(RESET)"; \
			break; \
		fi; \
		if [ $$i -eq 15 ]; then \
			echo "$(RED)Backend failed to start. Check $(PID_DIR)/backend.log$(RESET)"; \
			cat $(PID_DIR)/backend.log | tail -20; \
			kill $$(cat $(BACKEND_PID)) $$(cat $(FRONTEND_PID)) 2>/dev/null || true; \
			exit 1; \
		fi; \
		sleep 1; \
	done
	@for i in $$(seq 1 15); do \
		if curl -s http://localhost:3000 >/dev/null 2>&1; then \
			echo "$(GREEN)Frontend ready!$(RESET)"; \
			break; \
		fi; \
		if [ $$i -eq 15 ]; then \
			echo "$(RED)Frontend failed to start. Check $(PID_DIR)/frontend.log$(RESET)"; \
			cat $(PID_DIR)/frontend.log | tail -20; \
			kill $$(cat $(BACKEND_PID)) $$(cat $(FRONTEND_PID)) 2>/dev/null || true; \
			exit 1; \
		fi; \
		sleep 1; \
	done
	@# Run tests
	@echo ""
	@echo "$(CYAN)Running E2E tests...$(RESET)"
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" ginkgo -v tests/acceptance/ || \
		(kill $$(cat $(BACKEND_PID)) $$(cat $(FRONTEND_PID)) 2>/dev/null || true; exit 1)
	@# Cleanup
	@echo ""
	@echo "$(CYAN)Cleaning up servers...$(RESET)"
	@kill $$(cat $(BACKEND_PID)) $$(cat $(FRONTEND_PID)) 2>/dev/null || true
	@rm -f $(BACKEND_PID) $(FRONTEND_PID)
	@echo "$(BOLD)$(GREEN)E2E tests complete!$(RESET)"

# =============================================================================
# Linting & Formatting
# =============================================================================

lint: lint-backend lint-frontend ## Run all linters

lint-frontend: ## [Frontend] Run ESLint
	@echo "$(CYAN)Linting frontend...$(RESET)"
	@cd frontend && npm run lint 2>/dev/null || echo "$(YELLOW)ESLint not configured or no issues found.$(RESET)"

lint-backend: ## [Backend] Run Go linters (fmt check + vet)
	@echo "$(CYAN)Linting backend...$(RESET)"
	@cd backend && go fmt ./... && go vet ./...
	@echo "$(GREEN)Backend linting complete!$(RESET)"

fmt-backend: ## [Backend] Format Go code
	@echo "$(CYAN)Formatting backend code...$(RESET)"
	@cd backend && go fmt ./...
	@echo "$(GREEN)Backend code formatted!$(RESET)"

# =============================================================================
# Cleanup
# =============================================================================

clean: clean-frontend clean-backend ## Clean all build artifacts

clean-frontend: ## [Frontend] Clean Next.js build artifacts
	@echo "$(CYAN)Cleaning frontend...$(RESET)"
	@cd frontend && rm -rf .next out node_modules/.cache
	@echo "$(GREEN)Frontend cleaned!$(RESET)"

clean-backend: ## [Backend] Clean Go build artifacts
	@echo "$(CYAN)Cleaning backend...$(RESET)"
	@cd backend && go clean && rm -rf bin/ coverage.out coverage.html
	@echo "$(GREEN)Backend cleaned!$(RESET)"

clean-all: clean ## Deep clean including node_modules and Go cache
	@echo "$(CYAN)Deep cleaning...$(RESET)"
	@cd frontend && rm -rf node_modules
	@cd backend && go clean -cache -testcache
	@rm -rf $(PID_DIR)
	@echo "$(GREEN)Deep clean complete!$(RESET)"

# =============================================================================
# Docker
# =============================================================================

docker-build: ## Build Docker images
	@echo "$(CYAN)Building Docker images...$(RESET)"
	@cd backend && docker build -t team360-api:latest .
	@echo "$(YELLOW)TODO: Add frontend Docker build$(RESET)"

docker-run: docker-build ## Run in Docker containers
	@echo "$(CYAN)Running in Docker...$(RESET)"
	@docker run -p 8080:8080 --env DATABASE_URL="$(DATABASE_URL)" team360-api:latest

# =============================================================================
# Status & Info
# =============================================================================

status: ## Show project status
	@echo "$(BOLD)$(CYAN)Team Health Check Project Status$(RESET)"
	@echo ""
	@echo "$(CYAN)Frontend (Next.js 15 + TypeScript):$(RESET)"
	@echo "  Location: ./frontend"
	@if [ -d "frontend/node_modules" ]; then \
		echo "  Dependencies: $(GREEN)Installed$(RESET)"; \
	else \
		echo "  Dependencies: $(YELLOW)Not installed (run: make install)$(RESET)"; \
	fi
	@echo ""
	@echo "$(CYAN)Backend (Go 1.25 + Gin + DDD):$(RESET)"
	@echo "  Location: ./backend"
	@if [ -f "backend/go.sum" ]; then \
		echo "  Dependencies: $(GREEN)Installed$(RESET)"; \
	else \
		echo "  Dependencies: $(YELLOW)Not installed (run: make install)$(RESET)"; \
	fi
	@echo ""
	@echo "$(CYAN)Database:$(RESET)"
	@if docker ps 2>/dev/null | grep -q teams360; then \
		echo "  PostgreSQL: $(GREEN)Running (Docker)$(RESET)"; \
	elif lsof -i:5432 >/dev/null 2>&1; then \
		echo "  PostgreSQL: $(GREEN)Running (port 5432)$(RESET)"; \
	else \
		echo "  PostgreSQL: $(RED)Not running$(RESET)"; \
	fi
	@echo ""
	@echo "Run '$(CYAN)make help$(RESET)' for available commands"

info: status ## Alias for status

# =============================================================================
# CI Pipeline
# =============================================================================

ci: clean install lint test build ## Full CI pipeline
	@echo "$(BOLD)$(GREEN)CI pipeline completed successfully!$(RESET)"

all: ci ## Alias for ci

# =============================================================================
# Observability Stack (Jaeger, Prometheus, Grafana, OTel Collector)
# =============================================================================

otel-start: ## Start observability stack (Jaeger, Prometheus, Grafana)
	@echo "$(CYAN)Starting observability stack...$(RESET)"
	@docker compose -f backend/deploy/docker-compose.observability.yaml up -d
	@echo ""
	@echo "$(GREEN)Observability stack started!$(RESET)"
	@echo ""
	@echo "$(BOLD)Access URLs:$(RESET)"
	@echo "  $(CYAN)Jaeger UI:$(RESET)    http://localhost:16686  (traces)"
	@echo "  $(CYAN)Prometheus:$(RESET)   http://localhost:9090   (metrics)"
	@echo "  $(CYAN)Grafana:$(RESET)      http://localhost:3001   (dashboards, admin/admin)"
	@echo ""
	@echo "Run '$(CYAN)make run-with-otel$(RESET)' to start the full app with telemetry enabled"

otel-stop: ## Stop observability stack
	@echo "$(CYAN)Stopping observability stack...$(RESET)"
	@docker compose -f backend/deploy/docker-compose.observability.yaml down
	@echo "$(GREEN)Observability stack stopped$(RESET)"

otel-status: ## Check observability stack status
	@docker compose -f backend/deploy/docker-compose.observability.yaml ps

otel-logs: ## View OTel collector logs
	@docker compose -f backend/deploy/docker-compose.observability.yaml logs -f otel-collector

run-with-otel: _ensure-deps _ensure-db _kill-servers _ensure-otel ## Run full app with telemetry enabled
	@echo ""
	@echo "$(BOLD)$(CYAN)Starting Team Health Check with OpenTelemetry...$(RESET)"
	@echo ""
	@echo "  $(CYAN)Frontend:$(RESET)   http://localhost:3000"
	@echo "  $(CYAN)Backend:$(RESET)    http://localhost:8080"
	@echo "  $(CYAN)Jaeger:$(RESET)     http://localhost:16686  (traces)"
	@echo "  $(CYAN)Prometheus:$(RESET) http://localhost:9090   (metrics)"
	@echo "  $(CYAN)Grafana:$(RESET)    http://localhost:3001   (dashboards)"
	@echo ""
	@echo "$(YELLOW)Press Ctrl+C to stop$(RESET)"
	@echo ""
	@$(MAKE) -j2 _start-frontend-otel _start-backend-otel

_ensure-otel:
	@echo "$(CYAN)Checking observability stack...$(RESET)"
	@if ! docker compose -f backend/deploy/docker-compose.observability.yaml ps --status running 2>/dev/null | grep -q "otel-collector"; then \
		echo "$(CYAN)Starting observability stack (Jaeger, Prometheus, Grafana, OTel Collector)...$(RESET)"; \
		docker compose -f backend/deploy/docker-compose.observability.yaml up -d; \
		echo "$(CYAN)Waiting for OTel Collector to be ready...$(RESET)"; \
		for i in $$(seq 1 30); do \
			if curl -s http://localhost:13133/health >/dev/null 2>&1; then \
				echo "$(GREEN)OTel Collector ready!$(RESET)"; \
				break; \
			fi; \
			if [ $$i -eq 30 ]; then \
				echo "$(RED)OTel Collector failed to start. Check with 'make otel-logs'$(RESET)"; \
				exit 1; \
			fi; \
			sleep 1; \
		done; \
		echo "$(CYAN)Waiting for Jaeger to be ready...$(RESET)"; \
		for i in $$(seq 1 30); do \
			if curl -s http://localhost:16686 >/dev/null 2>&1; then \
				echo "$(GREEN)Jaeger ready!$(RESET)"; \
				break; \
			fi; \
			if [ $$i -eq 30 ]; then \
				echo "$(YELLOW)Jaeger may not be fully ready, continuing...$(RESET)"; \
			fi; \
			sleep 1; \
		done; \
	else \
		echo "$(GREEN)Observability stack already running.$(RESET)"; \
	fi

_start-frontend-otel:
	@cd frontend && NEXT_PUBLIC_OTEL_ENABLED=true npm run dev

_start-backend-otel:
	@cd backend && OTEL_ENABLED=true DATABASE_URL="$(DATABASE_URL)" go run cmd/api/main.go

# =============================================================================
# KubeVela + CNPG Deployment (optional include)
# =============================================================================

-include Makefile.kubevela
