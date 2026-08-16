# Action Items API

Registered by `SetupActionItemRoutes`. Action items are follow-up tasks raised against a
team, optionally linked to the health dimension that prompted them.

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/teams/:teamId/action-items` | `JWT` `+TeamMember` | List a team's action items |
| POST | `/api/v1/teams/:teamId/action-items` | `JWT` `+TeamMember` | Create an action item |
| PATCH | `/api/v1/teams/:teamId/action-items/:id` | `JWT` `+TeamMember` | Partially update an item |
| DELETE | `/api/v1/teams/:teamId/action-items/:id` | `JWT` `+TeamMember` | Delete an item |
| GET | `/api/v1/managers/:managerId/teams/action-items` | `JWT` `+Manager` | Open-count summary per team |

The handler works directly against `*sql.DB` rather than going through a repository — see
[Request Lifecycle](../architecture/request-lifecycle.md#per-request-path).

Status values are `open`, `in_progress`, and `done`, enforced both by a database check
constraint and by handler validation on update.

---

## GET /api/v1/teams/:teamId/action-items

**Path** — `teamId`.
**Query** — `status` (optional, no filter when empty) and `period` (optional, matched
against `assessment_period`).

!!! note "`status` is not validated on read"
    Unlike the update endpoint, the list filter accepts any string. An unrecognised value
    simply matches nothing and returns an empty list.

**Response** `200` — `dto.ActionItemsResponse`

```json
{
  "actionItems": [
    {
      "id": "9f1c...-uuid",
      "teamId": "platform-squad",
      "dimensionId": "speed",
      "dimensionName": "Speed",
      "createdBy": "teamlead1",
      "createdByName": "Dana Lee",
      "assignedTo": "alice",
      "assigneeName": "Alice Johnson",
      "title": "Reduce PR review turnaround",
      "description": "Introduce a review rota.",
      "status": "open",
      "dueDate": "2025-09-30",
      "assessmentPeriod": "2025 - 1st Half",
      "createdAt": "2025-06-20T09:12:00Z",
      "updatedAt": "2025-06-20T09:12:00Z"
    }
  ]
}
```

An empty result serializes as `[]`, not `null`. The nullable fields — `dimensionId`,
`dimensionName`, `assignedTo`, `assigneeName`, `dueDate`, `assessmentPeriod` — emit JSON
`null` rather than being omitted. Timestamps are RFC3339.

**Errors** — `500 Failed to fetch action items`, `500 Failed to scan action items`,
`500 Failed to read action items`, each with the underlying error in `message`.

---

## POST /api/v1/teams/:teamId/action-items

**Path** — `teamId`.

**Request** — `dto.CreateActionItemRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `dimensionId` | string \| null | optional |
| `assignedTo` | string \| null | optional — must be a member of the team |
| `title` | string | required, max 500 |
| `description` | string | optional |
| `dueDate` | string \| null | optional, `YYYY-MM-DD` |
| `assessmentPeriod` | string \| null | optional |

**Server-managed fields** — `id` is always a server-generated UUID v4 (the only true UUID
primary key in the system), `status` is always the literal `open` (it cannot be set at
creation), `created_by` is taken from `claims.UserID` and **not** from the body, and both
timestamps are set server-side.

**Response** `201`

```json
{"id": "9f1c...-uuid", "status": "open", "createdAt": "2025-06-20T09:12:00Z"}
```

!!! note "Create does not return the full resource"
    Unlike most create endpoints, this returns only three fields. Clients that need the
    resolved `dimensionName` or `assigneeName` must re-list.

**Errors**

| Status | `error` |
|--------|---------|
| 401 | `Unauthorized` (claims absent from context) |
| 400 | `Invalid request` (binding failure, including a title over 500 characters) |
| 400 | `Invalid dueDate format, expected YYYY-MM-DD` |
| 400 | `assignedTo user is not a member of this team` |
| 500 | `Failed to validate assignee` |
| 500 | `Failed to create action item` |

---

## PATCH /api/v1/teams/:teamId/action-items/:id

Partial update — every field is optional and omitted fields are preserved via `COALESCE`.

**Path** — `teamId`, `id`.

**Request** — `dto.UpdateActionItemRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `dimensionId` | string \| null | optional |
| `assignedTo` | string \| null | optional — must be a team member |
| `title` | string | optional, max 500 |
| `description` | string | optional |
| `status` | string | optional, one of `open`, `in_progress`, `done` |
| `dueDate` | string | optional, `YYYY-MM-DD` |
| `assessmentPeriod` | string | optional |

**Response** `200` — `{"updated": true}`

**Errors**

| Status | `error` | `message` |
|--------|---------|-----------|
| 400 | `Invalid request` | The binding error |
| 400 | `Invalid status` | `status must be open, in_progress, or done` |
| 400 | `Invalid dueDate format, expected YYYY-MM-DD` | — |
| 400 | `assignedTo user is not a member of this team` | — |
| 500 | `Failed to validate assignee` | Underlying error |
| 500 | `Failed to update action item` | Underlying error |
| 404 | `Action item not found` | Zero rows affected |

!!! warning "Any team member can edit or delete any of the team's action items"
    The update and delete handlers do not read the JWT claims and do not compare
    `created_by` or `assigned_to` against the caller. The `canEdit` restriction that
    limits editing to team leads and above exists only in the React component
    (`components/ActionItemsTab.tsx`) and is not enforced by the API.

---

## DELETE /api/v1/teams/:teamId/action-items/:id

**Path** — `teamId`, `id`. No body.

**Response** `200` — `{"deleted": true}`

**Errors** — `500 Failed to delete action item`; `404 Action item not found` when zero
rows are affected.

---

## GET /api/v1/managers/:managerId/teams/action-items

Per-team counts of outstanding action items, for the manager dashboard's Actions tab.

**Path** — `managerId`. No body or query parameters.

**Response** `200` — `dto.TeamsActionSummaryResponse`

```json
{
  "teams": [
    {"teamId": "platform-squad", "teamName": "Platform Squad", "openCount": 3}
  ]
}
```

`openCount` counts every item whose status is not `done` — so it includes `in_progress`,
not only `open`.

**Errors**

| Status | `error` |
|--------|---------|
| 403 | `Forbidden` |
| 500 | `Failed to fetch action summaries` |
| 500 | `Failed to read action summaries` |

!!! note "This is the only handler that authorizes itself"
    Its route group applies `ManagerOrAboveMiddleware` but **not**
    `SameUserOrManagerMiddleware`, so the handler performs its own check:
    `claims.UserID != managerID` returns `403 Forbidden`. That check is stricter than the
    middleware used elsewhere — here even a director cannot read another manager's
    summary.
