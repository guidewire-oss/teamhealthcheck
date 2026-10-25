# Team360 Health Check Application

## Skills Available

Use these slash commands for detailed guides on specific workflows:
- `/sparc` — SPARC development environment, Claude-Flow swarm coordination, agent execution patterns, MCP tools
- `/write-e2e-test` — TRUE E2E test writing with Playwright + Ginkgo, templates, and TDD workflow
- `/deploy-kubevela` — KubeVela + CNPG Kubernetes deployment guide

## File Organization Rules

**Project source directories:**
- `/frontend` — Next.js frontend application
- `/backend` — Go backend application
- `/tests` — End-to-end acceptance tests (spans both frontend and backend)
- `/docs` — Additional documentation
- `/scripts` — Utility scripts

**CLAUDE.md is an exception** — It lives in the root and contains project documentation for Claude Code.

**Important**: E2E acceptance tests live in `/tests/acceptance/` (not `/backend/tests/acceptance/`) because they test the complete application stack (frontend + backend + database). Backend unit and integration tests remain in `/backend/` subdirectories.

## Project Overview

Team360 is an open-source (Apache 2.0 licensed) web application based on Spotify's Squad Health Check Model, enhanced with organizational flexibility and hierarchical management capabilities.

**Architecture**: Next.js 15 frontend with Go backend (Gin framework), following Domain-Driven Design (DDD) principles and Test-Driven Development (TDD) practices.

**Core Philosophy** (from Spotify):
- Teams are intrinsically motivated and want to succeed
- Health checks are a **support tool**, not a performance evaluation mechanism
- "Red" doesn't mean "bad team" — it means "this team needs support in this area"
- Focus on building team self-awareness and driving targeted improvements

**Enhancements Over Spotify's Model**:
1. Hierarchical organization support (VP → Director → Manager → Team Lead → Team Member)
2. 11 configurable health dimensions (expanded from ~8)
3. Flexible cadences (weekly, biweekly, monthly, quarterly)
4. Assessment periods for trend tracking
5. Role-based access control
6. Aggregated insights for managers/executives
7. Digital-first for distributed teams

## Technology Stack

**Frontend**:
- Next.js 15 with App Router, TypeScript (strict), Tailwind CSS
- Recharts (charts), Lucide React (icons), React Hook Form, js-cookie (auth)
- Client-side state with localStorage (migrating to API calls)

**Backend** (In Development):
- Go 1.25+, Gin framework, Domain-Driven Design
- Ginkgo v2 (BDD testing) + Gomega (assertions)
- PostgreSQL (recommended), RESTful JSON API at `/api/v1/`

## Backend Architecture (DDD)

```
backend/
├── domain/           # Entities, value objects, domain services
├── application/      # Use cases (commands/ and queries/)
├── infrastructure/   # Repositories, external services, persistence
├── interfaces/       # API controllers (api/v1/), DTOs, middleware
└── tests/            # Integration tests
```

**Aggregates**: User, Team, HealthCheckSession, Organization
**Value Objects**: HierarchyLevel, HealthDimension, HealthCheckResponse

## Development Commands

**Frontend**:
```bash
npm install && npm run dev    # http://localhost:3000
npm run build && npm start    # Production
```

**Backend**:
```bash
cd backend
go mod download
go run cmd/api/main.go        # http://localhost:8080
ginkgo -v ./...               # Run all tests
```

**Root Makefile**:
```bash
make dev      # Run both frontend and backend
make install  # Install all dependencies
make build    # Build both services
make test     # Run all tests
```

**Mac ARM64 SWC fix**:
```bash
npm cache clean --force && rm -rf node_modules package-lock.json .next && npm install
```

## Authentication & Demo Credentials

Cookie-based using js-cookie. All demo passwords are "demo" except admin ("admin").

- VP: `vp/demo`
- Directors: `director1/demo`, `director2/demo`
- Managers: `manager1/demo`, `manager2/demo`, `manager3/demo`
- Team Leads: `teamlead1/demo` through `teamlead9/demo`
- Team Members: `demo/demo`, `alice/demo`, etc.
- Admin: `admin/admin`

## Route Structure

- `/` — Landing page (public)
- `/login` — Authentication (public)
- `/survey` — Health check survey (Team Members and up) — 11 questions, auto period detection
- `/dashboard` — Team Lead dashboard
- `/manager` — Manager/Director/VP dashboard with team filtering and analytics
- `/admin` — System administration (Admin only)

`frontend/middleware.ts` handles route protection and role-based redirects.

## Core Data Models

Located in `frontend/lib/types.ts`:

1. **Hierarchy System**: `HierarchyLevel`, `OrganizationConfig` — configurable levels with granular permissions
2. **Teams & Users**: `User` (hierarchyLevelId, reportsTo, teamIds), `Team` (supervisorChain, members, cadence)
3. **Health Checks**:
   - `HealthDimension`: 11 dimensions (Mission, Delivering Value, Speed, Fun, Health of Codebase, Learning, Support, Pawns or Players, Easy to Release, Suitable Process, Teamwork)
   - `HealthCheckSession`: User responses with assessmentPeriod
   - `HealthCheckResponse`: Score (1=red, 2=yellow, 3=green), trend, optional comment

## Assessment Period Logic

Automatic detection in `frontend/lib/assessment-period.ts`:
- Jan 1 – Jun 30 → "previous year - 2nd Half"
- Jul 1 – Dec 31 → "current year - 1st Half"

## Supervisor Chain & Access Control

Teams have `supervisorChain` arrays defining full reporting hierarchy. Access control in `frontend/lib/org-config.ts`:
- `getUserPermissions()` — permissions based on hierarchy level
- `canUserAccessTeam()` — checks access via permissions, membership, or supervisor chain
- `getSubordinates()` — recursively gets all reports

## Key Data Files

- `frontend/lib/auth.ts` — Authentication logic, USERS array (45+ demo users)
- `frontend/lib/data.ts` — HEALTH_DIMENSIONS (11), TEAMS (9 squads), health check sessions
- `frontend/lib/teams-data.ts` — Extended TEAMS_DATA with supervisor chains, mock sessions
- `frontend/lib/org-config.ts` — Organization hierarchy configuration and permissions
- `frontend/lib/assessment-period.ts` — Auto assessment period detection

## Data Architecture

- **Health check sessions**: `localStorage` key `healthCheckSessions`
- **Organization config**: `localStorage` key `orgConfig`
- **User authentication**: Cookies via js-cookie (1 day expiry)
- `TEAM_ASSIGNMENTS_VERSION` in `frontend/lib/data.ts` — increment when changing manager-team assignments

## Data Visualization

Uses Recharts: RadarChart (team health), BarChart (distributions), LineChart (trends).
Color scheme: Red (#EF4444), Yellow (#F59E0B), Green (#10B981).

## Common Development Patterns

1. **New health dimensions**: Update `HEALTH_DIMENSIONS` in `frontend/lib/data.ts`
2. **New hierarchy levels**: Use `addHierarchyLevel()` / `updateHierarchyLevel()` in `frontend/lib/org-config.ts`
3. **Access control**: Always check with `getUserPermissions()` and `canUserAccessTeam()`
4. **Data persistence**: Update localStorage when modifying sessions or config
5. **Mock data**: See `generateMockHealthSessions()` in `frontend/lib/teams-data.ts`
6. **Assessment periods**: Use `getAssessmentPeriod()` from `frontend/lib/assessment-period.ts`

## Design Principles

1. **Support, Not Surveillance**: Features help teams improve, never for performance reviews
2. **Flexibility First**: Support customization (dimensions, cadences, hierarchies)
3. **Trust-Based Design**: No features that incentivize gaming metrics
4. **Privacy Conscious**: Protect contributor anonymity in aggregates
5. **Accessibility**: Simple demo with localStorage for easy onboarding

## Backend Migration Roadmap

1. ~~Phase 1: Go backend DDD structure~~ COMPLETE
2. Phase 2: Domain models with Ginkgo tests
3. Phase 3: API endpoints with TDD
4. Phase 4: PostgreSQL persistence
5. Phase 5: Frontend → API migration
6. Phase 6: JWT/session auth
7. Phase 7: CSV/Excel export
8. Phase 8: Email/Slack notifications

## Code Style

- Files under 500 lines
- Never hardcode secrets
- Test-first (TDD with Ginkgo/Gomega)
- Clean architecture (DDD layers)
- TypeScript path alias: `@/*`

# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.
Never save working files, text/mds and tests to the root folder (except CLAUDE.md which is project documentation).
