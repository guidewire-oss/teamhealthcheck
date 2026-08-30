# Team Health Check Documentation

---

## Table of Contents

- **[Overview](./overview.md)**
  What Team Health Check does, who uses it, key problems it solves, and how it fits in the internal tooling ecosystem.

- **[Architecture & Key Flows](./architecture/overview.md)**
  System design (frontend to backend to DB), DDD layer breakdown, and sequence walkthroughs for the three most critical flows: authentication, survey submission, and manager dashboard. Includes troubleshooting matrix.

- **[Local Setup Guide](./getting-started/local-setup.md)**
  Step-by-step guide to clone the repo, configure environment variables, start PostgreSQL, run migrations, and verify a working local stack.

- **[Environment & Access](./getting-started/environments.md)**
  What environments actually exist today (local dev + self-hosted KubeVela deployment), plus a template for organizations adding their own shared Dev/Int/Prod environments.

- **[Database Provisioning & Configuration](./architecture/db-schema.md)**
  PostgreSQL schema (all 11 tables), migration history, seed data, performance indexes, and operational commands.

- **[Authentication & Authorization](./getting-started/auth.md)**
  JWT (access + refresh) and cookie-based session flow, bcrypt password handling, SSO / OIDC (PKCE) integration guide, hierarchy-level permission matrix, and route-protection middleware.

- **[Operations & Observability](./operations/observability.md)**
  OpenTelemetry setup, Prometheus metrics catalogue (40+ metrics), Grafana dashboard bootstrap, structured logging format, and alerting runbooks.

- **[CI/CD & Deployment](./operations/deployment.md)**
  GitHub Actions workflows (CI, release, security), Docker image tags, KubeVela + CNPG Kubernetes deployment, and release verification checklist.

- **[Onboarding Checklist for New Developers](./getting-started/onboarding.md)**
  Day-1 through Day-5 checklist covering access provisioning, local setup, codebase orientation, first commit, and definition-of-done for onboarding sign-off.

- **[Domain Model](./architecture/domain-model.md)**
  DDD aggregates, value objects, repository interfaces, assessment-period logic, and supervisor-chain access-control.

- **[API Reference](./api/endpoints.md)**
  Full REST endpoint catalogue with methods, paths, auth requirements, and example request / response bodies.

- **[Makefile Reference](./development/makefile.md)**
  All make targets with descriptions, expected output, and environment-variable overrides.

- **[Code Review Findings](./development/code-review.md)**
  Staff-engineer paired-review findings (Nov 2025), issue status tracker, and recommended patterns going forward.
