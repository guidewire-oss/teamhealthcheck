# Configuration

All configuration is via environment variables. `.env.example` at the repo root is the
starting point — copy it to `.env` and adjust.

## Backend

| Variable | Purpose | Default if unset | Source |
|---|---|---|---|
| `DATABASE_URL` | Postgres connection string | *(required — fatal if empty)* | `backend/cmd/api/main.go` |
| `API_PORT` | Port the API listens on when `PORT` is not set | `8080` | `.env.example` |
| `PORT` | Overrides `API_PORT` if set (common on PaaS platforms) | — | `backend/cmd/api/main.go:295` |
| `GIN_MODE` | Gin mode: `debug` or `release` | `release`-ish (Gin's own default) | `backend/cmd/api/main.go:62` |
| `APP_ENV` | `demo` seeds the full demo user/team dataset via `SeedDemoData`; any other value (`dev`, `prod`, …) skips it | `dev` | `backend/cmd/api/main.go:146`, `backend/infrastructure/persistence/postgres/seed.go` |
| `WEB_DIR` | Directory the Go binary serves the static frontend from | `./web` | `backend/cmd/api/main.go:224` |
| `LOG_LEVEL` | Log verbosity | logger default | `backend/cmd/api/main.go:35` |
| `LOG_PRETTY` | `true` for human-readable (non-JSON) logs | `false` | `backend/cmd/api/main.go:39` |
| `JWT_SECRET` | Signing secret for session/auth JWTs | — | `backend/application/services/jwt_service.go` |
| `JWT_ACCESS_EXPIRY` | Access token TTL | service default | `backend/application/services/jwt_service.go` |
| `JWT_REFRESH_EXPIRY` | Refresh token TTL | service default | `backend/application/services/jwt_service.go` |
| `JWT_ISSUER` | `iss` claim value | service default | `backend/application/services/jwt_service.go` |
| `OAUTH_CLIENT_ID` | SSO/OAuth client id | — | `backend/interfaces/api/v1/sso_routes.go`, `sso_handler.go` |
| `OAUTH_AUTHORIZE_URL` | SSO authorize endpoint | — | `backend/interfaces/api/v1/sso_routes.go` |
| `OAUTH_TOKEN_URL` | SSO token endpoint | — | `backend/interfaces/api/v1/sso_handler.go` |
| `OAUTH_REDIRECT_URI` | SSO redirect URI | — | `backend/interfaces/api/v1/sso_routes.go`, `sso_handler.go` |
| `AWS_SES_REGION` | SES region | — | `.env.example`, `backend/infrastructure/email/ses_service.go` |
| `AWS_SES_ACCESS_KEY_ID` / `AWS_SES_SECRET_ACCESS_KEY` | SES credentials | — | same |
| `SES_FROM_ADDRESS` | SES sender address | `noreply@teams360.example.com` | same |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD` / `SMTP_FROM` | SMTP fallback if SES isn't configured | port `587`, rest blank | `backend/infrastructure/email/smtp_service.go` |
| `OTEL_ENABLED` | Enable OpenTelemetry tracing | `false` | `backend/pkg/telemetry/telemetry.go` — see [Observability](../observability.md) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint | — | same |
| `ENVIRONMENT` | Telemetry resource attribute (`deployment.environment`) | — | same |
| `TEST_DATABASE_URL` | Database used by backend integration tests | — | `backend/tests/testhelpers/database.go` |

## Frontend

| Variable | Purpose | Default if unset | Source |
|---|---|---|---|
| `FRONTEND_PORT` | Port `next dev`/`next start` listens on | `3000` | `.env.example` |
| `NEXT_PUBLIC_API_URL` | Base URL the browser calls for the API | `''` (same-origin) | `frontend/lib/api/client.ts` |
| `NEXT_PUBLIC_DOCS_URL` | Where the "Docs" nav link points | `/docs/index.html` (the locally-built MkDocs site) | `frontend/components/DocsLink.tsx` — see [Docker](docker.md) |
| `NEXT_PUBLIC_OTEL_ENABLED` | Enable browser-side OpenTelemetry | `false` | `frontend/lib/telemetry/config.ts` |
| `NEXT_PUBLIC_OTEL_SERVICE_NAME` | Service name reported to the collector | `teams360-frontend` | same |
| `NEXT_PUBLIC_OTEL_SERVICE_VERSION` | Service version reported | `1.0.0` | same |
| `NEXT_PUBLIC_ENVIRONMENT` | Telemetry environment attribute | `development` | same |
| `NEXT_PUBLIC_OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP endpoint for browser traces | `http://localhost:4318` | same |
| `NEXT_PUBLIC_OTEL_SAMPLE_RATE` | Trace sample rate (0–1) | `1.0` | same |
| `NEXT_PUBLIC_OTEL_DEBUG` | Verbose telemetry console logging | `false` | same |

## Other

| Variable | Purpose |
|---|---|
| `VERSION` | Docker image tag used by `docker-compose.yml` |

!!! warning "Several env vars are read in code but absent from `.env.example`"
    `APP_ENV`, `LOG_LEVEL`, `LOG_PRETTY`, `WEB_DIR`, `PORT`, `JWT_SECRET`, `JWT_ACCESS_EXPIRY`,
    `JWT_REFRESH_EXPIRY`, `JWT_ISSUER`, `OAUTH_CLIENT_ID`, `OAUTH_AUTHORIZE_URL`,
    `OAUTH_TOKEN_URL`, `OAUTH_REDIRECT_URI`, `TEST_DATABASE_URL`, `OTEL_ENABLED`,
    `OTEL_EXPORTER_OTLP_ENDPOINT`, `ENVIRONMENT`, and all `NEXT_PUBLIC_OTEL_*`/
    `NEXT_PUBLIC_ENVIRONMENT` vars are consumed by the code but not listed in the
    root `.env.example`. In particular, `JWT_SECRET` with no fallback shown in
    `jwt_service.go` beyond an empty string is worth confirming before any
    non-local deployment. Treat this table, not `.env.example`, as authoritative
    for what the application actually reads.

!!! note "Demo data is opt-in"
    `APP_ENV=demo` is what makes `demo/demo`, `manager1/demo`, etc. exist (see
    [Home](../index.md#demo-credentials)). Any other value skips the seeder, and the
    only account is the `admin` user inserted directly by migration `000007`.
