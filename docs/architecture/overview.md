# Architecture & Key Flows

This document describes Team Health Check's system architecture, component interactions, and the three most critical user flows. Read this before touching the backend or adding new API routes.

---

## System Overview

Team Health Check is a decoupled frontend/backend application:

```
Browser
  |-- loads UI ----------> Next.js 15 (port 3000)
  |                         App Router, TypeScript, Tailwind CSS
  |
  |-- HTTP/JSON ---------> Go / Gin backend (port 8080)
                            Domain-Driven Design, database/sql + lib/pq
                              |
                              | SQL (database/sql, lib/pq driver)
                              v
                            PostgreSQL 17 (port 5432)
```

The browser calls the Go backend **directly**: `frontend/lib/api/client.ts` sets `API_BASE_URL` to `NEXT_PUBLIC_API_URL` when set (e.g. `http://localhost:8080` in local dev) or to an empty string otherwise, so in production (same-origin, unified container) requests go to relative `/api/v1/...` paths served by the same Go binary. There is no Next.js route handler under `frontend/app/api/`. `frontend/next.config.ts` does define a dev-only `rewrites()` proxy from `/api/:path*` to `http://localhost:8080/api/:path*`, but it's unused in practice because the API client always calls an absolute `NEXT_PUBLIC_API_URL` in local dev; cross-origin requests in that scenario are handled by the backend's own `CORSMiddleware` (`backend/interfaces/api/middleware/cors.go`), registered in `backend/cmd/api/main.go`.

---

## Technology Stack

### Frontend

| Technology | Version | Purpose |
|-----------|---------|---------|
| Next.js | 15.x | React framework, App Router |
| TypeScript | 5.x | Type safety |
| Tailwind CSS | 3.x | Utility-first styling |
| Recharts | 3.x | Radar, bar, and line charts |
| Lucide React | Latest | Icons |
| js-cookie | Latest | Cookie-based auth token |

### Backend

| Technology | Version | Purpose |
|-----------|---------|---------|
| Go | 1.25+ | Application language |
| Gin | Latest | HTTP router and middleware |
| database/sql + lib/pq | v1.10.9 | PostgreSQL driver |
| golang-migrate | Latest | Database migrations |
| golang.org/x/crypto/bcrypt | External package | Password hashing (`bcrypt.DefaultCost`, cost 10) |

### Testing

| Technology | Purpose |
|-----------|---------|
| Ginkgo v2 | BDD test framework for Go |
| Gomega | Assertion / matcher library |
| Playwright | Browser automation for E2E tests |

---

## Directory Structure

```
teams360/
├── frontend/                    # Next.js 15 application
│   ├── app/                     # App Router pages (no /api proxy — calls backend directly)
│   │   ├── home/               # Team Member home page
│   │   ├── survey/             # Health check survey
│   │   ├── dashboard/          # Team Lead dashboard
│   │   ├── manager/            # Manager / VP dashboard
│   │   ├── admin/              # Admin panel
│   │   ├── login/              # Authentication page
│   │   └── auth/callback/      # SSO OAuth callback
│   ├── components/             # Reusable React components
│   ├── lib/                    # Utilities, types, API client
│   │   ├── api/                # API client functions
│   │   ├── auth.ts             # Authentication utilities
│   │   ├── types.ts            # TypeScript type definitions
│   │   └── assessment-period.ts # Auto-detect assessment period
│   └── middleware.ts           # Route protection (role-based redirects)
│
├── backend/                     # Go API server
│   ├── cmd/api/main.go         # Entry point: bootstraps server and registers routes
│   ├── domain/                 # DOMAIN LAYER — business logic only
│   │   ├── user/
│   │   ├── team/
│   │   ├── healthcheck/
│   │   └── organization/
│   ├── application/            # APPLICATION LAYER — use cases
│   │   ├── commands/           # Write operations
│   │   └── queries/            # Read operations
│   ├── infrastructure/         # INFRASTRUCTURE LAYER — external concerns
│   │   ├── persistence/postgres/
│   │   │   ├── migrations/     # SQL migration files (000001 - 000020+)
│   │   │   ├── user_repository.go
│   │   │   ├── team_repository.go
│   │   │   ├── health_check_repository.go
│   │   │   └── organization_repository.go
│   │   └── email/              # SES/SMTP email sending
│   └── interfaces/             # INTERFACES LAYER — HTTP handlers
│       ├── api/v1/             # Gin handlers
│       ├── dto/                # Data Transfer Objects
│       └── middleware/         # Auth middleware
│
├── tests/                       # E2E acceptance tests (full stack)
│   └── acceptance/
│       ├── suite_test.go        # Starts real servers + Playwright
│       ├── e2e_authentication_test.go
│       ├── e2e_survey_submission_test.go
│       └── e2e_manager_dashboard_test.go
│
└── docs/                        # This documentation
```

---

## DDD Layer Responsibilities

| Layer | Location | What lives here |
|-------|---------|----------------|
| **Domain** | `backend/domain/` | Entities, value objects, repository interfaces, domain events. Zero external dependencies. |
| **Application** | `backend/application/` | Use-case orchestration. Calls domain + infrastructure via interfaces. |
| **Infrastructure** | `backend/infrastructure/` | PostgreSQL repository implementations (database/sql + lib/pq), migrations, email sending. |
| **Interfaces** | `backend/interfaces/` | Gin HTTP handlers, DTOs, middleware. Thin layer: parse request, call application, return response. |

### DDD Aggregates

| Aggregate | Root entity | Value objects |
|-----------|------------|---------------|
| User | `User` | — |
| Team | `Team` | `TeamMember`, `SupervisorLink` |
| HealthCheck | `HealthCheckSession` | `HealthCheckResponse` |
| Organization | `OrganizationConfig` | `HierarchyLevel`, `HealthDimension`, `Permissions` |

---

## Role-Based Routing

| Role | Route | Capabilities |
|------|-------|-------------|
| Team Member | `/home` | Take survey, view personal history |
| Team Lead | `/dashboard` | Team health, radar chart, individual responses |
| Manager / Director / VP | `/manager` | Multi-team health cards, trends, radar comparison |
| Admin | `/admin` | CRUD for users, teams, hierarchy levels, dimensions |

`frontend/middleware.ts` reads the user cookie on every request and redirects unauthenticated users to `/login`. It also enforces role-level route guards based on `hierarchy_level` permissions.

---

## Critical Flow 1: Authentication (Login)

**Entry point**: `GET /login` → user fills form → `POST /api/v1/auth/login`

```
Browser                                  Go / Gin            PostgreSQL
  |                                         |                    |
  | 1. POST /api/v1/auth/login              |                    |
  |    username + password                  |                    |
  | ──────────────────────────────────────> |                    |
  |                                         | 2. SELECT user     |
  |                                         |    WHERE username  |
  |                                         | ─────────────────> |
  |                                         |                    |
  |                                         | 3. Return user row |
  |                                         | <───────────────── |
  |                                         |                    |
  |                                         | 4. bcrypt.Compare  |
  |                                         |    (pw, hash)      |
  |                                         |                    |
  | 5. 200 + JWT access/refresh             |                    |
  | <────────────────────────────────────── |                    |
  |                                         |                    |
  | 6. Store tokens in localStorage and set `user` cookie        |
  | 7. Redirect to role dashboard            |                    |
```

**Components involved:**
- `frontend/app/login/page.tsx` — login form; calls the backend directly at `NEXT_PUBLIC_API_URL` (no proxy route)
- `backend/interfaces/api/v1/auth_handler.go` — validates credentials, issues JWTs
- `backend/infrastructure/persistence/postgres/user_repository.go` — `FindByUsername()`
- `frontend/lib/auth.ts` — stores `accessToken`/`refreshToken` in localStorage and the `user` cookie
- `frontend/middleware.ts` — enforces the `user` cookie on subsequent page loads

**Data touched:** `users` table (read-only), `password_hash` column (bcrypt compare)

**SSO variant (OIDC / PKCE):**
- Frontend redirects to provider's `/authorize` endpoint
- Provider redirects back to `frontend/app/auth/callback/page.tsx` with auth code
- Frontend exchanges code via `POST /api/v1/auth/sso/callback`
- Backend calls provider's token endpoint, extracts `email` from ID token, looks up user in DB
- Same cookie-set flow follows

---

## Critical Flow 2: Survey Submission

**Entry point**: `/survey` page → user completes 11 dimensions → submits

The Next.js server only renders the `/survey` page shell; all API calls below go directly from browser JS to the Go backend (`NEXT_PUBLIC_API_URL`), not through Next.js:

```
Browser (Team Member)                        Go / Gin           PostgreSQL
  |                                              |                    |
  | 1. GET /survey  (Next.js renders page shell) |                    |
  |                                              |                    |
  | 2. GET /api/v1/health-dimensions             |                    |
  | ────────────────────────────────────────────>|                    |
  |                                              | 3. SELECT * FROM   |
  |                                              |    health_dimensions|
  |                                              |    WHERE is_active  |
  |                                              | ─────────────────> |
  |                                              | <───────────────── |
  | 4. 11 dimensions                             |                    |
  | <────────────────────────────────────────────|                    |
  | 5. Render survey form                        |                    |
  |                                              |                    |
  | 6. User selects score + trend for            |                    |
  |    each dimension                            |                    |
  |                                              |                    |
  | 7. POST /api/v1/health-checks                |                    |
  |    {teamId, userId, responses[11]}           |                    |
  |    (authenticatedFetch, Bearer JWT)          |                    |
  | ────────────────────────────────────────────>|                    |
  |                                              | 8. BEGIN TX        |
  |                                              |    INSERT session  |
  |                                              |    INSERT 11 rows  |
  |                                              |    into responses  |
  |                                              |    COMMIT          |
  |                                              | ─────────────────> |
  |                                              | <───────────────── |
  | 9. 201 session ID                            |                    |
  | <────────────────────────────────────────────|                    |
  | 10. Redirect to /home                        |                    |
```

**Components involved:**
- `frontend/app/survey/page.tsx` — 11-question survey form
- `frontend/lib/assessment-period.ts` — auto-computes period from submission date
- `backend/interfaces/api/v1/health_check_handler.go` — `SubmitHealthCheck()`
- `backend/infrastructure/persistence/postgres/health_check_repository.go` — transaction: insert session + responses

**Data touched:**
- `health_dimensions` (read)
- `health_check_sessions` (write)
- `health_check_responses` (write — 11 rows per submission)

**Assessment period logic:** cadence-driven, computed by `frontend/lib/assessment-period.ts` — see [Domain Model](domain-model.md#assessment-period-logic) for the full format table and legacy-period mapping.

---

## Critical Flow 3: Manager Dashboard Data Load

**Entry point**: Manager navigates to `/manager`

Next.js renders the `/manager` page shell; the health data call goes directly from browser JS to the Go backend:

```
Browser (Manager)                          Go / Gin           PostgreSQL
  |                                            |                     |
  | 1. GET /manager (Next.js renders shell)    |                     |
  |                                            |                     |
  | 2. GET /api/v1/managers/{id}/teams/health  |                     |
  |    (authenticatedFetch, Bearer JWT)        |                     |
  | ──────────────────────────────────────────>|                     |
  |                                            | 3. Find supervised  |
  |                                            |    teams via        |
  |                                            |    team_supervisors |
  |                                            | ───────────────── > |
  |                                            |                     |
  |                                            | 4. For each team:   |
  |                                            |    aggregate scores |
  |                                            |    by dimension     |
  |                                            |    for current      |
  |                                            |    period           |
  |                                            | ───────────────── > |
  |                                            | <────────────────   |
  |                                            |                     |
  | 5. JSON array of team health objects       |                     |
  | <──────────────────────────────────────────|                     |
  |                                            |                     |
  | 6. Render team cards + radar charts        |                     |
  |    (Recharts)                              |                     |
```

**Components involved:**
- `frontend/app/manager/page.tsx` — filters teams using `canUserAccessTeam()` based on supervisor chain
- `frontend/lib/org-config.ts` — `canUserAccessTeam()`, `getSubordinates()`
- `backend/interfaces/api/v1/manager_handler.go` — query supervised teams + aggregate
- `backend/infrastructure/persistence/postgres/` — joins `team_supervisors`, `health_check_sessions`, `health_check_responses`

**Data touched:**
- `team_supervisors` (read — find teams under this manager)
- `health_check_sessions` (read — filter by team + assessment period)
- `health_check_responses` (read — aggregate scores per dimension)
- `health_dimensions` (read — dimension names for chart labels)

**Critical implementation note:** Routes for the manager dashboard **must be registered in `backend/cmd/api/main.go`**. A past bug where `v1.SetupManagerRoutes()` was omitted caused 404s that integration tests did not catch — only E2E tests caught it.

---

## Troubleshooting Matrix

| Symptom | Likely cause | First check |
|---------|-------------|-------------|
| `/manager` returns 404 from backend | Route not registered in `main.go` | Run `grep "SetupManagerRoutes" backend/cmd/api/main.go` |
| Login succeeds but `/dashboard` shows no data | Team lead not assigned to any team | Check `team_members` table: `SELECT * FROM team_members WHERE user_id = 'teamlead1'` |
| Survey submit returns 500 | Transaction rollback — duplicate session or FK violation | Check backend logs; run `SELECT * FROM health_check_sessions WHERE user_id = '...' ORDER BY created_at DESC LIMIT 5` |
| Manager sees 0 teams | Supervisor chain not populated | Check `team_supervisors` table for manager's user_id |
| Dimension scores show 0 or NaN | No completed sessions in current assessment period | Confirm period label matches: `SELECT DISTINCT assessment_period FROM health_check_sessions` |
| Frontend blank white page | Next.js build error or NEXT_PUBLIC_API_URL misconfigured | Check browser console; verify `NEXT_PUBLIC_API_URL=http://localhost:8080` |
| `bcrypt: too many arguments` error on login | password_hash seeded as plaintext, not bcrypt hash | Run `make db-reset && make db-setup` to re-seed with hashed passwords |
| SSO login returns "user not found" | User email does not match any DB record | Pre-create user in Admin panel with matching email; SSO does not auto-provision accounts |
| `port already in use` on startup | Prior server process still running | `lsof -ti:3000 | xargs kill -9` and `lsof -ti:8080 | xargs kill -9` |
| `SWC error` on Mac ARM64 | Wrong native SWC binary installed | Run `npm install --force @next/swc-darwin-arm64` in `frontend/` |

---

## Observability Hooks

The backend emits OpenTelemetry spans and metrics on every request. See [Operations & Observability](../operations/observability.md) for the full catalogue. Key spans:

| Span name | Triggered by |
|-----------|-------------|
| `auth.login` | POST `/api/v1/auth/login` |
| `healthcheck.submit` | POST `/api/v1/health-checks` |
| `manager.teams.health` | GET `/api/v1/managers/{id}/teams/health` |
| `db.query` | Every database/sql query execution |
