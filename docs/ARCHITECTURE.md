# Team Health Check Architecture Documentation

This document provides a comprehensive overview of Team Health Check's architecture, including system design, data flow, database schema, and technology decisions.

## Table of Contents

1. [System Overview](#system-overview)
2. [Technology Stack](#technology-stack)
3. [Architecture Diagram](#architecture-diagram)
4. [Frontend Architecture](#frontend-architecture)
5. [Backend Architecture](#backend-architecture)
6. [Database Schema](#database-schema)
7. [Data Flow](#data-flow)
8. [API Design](#api-design)
9. [Security & Authentication](#security--authentication)
10. [Testing Strategy](#testing-strategy)

---

## System Overview

Team Health Check is a full-stack web application built with a **decoupled frontend-backend architecture**:

```
┌─────────────────┐     HTTP/JSON     ┌─────────────────┐     SQL      ┌─────────────────┐
│   Next.js 15    │ ◄──────────────► │   Go/Gin API    │ ◄──────────► │   PostgreSQL    │
│   (Frontend)    │     REST API      │   (Backend)     │    Queries   │   (Database)    │
│   Port 3000     │                   │   Port 8080     │              │   Port 5432     │
└─────────────────┘                   └─────────────────┘              └─────────────────┘
```

### Why This Architecture?

| Decision | Rationale |
|----------|-----------|
| **Decoupled frontend/backend** | Independent deployment, scaling, and technology evolution |
| **Next.js for frontend** | Server-side rendering, App Router, excellent DX, React ecosystem |
| **Go for backend** | Performance, type safety, excellent concurrency, simple deployment |
| **PostgreSQL** | ACID compliance, complex queries for aggregations, JSON support |
| **REST API** | Simplicity, cacheability, wide tooling support |

---

## Technology Stack

### Frontend

| Technology | Version | Purpose |
|------------|---------|---------|
| **Next.js** | 15.x | React framework with App Router |
| **TypeScript** | 5.x | Type safety and developer experience |
| **Tailwind CSS** | 3.x | Utility-first styling |
| **Recharts** | 2.x | Data visualization (radar, bar, line charts) |
| **Lucide React** | Latest | Icon library |
| **js-cookie** | Latest | Cookie-based authentication |

**Why Next.js?**
- **App Router**: File-based routing with layouts and loading states
- **API Routes**: Proxy layer to backend (avoids CORS in development)
- **Server Components**: Improved performance for static content
- **Built-in Optimization**: Image optimization, code splitting, prefetching

### Backend

| Technology | Version | Purpose |
|------------|---------|---------|
| **Go** | 1.25+ | Application language |
| **Gin** | Latest | HTTP web framework |
| **pgx** | v5 | PostgreSQL driver |
| **golang-migrate** | Latest | Database migrations |

**Why Go?**
- **Performance**: Compiled language with low memory footprint
- **Simplicity**: Easy to read, maintain, and onboard new developers
- **Concurrency**: Goroutines for handling many concurrent requests
- **Single Binary**: Simple deployment without runtime dependencies
- **Strong Typing**: Catches errors at compile time

### Database

| Technology | Version | Purpose |
|------------|---------|---------|
| **PostgreSQL** | 14+ | Primary data store |

**Why PostgreSQL?**
- **Complex Queries**: Aggregations for health metrics across teams
- **ACID Transactions**: Data integrity for survey submissions
- **Foreign Keys**: Referential integrity for hierarchical data
- **JSON Support**: Flexible data storage when needed
- **Mature Ecosystem**: Excellent tooling and community support

### Testing

| Technology | Purpose |
|------------|---------|
| **Ginkgo v2** | BDD-style test framework for Go |
| **Gomega** | Matcher/assertion library |
| **Playwright** | Browser automation for E2E tests |

---

## Architecture Diagram

### High-Level Component Diagram

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                    FRONTEND                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                           Next.js 15 (App Router)                            │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│  │  │   /login    │  │   /survey   │  │  /dashboard │  │     /manager        │ │ │
│  │  │  (Auth UI)  │  │ (Team Mbr)  │  │ (Team Lead) │  │ (Mgr/Dir/VP View)   │ │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌───────────────────────────────────────┐ │ │
│  │  │   /home     │  │   /admin    │  │              /api/v1/*                │ │ │
│  │  │ (Mbr Home)  │  │ (Admin UI)  │  │         (Proxy to Backend)            │ │ │
│  │  └─────────────┘  └─────────────┘  └───────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        │ HTTP/JSON (REST)
                                        ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                    BACKEND                                        │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                              Gin HTTP Router                                 │ │
│  │                          /api/v1/* endpoints                                 │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
│                                        │                                          │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                          INTERFACES LAYER (API)                              │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│  │  │ AuthHandler │  │ HealthCheck │  │TeamDashboard│  │   ManagerHandler    │ │ │
│  │  │             │  │  Handler    │  │   Handler   │  │                     │ │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│  │  │ UserHandler │  │ TeamHandler │  │HierarchyAdm │  │ SettingsAdminHdlr   │ │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
│                                        │                                          │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                            DOMAIN LAYER (DDD)                                │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│  │  │    User     │  │    Team     │  │ HealthCheck │  │    Organization     │ │ │
│  │  │ (Aggregate) │  │ (Aggregate) │  │ (Aggregate) │  │    (Aggregate)      │ │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
│                                        │                                          │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                        INFRASTRUCTURE LAYER                                  │ │
│  │  ┌───────────────────────────────────────────────────────────────────────┐  │ │
│  │  │                    PostgreSQL Repositories                             │  │ │
│  │  │  UserRepo │ TeamRepo │ HealthCheckRepo │ OrganizationRepo              │  │ │
│  │  └───────────────────────────────────────────────────────────────────────┘  │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        │ SQL (pgx driver)
                                        ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                                  DATABASE                                         │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                              PostgreSQL 14+                                  │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│  │  │    users    │  │    teams    │  │health_check_│  │  hierarchy_levels   │ │ │
│  │  │             │  │             │  │  sessions   │  │                     │ │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│  │  │team_members │  │   team_     │  │health_check_│  │  health_dimensions  │ │ │
│  │  │             │  │ supervisors │  │  responses  │  │                     │ │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## Frontend Architecture

### Directory Structure

```
frontend/
├── app/                      # Next.js App Router
│   ├── admin/               # Admin dashboard page
│   ├── api/v1/              # API proxy routes to backend
│   │   ├── admin/           # Admin API proxies
│   │   ├── auth/            # Auth API proxies
│   │   └── ...
│   ├── dashboard/           # Team Lead dashboard
│   ├── home/                # Team Member home page
│   ├── login/               # Authentication page
│   ├── manager/             # Manager/Director/VP dashboard
│   ├── survey/              # Health check survey
│   └── teams/               # Team listing
├── components/              # Reusable React components
│   ├── DimensionConfig.tsx  # Health dimension management
│   ├── HierarchyConfig.tsx  # Hierarchy level management
│   └── ...
├── lib/                     # Utilities and shared code
│   ├── api/                 # API client functions
│   │   └── admin.ts         # Admin API client
│   ├── auth.ts              # Authentication utilities
│   ├── types.ts             # TypeScript type definitions
│   └── ...
└── middleware.ts            # Route protection middleware
```

### Page Routing by Role

```
┌─────────────────────────────────────────────────────────────────┐
│                        User Roles & Routes                       │
├─────────────────┬───────────────────────────────────────────────┤
│ Role            │ Primary Route      │ Capabilities             │
├─────────────────┼────────────────────┼──────────────────────────┤
│ Team Member     │ /home              │ Take survey, view history│
│ Team Lead       │ /dashboard         │ Team health, trends      │
│ Manager         │ /manager           │ Multi-team view          │
│ Director        │ /manager           │ Department-wide view     │
│ VP              │ /manager           │ Organization-wide view   │
│ Admin           │ /admin             │ System configuration     │
└─────────────────┴────────────────────┴──────────────────────────┘
```

### API Proxy Pattern

Next.js API routes act as a proxy to the Go backend, solving CORS issues in development:

```typescript
// frontend/app/api/v1/admin/teams/route.ts
export async function GET() {
  const response = await fetch(`${BACKEND_URL}/api/v1/admin/teams`);
  return Response.json(await response.json());
}
```

**Benefits:**
- No CORS configuration needed in development
- Same-origin requests from browser
- Can add frontend-specific logic (caching, auth)

---

## Backend Architecture

### Domain-Driven Design (DDD)

The backend follows DDD principles with clear layer separation:

```
backend/
├── cmd/api/                 # Application entry point
│   └── main.go             # Server bootstrap, route registration
├── domain/                  # DOMAIN LAYER - Business logic
│   ├── user/               # User aggregate
│   │   └── user.go         # User entity, repository interface
│   ├── team/               # Team aggregate
│   │   └── team.go         # Team entity, repository interface
│   ├── healthcheck/        # HealthCheck aggregate
│   │   └── healthcheck.go  # Session entity, repository interface
│   └── organization/       # Organization aggregate
│       └── organization.go # HierarchyLevel, Dimension entities
├── application/             # APPLICATION LAYER - Use cases
│   ├── commands/           # Write operations
│   └── queries/            # Read operations
├── infrastructure/          # INFRASTRUCTURE LAYER - External concerns
│   └── persistence/
│       └── postgres/       # PostgreSQL implementations
│           ├── migrations/ # SQL migration files
│           ├── user_repository.go
│           ├── team_repository.go
│           ├── health_check_repository.go
│           └── organization_repository.go
└── interfaces/              # INTERFACES LAYER - API handlers
    ├── api/v1/             # HTTP handlers
    │   ├── auth_handler.go
    │   ├── health_check_handler.go
    │   ├── manager_handler.go
    │   └── ...
    └── dto/                # Data Transfer Objects
        └── admin_dto.go
```

### DDD Aggregates

| Aggregate | Root Entity | Value Objects | Repository |
|-----------|-------------|---------------|------------|
| **User** | `User` | - | `UserRepository` |
| **Team** | `Team` | `TeamMember`, `SupervisorLink` | `TeamRepository` |
| **HealthCheck** | `HealthCheckSession` | `HealthCheckResponse` | `HealthCheckRepository` |
| **Organization** | `OrganizationConfig` | `HierarchyLevel`, `HealthDimension`, `Permissions` | `OrganizationRepository` |

### Repository Pattern

Each domain defines a repository interface; infrastructure provides implementations:

```go
// domain/user/user.go - Interface definition
type Repository interface {
    FindByID(ctx context.Context, id string) (*User, error)
    FindByUsername(ctx context.Context, username string) (*User, error)
    Save(ctx context.Context, user *User) error
    // ...
}

// infrastructure/persistence/postgres/user_repository.go - Implementation
type PostgresUserRepository struct {
    db *pgxpool.Pool
}

func (r *PostgresUserRepository) FindByID(ctx context.Context, id string) (*User, error) {
    // SQL query implementation
}
```

---

## Database Schema

### Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DATABASE SCHEMA                                 │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────┐       ┌─────────────────────┐
│  hierarchy_levels   │       │  health_dimensions  │
├─────────────────────┤       ├─────────────────────┤
│ id (PK)             │       │ id (PK)             │
│ name                │       │ name                │
│ position            │       │ description         │
│ color               │       │ good_description    │
│ can_view_all_teams  │       │ bad_description     │
│ can_edit_teams      │       │ is_active           │
│ can_manage_users    │       │ weight              │
│ can_take_survey     │       │ created_at          │
│ can_view_analytics  │       │ updated_at          │
│ created_at          │       └─────────────────────┘
│ updated_at          │                │
└─────────────────────┘                │
         │                             │
         │ FK                          │ FK
         ▼                             ▼
┌─────────────────────┐       ┌─────────────────────┐
│       users         │       │health_check_responses│
├─────────────────────┤       ├─────────────────────┤
│ id (PK)             │       │ id (PK, SERIAL)     │
│ username (UNIQUE)   │       │ session_id (FK)     │──┐
│ email (UNIQUE)      │       │ dimension_id (FK)   │──┘
│ full_name           │       │ score (1-3)         │
│ hierarchy_level_id  │──┐    │ trend               │
│ reports_to (FK)     │◄─┘    │ comment             │
│ password_hash       │       │ created_at          │
│ created_at          │       └─────────────────────┘
│ updated_at          │                ▲
└─────────────────────┘                │ FK
         │                             │
         │                    ┌─────────────────────┐
         │                    │health_check_sessions│
         ▼                    ├─────────────────────┤
┌─────────────────────┐       │ id (PK)             │
│       teams         │       │ team_id             │──┐
├─────────────────────┤       │ user_id             │  │
│ id (PK)             │◄──────│ date                │  │
│ name                │       │ assessment_period   │  │
│ team_lead_id (FK)   │───────│ completed           │  │
│ cadence             │       │ created_at          │  │
│ created_at          │       │ updated_at          │  │
│ updated_at          │       └─────────────────────┘  │
└─────────────────────┘                                │
         │                                             │
         │                                             │
    ┌────┴────┐                                        │
    ▼         ▼                                        │
┌─────────────────────┐       ┌─────────────────────┐  │
│   team_members      │       │  team_supervisors   │  │
├─────────────────────┤       ├─────────────────────┤  │
│ team_id (PK, FK)    │       │ team_id (PK, FK)    │  │
│ user_id (PK, FK)    │       │ user_id (PK, FK)    │  │
│ joined_at           │       │ hierarchy_level_id  │  │
└─────────────────────┘       │ position            │  │
                              └─────────────────────┘  │
                                                       │
              ┌────────────────────────────────────────┘
              │ (team_id references teams.id)
              │ (user_id references users.id)
              ▼
```

### Table Descriptions

| Table | Purpose | Key Relationships |
|-------|---------|-------------------|
| `hierarchy_levels` | Organizational levels (VP, Director, etc.) with permissions | Referenced by `users.hierarchy_level_id` |
| `users` | User accounts with authentication and hierarchy | Self-referential `reports_to`, FK to `hierarchy_levels` |
| `teams` | Team definitions with cadence settings | FK to `users` for team lead |
| `team_members` | Many-to-many: users ↔ teams | Junction table |
| `team_supervisors` | Denormalized supervisor chain for performance | Links team to supervisor hierarchy |
| `health_dimensions` | 11 health check dimensions | Referenced by responses |
| `health_check_sessions` | Survey submissions | FK to `teams`, `users` |
| `health_check_responses` | Individual dimension scores | FK to `sessions`, `dimensions` |

### Key Indexes

```sql
-- Performance indexes for common query patterns
CREATE INDEX idx_users_reports_to ON users(reports_to);
CREATE INDEX idx_users_hierarchy_level ON users(hierarchy_level_id);
CREATE INDEX idx_sessions_team_date ON health_check_sessions(team_id, date DESC);
CREATE INDEX idx_sessions_assessment_period ON health_check_sessions(assessment_period);
CREATE INDEX idx_responses_session_dimension ON health_check_responses(session_id, dimension_id);
```

---

## Data Flow

### Survey Submission Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Team Member │     │   Next.js   │     │   Go API    │     │ PostgreSQL  │
│   Browser   │     │   Frontend  │     │   Backend   │     │  Database   │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │                   │
       │ 1. Fill survey    │                   │                   │
       │   (11 dimensions) │                   │                   │
       │ ──────────────────>                   │                   │
       │                   │                   │                   │
       │                   │ 2. POST /api/v1/  │                   │
       │                   │    health-checks  │                   │
       │                   │ ──────────────────>                   │
       │                   │                   │                   │
       │                   │                   │ 3. INSERT session │
       │                   │                   │    + responses    │
       │                   │                   │ ──────────────────>
       │                   │                   │                   │
       │                   │                   │ 4. Return session │
       │                   │                   │ <──────────────────
       │                   │                   │                   │
       │                   │ 5. Return success │                   │
       │                   │ <──────────────────                   │
       │                   │                   │                   │
       │ 6. Redirect to    │                   │                   │
       │    /home          │                   │                   │
       │ <──────────────────                   │                   │
       │                   │                   │                   │
```

### Manager Dashboard Data Flow

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Manager   │     │   Next.js   │     │   Go API    │     │ PostgreSQL  │
│   Browser   │     │   Frontend  │     │   Backend   │     │  Database   │
└──────┬──────┘     └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
       │                   │                   │                   │
       │ 1. Load /manager  │                   │                   │
       │ ──────────────────>                   │                   │
       │                   │                   │                   │
       │                   │ 2. GET /managers/ │                   │
       │                   │    {id}/teams/    │                   │
       │                   │    health         │                   │
       │                   │ ──────────────────>                   │
       │                   │                   │                   │
       │                   │                   │ 3. Query:         │
       │                   │                   │ - Find supervised │
       │                   │                   │   teams           │
       │                   │                   │ - Aggregate scores│
       │                   │                   │   by dimension    │
       │                   │                   │ - Join team names │
       │                   │                   │ ──────────────────>
       │                   │                   │                   │
       │                   │                   │ 4. Return         │
       │                   │                   │    aggregated     │
       │                   │                   │    health data    │
       │                   │                   │ <──────────────────
       │                   │                   │                   │
       │                   │ 5. Return JSON    │                   │
       │                   │ <──────────────────                   │
       │                   │                   │                   │
       │ 6. Render charts  │                   │                   │
       │    (Recharts)     │                   │                   │
       │ <──────────────────                   │                   │
       │                   │                   │                   │
```

---

## API Design

### RESTful Endpoints

| Method | Endpoint | Purpose | Auth Required |
|--------|----------|---------|---------------|
| **Authentication** ||||
| POST | `/api/v1/auth/login` | User login | No |
| **Health Checks** ||||
| POST | `/api/v1/health-checks` | Submit survey | Yes |
| GET | `/api/v1/health-checks/:id` | Get session by ID | Yes |
| GET | `/api/v1/health-dimensions` | List all dimensions | Yes |
| **Teams** ||||
| GET | `/api/v1/teams` | List teams | Yes |
| GET | `/api/v1/teams/:teamId/info` | Get team details | Yes |
| GET | `/api/v1/teams/:teamId/dashboard/health-summary` | Team health summary | Yes |
| GET | `/api/v1/teams/:teamId/dashboard/trends` | Team health trends | Yes |
| **Managers** ||||
| GET | `/api/v1/managers/:managerId/teams/health` | Supervised teams health | Yes |
| GET | `/api/v1/managers/:managerId/dashboard/trends` | Aggregated trends | Yes |
| GET | `/api/v1/managers/:managerId/dashboard/radar` | Radar chart data | Yes |
| **Users** ||||
| GET | `/api/v1/users/:userId/survey-history` | User's survey history | Yes |
| **Admin - Hierarchy** ||||
| GET | `/api/v1/admin/hierarchy-levels` | List hierarchy levels | Admin |
| POST | `/api/v1/admin/hierarchy-levels` | Create hierarchy level | Admin |
| PUT | `/api/v1/admin/hierarchy-levels/:id` | Update hierarchy level | Admin |
| DELETE | `/api/v1/admin/hierarchy-levels/:id` | Delete hierarchy level | Admin |
| **Admin - Teams** ||||
| GET | `/api/v1/admin/teams` | List all teams | Admin |
| POST | `/api/v1/admin/teams` | Create team | Admin |
| PUT | `/api/v1/admin/teams/:id` | Update team | Admin |
| DELETE | `/api/v1/admin/teams/:id` | Delete team | Admin |
| **Admin - Users** ||||
| GET | `/api/v1/admin/users` | List all users | Admin |
| POST | `/api/v1/admin/users` | Create user | Admin |
| PUT | `/api/v1/admin/users/:id` | Update user | Admin |
| DELETE | `/api/v1/admin/users/:id` | Delete user | Admin |
| **Admin - Settings** ||||
| GET | `/api/v1/admin/settings/dimensions` | List dimensions | Admin |
| POST | `/api/v1/admin/settings/dimensions` | Create dimension | Admin |
| PUT | `/api/v1/admin/settings/dimensions/:id` | Update dimension | Admin |
| DELETE | `/api/v1/admin/settings/dimensions/:id` | Delete dimension | Admin |

### Request/Response Examples

**Submit Health Check:**
```json
// POST /api/v1/health-checks
{
  "teamId": "platform-squad",
  "userId": "alice",
  "responses": [
    { "dimensionId": "mission", "score": 3, "trend": "stable", "comment": "" },
    { "dimensionId": "value", "score": 2, "trend": "improving", "comment": "Getting better" }
  ]
}

// Response
{
  "id": "session-12345",
  "teamId": "platform-squad",
  "userId": "alice",
  "date": "2025-11-27",
  "assessmentPeriod": "2025 - 1st Half",
  "completed": true
}
```

---

## Security & Authentication

### Current Implementation

| Aspect | Implementation |
|--------|----------------|
| **Authentication** | Cookie-based session with user ID |
| **Password Storage** | Bcrypt hashed in database |
| **Route Protection** | Next.js middleware checks cookie |
| **API Protection** | Handler extracts user from context |

### Authentication Flow

```
1. User submits credentials → POST /api/v1/auth/login
2. Backend verifies password hash → bcrypt.CompareHashAndPassword
3. On success, returns user data with set-cookie header
4. Frontend stores user ID in cookie (js-cookie)
5. Subsequent requests include cookie
6. Middleware validates cookie, redirects if missing
```

### Future Improvements (Roadmap)

- [ ] JWT tokens for stateless authentication
- [ ] Refresh token rotation
- [ ] Role-based API middleware
- [ ] Rate limiting
- [ ] HTTPS enforcement

---

## Testing Strategy

### Test Pyramid

```
        ┌───────────────┐
        │    E2E Tests  │  ◄── 68 tests (Playwright + Ginkgo)
        │   (Slowest)   │      Full stack: Browser → API → DB
        └───────────────┘
       ┌─────────────────┐
       │ Integration     │  ◄── Repository tests, API handler tests
       │    Tests        │      Test with real database
       └─────────────────┘
      ┌───────────────────┐
      │    Unit Tests     │  ◄── Domain logic, value objects
      │    (Fastest)      │      No external dependencies
      └───────────────────┘
```

### E2E Test Coverage

| Test Suite | Tests | Coverage |
|------------|-------|----------|
| Admin Dashboard | 30 | Hierarchy, Teams, Users, Dimensions CRUD |
| Authentication | 4 | Login flows, redirects |
| Survey Submission | 6 | Complete survey workflow |
| Member Home | 8 | Survey history, visualizations |
| Manager Dashboard | 10 | Team health, trends, radar |
| VP Dashboard | 10 | Organization-wide views |

### Running Tests

```bash
# Run all E2E tests
cd tests
export TEST_DATABASE_URL="postgres://..."
ginkgo -v acceptance/

# Run specific test suite
ginkgo -v -focus="E2E: Admin" acceptance/

# Run backend unit tests
cd backend
go test ./...
```

---

## Appendix: Migration History

| Migration | Purpose |
|-----------|---------|
| 000001 | Create health_dimensions table |
| 000002 | Create health_check_sessions table |
| 000003 | Create health_check_responses table |
| 000004 | Seed 11 health dimensions |
| 000005 | Create users, teams, team_members, team_supervisors |
| 000006 | Add password_hash to users |
| 000007 | Seed demo users |
| 000008 | Seed demo teams |
| 000009 | Add cadence to teams |
| 000010 | Create hierarchy_levels with permissions |
| 000011+ | Additional schema refinements |
