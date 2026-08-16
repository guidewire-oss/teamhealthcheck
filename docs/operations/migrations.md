# Migrations & Data

Schema migrations are managed with [golang-migrate](https://github.com/golang-migrate/migrate),
using its `file://` source pointed at
`backend/infrastructure/persistence/postgres/migrations/`.

## They run automatically at startup

`backend/cmd/api/main.go` runs `migrationEngine.Up()` unconditionally every time the API
process starts (after establishing the database connection, before serving any routes).
There is no separate "run migrations" step required in normal operation — starting the
backend (`go run cmd/api/main.go`, the Docker image, or a Kubernetes pod) is what applies
any pending migrations.

!!! warning "`go run cmd/api/main.go migrate` is not a real subcommand"
    `backend/cmd/api/main.go` does not parse `os.Args` at all — there is no `migrate`
    subcommand. `Makefile`'s `db-setup` target (line ~218) attempts
    `go run cmd/api/main.go migrate 2>/dev/null`, which always fails silently, and then
    falls back to its `||` branch: starting the real server for 5 seconds (which runs the
    normal startup migration path) and killing it. In other words, the "migrate" command
    in `db-setup` is dead code that happens to work anyway because of the fallback —
    don't rely on `main.go migrate` existing elsewhere.

## Migration list

Reconstructed from `backend/infrastructure/persistence/postgres/migrations/*.up.sql`, in
order. Every migration has a matching `.down.sql` (20 up, 20 down).

| # | Name | What it does |
|---|---|---|
| 000001 | create_health_dimensions | Creates `health_dimensions` |
| 000002 | create_health_check_sessions | Creates `health_check_sessions` |
| 000003 | create_health_check_responses | Creates `health_check_responses` |
| 000004 | seed_health_dimensions | Seeds the 11 Spotify + Team Health Check dimensions |
| 000005 | create_users_and_teams | Creates `users` and `teams` |
| 000006 | add_password_hash | Adds `password_hash` to `users` (`NOT NULL DEFAULT 'demo'`) |
| 000007 | seed_demo_users | Inserts the `admin` account (see [Data Model](../data-model.md)) |
| 000008 | add_team_cadence | Adds `cadence` to `teams` (default `quarterly`) |
| 000009 | create_hierarchy_levels | Creates `hierarchy_levels` |
| 000010 | add_hierarchy_level_color | Adds `color` to `hierarchy_levels` |
| 000011 | add_hierarchy_permissions | Adds `can_configure_system`, `can_view_reports`, and related permission columns to `hierarchy_levels` |
| 000012 | create_password_reset_tokens | Creates `password_reset_tokens` (single-use, 1-hour expiry) |
| 000013 | add_security_constraints | Adds `CHECK` constraints (e.g. email format) at the database level |
| 000014 | add_survey_type | Adds `survey_type` to `health_check_sessions` (`individual` default) to distinguish post-workshop surveys |
| 000015 | create_app_settings | Creates the singleton `app_settings` table |
| 000016 | update_cadence_values | Remaps `weekly`/`biweekly` cadence values to `monthly` |
| 000017 | add_auth_type | Adds `auth_type` (`local` / `sso`) to `users` |
| 000018 | add_branding_settings | Adds `company_name`, `logo_url`, etc. to `app_settings` |
| 000019 | add_team_distribution_list_email | Adds `distribution_list_email` to `teams` |
| 000020 | create_action_items | Creates `action_items` |

!!! warning "Migration 000016 is lossy"
    Its own comment states it directly: mapping `weekly`/`biweekly` cadence values to
    `monthly` cannot be reversed by the down migration — any team previously on a
    weekly/biweekly cadence loses that distinction permanently.

## Rollback

`golang-migrate`'s `.down.sql` files exist for all 20 migrations, but `main.go` only ever
calls `.Up()` — there is no wired-up command to run `.Down()` from this codebase. Rolling
back requires invoking the `migrate` CLI tool directly against the same
`file://infrastructure/persistence/postgres/migrations` source and `DATABASE_URL`, outside
of anything the Makefile or backend binary currently automates.

## Bringing up a local database

`make db-setup` (or `make run`/`make dev`, which depend on `_ensure-db`) starts (or
reuses) a `teams360-db` Postgres 16 Docker container on port 5432 — see the `_ensure-db`
target in the `Makefile`. Migrations then apply on first backend startup as described
above.

## Backup & restore

!!! warning "No backup/restore mechanism exists in this repository"
    There is no backup script, scheduled dump job, or restore tooling anywhere in the
    repo, Makefile, Docker Compose files, or KubeVela manifests. Persistence is whatever
    the Postgres container's data volume provides (see [Docker](docker.md) and
    [Kubernetes](kubernetes.md) for where that volume lives in each deployment mode) —
    operators are responsible for their own `pg_dump`/`pg_restore` or volume-snapshot
    strategy.
