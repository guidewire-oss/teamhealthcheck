# =============================================================================
# Team Health Check Unified Image — Frontend (static export) + Backend (Go API)
# Uses --platform=$BUILDPLATFORM so npm/go builds run natively (no QEMU)
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Build MkDocs documentation site
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM python:3.12-alpine AS docs-builder

WORKDIR /docs-src

COPY mkdocs.yml ./
COPY docs/ ./docs/

RUN pip install --no-cache-dir mkdocs-material && \
    mkdocs build -d /site

# -----------------------------------------------------------------------------
# Stage 2: Build frontend static export
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ .
COPY --from=docs-builder /site/ ./public/docs/
ENV NEXT_TELEMETRY_DISABLED=1
ENV STATIC_EXPORT=true
RUN npm run build

# -----------------------------------------------------------------------------
# Stage 3: Build backend binary (cross-compile for target arch)
# -----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend-builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /backend

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" \
    -trimpath \
    -o /backend/team360-api \
    cmd/api/main.go

# -----------------------------------------------------------------------------
# Stage 4: Runtime — single container serving API + static frontend
# -----------------------------------------------------------------------------
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata wget

RUN adduser -D -g '' appuser

WORKDIR /app

# Copy Go binary
COPY --from=backend-builder /backend/team360-api ./team360-api

# Copy migration SQL files (required by golang-migrate file:// source at runtime)
COPY --from=backend-builder /backend/infrastructure/persistence/postgres/migrations/ \
    ./infrastructure/persistence/postgres/migrations/

# Copy static frontend into ./web/ (Go serves from WEB_DIR, default ./web)
COPY --from=frontend-builder /frontend/out/ ./web/

# Copy license and notice files for redistribution/compliance tooling
COPY LICENSE NOTICE /licenses/

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./team360-api"]
