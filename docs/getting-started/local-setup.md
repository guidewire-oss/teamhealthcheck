# Local Setup Guide

This guide gets a fully working Team Health Check stack (frontend + backend + PostgreSQL) running on your laptop. Expected time: **under 10 minutes** with Docker available.

> For environment-specific access (dev / int / prod), see [Environment & Access](./environments.md).

---

## Prerequisites

### Required tools

| Tool | Minimum version | Install |
|------|----------------|---------|
| Git | Any recent | `brew install git` |
| Node.js | 22+ | `brew install node` or [nvm](https://github.com/nvm-sh/nvm) |
| Go | 1.25+ | `brew install go` |
| Docker | 24+ | [Docker Desktop](https://www.docker.com/products/docker-desktop/) |

Check versions:

```bash
node --version   # must be >= 22
go version       # must be >= 1.25
docker --version # must be >= 24
```

### Required access

- Read access to `github.com/guidewire-oss/teams360` (public repo — no approval needed)

### Optional tools

| Tool | Purpose |
|------|---------|
| `psql` (PostgreSQL CLI) | Inspect the database directly |
| `air` | Hot-reload for Go backend (`go install github.com/air-verse/air@latest`) |
| `ginkgo` CLI | Run backend tests directly (`go install github.com/onsi/ginkgo/v2/ginkgo@latest`) |

---

## 1. Clone the Repository

```bash
git clone https://github.com/guidewire-oss/teams360.git
cd teams360
```

---

## 2. Configure Environment Variables

Copy the provided example and fill in values:

```bash
cp .env.example .env
```

**Nothing loads this file automatically.** Neither the Makefile nor the Go backend (there is no `godotenv`/dotenv dependency) reads `.env` on its own — it's a reference template. To actually get these variables into `make run` / `make dev`, export them into your shell first:

```bash
set -a
source .env
set +a
make run
```

The Makefile itself only ever hardcodes and passes `DATABASE_URL` (and `TEST_DATABASE_URL`) explicitly to `go run`, each with a `?=` default you can override either by exporting it beforehand (as above) or passing it on the command line: `make run DATABASE_URL=...`. Every other backend variable (`GIN_MODE`, `PORT`, `OAUTH_*`, `AWS_SES_*`, `SMTP_*`) is read by the Go process via plain `os.Getenv`, so it must already be present in the shell's environment when `go run cmd/api/main.go` starts — sourcing `.env` as shown above is the simplest way to do that for local dev.

Note: the backend listens on the port in the `PORT` variable (default `8080`), not `API_PORT`. `API_PORT` in `.env.example` is a host-port convenience variable used by Docker Compose to both set the container's `PORT` and map the host port; it has no effect when running `go run cmd/api/main.go` directly.

Below is a realistic local configuration (all values are safe for local dev — never commit real secrets):

```env
# =============================================================================
# Team Health Check Environment Variables — LOCAL DEV
# =============================================================================

# Database connection (REQUIRED)
DATABASE_URL=postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable

# API server (the Go process reads PORT directly, not API_PORT)
PORT=8080
GIN_MODE=debug

# Frontend
FRONTEND_PORT=3000
NEXT_PUBLIC_API_URL=http://localhost:8080

# Email — leave blank to disable notifications locally
AWS_SES_REGION=
AWS_SES_ACCESS_KEY_ID=
AWS_SES_SECRET_ACCESS_KEY=
SES_FROM_ADDRESS=noreply@teams360.example.com

SMTP_HOST=
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=noreply@teams360.example.com

# Docker image version (unused in local dev)
VERSION=latest
```

### Frontend-only env (optional)

Next.js automatically loads `.env` / `.env.local` files from **`frontend/`** (not the repo root) at dev-server startup. If you need to override a frontend variable directly, create `frontend/.env.local`:

```env
NEXT_PUBLIC_API_URL=http://localhost:8080
```

There is no `NEXT_PUBLIC_OAUTH_*` variable — SSO is entirely a **backend** concern. The frontend fetches SSO configuration at runtime from `GET /api/v1/config`, which reflects whatever `OAUTH_*` variables the backend has (see below); if `OAUTH_CLIENT_ID` isn't set on the backend, the "Sign in with SSO" button is hidden automatically.

### Backend-only env (optional)

`backend/.env` is **not** loaded automatically (no dotenv dependency) — like the root `.env`, treat it as a template and `source`/`export` its contents into your shell before running the backend if you want these set:

```env
PORT=8080
DATABASE_URL=postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable
GIN_MODE=debug

# SSO — optional, enables the "Sign in with SSO" flow
OAUTH_CLIENT_ID=your-client-id
OAUTH_AUTHORIZE_URL=https://your-provider.com/oauth/authorize
OAUTH_TOKEN_URL=https://your-provider.com/oauth/token
OAUTH_REDIRECT_URI=http://localhost:3000/auth/callback
OAUTH_SCOPES=openid email profile
```

---

## 3. Database Setup

### Option A — Docker (recommended)

The Makefile handles this automatically. To start PostgreSQL only:

```bash
make db-start
```

This runs:

```bash
docker run -d --name teams360-db -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16-alpine
```

### Option B — Existing PostgreSQL

Set `DATABASE_URL` in your `.env` to point to your PostgreSQL instance:

```bash
DATABASE_URL=postgres://myuser:mypassword@myhost:5432/teams360?sslmode=disable
```

The database and user must already exist. The backend runs migrations automatically on startup.

### Run Migrations and Seed Data

```bash
make db-setup
```

This applies all migrations (`000001` through `000020+`) and seeds:
- 11 health dimensions
- Demo hierarchy levels (VP through Team Member, Admin)
- Demo users and teams

To reset the database (WARNING: deletes all data):

```bash
make db-reset
```

Verify the database is reachable:

```bash
psql "postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable" -c "SELECT count(*) FROM health_dimensions;"
# Expected: count = 11
```

---

## 4. Start the Application

### Option A — One command (recommended)

```bash
make run
```

This installs dependencies (if needed), starts PostgreSQL, runs migrations, and starts both servers. Demo credentials are printed to the terminal.

### Option B — Services separately

**Backend:**

```bash
cd backend
go mod download
go run cmd/api/main.go
```

The API starts at http://localhost:8080.

**Frontend (in a separate terminal):**

```bash
cd frontend
npm install
npm run dev
```

The app starts at http://localhost:3000.

### Option C — Hot reload (development)

```bash
make dev
```

Backend uses `air` for hot reload; frontend uses Next.js's built-in fast refresh.

---

## 5. Validate the Setup

### Check services are up

```bash
# Backend is up (this endpoint requires a JWT, so 401 — not 000/connection-refused — means it's listening)
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/v1/health-dimensions
# Expected: 401

# Backend health-dimensions with a real token (log in first to get one)
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo"}' | jq -r '.accessToken')
curl -s http://localhost:8080/api/v1/health-dimensions -H "Authorization: Bearer $TOKEN" | jq '.[0].name'
# Expected: "Mission" (or the first dimension name)

# Frontend
curl -s -o /dev/null -w "%{http_code}" http://localhost:3000
# Expected: 200
```

### Log in and walk through a basic flow

1. Open http://localhost:3000 in your browser.
2. Log in with the **Team Member** demo account: username `demo`, password `demo`.
3. Navigate to `/home` — you should see your team's health summary.
4. Click **Take Survey** and submit a health check across all 11 dimensions.
5. Log out and log back in as **Team Lead**: username `teamlead1`, password `demo`.
6. Navigate to `/dashboard` — you should see the team's aggregated health data and your response in the list.
7. Log in as **Manager**: username `manager1`, password `demo`.
8. Navigate to `/manager` — you should see health cards for all supervised teams.

### Demo credentials

| Role | Username | Password | Route after login |
|------|----------|----------|------------------|
| Vice President | `vp` | `demo` | `/manager` |
| Director | `director1` | `demo` | `/manager` |
| Manager | `manager1` | `demo` | `/manager` |
| Team Lead | `teamlead1` | `demo` | `/dashboard` |
| Team Member | `demo` | `demo` | `/home` |
| Administrator | `admin` | `admin` | `/admin` |

---

## Troubleshooting

### Mac ARM64 (Apple Silicon) — SWC errors

```bash
npm cache clean --force
rm -rf frontend/node_modules frontend/package-lock.json frontend/.next
cd frontend && npm install
npm install --force @next/swc-darwin-arm64
```

If the above does not work, run the fix script:

```bash
bash fix-mac-issues.sh
```

### PostgreSQL not running

```bash
# Check Docker
docker ps | grep postgres

# Restart the container
make db-start

# Test connection
psql "postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable" -c "SELECT 1"
```

### Port already in use

```bash
# Kill frontend (port 3000)
lsof -ti:3000 | xargs kill -9

# Kill backend (port 8080)
lsof -ti:8080 | xargs kill -9
```

### Backend fails to start — "no such table" or migration errors

```bash
# Re-run migrations and seed
make db-setup

# Or full reset (WARNING: deletes all data)
make db-reset
make db-setup
```

### `go: module lookup disabled by GONOSUMCHECK` or proxy errors

```bash
export GONOSUMCHECK=*
export GOFLAGS=-mod=mod
cd backend && go mod download
```

---

## Next Steps

- [Environment & Access](./environments.md) — connect to dev / int / prod environments
- [Architecture & Key Flows](../architecture/overview.md) — understand the system design
- [Onboarding Checklist](./onboarding.md) — complete your Day-1 checklist
