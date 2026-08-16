# Teams API

Registered by `SetupTeamRoutes`. The group requires `JWTAuthMiddleware`; the
`:teamId` subgroup additionally applies `TeamMembershipMiddleware`.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/teams` | `JWT` | List all teams |
| GET | `/api/v1/teams/:teamId/info` | `JWT` `+TeamMember` | Team detail with members |
| GET | `/api/v1/teams/:teamId/sessions` | `JWT` `+TeamMember` | All sessions for a team |

!!! note "Team-scoped routes registered elsewhere"
    Several other route groups also live under `/api/v1/teams/:teamId` but are registered
    by different setup functions and documented on their own pages:
    [`/dashboard/*`](team-dashboard.md), [`/action-items`](action-items.md), and
    [`/submission-status`](health-checks.md#get-apiv1teamsteamidsubmission-status).

!!! warning "`GET /api/v1/teams` exposes every team to every authenticated user"
    The list endpoint sits outside the `:teamId` subgroup, so `TeamMembershipMiddleware`
    does not apply. Any authenticated user — including a `level-5` team member — receives
    the full list of teams with lead names and member counts.

---

## GET /api/v1/teams

No parameters.

**Response** `200` — `dto.TeamListResponse`

```json
{
  "teams": [
    {
      "id": "platform-squad",
      "name": "Platform Squad",
      "cadence": "quarterly",
      "memberCount": 6,
      "teamLeadId": "teamlead1",
      "teamLeadName": "Dana Lee"
    }
  ],
  "total": 9
}
```

`teamLeadId` and `teamLeadName` are omitted when the team has no lead. `total` is the
length of the returned slice, not a separate database count.

**Errors** — `500 Failed to fetch teams` with the underlying error in `message`.

---

## GET /api/v1/teams/:teamId/info

**Path** — `teamId`.

**Response** `200` — `dto.TeamInfoResponse`

```json
{
  "id": "platform-squad",
  "name": "Platform Squad",
  "cadence": "quarterly",
  "teamLeadId": "teamlead1",
  "teamLeadName": "Dana Lee",
  "members": [
    {"id": "alice", "username": "alice", "fullName": "Alice Johnson"}
  ]
}
```

`cadence` is one of `monthly`, `quarterly`, `half-yearly`, `yearly` — the frontend uses it
to compute the current assessment period and the next survey due date.

**Errors**

| Status | `error` | Cause |
|--------|---------|-------|
| 400 | `Team ID is required` | Unreachable (empty path parameter) |
| 404 | `Team not found` | Lookup failed |
| 500 | `Failed to fetch team members` | Member query failed |

---

## GET /api/v1/teams/:teamId/sessions

Returns every health check session for the team, with full per-dimension responses
including comments and the submitting user.

**Path** — `teamId`.
**Query** — `assessmentPeriod` (optional).

**Response** `200` — `dto.HealthCheckSessionsResponse`, the same shape as
[`GET /api/v1/health-checks/team/:id`](health-checks.md#get-apiv1health-checksteamid).

**Errors** — `500 Failed to fetch team sessions` with the underlying error in `message`.

!!! note "Period filtering happens in application memory"
    When `assessmentPeriod` is supplied, the handler fetches **all** sessions for the team
    via `FindByTeamID` and filters the slice in Go rather than pushing the predicate into
    SQL. This is fine at demo scale but does not bound the result set.

!!! warning "The team results page calls a route that does not exist"
    `frontend/app/teams/[teamId]/TeamResultsClient.tsx:51` requests
    `GET /api/v1/teams/{teamId}` — the bare path, with no suffix. No such route is
    registered: `SetupTeamRoutes` defines `GET /api/v1/teams`, and beneath `:teamId` only
    `/info` and `/sessions`. The request falls through to the `NoRoute` handler and
    returns `404 {"error": "not found"}`. The intended target is almost certainly
    `/sessions`, whose DTO matches what the page renders. This is a live defect, recorded
    here rather than papered over.
