# API Endpoint Reference — v1

Complete index of all registered routes in the Go backend (`backend/cmd/api/main.go`).  
All endpoints are prefixed with `/api/v1/` unless noted otherwise.

> For full request/response examples on Team Lead Dashboard endpoints, see [team-dashboard-api.md](./team-dashboard-api.md).

---

## Auth required?

| Symbol | Meaning |
|--------|---------|
| `—` | Public (no auth) |
| `JWT` | Requires a valid, non-expired JWT access token (`Authorization: Bearer <token>`) |
| `Admin` | `AdminOnlyMiddleware` — requester's hierarchy level must be `level-1` or `level-admin` |
| `Manager+` | `ManagerOrAboveMiddleware` — requester's hierarchy level must be `level-1`, `level-2`, `level-3`, or `level-admin` |

These backend checks are hardcoded hierarchy-level IDs (`backend/interfaces/middleware/jwt_auth.go`), independent of the `hierarchy_levels` permission booleans (`canConfigureSystem`, etc.) that drive the frontend's UI-level RBAC — see [Authentication & Authorization](../getting-started/auth.md).

---

## System

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | — | Liveness probe — returns `{"status":"healthy"}` |
| GET | `/api/v1/config` | — | Returns SSO / OAuth client configuration (client ID, authorize URL, scopes). Used by frontend to render the SSO button. |

---

## Authentication

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/login` | — | Username + password login. Returns JWT access + refresh tokens and the user object. |
| POST | `/api/v1/auth/refresh` | — | Public — takes a refresh token in the body and returns a new access token. Registered without `JWTAuthMiddleware` in `SetupAuthRoutes` (an expired access token is exactly when this is called). |
| POST | `/api/v1/auth/logout` | — | Public — also registered without `JWTAuthMiddleware` in `SetupAuthRoutes`. |
| POST | `/api/v1/auth/sso/callback` | — | Exchange OAuth authorization code for session (PKCE flow). Extracts `email` from ID token and matches to existing user. |
| POST | `/api/v1/auth/forgot-password` | — | Initiate password reset. Accepts `email`; creates a short-lived reset token and sends email. |
| POST | `/api/v1/auth/reset-password` | — | Complete password reset. Accepts `token` + `new_password`; marks token used. |

---

## Users

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/users/me` | JWT | Return the currently authenticated user's profile and permissions. |
| GET | `/api/v1/users/:userId/survey-history` | JWT | Return a user's health check submission history. |

---

## Health Checks & Dimensions

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/health-dimensions` | JWT | List all active health dimensions (11 by default). |
| POST | `/api/v1/health-checks` | JWT | Submit a health check session (11 dimension responses). |
| GET | `/api/v1/health-checks/:id` | JWT | Get a specific health check session by ID. |
| GET | `/api/v1/health-checks/team/:id` | JWT | Get all health check sessions for a specific team. |
| GET | `/api/v1/assessment-periods` | JWT | List all assessment periods that have at least one completed session. |

---

## Teams

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/teams` | JWT | List teams accessible to the authenticated user. |
| GET | `/api/v1/teams/:teamId/info` | JWT | Get team metadata (name, lead, cadence, member count). |
| GET | `/api/v1/teams/:teamId/sessions` | JWT | List all health check sessions for a team. |
| GET | `/api/v1/teams/:teamId/submission-status` | JWT | Check whether the authenticated user has submitted a health check for the current assessment period. |

---

## Team Lead Dashboard

All dashboard endpoints are prefixed with `/api/v1/teams/:teamId/dashboard`.  
See [team-dashboard-api.md](./team-dashboard-api.md) for full request/response examples.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/teams/:teamId/dashboard/health-summary` | JWT | Aggregated average scores per dimension. Feeds the radar chart. Supports `?assessmentPeriod=` filter. |
| GET | `/api/v1/teams/:teamId/dashboard/response-distribution` | JWT | Red / yellow / green count per dimension. Feeds the bar chart. Supports `?assessmentPeriod=` filter. |
| GET | `/api/v1/teams/:teamId/dashboard/individual-responses` | JWT | Individual team member responses with scores, trends, and comments. Supports `?assessmentPeriod=` filter. |
| GET | `/api/v1/teams/:teamId/dashboard/trends` | JWT | Average scores per dimension across all assessment periods. Feeds the line chart. |

---

## Team Action Items

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/teams/:teamId/action-items` | JWT + team member | List action items for a team. |
| POST | `/api/v1/teams/:teamId/action-items` | JWT + team member | Create an action item for a team (linked to optional dimension). |
| PATCH | `/api/v1/teams/:teamId/action-items/:id` | JWT + team member | Update an action item (status, title, description, due date, assignee). |
| DELETE | `/api/v1/teams/:teamId/action-items/:id` | JWT + team member | Delete an action item. |

---

## Managers

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/managers/:managerId/teams/health` | Manager+ | Aggregated health data for all teams supervised by this manager. |
| GET | `/api/v1/managers/:managerId/dashboard/radar` | Manager+ | Radar chart data aggregated across all supervised teams for a given period. |
| GET | `/api/v1/managers/:managerId/dashboard/trends` | Manager+ | Trend lines aggregated across all supervised teams. |
| GET | `/api/v1/managers/:managerId/subordinates` | Manager+ | List all users in the manager's reporting chain. |
| GET | `/api/v1/managers/:managerId/teams/action-items` | Manager+ | Action item summary across all supervised teams. |

---

## Admin — Hierarchy Levels

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/hierarchy-levels` | Admin | List all hierarchy levels with permissions. |
| POST | `/api/v1/admin/hierarchy-levels` | Admin | Create a new hierarchy level. |
| PUT | `/api/v1/admin/hierarchy-levels/:id` | Admin | Update a hierarchy level's name, color, or permissions. |
| PUT | `/api/v1/admin/hierarchy-levels/:id/position` | Admin | Reorder a hierarchy level (drag-and-drop). |
| DELETE | `/api/v1/admin/hierarchy-levels/:id` | Admin | Delete a hierarchy level. Fails if any users reference it. |

---

## Admin — Users

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/users` | Admin | List all users. |
| POST | `/api/v1/admin/users` | Admin | Create a user account. |
| PUT | `/api/v1/admin/users/:id` | Admin | Update a user (name, email, hierarchy level, reports-to). |
| DELETE | `/api/v1/admin/users/:id` | Admin | Delete a user account. |

---

## Admin — Teams

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/teams` | Admin | List all teams. |
| POST | `/api/v1/admin/teams` | Admin | Create a team. |
| PUT | `/api/v1/admin/teams/:id` | Admin | Update a team (name, lead, cadence, distribution email). |
| DELETE | `/api/v1/admin/teams/:id` | Admin | Delete a team (cascades to sessions and responses). |
| GET | `/api/v1/admin/teams/:id/members` | Admin | List members of a team. |
| POST | `/api/v1/admin/teams/:id/members` | Admin | Add a user to a team. |
| DELETE | `/api/v1/admin/teams/:id/members/:userId` | Admin | Remove a user from a team. |
| GET | `/api/v1/admin/teams/:id/supervisors` | Admin | Get the supervisor chain for a team. |
| PUT | `/api/v1/admin/teams/:id/supervisors` | Admin | Replace the entire supervisor chain for a team. |

---

## Admin — Settings

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/v1/admin/settings/dimensions` | Admin | List all health dimensions (active and inactive). |
| POST | `/api/v1/admin/settings/dimensions` | Admin | Create a new health dimension. |
| PUT | `/api/v1/admin/settings/dimensions/:id` | Admin | Update a dimension (name, descriptions, weight, active flag). |
| DELETE | `/api/v1/admin/settings/dimensions/:id` | Admin | Delete a dimension. Fails if responses reference it. |
| GET | `/api/v1/admin/settings/branding` | Admin | Get branding settings (company name, logo URL). |
| PUT | `/api/v1/admin/settings/branding` | Admin | Update branding settings. |
| GET | `/api/v1/admin/settings/notifications` | Admin | Get notification settings (email, Slack, weekly digest). |
| PUT | `/api/v1/admin/settings/notifications` | Admin | Update notification settings. |
| GET | `/api/v1/admin/settings/retention` | Admin | Get data retention policy (months). |
| PUT | `/api/v1/admin/settings/retention` | Admin | Update data retention policy. |

---

## Score & Trend Reference

| Value | Score meaning | Trend values |
|-------|--------------|-------------|
| 1 | Red — needs support | `improving` / `stable` / `declining` |
| 2 | Yellow — mixed | |
| 3 | Green — healthy | |

---

## Common Error Responses

All endpoints return errors in this shape:

```json
{
  "error": "Brief error type",
  "message": "Human-readable detail"
}
```

| HTTP Status | When |
|------------|------|
| 400 | Malformed request body or missing required field |
| 401 | Missing or invalid auth token / cookie |
| 403 | Authenticated but insufficient permissions |
| 404 | Resource not found |
| 409 | Conflict (e.g., duplicate username) |
| 500 | Internal server or database error |
