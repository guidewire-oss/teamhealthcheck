# Onboarding Checklist for New Developers

Welcome to Team Health Check. This checklist walks you through Day 1 to Day 5 of contributing to this open-source project. Complete each item in order. The goal: by end of Day 5 you understand the system, have a working local stack, and have made your first pull request.

This project has no shared Dev/Int/Prod environment, Jira board, or Slack workspace (see [Environment & Access](./environments.md)) — everything here happens against your local stack and GitHub. If your organization has deployed its own instance of Team Health Check with additional internal tooling, adapt the checklist below accordingly.

Find a buddy engineer (another contributor/maintainer) willing to review your first PR and answer questions.

---

## Before Day 1 — Access

- [ ] Fork or clone `github.com/guidewire-oss/teams360` (public repo — no approval needed)
- [ ] Read [CONTRIBUTING.md](https://github.com/guidewire-oss/teams360/blob/main/CONTRIBUTING.md) for the contribution workflow
- [ ] Join [GitHub Discussions](https://github.com/guidewire-oss/teams360/discussions) if you have questions before Day 1

---

## Day 1 — Setup & Orientation

### Morning: Local environment

- [ ] Read [Overview](../overview.md) — understand what Team Health Check does and who uses it
- [ ] Follow [Local Setup Guide](./local-setup.md) completely — do not skip steps
- [ ] Verify setup validation: log in as `demo/demo`, submit a survey, then log in as `manager1/demo` and confirm health data appears
- [ ] Run backend tests: `cd backend && go test ./...` — all should pass
- [ ] Run E2E tests: `make test-e2e` — all should pass (takes 2–5 minutes)

### Afternoon: Codebase orientation

- [ ] Read [Architecture & Key Flows](../architecture/overview.md) — understand frontend, backend, and DB layers
- [ ] Open `backend/cmd/api/main.go` — trace how routes are registered
- [ ] Open `frontend/middleware.ts` — trace how role-based routing works
- [ ] Open `frontend/lib/types.ts` — familiarize yourself with core data models
- [ ] Ask your buddy to explain any architecture decisions that are unclear

---

## Day 2 — Database & Domain Model

- [ ] Read [Database Provisioning & Configuration](../architecture/db-schema.md) — know all 11 tables
- [ ] Read [Domain Model](../architecture/domain-model.md) — understand DDD layers
- [ ] Connect to local DB and run these queries to understand data shape:
  ```bash
  psql "postgres://postgres:postgres@localhost:5432/teams360?sslmode=disable"
  \dt                              -- list all tables
  SELECT * FROM hierarchy_levels;  -- see org levels
  SELECT * FROM health_dimensions; -- see 11 dimensions
  SELECT * FROM users LIMIT 5;     -- see demo users
  SELECT * FROM teams;             -- see demo teams
  SELECT * FROM team_supervisors WHERE team_id = 'platform-squad';
  ```
- [ ] Trace a health check session end-to-end in the DB:
  1. Submit a survey as `demo/demo`
  2. Find the session: `SELECT * FROM health_check_sessions ORDER BY created_at DESC LIMIT 1`
  3. Find its responses: `SELECT * FROM health_check_responses WHERE session_id = '<id from above>'`
- [ ] Read [Authentication & Authorization](./auth.md) — understand the JWT + cookie flow and RBAC

---

## Day 3 — Backend Deep Dive

- [ ] Trace the survey submission flow in code:
  - `frontend/app/survey/page.tsx` → `authenticatedFetch()` POST directly to the Go backend (`NEXT_PUBLIC_API_URL` — there is no Next.js proxy route)
  - `backend/interfaces/api/v1/health_check_handler.go` → `SubmitHealthCheck()`
  - `backend/infrastructure/persistence/postgres/health_check_repository.go` → `Save()`
- [ ] Read `backend/interfaces/api/v1/auth_handler.go` — understand login and SSO callback handlers
- [ ] Read one complete repository implementation:
  ```
  backend/infrastructure/persistence/postgres/user_repository.go
  ```
- [ ] Understand Ginkgo test structure by reading:
  ```
  tests/acceptance/e2e_survey_submission_test.go
  ```
- [ ] Run a single E2E test suite and observe output:
  ```bash
  cd tests
  export TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
  ginkgo -v -focus="E2E: Survey Submission" acceptance/
  ```

---

## Day 4 — Frontend Deep Dive

- [ ] Trace the manager dashboard load in code:
  - `frontend/app/manager/page.tsx` → fetches `/api/v1/managers/{id}/teams/health`
  - `frontend/lib/org-config.ts` → `canUserAccessTeam()` filters teams client-side
  - `backend/interfaces/api/v1/manager_handler.go` → `GetManagerTeamsHealth()`
- [ ] Read `frontend/lib/assessment-period.ts` — understand automatic period detection
- [ ] Read `frontend/lib/org-config.ts` — understand supervisor-chain access control
- [ ] Open the Admin panel as `admin/admin` and explore:
  - Hierarchy Levels tab — try reordering levels
  - Teams tab — create a test team, assign members
  - Dimensions tab — toggle a dimension inactive and see it disappear from the survey
- [ ] Read the Makefile documentation: [Makefile Reference](../development/makefile.md)
- [ ] Review [CI/CD & Deployment](../operations/deployment.md) — understand the GitHub Actions pipeline

---

## Day 5 — First Contribution

- [ ] Pick a [`good first issue`](https://github.com/guidewire-oss/teams360/labels/good%20first%20issue) from GitHub Issues (ask your buddy if none look approachable)
- [ ] Create a feature branch:
  ```bash
  git checkout -b your-name/issue-description
  ```
- [ ] Write a failing test first (Ginkgo for backend, or Playwright for E2E):
  ```bash
  # For backend unit/integration:
  cd backend && ginkgo -v -focus="your test description" ./...

  # For E2E:
  cd tests && ginkgo -v -focus="E2E: Your Feature" acceptance/
  ```
- [ ] Implement the fix until tests pass
- [ ] Run the full test suite:
  ```bash
  make test          # backend unit tests
  make test-e2e      # E2E tests
  ```
- [ ] Open a Pull Request with:
  - Link to the GitHub issue it closes
  - Description of what changed and why
  - Screenshot or curl output demonstrating the fix
- [ ] Request review from your buddy (and any other maintainer, per [CONTRIBUTING.md](https://github.com/guidewire-oss/teams360/blob/main/CONTRIBUTING.md))

---


## Useful References

| Resource | URL / Path |
|----------|------------|
| Repo | `github.com/guidewire-oss/teams360` |
| Local frontend | http://localhost:3000 |
| Local backend | http://localhost:8080 |
| Environments (there's only Local — see why) | [environments.md](./environments.md) |
| GitHub Discussions | https://github.com/guidewire-oss/teams360/discussions |
| Grafana (local) | http://localhost:3001 (after `make run-with-otel`) |
| API docs | [api/endpoints.md](../api/endpoints.md) |
| Makefile reference | [development/makefile.md](../development/makefile.md) |

---

## If Your Organization Runs Its Own Instance

If you're onboarding onto a fork or internal deployment of Team Health Check with its own Jira board, Slack workspace, shared Dev/Int/Prod environments, or security/compliance requirements, add those steps here — they don't exist in the upstream OSS project and can't be documented generically.
