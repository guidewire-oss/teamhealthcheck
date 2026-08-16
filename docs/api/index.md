# API Reference

Every route below was read from `backend/interfaces/api/v1/*_routes.go` and
`backend/cmd/api/main.go`. Nothing here is inferred.

## Base URL and versioning

All application routes are prefixed `/api/v1`. The only exception is `GET /health`,
registered at the root in `main.go`.

## Response conventions

There is **no response envelope**, despite the helper names in
`backend/interfaces/dto/responses.go`:

| Helper | Emits |
|--------|-------|
| `dto.RespondSuccess(c, status, data)` | `data` verbatim at the top level |
| `dto.RespondError(c, status, msg)` | `{"error": msg}` |
| `dto.RespondErrorWithDetails(c, status, msg, details)` | `{"error": msg, "message": details}` |
| `dto.RespondMessage(c, status, msg)` | `{"message": msg}` |
| `dto.RespondList(c, status, items, total)` | `{"items": ..., "total": ...}` — unused |

The error type is `dto.ErrorResponse`:

```go
type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message,omitempty"`
    Code    string `json:"code,omitempty"`
}
```

!!! warning "There are no machine-readable error codes"
    The `code` field exists on the struct but **no handler in the codebase ever sets it**,
    and there is no `errorType` field. Clients must match on the human-readable `error`
    string. The `message` field, when present, is usually the raw underlying error —
    including driver errors, which are passed through to the client unfiltered.

Some handlers use the `dto.Respond*` helpers and others construct `dto.ErrorResponse`
inline with `c.JSON`. The wire format is identical either way.

## Authentication

Guarded routes require:

```
Authorization: Bearer <accessToken>
```

Obtain the token from [`POST /api/v1/auth/login`](auth.md#post-apiv1authlogin). The
guard middleware and the hierarchy levels each one accepts are documented in
[Authentication & Authorization](../architecture/authentication.md). In the tables that
follow, the **Auth** column uses these shorthands:

| Shorthand | Meaning |
|-----------|---------|
| *public* | No middleware |
| `JWT` | `JWTAuthMiddleware` — valid access token required |
| `+Admin` | `AdminOnlyMiddleware` — `level-1` or `level-admin` |
| `+Manager` | `ManagerOrAboveMiddleware` — `level-1`, `level-2`, `level-3`, `level-admin` |
| `+SameUser` | `SameUserOrManagerMiddleware` — self, or `level-1`/`level-2`/`level-admin` |
| `+TeamMember` | `TeamMembershipMiddleware` — team in the `teamIDs` claim, or manager and above |

### Errors common to all guarded routes

| Status | `error` | Cause |
|--------|---------|-------|
| 401 | `Authorization header is required` | Header absent |
| 401 | `Authorization header must be Bearer token` | Malformed header |
| 401 | `Token has expired` | Access token past expiry |
| 401 | `Invalid token` | Bad signature or malformed token |
| 401 | `Authentication failed` | Any other validation error |
| 403 | `Access denied: unable to determine user role` | Claims missing from context |
| 403 | `Access denied: admin privileges required` | Failed `AdminOnly` |
| 403 | `Access denied: manager or above privileges required` | Failed `ManagerOrAbove` |
| 403 | `Access denied: cannot access other user's data` | Failed `SameUserOrManager` |
| 403 | `Access denied: you are not a member of this team` | Failed `TeamMembership` |

### Errors common to all routes

| Status | `error` | Cause |
|--------|---------|-------|
| 415 | `Content-Type must be application/json` | Non-JSON `Content-Type` on POST/PUT/PATCH |
| 404 | `not found` | Unmatched path beginning `/api/` |

Request bodies larger than 10 MB are truncated by `MaxBodySizeMiddleware`, which surfaces
as a bind failure.

## Route index

| Area | Page |
|------|------|
| Login, refresh, logout, SSO, password reset, runtime config | [Authentication](auth.md) |
| Survey submission, dimensions, sessions, assessment periods | [Health Checks](health-checks.md) |
| Team listing, team info, team sessions | [Teams](teams.md) |
| Team lead dashboard aggregates | [Team Dashboard](team-dashboard.md) |
| Manager rollups, radar, trends, subordinates | [Managers](managers.md) |
| Action item CRUD and manager summary | [Action Items](action-items.md) |
| Hierarchy, users, teams, settings | [Admin](admin.md) |

## Non-versioned routes

| Method | Path | Auth | Response |
|--------|------|------|----------|
| GET | `/health` | *public* | `200 {"status": "healthy"}` — static, does not check the database |

## Cross-cutting caveats

!!! warning "Unreachable validation branches"
    Handlers in `manager_handler.go`, `team_dashboard_handler.go`, and
    `GetTeamSubmissionStatus` begin with a `400 ... ID is required` guard against an empty
    path parameter. Gin never routes a request with an empty path segment to these
    handlers, so these branches are dead code. They are listed in the error tables for
    completeness but cannot be triggered.

!!! warning "Submission does not verify the submitting user"
    `POST /api/v1/health-checks` takes `userId` and `teamId` from the request body and
    never compares them against the JWT claims. Any authenticated user can submit a
    health check on behalf of another user, for any team.

!!! warning "List endpoints have no pagination"
    No endpoint anywhere in the API accepts `page`, `limit`, or `offset` — with the sole
    exception of `GET /api/v1/users/:userId/survey-history`, which accepts `limit`
    (default 10). All other list endpoints return the full result set.
