# Docker Deployment Guide

This guide explains how to deploy Team Health Check using Docker containers.

## Quick Start

### Prerequisites

- Docker 24+ and Docker Compose v2
- Access to a PostgreSQL 14+ database
- (Optional) GitHub Container Registry access for pre-built images

### Production Deployment

Team Health Check containers require an external PostgreSQL database. The database is NOT included in the container images to follow security best practices.

#### 1. Create Environment File

```bash
# Copy the example environment file
cp .env.example .env

# Edit with your database credentials
nano .env
```

Required environment variables:

```bash
# Database connection (REQUIRED)
DATABASE_URL=postgres://user:password@host:5432/teams360?sslmode=require

# Optional configuration
API_PORT=8080
FRONTEND_PORT=3000
GIN_MODE=release
NEXT_PUBLIC_API_URL=https://api.yourdomain.com
```

#### 2. Pull and Run

```bash
# Using pre-built images from GHCR
docker compose up -d

# Or build locally
docker compose up -d --build
```

#### 3. Verify Deployment

```bash
# Check service health
docker compose ps

# View logs
docker compose logs -f
```

---

## Local Development

For local development with a PostgreSQL container included:

```bash
# Start with development database
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d

# Or use the Makefile
make docker-dev
```

This starts:
- PostgreSQL 17 on port 5432
- Backend API on port 8080
- Frontend on port 3000

---

## Environment Variables

### Required

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@host:5432/db?sslmode=require` |

### Optional

| Variable | Description | Default |
|----------|-------------|---------|
| `API_PORT` | Backend API port | `8080` |
| `FRONTEND_PORT` | Frontend port | `3000` |
| `GIN_MODE` | Gin framework mode | `release` |
| `NEXT_PUBLIC_API_URL` | API URL for frontend | `http://localhost:8080` |
| `VERSION` | Image version tag | `latest` |

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

## Container Images

### Pre-built Images

Images are available from GitHub Container Registry:

```bash
# Backend API
docker pull ghcr.io/anthropics/teams360-api:latest
docker pull ghcr.io/anthropics/teams360-api:v1.0.0

# Frontend
docker pull ghcr.io/anthropics/teams360-frontend:latest
docker pull ghcr.io/anthropics/teams360-frontend:v1.0.0
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

All images are signed with Sigstore. Verify with:

```bash
cosign verify ghcr.io/anthropics/teams360-api:v1.0.0 \
  --certificate-identity-regexp="https://github.com/anthropics/teams360" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

---

## Building Locally

### Build Both Images

```bash
docker compose build
```

### Build Individually

```bash
# Backend
docker build -t teams360-api:local ./backend

# Frontend
docker build -t teams360-frontend:local ./frontend \
  --build-arg NEXT_PUBLIC_API_URL=http://localhost:8080
```

### Multi-Architecture Builds

```bash
# Build for multiple platforms
docker buildx build --platform linux/amd64,linux/arm64 \
  -t teams360-api:local ./backend
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
  api:
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

Both containers include health checks:

- **Backend**: `GET /health` returns 200 OK
- **Frontend**: `GET /` returns 200 OK

Monitor with:
```bash
docker inspect --format='{{.State.Health.Status}}' teams360-api
```

---

## Kubernetes Deployment

For Kubernetes deployment, use the provided Helm chart or create your own manifests:

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
  name: teams360-api
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/anthropics/teams360-api:v1.0.0
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
docker compose logs api

# Common issues:
# - DATABASE_URL not set
# - Database not accessible
# - Port already in use
```

### Database Connection Failed

```bash
# Test connectivity from container
docker compose exec api wget -qO- http://localhost:8080/health

# Check database is reachable
docker compose exec api nc -zv <db-host> 5432
```

### Health Check Failing

```bash
# Check health status
docker inspect teams360-api | jq '.[0].State.Health'

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
# Pull new images
docker compose pull

# Restart with zero downtime (if using replicas)
docker compose up -d --no-deps api
docker compose up -d --no-deps frontend
```

### Database Migrations

Migrations run automatically on startup. For manual control:

```bash
# Run migrations only
docker compose exec api /team360-api migrate

# Check migration status
docker compose exec api /team360-api migrate status
```
