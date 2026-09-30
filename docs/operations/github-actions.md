# GitHub Actions Documentation

This document describes the GitHub Actions workflows used in the Team Health Check project for CI/CD, security scanning, and automated dependency management.

## Quick Reference

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci.yml` | PR, push to main | Lint, test, build |
| `release.yml` | Version tags (v*.*.*) | Build & push Docker images |
| `security.yml` | PR, push, weekly | CodeQL, dependency review, secret scanning |

## Workflows

### CI Workflow (`ci.yml`)

**Purpose**: Ensures code quality on every pull request and push to main.

**Triggers**:
- Push to `main` branch
- Pull requests targeting `main`

**Jobs**:

| Job | Description | Dependencies |
|-----|-------------|--------------|
| `backend-lint` | Go formatting, vet, staticcheck | None |
| `backend-test` | Run Ginkgo tests with PostgreSQL | backend-lint |
| `backend-build` | Build Go binary | backend-lint |
| `frontend-lint` | ESLint and TypeScript check | None |
| `frontend-build` | Next.js production build | frontend-lint |
| `e2e-test` | Playwright E2E tests | backend-build, frontend-build |
| `ci-success` | Final status gate | All above |

**Security Features**:
- Read-only permissions (no write access to repo)
- No secrets exposed to PR workflows from forks
- Concurrency controls prevent duplicate runs

**E2E Test Conditions**:
E2E tests only run when:
- Pushing to `main` branch
- PR has the `e2e` label (add label to run on specific PRs)

---

### Release Workflow (`release.yml`)

**Purpose**: Build and publish Docker images to GitHub Container Registry (GHCR).

**Triggers**:
- Push of semantic version tags (e.g., `v1.0.0`, `v2.1.3`)
- Manual workflow dispatch with version input

**Jobs**:

| Job | Description | Outputs |
|-----|-------------|---------|
| `build-backend` | Build multi-arch backend image | Image digest, tags |
| `build-frontend` | Build multi-arch frontend image | Image digest, tags |
| `create-release` | Create GitHub Release | Release URL |

**Security Features**:
- ✅ **Minimal permissions**: Only `packages: write` for GHCR
- ✅ **Sigstore signing**: All images are signed with Cosign (keyless)
- ✅ **SBOM generation**: Software Bill of Materials included
- ✅ **Provenance attestations**: Build provenance for supply chain security
- ✅ **Multi-arch builds**: Supports `linux/amd64` and `linux/arm64`
- ✅ **No external secrets**: Uses only `GITHUB_TOKEN`

**Image Tags Generated**:
For tag `v1.2.3`:
- `ghcr.io/OWNER/teams360-api:1.2.3` (full version)
- `ghcr.io/OWNER/teams360-api:1.2` (major.minor)
- `ghcr.io/OWNER/teams360-api:1` (major)
- `ghcr.io/OWNER/teams360-api:latest` (if default branch)
- `ghcr.io/OWNER/teams360-api:sha-abc1234` (commit SHA)

**Verifying Image Signatures**:
```bash
# Install cosign
brew install cosign  # or appropriate package manager

# Verify signature
cosign verify ghcr.io/OWNER/teams360-api:v1.0.0 \
  --certificate-identity-regexp="https://github.com/OWNER/teams360" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

---

### Security Workflow (`security.yml`)

**Purpose**: Automated security scanning and vulnerability detection.

**Triggers**:
- Push to `main` branch
- Pull requests targeting `main`
- Weekly schedule (Sundays at midnight UTC)

**Jobs**:

| Job | Description | When |
|-----|-------------|------|
| `codeql` | Static analysis for Go and TypeScript | Always |
| `dependency-review` | Block PRs with vulnerable dependencies | PRs only |
| `trivy-scan` | Container vulnerability scanning | Main branch only |
| `secrets-scan` | TruffleHog secret detection | Always |

**CodeQL Languages**:
- Go (backend)
- JavaScript/TypeScript (frontend)

**Dependency Review**:
- Fails on `high` or `critical` severity vulnerabilities
- Allows common OSS licenses: MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD

---

## Dependabot Configuration

**File**: `.github/dependabot.yml`

Automated dependency updates for:

| Ecosystem | Directory | Schedule | Day |
|-----------|-----------|----------|-----|
| GitHub Actions | `/` | Weekly | Monday |
| Go (backend) | `/backend` | Weekly | Tuesday |
| Go (tests) | `/tests` | Weekly | Tuesday |
| npm (frontend) | `/frontend` | Weekly | Wednesday |
| Docker | `/backend`, `/frontend` | Weekly | Thursday |

**Grouping Strategy**:
- Minor and patch updates are grouped together
- Production and development dependencies separated (npm)
- Pre-release versions are ignored

---

## Security Best Practices Implemented

### 1. Principle of Least Privilege

```yaml
permissions:
  contents: read  # Default: read-only
  packages: write # Only when needed (releases)
```

### 2. No Secrets Exposure to Forks

PR workflows from forks:
- Cannot access repository secrets
- Have read-only permissions
- Cannot push to GHCR

### 3. Supply Chain Security

- **Signed images**: Cosign keyless signing with Sigstore
- **SBOM**: Software Bill of Materials attached to images
- **Provenance**: Build attestations for SLSA compliance
- **Pinned actions**: All GitHub Actions use specific versions

### 4. Container Security

**Backend Dockerfile**:
- Multi-stage build (separate build and runtime)
- `scratch` base image (minimal attack surface)
- Non-root user (`appuser`)
- No secrets in layers
- Stripped binary (`-ldflags="-s -w"`)

**Frontend Dockerfile**:
- Multi-stage build
- `node:alpine` base (minimal)
- Non-root user (`nextjs`)
- Production dependencies only

### 5. Secret Detection

- TruffleHog scans for leaked credentials
- `.dockerignore` excludes sensitive files
- Environment files never committed

---

## Creating a Release

### Automated (Recommended)

```bash
# Create and push a version tag
git tag v1.0.0
git push origin v1.0.0
```

This triggers:
1. CI workflow runs tests
2. Release workflow builds Docker images
3. Images are pushed to GHCR and signed
4. GitHub Release is created with release notes

### Manual

Use the workflow dispatch feature in GitHub Actions UI:
1. Go to Actions → Release workflow
2. Click "Run workflow"
3. Enter version (e.g., `v1.0.0`)
4. Click "Run workflow"

---

## Pulling Images

### Public Access (after release)

```bash
# Backend API
docker pull ghcr.io/OWNER/teams360-api:latest

# Frontend
docker pull ghcr.io/OWNER/teams360-frontend:latest

# Specific version
docker pull ghcr.io/OWNER/teams360-api:1.0.0
```

### Running with Docker Compose

```bash
# Production images
docker compose up -d

# Local build
docker compose up -d --build
```

---

## Troubleshooting

### CI Failures

**Backend lint fails**:
```bash
cd backend
go fmt ./...          # Fix formatting
go vet ./...          # Check for issues
staticcheck ./...     # Run static analysis
```

**Frontend lint fails**:
```bash
cd frontend
npm run lint -- --fix  # Auto-fix ESLint issues
npx tsc --noEmit       # Check TypeScript
```

### E2E Test Failures

1. Check if PostgreSQL service is healthy
2. Verify backend starts successfully
3. Review Playwright logs in artifacts

### Release Failures

**Image push fails**:
- Verify `GITHUB_TOKEN` has `packages: write` permission
- Check GHCR rate limits

**Signing fails**:
- Sigstore/Cosign requires `id-token: write` permission
- Verify OIDC token can be obtained

---

## Local Development

### Skip CI Workflows

Add `[skip ci]` or `[ci skip]` to commit message:
```bash
git commit -m "docs: update README [skip ci]"
```

### Run CI Checks Locally

```bash
# Backend
cd backend
make lint
make test

# Frontend
cd frontend
npm run lint
npm run build

# Full stack
make ci
```

---

## Environment Variables

### CI Workflow

| Variable | Description | Default |
|----------|-------------|---------|
| `GO_VERSION` | Go version for backend | `1.25` |
| `NODE_VERSION` | Node.js version for frontend | `22` |

### Release Workflow

| Variable | Description |
|----------|-------------|
| `REGISTRY` | Container registry | `ghcr.io` |
| `IMAGE_PREFIX` | Image name prefix | `ghcr.io/${{ github.repository_owner }}` |

### Secrets Used

| Secret | Purpose | Source |
|--------|---------|--------|
| `GITHUB_TOKEN` | GHCR push, releases | Auto-provided by GitHub |

**No external secrets required** - all authentication uses GitHub's built-in token.
