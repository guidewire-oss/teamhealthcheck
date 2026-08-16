# Team Health Check

Team Health Check is an open-source (Apache 2.0) web application based on
[Spotify's Squad Health Check Model](https://engineering.atspotify.com/2014/09/squad-health-check-model),
extended with organizational hierarchy, configurable dimensions, and role-based
aggregation for managers and executives.

Teams score themselves across health dimensions on a red / yellow / green scale, add
a trend and an optional comment, and the results roll up through the supervisor chain
so that team leads, managers, directors, and VPs can see where their teams need support.

!!! note "Support, not surveillance"
    "Red" does not mean "bad team" — it means "this team needs support in this area."
    Team Health Check is designed as a support tool, not a performance-evaluation mechanism.

## What's in this site

| Section | Contents |
|---------|----------|
| [Architecture](architecture/overview.md) | System design, DDD layering, request lifecycle, auth model, external services |
| [API Reference](api/index.md) | Every HTTP route, its guard middleware, request/response shape, and error codes |
| [Data Model](data-model.md) | The Postgres schema reconstructed from migrations, plus the Go domain entities |
| [Guides](guides/index.md) | Task-oriented walkthroughs of the workflows that exist in the UI |
| [Operations](operations/configuration.md) | Environment variables, Docker, Makefile, CI, Kubernetes, migrations |
| [Observability](observability.md) | OpenTelemetry tracing, metrics, logging, and the Grafana/Prometheus stack |
| [Testing](testing.md) | Where tests live and how to run them |

## Technology stack

**Frontend** — Next.js 15 (App Router), TypeScript, Tailwind CSS, Recharts, Playwright
for E2E. Route protection lives in `frontend/middleware.ts`; all domain data comes from
the Go API.

**Backend** — Go 1.25 with Gin, laid out in Domain-Driven Design layers under
`backend/{domain,application,infrastructure,interfaces}`. Tests use Ginkgo v2 and Gomega.

**Database** — PostgreSQL, with schema managed by `golang-migrate` SQL migrations under
`backend/infrastructure/persistence/postgres/migrations/`. Migrations run automatically
at API startup (see [Migrations & Data](operations/migrations.md)).

**Packaging** — a single unified container image built from the root `Dockerfile`: the
Next.js app is statically exported and served by the Go binary, so one process serves
both the API and the SPA.

## Quick start

```bash
# Install dependencies for both services
make install

# Start Postgres, run the backend and the frontend together
make run
```

The frontend is served at `http://localhost:3000` and the API at `http://localhost:8080`.

To exercise the full stack from a single container image instead, see
[Docker](operations/docker.md).

## Demo credentials

Demo users are seeded **only when `APP_ENV=demo`** (`backend/cmd/api/main.go`, which calls
`postgres.SeedDemoData`). All demo passwords are `demo`, except `admin`, whose password is
`admin`.

| Hierarchy level | Example usernames |
|-----------------|-------------------|
| VP (`level-1`) | `vp` |
| Director (`level-2`) | `director1`, `director2` |
| Manager (`level-3`) | `manager1`, `manager2`, `manager3` |
| Team Lead (`level-4`) | `teamlead1` … `teamlead9` |
| Team Member (`level-5`) | `demo`, `alice`, … |
| Admin (`level-admin`) | `admin` |

!!! warning "The `admin` user is seeded by migration, not by the demo seeder"
    Migration `000007_seed_demo_users.up.sql` inserts only the `admin` account. Every
    other demo user comes from the runtime seeder gated on `APP_ENV=demo`. On a
    non-demo deployment, `admin` is the only account that exists after a fresh migrate.

## Health dimensions

Eleven dimensions ship seeded by migration `000004`. Eight come from Spotify's original
model — Mission, Delivering Value, Speed, Fun, Health of Codebase, Learning, Support,
Pawns or Players — and three are Team Health Check additions: Easy to Release, Suitable Process,
and Teamwork. Administrators can add, edit, disable, and weight dimensions through the
admin settings API.

!!! warning "The survey UI does not read dimensions from the API"
    Admins can CRUD dimensions via `/api/v1/admin/settings/dimensions`, and
    `GET /api/v1/health-dimensions` exists, but the survey page renders the hardcoded
    `HEALTH_DIMENSIONS` constant in `frontend/lib/data.ts`. Dimension changes made in
    the admin UI will not appear in the survey. See
    [Submitting a Health Check](guides/survey.md).
