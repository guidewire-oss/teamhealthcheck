# Request Lifecycle

This page traces a request from the network through to the database and back, as wired
in `backend/cmd/api/main.go`.

## Startup sequence

`main()` performs the following in order before the server accepts traffic:

1. **Logger** — `logger.Init` using `LOG_LEVEL` (default `info`) and `LOG_PRETTY`.
2. **Telemetry** — `telemetry.Init`. A failure here logs a warning and the process
   continues without tracing.
3. **Gin mode** — `gin.SetMode(os.Getenv("GIN_MODE"))`, defaulting to `gin.DebugMode`.
4. **Database** — `otelsql.Open("postgres", DATABASE_URL)`. If `Ping` fails, the process
   attempts to `CREATE DATABASE teams360` through a hardcoded
   `postgres://postgres:postgres@localhost:5432/postgres` admin connection, then
   reconnects.
5. **App config** — `postgres.EnsureAppConfig(db)`.
6. **Migrations** — `golang-migrate` runs `Up()` against the live connection using the
   source `file://infrastructure/persistence/postgres/migrations`. Any error other than
   `ErrNoChange` is fatal.
7. **Demo seed** — `postgres.SeedDemoData(db)` runs only when `APP_ENV=demo`.
8. **Repositories and services** — health check, user, team, and organization
   repositories; the trends service; the JWT service; the email sender; the notification
   service; the password reset service.
9. **Router construction and route registration** (below).
10. **Static file serving** for the exported frontend, if `WEB_DIR` exists.

!!! warning "The auto-create-database fallback ignores `DATABASE_URL`"
    The recovery path in `main.go` connects to `postgres://postgres:postgres@localhost:5432/postgres`
    literally, and issues `CREATE DATABASE teams360` with a hardcoded name. It cannot
    work against a remote or differently-credentialed Postgres. In containerised
    deployments the database must already exist.

## Global middleware chain

The router is built with `gin.New()` — Gin's default logger is deliberately not used.
Middleware is applied in this order, and therefore runs in this order for every request:

| # | Middleware | Source | Effect |
|---|-----------|--------|--------|
| 1 | `gin.Recovery()` | Gin | Recovers from panics, returns 500 |
| 2 | `otelgin.Middleware("teams360-api")` | contrib | Starts the server span |
| 3 | `RequestIDMiddleware()` | `interfaces/api/middleware/request_logger.go` | Assigns `request_id` into the context |
| 4 | `RequestLoggerMiddleware()` | `interfaces/api/middleware/request_logger.go` | Structured zerolog access log; skips `/health` |
| 5 | `CORSMiddleware()` | `interfaces/api/middleware/cors.go` | Sets CORS headers; short-circuits `OPTIONS` with 204 |
| 6 | `ContentTypeValidator()` | `interfaces/api/middleware/validation.go` | 415 if a POST/PUT/PATCH sends a non-JSON `Content-Type` |
| 7 | `MaxBodySizeMiddleware(10MB)` | `interfaces/api/middleware/validation.go` | Wraps the body in `http.MaxBytesReader` |

!!! warning "CORS is fully permissive"
    `CORSMiddleware` sets `Access-Control-Allow-Origin: *` together with
    `Access-Control-Allow-Credentials: true`, unconditionally and with no configuration
    hook. There is no origin allowlist.

!!! warning "Rate limiting is implemented but never wired up"
    `validation.go` defines `RateLimiter`, `RateLimitMiddleware`, and an
    `AuthRateLimiter` preset of 10 requests/minute, but `main.go` never applies any of
    them — including on `/api/v1/auth/login`. Login is not rate limited.

## Route registration

After the global chain, `main.go` registers route groups in this order. Each `Setup*`
function attaches its own guard middleware to its group (see
[Authentication & Authorization](authentication.md)).

```go
router.GET("/health", ...)                  // static 200, no auth

v1.SetupHealthCheckRoutes(...)              // JWT
v1.SetupAuthRoutes(...)                     // public
v1.SetupSSORoutes(...)                      // public (+ GET /api/v1/config)
v1.SetupManagerRoutes(...)                  // JWT + ManagerOrAbove + SameUserOrManager
v1.SetupTeamRoutes(...)                     // JWT (+ TeamMembership on /:teamId)
v1.SetupTeamDashboardRoutes(...)            // JWT + TeamMembership
v1.SetupActionItemRoutes(...)               // JWT + TeamMembership / ManagerOrAbove
v1.SetupUserRoutes(...)                     // JWT + SameUserOrManager
v1.SetupProtectedUserRoutes(...)            // JWT
v1.SetupAdminRoutes(...)                    // JWT + AdminOnly
v1.SetupPasswordResetRoutes(...)            // public
```

## Per-request path

```
Client
  │
  ▼
gin.Recovery → otelgin → RequestID → RequestLogger → CORS → ContentType → MaxBodySize
  │
  ▼
Route group guards  (JWTAuthMiddleware → role/ownership middleware)
  │  claims placed on the context: userID, username, email, hierarchyLevel, teamIDs, claims
  ▼
Handler  (interfaces/api/v1/*_handler.go)
  │  binds+validates the DTO, reads path/query params
  ▼
Application layer  (application/commands, application/queries, application/services,
  │                 application/trends)  — where the handler uses one
  ▼
Domain  (domain/{user,team,healthcheck,organization}) — entities, value objects,
  │      repository interfaces
  ▼
Postgres repository  (infrastructure/persistence/postgres/*_repository.go)
  │
  ▼
PostgreSQL
```

The response travels back through the DTO layer: handlers emit either `dto.Respond*`
helpers or a raw `c.JSON`. Both produce the same wire shape — there is no envelope.
See [API Overview](../api/index.md#response-conventions).

!!! note "Not every handler goes through the application layer"
    Several handlers take `*sql.DB` directly and run SQL inline rather than going
    through a repository: `TeamDashboardHandler`, `ActionItemHandler`, and
    `UserHandler.GetUserSurveyHistory`. This is a deliberate read-model shortcut for
    aggregate queries, but it means those paths bypass the domain layer entirely.

## Static frontend serving

When the directory named by `WEB_DIR` (default `./web`) exists, `main.go` registers a
`NoRoute` handler that serves the Next.js static export:

- Paths under `/api/` return JSON `{"error": "not found"}` with 404.
- `/_next/static/*` gets `Cache-Control: public, max-age=31536000, immutable`.
- The resolved path is checked against the absolute web directory prefix to block
  path traversal, both for the exact file and for the `.html` variant.
- An exact file match is served; otherwise `<path>.html` is tried (Next.js static export
  maps `/login` to `login.html`).
- Remaining `/_next/` paths 404 rather than falling through.
- Everything else falls back to `index.html` for SPA routing.

## Graceful shutdown

`main.go` starts `router.Run` in a goroutine and blocks on `SIGINT`/`SIGTERM`. On signal
it logs "shutting down server gracefully..." and returns.

!!! warning "Shutdown is not actually graceful"
    There is no `http.Server.Shutdown` call and no connection draining — the process
    simply returns from `main`, terminating in-flight requests. The deferred telemetry
    shutdown does run.
