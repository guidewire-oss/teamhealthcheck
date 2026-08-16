# Testing

Team Health Check follows outer-loop TDD with Ginkgo/Gomega on the backend and true end-to-end
Playwright tests for full-stack flows — the rationale and philosophy are documented at
length in `CLAUDE.md` ("Testing Strategy & End-to-End Testing") and aren't repeated here.
This page is the map of where tests actually live and the commands that actually run them.

## Where tests live

| Location | What it covers | Framework |
|---|---|---|
| `backend/domain/**/*_test.go`, `backend/application/**/*_test.go` | Domain/application unit tests | Ginkgo v2 + Gomega |
| `backend/interfaces/api/v1/*_test.go` (`admin_api_test.go`, `health_check_api_test.go`, `user_handler_test.go`, `suite_test.go`) | API handler unit tests | Ginkgo v2 + Gomega |
| `backend/tests/integration/*.go` | Integration tests against a real Postgres (auth, SSO, password reset, supervisor chain, dashboards, validation middleware, DB constraints) | Ginkgo v2 + Gomega |
| `backend/tests/acceptance/*.go` | Despite the directory name, these are **repository/migration** tests (`database_migration_test.go`, `health_check_repository_test.go`), not E2E — explicitly skipped by the default test run (see below) | Ginkgo v2 + Gomega |
| `tests/acceptance/*.go` (repo root) | **True E2E** tests — full stack via Playwright (26+ `e2e_*_test.go` files: auth, authorization, JWT, survey submission/autosave, dashboards per role, admin flows, session timeout, multi-team, data validation, etc.) | Ginkgo v2 + Gomega + Playwright, own `tests/go.mod` |
| `frontend/lib/__tests__/*.test.ts` (`assessment-period.test.ts`, `sso.test.ts`) | Frontend unit tests | Vitest (`frontend/vitest.config.ts`) |

!!! warning "`backend/tests/acceptance` is not the E2E suite"
    The name is easy to confuse with the real acceptance suite at repo-root
    `tests/acceptance/`. `backend/tests/acceptance` only contains a migration test and a
    repository test — plain integration tests that happen to be grouped under an
    "acceptance" folder name. The Makefile's unit/integration targets explicitly
    `--skip-package=tests/acceptance` to exclude this folder (it needs a live DB and isn't
    part of the fast unit run), and the actual browser-driven E2E suite lives at the repo
    root, not under `backend/`.

## Running tests

Commands are `Makefile` targets, verified against the current file (not assumed from
`CLAUDE.md`):

```bash
make test                  # everything: frontend + backend + E2E
make test-unit             # frontend unit + backend unit/integration (no E2E)

make test-frontend         # alias for test-frontend-unit (npm test -> vitest run)
make test-frontend-watch   # vitest watch mode
make test-frontend-coverage

make test-backend          # alias for test-backend-unit
make test-backend-unit     # ginkgo -r --skip-package=tests/acceptance ./...
make test-backend-verbose  # + -v --race
make test-backend-coverage # + --cover --coverprofile, then go tool cover -> backend/coverage.html
make test-backend-watch    # ginkgo watch

make test-e2e              # starts Postgres, backend, and frontend itself, then runs
                            # the Ginkgo/Playwright suite in tests/acceptance/
```

!!! note "`CLAUDE.md`'s example commands differ slightly from the Makefile"
    `CLAUDE.md` shows running Ginkgo directly (`ginkgo -v ./...`,
    `ginkgo -focus="..." ./tests/integration`) from inside `backend/`. That still works,
    but the Makefile targets are the source of truth for CI and add
    `--skip-package=tests/acceptance` — running bare `ginkgo -r ./...` from `backend/`
    without that flag will also try to run the repository/migration tests in
    `backend/tests/acceptance`, which need `TEST_DATABASE_URL` set and a live database.

## Prerequisites

- Backend integration/acceptance tests need `TEST_DATABASE_URL` pointing at a reachable
  Postgres (see [Configuration](operations/configuration.md)).
- `make test-e2e` manages its own Postgres, backend, and frontend processes — no manual
  setup needed beyond Docker being available (see `_ensure-db` in the `Makefile`).
- The root `tests/` module has its own `go.mod` (`tests/go.mod`), separate from
  `backend/go.mod`, since it depends on Playwright-Go in addition to Ginkgo/Gomega.
