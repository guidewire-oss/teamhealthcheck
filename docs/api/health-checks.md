# Health Checks API

Registered by `SetupHealthCheckRoutes`. The whole group is wrapped in
`JWTAuthMiddleware`; none of these routes carries an additional role or team guard.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/health-checks` | `JWT` | Submit a completed survey |
| GET | `/api/v1/health-dimensions` | `JWT` | List active dimensions |
| GET | `/api/v1/health-checks/:id` | `JWT` | Fetch one session by ID |
| GET | `/api/v1/health-checks/team/:id` | `JWT` | List a team's sessions |
| GET | `/api/v1/teams/:teamId/submission-status` | `JWT` | Per-period submission progress |
| GET | `/api/v1/assessment-periods` | `JWT` | Distinct periods present in the data |

!!! note "Why `/health-checks/team/:id` and not `/teams/:id/health-checks`"
    The route comment in `health_check_routes.go` explains the shape: nesting under
    `/teams/:id` would collide with the `:teamId` parameter name used by the team route
    group, which Gin rejects.

!!! warning "These routes have no team-level authorization"
    `GET /api/v1/health-checks/team/:id` and `GET /api/v1/health-checks/:id` are guarded
    only by `JWTAuthMiddleware` — unlike the dashboard routes, they do **not** apply
    `TeamMembershipMiddleware`. Any authenticated user, at any hierarchy level, can read
    any team's raw sessions, including individual comments, by supplying the team ID.

---

## POST /api/v1/health-checks

**Request** — `dto.SubmitHealthCheckRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `id` | string | optional — server generates `session-<unixnano>` when empty |
| `teamId` | string | required |
| `userId` | string | required |
| `date` | string | required, RFC3339 |
| `assessmentPeriod` | string | optional |
| `surveyType` | string | optional — `individual` (default) or `post_workshop` |
| `responses` | array | required, `min=1`, each element validated |
| `completed` | bool | — |

Each element of `responses` is a `dto.HealthCheckResponseRequest`:

| Field | Type | Validation |
|-------|------|-----------|
| `dimensionId` | string | required |
| `score` | int | required, 1–3 |
| `trend` | string | required, one of `improving`, `stable`, `declining` |
| `comment` | string | optional (the database caps it at 1000 characters) |

Score meanings: `1` red (poor), `2` yellow (medium), `3` green (good).

**Response** `201` — `dto.HealthCheckSessionResponse`

```json
{
  "id": "session-1718452800000000000",
  "teamId": "platform-squad",
  "userId": "alice",
  "date": "2025-06-15T10:30:00Z",
  "assessmentPeriod": "2025 - 1st Half",
  "surveyType": "individual",
  "responses": [
    {"dimensionId": "mission", "score": 3, "trend": "stable", "comment": ""}
  ],
  "completed": true
}
```

**Errors**

| Status | `error` | `message` |
|--------|---------|-----------|
| 400 | `Invalid request` | The binding error |
| 400 | `Invalid date format` | `Date must be in RFC3339 format (e.g., 2024-01-15T10:30:00Z)` |
| 400 | `Invalid date: future dates are not allowed` | `Health check date cannot be in the future` |
| 400 | `invalid assessment period: ...` (the validation error text) | The list of accepted period formats |
| 400 | `Failed to submit health check` | Domain or command error |

Accepted assessment period formats: `YYYY Mon`, `YYYY Q1`–`YYYY Q4`, `YYYY H1`/`YYYY H2`,
`YYYY`, and `YYYY - 1st Half` / `YYYY - 2nd Half`. Future periods are rejected.

**Side effect** — after a successful submission the handler starts a goroutine that
sends notification email through `NotificationService`: `SendIndividualSurveyEmail` for
`individual` (or empty) survey types, `SendPostWorkshopEmails` for `post_workshop`. The
email is fire-and-forget; failures never affect the response. See
[Background & External Services](../architecture/services.md#email).

!!! warning "`userId` is trusted from the body"
    The handler never compares `req.UserID` against the JWT claims. Any authenticated
    user can submit a health check attributed to a different user, on a team they do not
    belong to.

!!! note "Upsert semantics"
    The repository writes sessions with `ON CONFLICT DO UPDATE`, and
    `health_check_responses` has a unique constraint on `(session_id, dimension_id)`.
    Re-submitting with the same session `id` updates rather than duplicating.

---

## GET /api/v1/health-dimensions

No parameters. The query is fixed to active dimensions only — this is not
client-controllable.

**Response** `200` — `dto.HealthDimensionsResponse`

```json
{
  "dimensions": [
    {
      "id": "mission",
      "name": "Mission",
      "description": "...",
      "goodDescription": "We know exactly why we are here...",
      "badDescription": "We have no idea why we are here...",
      "isActive": true,
      "weight": 1
    }
  ]
}
```

`isActive` and `weight` are `omitempty`.

**Errors** — `500 Failed to fetch dimensions`.

!!! warning "The survey UI ignores this endpoint"
    A client helper exists at `frontend/lib/api/health-checks.ts`, but no page calls it.
    The survey renders the hardcoded `HEALTH_DIMENSIONS` constant from
    `frontend/lib/data.ts` instead, so dimensions created or disabled through the admin
    API will not appear or disappear in the survey.

---

## GET /api/v1/health-checks/:id

**Path** — `id`, the session ID.

**Response** `200` — `dto.HealthCheckSessionResponse` (same shape as the submit response).

**Errors** — `404 Session not found`.

---

## GET /api/v1/health-checks/team/:id

**Path** — `id`, the team ID.
**Query** — `assessmentPeriod` (optional; empty means no filter).

**Response** `200` — `dto.HealthCheckSessionsResponse`

```json
{"sessions": [ /* HealthCheckSessionResponse */ ], "total": 12}
```

`total` is `omitempty`, so it is **absent** rather than `0` when there are no sessions.

**Errors** — `500 Failed to fetch sessions`.

---

## GET /api/v1/teams/:teamId/submission-status

Reports how many of a team's members have submitted for a given period, and whether the
post-workshop session exists. Used by the dashboard to decide whether to offer the
"Post-Workshop Survey" button.

**Path** — `teamId`.
**Query** — `assessmentPeriod`, **required** (unlike every other endpoint, where it is
an optional filter).

**Response** `200` — `dto.TeamSubmissionStatusResponse`

```json
{
  "teamId": "platform-squad",
  "assessmentPeriod": "2025 - 1st Half",
  "totalMembers": 6,
  "submittedMembers": 4,
  "allSubmitted": false,
  "postWorkshopExists": false
}
```

**Errors**

| Status | `error` |
|--------|---------|
| 400 | `Team ID is required` (unreachable — see [cross-cutting caveats](index.md#cross-cutting-caveats)) |
| 400 | `assessmentPeriod query parameter is required` |
| 500 | `Failed to get submission status` |

---

## GET /api/v1/assessment-periods

No parameters. Returns the distinct assessment periods present in the data, so the UI can
populate period dropdowns dynamically instead of hardcoding them.

**Response** `200` — `{"periods": ["2024 - 2nd Half", "2025 - 1st Half"]}`

**Errors** — `500 Failed to fetch assessment periods`.
