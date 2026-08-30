# Makefile Documentation

This document describes all available Make targets for the Team Health Check project. The Makefile orchestrates both the frontend (Next.js/TypeScript) and backend (Go/Gin) services.

## Quick Reference

| Command | Description |
|---------|-------------|
| `make run` | Start the full application (recommended) |
| `make dev` | Start with hot reload for development |
| `make test` | Run all backend tests |
| `make test-e2e` | Run E2E acceptance tests |
| `make build` | Build for production |
| `make help` | Show all available commands |

## Getting Started

### First Time Setup

```bash
# Clone and run - that's it!
git clone https://github.com/anthropics/teams360.git
cd teams360
make run
```

The `make run` command automatically:
1. Installs frontend npm dependencies (if missing)
2. Installs backend Go dependencies (if missing)
3. Starts PostgreSQL via Docker (if available and not running)
4. Kills any existing servers on ports 3000/8080
5. Starts both frontend and backend servers concurrently
6. Displays demo credentials for login

### Environment Variables

The Makefile uses these environment variables (with defaults):

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable` | Production database URL |
| `TEST_DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable` | Test database URL |

Override them as needed:

```bash
DATABASE_URL="postgres://user:pass@host:5432/mydb" make run
```

---

## Target Categories

### Installation

#### `make install`
Install all dependencies for both frontend and backend.

```bash
make install
```

**What it does:**
- Runs `npm install` in the frontend directory
- Runs `go mod download` in the backend directory

#### `make install-frontend`
Install only frontend npm dependencies.

#### `make install-backend`
Install only backend Go dependencies.

---

### Running the Application

#### `make run` ⭐ (Recommended)
Start the full application locally with automatic setup.

```bash
make run
```

**Features:**
- Auto-installs dependencies if missing
- Auto-starts PostgreSQL via Docker if available
- Kills any existing servers on ports 3000/8080
- Starts frontend (port 3000) and backend (port 8080) concurrently
- Displays demo credentials

**Output:**
```
Starting Team Health Check...

  Frontend: http://localhost:3000
  Backend:  http://localhost:8080

Demo Credentials:
  demo/demo      - Team Member
  teamlead1/demo - Team Lead
  manager1/demo  - Manager
  admin/admin    - Administrator

Press Ctrl+C to stop
```

#### `make dev`
Start with hot reload for active development.

```bash
make dev
```

**Differences from `make run`:**
- Uses `air` for backend hot reload (if installed)
- Automatically recompiles Go code on file changes
- Ideal for rapid iteration

**Install air:**
```bash
go install github.com/air-verse/air@latest
```

#### `make run-frontend`
Start only the frontend server.

```bash
make run-frontend
```

#### `make run-backend`
Start only the backend server.

```bash
make run-backend
```

---

### Database Management

#### `make db-start`
Start the PostgreSQL Docker container.

```bash
make db-start
```

**Behavior:**
- If container exists and running: no-op
- If container exists but stopped: starts it
- If no container: creates new one with `postgres:16-alpine`

#### `make db-stop`
Stop the PostgreSQL Docker container.

```bash
make db-stop
```

#### `make db-setup`
Initialize the database with migrations and seed data.

```bash
make db-setup
```

**What it does:**
1. Ensures PostgreSQL is running
2. Starts the backend briefly to run migrations
3. Seed data (demo users, teams, dimensions) is created automatically

#### `make db-reset`
Reset the database (WARNING: deletes all data).

```bash
make db-reset
```

**Prompts for confirmation before proceeding.**

#### `make db-test-setup`
Setup the test database (`teams360_test`).

```bash
make db-test-setup
```

---

### Building

#### `make build`
Build both frontend and backend for production.

```bash
make build
```

**Outputs:**
- Frontend: `.next/` directory (Next.js production build)
- Backend: `backend/bin/team360-api` binary

#### `make build-frontend`
Build only the frontend (Next.js production build).

#### `make build-backend`
Build only the backend Go binary.

---

### Testing

#### `make test`
Run all backend tests.

```bash
make test
```

**Uses:** Ginkgo test framework
**Excludes:** E2E acceptance tests (for speed)

#### `make test-backend`
Same as `make test` - runs backend unit and integration tests.

#### `make test-backend-verbose`
Run backend tests with verbose output and race detection.

```bash
make test-backend-verbose
```

#### `make test-backend-coverage`
Run backend tests and generate HTML coverage report.

```bash
make test-backend-coverage
```

**Output:** `backend/coverage.html`

#### `make test-backend-watch`
Run tests in watch mode - automatically re-runs on file changes.

```bash
make test-backend-watch
```

**Ideal for TDD workflow.**

#### `make test-e2e` ⭐
Run full E2E acceptance tests with Playwright.

```bash
make test-e2e
```

**What it does:**
1. Ensures test database is ready
2. Kills any existing servers
3. Starts backend with test database
4. Starts frontend
5. Waits for both servers to be healthy
6. Runs Ginkgo E2E tests with Playwright
7. Cleans up servers after completion

**Duration:** ~2-5 minutes (includes server startup)

**Logs:** Available at `/tmp/team360/backend.log` and `/tmp/team360/frontend.log`

#### `make test-frontend`
Run frontend tests (placeholder - not yet configured).

---

### Linting & Formatting

#### `make lint`
Run all linters (backend + frontend).

```bash
make lint
```

#### `make lint-backend`
Run Go linters (`go fmt` check + `go vet`).

#### `make lint-frontend`
Run ESLint on frontend code.

#### `make fmt-backend`
Format Go code using `go fmt`.

```bash
make fmt-backend
```

---

### Cleanup

#### `make clean`
Clean build artifacts (keeps dependencies).

```bash
make clean
```

**Removes:**
- Frontend: `.next/`, `out/`, `node_modules/.cache`
- Backend: `bin/`, coverage files

#### `make clean-frontend`
Clean only frontend build artifacts.

#### `make clean-backend`
Clean only backend build artifacts.

#### `make clean-all`
Deep clean including dependencies and caches.

```bash
make clean-all
```

**Removes:**
- Everything from `make clean`
- `frontend/node_modules/`
- Go module cache
- Test cache

**Use when:** Troubleshooting dependency issues or freeing disk space.

---

### Docker

#### `make docker-build`
Build Docker images for the backend.

```bash
make docker-build
```

#### `make docker-run`
Build and run the backend in a Docker container.

```bash
make docker-run
```

---

### Status & Information

#### `make status` / `make info`
Show project status including dependency installation state and database status.

```bash
make status
```

**Output example:**
```
Team Health Check Project Status

Frontend (Next.js 15 + TypeScript):
  Location: ./frontend
  Dependencies: Installed

Backend (Go 1.25 + Gin + DDD):
  Location: ./backend
  Dependencies: Installed

Database:
  PostgreSQL: Running (Docker)
```

#### `make help`
Display all available commands with descriptions.

```bash
make help
```

---

### CI Pipeline

#### `make ci` / `make all`
Run full CI pipeline: clean → install → lint → test → build.

```bash
make ci
```

**Use for:** Pre-commit verification, CI/CD pipelines.

---

## Architecture Notes

### Monorepo Structure

```
teams360/
├── Makefile              # Root orchestration (this file)
├── frontend/             # Next.js application
│   └── package.json      # npm commands
├── backend/              # Go API server
│   ├── Makefile          # Backend-specific targets
│   └── go.mod            # Go dependencies
└── tests/                # E2E acceptance tests
    └── acceptance/       # Playwright + Ginkgo tests
```

### Internal Targets

Targets prefixed with `_` are internal and not shown in `make help`:

| Target | Purpose |
|--------|---------|
| `_ensure-deps` | Install deps if missing |
| `_ensure-db` | Start database if needed |
| `_kill-servers` | Kill existing processes on 3000/8080 |
| `_print-banner` | Display startup info |
| `_start-frontend` | Start frontend server |
| `_start-backend` | Start backend server |
| `_ensure-pid-dir` | Create PID file directory |

### PID Management

For E2E tests, process IDs are stored in `/tmp/team360/`:
- `backend.pid` - Backend server PID
- `frontend.pid` - Frontend server PID
- `backend.log` - Backend server logs
- `frontend.log` - Frontend server logs

---

## Troubleshooting

### Port Already in Use

```bash
# Kill processes manually
lsof -ti:3000 | xargs kill -9  # Frontend port
lsof -ti:8080 | xargs kill -9  # Backend port

# Or let make handle it
make run  # Automatically kills existing servers
```

### Docker Not Available

If Docker isn't installed or running:

```bash
# Install PostgreSQL manually and ensure it's running on port 5432
# Then run without Docker:
make run
```

The Makefile will detect if port 5432 is in use and skip Docker setup.

### Dependency Issues

```bash
# Deep clean and reinstall
make clean-all
make install
```

### E2E Tests Failing

1. Check server logs:
   ```bash
   cat /tmp/team360/backend.log
   cat /tmp/team360/frontend.log
   ```

2. Ensure test database exists:
   ```bash
   make db-test-setup
   ```

3. Run with verbose output:
   ```bash
   TEST_DATABASE_URL="..." ginkgo -v tests/acceptance/
   ```

---

## Best Practices

### For Development

```bash
# Daily workflow
make run          # Start the app
# Make changes...
# Ctrl+C to stop
make test         # Run tests before committing
```

### For TDD

```bash
# In one terminal:
make test-backend-watch

# In another terminal:
make run-backend
```

### Before Committing

```bash
make lint         # Check code style
make test         # Run unit tests
make build        # Ensure it builds
```

### For CI/CD

```bash
make ci           # Full pipeline
# Or individually:
make install && make lint && make test && make build
```
