# Domain Model

The Go backend follows **Domain-Driven Design (DDD)** with clear separation of concerns:

```
backend/
├── cmd/api/            # Application entry point
├── domain/            # Domain layer (entities, value objects, domain services)
│   ├── user/          # User aggregate
│   ├── team/          # Team aggregate
│   ├── healthcheck/   # Health check aggregate
│   └── organization/  # Organization aggregate
├── application/       # Application layer (use cases)
│   ├── commands/      # Command handlers (write operations)
│   └── queries/       # Query handlers (read operations)
├── infrastructure/    # Infrastructure layer
│   ├── persistence/   # PostgreSQL implementations (database/sql + lib/pq)
│   └── email/         # SES/SMTP email sending
├── interfaces/        # Interface layer
│   ├── api/v1/        # Gin HTTP handlers (API version 1)
│   ├── dto/           # Data Transfer Objects
│   └── middleware/    # Gin middleware
└── tests/             # Integration and acceptance tests
    └── acceptance/    # Ginkgo acceptance tests
```

**Key DDD concepts:**

- **Aggregates**: `User`, `Team`, `HealthCheckSession`, `OrganizationConfig` are aggregate roots
- **Value Objects**: `HierarchyLevel`, `HealthDimension`, `HealthCheckResponse`
- **Repositories**: Abstract data access with domain-focused interfaces
- **Domain Services**: Cross-aggregate business logic

## Test-Driven Development

We follow **outer-loop TDD** with Ginkgo:

1. Write acceptance tests first (describes user behavior)
2. Implement domain logic to make tests pass
3. Refactor with confidence

## Core Data Models

Located in `frontend/lib/types.ts`:

1. **Hierarchy System** — configurable organizational levels with granular permissions
   - `HierarchyLevel`: Defines levels (VP, Director, Manager, Team Lead, Team Member)
   - `OrganizationConfig`: Company-wide hierarchy configuration
   - Each level has specific permissions (canViewAllTeams, canEditTeams, etc.)

2. **Teams & Users**
   - `User`: Has hierarchyLevelId, reportsTo (supervisor), and teamIds (can be in multiple teams)
   - `Team`: Has supervisorChain (full chain of supervisors), members, cadence (survey frequency)

3. **Health Checks**
   - `HealthDimension`: 11 dimensions based on Spotify's model with Team Health Check enhancements:
     - **From Spotify's model**: Mission, Delivering Value, Speed, Fun, Health of Codebase, Learning, Support, Pawns or Players
     - **Team Health Check additions**: Easy to Release, Suitable Process, Teamwork
     - Each dimension has goodDescription/badDescription for clarity (e.g., "We deliver great stuff!" vs "We deliver crap")
     - Dimensions can be enabled/disabled and weighted via isActive and weight properties
   - `HealthCheckSession`: User's responses to health check, includes assessmentPeriod (e.g., "2026 H1", or "2024 - 1st Half" for legacy sessions)
   - `HealthCheckResponse`: Score (1=red, 2=yellow, 3=green), trend (improving/stable/declining), optional comment

## Assessment Period Logic

Assessment periods are cadence-driven, computed by `frontend/lib/assessment-period.ts`. The format depends on the team's configured cadence:

| Cadence | Format | Example |
|---------|--------|---------|
| Monthly | `YYYY Mon` | `2026 Mar` |
| Quarterly | `YYYY Q#` | `2026 Q1` |
| Half-yearly (default) | `YYYY H#` | `2026 H1` |
| Yearly | `YYYY` | `2026` |

```typescript
export function getAssessmentPeriod(date?: Date | string, cadence?: Cadence): string {
  const d = date ? (typeof date === 'string' ? new Date(date) : date) : new Date();
  const year = d.getFullYear();
  const month = d.getMonth(); // 0-indexed

  switch (cadence) {
    case 'monthly':   return `${year} ${MONTH_NAMES[month]}`;
    case 'quarterly': return `${year} Q${Math.floor(month / 3) + 1}`;
    case 'yearly':    return `${year}`;
    case 'half-yearly':
    default:          return `${year} H${month < 6 ? 1 : 2}`;
  }
}
```

**Legacy format:** Sessions created before cadence-driven periods used the fixed half-year string `"YYYY - 1st/2nd Half"`, which is still parsed (but no longer generated) for backward compatibility:

- `"YYYY - 1st Half"` covers Jul–Dec of `YYYY` (equivalent to `YYYY H2`)
- `"YYYY - 2nd Half"` covers Jan–Jun of `YYYY + 1` (equivalent to `(YYYY + 1) H1`) — **not** Jan–Jun of `YYYY`

See `parseAssessmentPeriod()` / `compareAssessmentPeriods()` in `frontend/lib/assessment-period.ts` for the full parsing and chronological-sort logic.
