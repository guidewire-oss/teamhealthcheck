# Docker Deployment Guide

This guide explains how to deploy Team Health Check using Docker containers.

## Quick Start

### Prerequisites

- Docker 24+ and Docker Compose v2
- Access to a PostgreSQL 14+ database
- (Optional) GitHub Container Registry access for pre-built images

Team Health Check ships as a single unified image (Go API + statically
exported Next.js frontend + generated docs, all served from one port) —
there are no separate `teams360-api`/`teams360-frontend` images.

### Production Deployment

Team Health Check containers require an external PostgreSQL database. The database is NOT included in the container images to follow security best practices.

#### 1. Create Environment File

```bash
# Copy the example environment file
cp .env.example .env

# Edit with your database credentials
nano .env
```

`.env.example` is written primarily for `docker-compose` (its `API_PORT` maps
to the container's `PORT` via compose variable substitution — see
[Local Setup](../getting-started/local-setup.md#2-configure-environment-variables)).
A plain `docker run --env-file .env` injects the file's variables unchanged,
so `API_PORT` has no effect here — if you need a non-default port, set `PORT`
in `.env` instead.

Required environment variables:

```bash
# Database connection (REQUIRED)
DATABASE_URL=postgres://user:password@host:5432/teams360?sslmode=require

# Optional configuration — the Go binary reads PORT directly (default 8080);
# there is no separate frontend process/port in the unified image, so
# FRONTEND_PORT and NEXT_PUBLIC_API_URL (a Next.js build-time variable) do
# not apply here.
PORT=8080
GIN_MODE=release
```

#### 2. Pull and Run

```bash
# Using the pre-built image from GHCR
docker pull ghcr.io/guidewire-oss/teams360:latest
docker run -d --name teams360 -p 8080:8080 --env-file .env \
  ghcr.io/guidewire-oss/teams360:latest

# Or build locally
docker build -t teams360:local .
docker run -d --name teams360 -p 8080:8080 --env-file .env teams360:local
```

#### 3. Verify Deployment

```bash
# Check container status
docker ps --filter name=teams360

# View logs
docker logs -f teams360
```

---

## Local Development

For local development, run the frontend and backend as native processes (not
in Docker) so you get hot reload; the Makefile starts PostgreSQL in a
container for you:

```bash
make db-start   # PostgreSQL 16 container on port 5432
make run        # Backend (go run) + frontend (next dev) together
```

See [Local Setup](../getting-started/local-setup.md) for details.

---

## Environment Variables

### Required

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@host:5432/db?sslmode=require` |

### Optional

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Port the server listens on | `8080` |
| `GIN_MODE` | Gin framework mode | `release` |
| `WEB_DIR` | Directory the server serves static frontend/docs assets from | `./web` |

---

## Database Configuration

### Supported Databases

Team Health Check requires PostgreSQL 14 or higher. Tested with:
- PostgreSQL 14, 15, 16, 17
- Amazon RDS for PostgreSQL
- Google Cloud SQL for PostgreSQL
- Azure Database for PostgreSQL
- Supabase

### Connection String Format

```
postgres://[user]:[password]@[host]:[port]/[database]?sslmode=[mode]
```

**SSL Modes:**
- `disable` - No SSL (local development only)
- `require` - Require SSL but don't verify certificate
- `verify-ca` - Require SSL and verify CA
- `verify-full` - Require SSL and verify CA + hostname (recommended for production)

### Examples

**Local PostgreSQL:**
```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable
```

**Amazon RDS:**
```bash
DATABASE_URL=postgres://admin:secret@mydb.abc123.us-east-1.rds.amazonaws.com:5432/teams360?sslmode=verify-full
```

**Supabase:**
```bash
DATABASE_URL=postgres://postgres:[password]@db.abcdefghijklmnop.supabase.co:5432/postgres?sslmode=require
```

---

## Container Image

### Pre-built Image

The single unified image is available from GitHub Container Registry:

```bash
docker pull ghcr.io/guidewire-oss/teams360:latest
docker pull ghcr.io/guidewire-oss/teams360:v1.0.0
```

### Image Tags

| Tag | Description |
|-----|-------------|
| `latest` | Latest stable release |
| `vX.Y.Z` | Specific version (e.g., v1.0.0) |
| `vX.Y` | Latest patch for major.minor |
| `vX` | Latest minor.patch for major |
| `sha-abc1234` | Specific commit |

### Verifying Image Signatures

The image is signed with Sigstore. Verify with:

```bash
cosign verify ghcr.io/guidewire-oss/teams360:v1.0.0 \
  --certificate-identity-regexp="https://github.com/guidewire-oss/teams360" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

---

## Building Locally

The image is built from the repository root using the single `Dockerfile`
(there is no separate `backend/Dockerfile` or `frontend/Dockerfile`):

```bash
docker build -t teams360:local .
```

### Multi-Architecture Builds

```bash
docker buildx build --platform linux/amd64,linux/arm64 -t teams360:local .
```

---

## Production Best Practices

### Security

1. **Use SSL for database connections** - Always use `sslmode=verify-full` in production
2. **Rotate credentials** - Use short-lived credentials or secret managers
3. **Network isolation** - Run containers in isolated networks
4. **Read-only filesystem** - Consider `--read-only` flag for containers
5. **Resource limits** - Set memory and CPU limits

### Example Production Config

```yaml
# docker-compose.prod.yml
services:
  teams360:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
    read_only: true
    tmpfs:
      - /tmp
    security_opt:
      - no-new-privileges:true
```

### Health Checks

The image has a built-in `HEALTHCHECK`:

- `GET /health` returns `{"status":"healthy"}`

Monitor with:
```bash
docker inspect --format='{{.State.Health.Status}}' teams360
```

---

## Kubernetes Deployment

For Kubernetes deployment, use the KubeVela + CloudNativePG setup documented
in the root `CLAUDE.md` (`kubevela/` directory and `Makefile.kubevela`), or
create your own manifests. There is no Helm chart in this repository.

```yaml
# Example Kubernetes Secret for database
apiVersion: v1
kind: Secret
metadata:
  name: teams360-db-credentials
type: Opaque
stringData:
  DATABASE_URL: postgres://user:password@host:5432/teams360?sslmode=verify-full
```

```yaml
# Example Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: teams360
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: teams360
          image: ghcr.io/guidewire-oss/teams360:v1.0.0
          envFrom:
            - secretRef:
                name: teams360-db-credentials
          ports:
            - containerPort: 8080
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
```

---

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs teams360

# Common issues:
# - DATABASE_URL not set
# - Database not accessible
# - Port already in use
```

### Database Connection Failed

```bash
# Test connectivity from container
docker exec teams360 wget -qO- http://localhost:8080/health

# Check the configured database host resolves from the container
docker exec teams360 getent hosts <db-host>
```

### Health Check Failing

```bash
# Check health status
docker inspect teams360 | jq '.[0].State.Health'

# Manual health check
curl http://localhost:8080/health
```

### Permission Denied

Containers run as non-root. If mounting volumes:
```bash
# Ensure correct ownership
chown -R 1001:1001 /path/to/volume
```

---

## Upgrading

### Rolling Update

```bash
# Pull the new image
docker pull ghcr.io/guidewire-oss/teams360:latest

# Recreate the container
docker stop teams360 && docker rm teams360
docker run -d --name teams360 -p 8080:8080 --env-file .env \
  ghcr.io/guidewire-oss/teams360:latest
```

### Database Migrations

Migrations run automatically on startup via `golang-migrate` — there is no
separate manual migration command in the image; restarting the container
against an older database applies any pending migrations.
