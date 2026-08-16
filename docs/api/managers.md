# Managers API

Registered by `SetupManagerRoutes`. The group applies three guards in sequence:
`JWTAuthMiddleware`, `ManagerOrAboveMiddleware`, then
`SameUserOrManagerMiddleware("managerId")`.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/managers/:managerId/teams/health` | `JWT` `+Manager` `+SameUser` | Health rollup for supervised teams |
| GET | `/api/v1/managers/:managerId/dashboard/radar` | `JWT` `+Manager` `+SameUser` | Aggregated per-dimension averages |
| GET | `/api/v1/managers/:managerId/dashboard/trends` | `JWT` `+Manager` `+SameUser` | Scores per dimension across periods |
| GET | `/api/v1/managers/:managerId/subordinates` | `JWT` `+Manager` `+SameUser` | Reporting tree beneath the manager |

The action item summary route `/api/v1/managers/:managerId/teams/action-items` is
registered separately and documented under [Action Items](action-items.md).

None of these handlers reads the JWT claims itself — authorization is entirely delegated
to the middleware chain.

!!! warning "Directors and VPs can read any manager's data"
    `SameUserOrManagerMiddleware` lets `level-1`, `level-2`, and `level-admin` through for
    **any** `managerId`, without checking that the target is actually their subordinate.
    A `TODO` in `jwt_auth.go` acknowledges the missing check.

Every endpoint on this page shares the same unreachable `400 Manager ID is required`
guard against an empty path parameter.

---

## GET /api/v1/managers/:managerId/teams/health

**Path** — `managerId`.
**Query** — `assessmentPeriod` (optional).

**Response** `200` — `dto.ManagerTeamsHealthResponse`

```json
{
  "managerId": "manager1",
  "teams": [
    {
      "teamId": "platform-squad",
      "teamName": "Platform Squad",
      "overallHealth": 2.31,
      "submissionCount": 4,
      "dimensions": [
        {"dimensionId": "mission", "avgScore": 2.5, "responseCount": 4}
      ],
      "postWorkshopStatus": "submitted"
    }
  ],
  "totalTeams": 3,
  "assessmentPeriod": "2025 - 1st Half"
}
```

`assessmentPeriod` and `postWorkshopStatus` are `omitempty`.

**Errors** — `400 Manager ID is required` (unreachable); `500 Database query failed` with
the underlying error in `message`.

---

## GET /api/v1/managers/:managerId/dashboard/radar

Aggregates every supervised team into one per-dimension average, for the radar chart.

**Path** — `managerId`.
**Query** — `assessmentPeriod` (optional).

**Response** `200` — `dto.ManagerRadarResponse`

```json
{
  "managerId": "manager1",
  "dimensions": [
    {"dimensionId": "mission", "avgScore": 2.4, "responseCount": 18}
  ],
  "assessmentPeriod": "2025 - 1st Half"
}
```

**Errors** — `400 Manager ID is required` (unreachable); `500 Database query failed`.

---

## GET /api/v1/managers/:managerId/dashboard/trends

**Path** — `managerId`. No query parameters.

!!! note "This endpoint ignores `assessmentPeriod`"
    Unlike the health and radar endpoints, the trends handler does not read
    `assessmentPeriod` — it always returns every period. That is by design: a trend line
    needs the full series.

**Response** `200` — `dto.ManagerTrendsResponse`

```json
{
  "managerId": "manager1",
  "periods": ["2024 - 2nd Half", "2025 - 1st Half"],
  "dimensions": [
    {"dimensionId": "mission", "scores": [1.9, 2.4]}
  ]
}
```

The `scores` array is positionally aligned with `periods`.

**Errors** — `400 Manager ID is required` (unreachable); `500 Failed to fetch trend data`.

---

## GET /api/v1/managers/:managerId/subordinates

Returns the users reporting to this manager, used to render the hierarchy tree on the
manager dashboard.

**Path** — `managerId`. No query parameters.

**Response** `200` — `dto.SubordinatesResponse`

```json
{
  "managerId": "manager1",
  "subordinates": [
    {
      "id": "teamlead1",
      "username": "teamlead1",
      "name": "Dana Lee",
      "hierarchyLevelId": "level-4",
      "reportsTo": "manager1",
      "teamIds": ["platform-squad"]
    }
  ]
}
```

`reportsTo` is `omitempty`; a `NULL` in the database is flattened to an empty string and
therefore omitted.

**Errors** — `400 Manager ID is required` (unreachable); `500 Failed to fetch subordinates`.
