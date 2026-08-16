# Admin API

Registered by `SetupAdminRoutes`. Every route under `/api/v1/admin` carries
`JWTAuthMiddleware` followed by `AdminOnlyMiddleware`.

!!! warning "VPs have full admin access"
    `AdminOnlyMiddleware` accepts both `level-admin` and `level-1`. The seed data assigns
    `level-1` to **VP**, so every VP can create and delete users, change hierarchy levels,
    and alter system settings. See
    [Authentication & Authorization](../architecture/authentication.md#adminonlymiddleware).

`AdminHandler` is a pure facade: it holds four sub-handlers
(`HierarchyAdminHandler`, `UserAdminHandler`, `TeamAdminHandler`, `SettingsAdminHandler`)
and every one of its 26 methods is a single-line delegation with no added logic. Two
methods are renamed at the boundary — `GetTeamSupervisors` → `GetSupervisorChain` and
`UpdateTeamSupervisors` → `UpdateSupervisorChain`.

## Conventions across this surface

- **No query parameters anywhere.** There is no pagination, filtering, or sorting on any
  admin list endpoint; they return the full table.
- **Delete endpoints do not return 404** for a missing resource — they return 200. The
  only exceptions are `RemoveTeamMember` (via substring matching on the repository error)
  and `DeleteHierarchyLevel`, which returns 409 when the level is still in use.
- **Create responses may carry zero timestamps.** Create handlers serialize the freshly
  built domain struct rather than re-reading the row, so `createdAt` / `updatedAt` reflect
  whatever the repository left on the struct.
- **No `code` field** is ever set on error responses.

---

## Hierarchy Levels

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/admin/hierarchy-levels` | List levels |
| POST | `/api/v1/admin/hierarchy-levels` | Create a level |
| PUT | `/api/v1/admin/hierarchy-levels/:id` | Update name and permissions |
| PUT | `/api/v1/admin/hierarchy-levels/:id/position` | Reorder a level |
| DELETE | `/api/v1/admin/hierarchy-levels/:id` | Delete a level |

### GET /api/v1/admin/hierarchy-levels

**Response** `200` — `{"levels": [HierarchyLevelDTO]}` where each entry is
`{id, name, position, permissions: {canViewAllTeams, canEditTeams, canManageUsers, canTakeSurvey, canViewAnalytics}, createdAt, updatedAt}`.

!!! warning "Three stored permissions are not exposed by this API"
    The `hierarchy_levels` table carries eight permission columns — migration 000011 added
    `can_configure_system`, `can_view_reports`, and `can_export_data` — but
    `HierarchyPermissionsDTO` exposes only five. The other three can be neither read nor
    written through the admin API, even though `canExportData` gates the Export button in
    the frontend. See [Data Model](../data-model.md).

**Errors** — `500 Failed to query hierarchy levels`.

### POST /api/v1/admin/hierarchy-levels

**Request** — `dto.CreateHierarchyLevelRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `id` | string | optional — slug-generated from `name` when empty |
| `name` | string | required |
| `permissions` | object | optional, five booleans, each defaulting to `false` |

`position` is **always** server-assigned as the current maximum plus one; the request has
no `position` field. A client-supplied `id` is used verbatim with no sanitization.

**Response** `201` — a bare `HierarchyLevelDTO` (not wrapped).

**Errors** — `400 Invalid request body`; `500 Failed to determine position`;
`500 Failed to create hierarchy level`.

### PUT /api/v1/admin/hierarchy-levels/:id

**Request** — `{name?: string, permissions?: {...}}`. `name` is applied only when
non-empty, so a name cannot be cleared. When a `permissions` object is supplied, **all
five booleans are overwritten** — omitting one inside the object sets it to `false`.

**Response** `200` — a bare `HierarchyLevelDTO`.

**Errors** — `400 Invalid request body`; `404 Hierarchy level not found` (any lookup error
maps to 404); `500 Failed to update hierarchy level`.

### PUT /api/v1/admin/hierarchy-levels/:id/position

**Request** — `{"newPosition": int}`, validated as `required,min=1`.

!!! warning "Position 0 cannot be set"
    Because `binding:"required"` treats the zero value as absent, `newPosition: 0` is
    rejected as missing — even though the Admin level occupies position 0 in the seed data
    and the database constraint permits `position >= 0`.

**Response** `200` — `{"message": "Position updated successfully"}`. The updated level is
not returned.

Reordering shifts the other rows inside a transaction.

**Errors** — `400 Invalid request body`; `404 Hierarchy level not found`;
`500 Failed to start transaction`; `500 Failed to reorder levels`;
`500 Failed to update position`; `500 Failed to commit transaction`.

### DELETE /api/v1/admin/hierarchy-levels/:id

**Response** `200` — `{"message": "Hierarchy level deleted successfully"}`.

**Errors** — `500 Database error`; **`409 Cannot delete hierarchy level`** with
`message: "Users are assigned to this level. Reassign them first."`;
`500 Failed to delete hierarchy level`.

---

## Users

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/admin/users` | List all users |
| POST | `/api/v1/admin/users` | Create a user |
| PUT | `/api/v1/admin/users/:id` | Update a user |
| DELETE | `/api/v1/admin/users/:id` | Delete a user |

`AdminUserDTO` is
`{id, username, email, fullName, hierarchyLevel, reportsTo, teamIds, authType, createdAt, updatedAt}`.
The password hash is never returned.

### GET /api/v1/admin/users

**Response** `200` — `{"users": [AdminUserDTO], "total": int}`.

**Errors** — `500 Failed to query users`.

!!! note "N+1 query"
    Team IDs are fetched with one additional query per user, and a failure on any of them
    is silently swallowed to an empty list.

### POST /api/v1/admin/users

**Request** — `dto.CreateUserRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `id` | string | optional — slug-generated from `username` when empty |
| `username` | string | required |
| `email` | string | required, valid email |
| `fullName` | string | required |
| `password` | string | required for local users, minimum 4 characters |
| `authType` | string | optional, `local` or `sso` (default `local`) |
| `hierarchyLevel` | string | required |
| `reportsTo` | string \| null | optional |

The password is bcrypt-hashed server-side. SSO users are stored with an empty hash.
`teamIds` is forced to `[]` — membership cannot be set at creation; use the team member
endpoints below.

**Response** `201` — a bare `AdminUserDTO`.

**Errors** — `400 Invalid request body`;
`400 Password is required for local users (min 4 characters)`;
`500 Failed to hash password`; `500 Failed to create user`.

!!! warning "Duplicate username or email surfaces as a 500"
    There is no pre-check for uniqueness. A collision hits the database unique constraint
    and is reported as `500 Failed to create user` rather than a 409.

!!! warning "The minimum password length is inconsistent"
    Admin user creation enforces 4 characters; the password reset flow enforces 8.

### PUT /api/v1/admin/users/:id

**Request** — `dto.UpdateUserRequest`. Every field is a pointer and optional; `null` or
omitted means "leave unchanged".

| Field | Type | Validation |
|-------|------|-----------|
| `username` | string | — |
| `email` | string | — (**not** validated on update, unlike create) |
| `fullName` | string | — |
| `password` | string | — |
| `authType` | string | `local` or `sso` |
| `hierarchyLevel` | string | — |
| `reportsTo` | string | — |

**Response** `200` — a bare `AdminUserDTO` with `teamIds` re-fetched.

**Errors**

| Status | `error` |
|--------|---------|
| 400 | `Invalid request body` |
| 400 | `Cannot set password for SSO users` |
| 400 | `Password is required when switching from SSO to local authentication` |
| 400 | `Invalid authType (must be 'local' or 'sso')` (unreachable — binding rejects first) |
| 404 | `User not found` |
| 500 | `Failed to hash password` / `Failed to update user` / `Failed to update password` |

**Side effect** — if `reportsTo` or `hierarchyLevel` actually changed, the handler rebuilds
supervisor chains for every team where the user is a lead or supervisor. Failures there
are logged as warnings and never surfaced to the client, so a partially rebuilt chain
still returns 200.

!!! note "A manager cannot be cleared via `null`"
    Assignment is gated on `req.ReportsTo != nil`, so sending `"reportsTo": null` is
    treated as "unchanged" rather than "clear". The same pattern applies to
    `teamLeadId` on teams.

### DELETE /api/v1/admin/users/:id

**Response** `200` — `{"message": "User deleted successfully"}`. No 404 for a
non-existent user.

**Errors** — `500 Failed to delete user`.

---

## Teams

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/admin/teams` | List all teams |
| POST | `/api/v1/admin/teams` | Create a team |
| PUT | `/api/v1/admin/teams/:id` | Update a team |
| DELETE | `/api/v1/admin/teams/:id` | Delete a team |
| GET | `/api/v1/admin/teams/:id/members` | List members |
| POST | `/api/v1/admin/teams/:id/members` | Add a member |
| DELETE | `/api/v1/admin/teams/:id/members/:userId` | Remove a member |
| GET | `/api/v1/admin/teams/:id/supervisors` | Read the supervisor chain |
| PUT | `/api/v1/admin/teams/:id/supervisors` | Replace the supervisor chain |

`AdminTeamDTO` is
`{id, name, teamLeadId, teamLeadName, cadence, distributionListEmail, memberCount, createdAt, updatedAt}`.

### GET /api/v1/admin/teams

**Response** `200` — `{"teams": [AdminTeamDTO], "total": int}`.
**Errors** — `500 Failed to query teams`.

### POST /api/v1/admin/teams

**Request** — `dto.CreateTeamRequest`

| Field | Type | Validation |
|-------|------|-----------|
| `id` | string | optional — slug-generated from `name` when empty |
| `name` | string | required |
| `teamLeadId` | string \| null | optional |
| `cadence` | string | required, one of `monthly`, `quarterly`, `half-yearly`, `yearly` |
| `distributionListEmail` | string \| null | optional, valid email |

**Response** `201` — a bare `AdminTeamDTO` with `memberCount` hardcoded to `0`.

**Side effect** — when a team lead is set, the supervisor chain is derived by walking up
the `reportsTo` graph and written to `team_supervisors`. Errors are only logged.

**Errors** — `400 Invalid request body`; `500 Failed to create team`.

!!! warning "`teamLeadId` is not validated"
    There is no check that the referenced user exists. An unknown ID is written and the
    supervisor derivation simply produces nothing.

### PUT /api/v1/admin/teams/:id

**Request** — `dto.UpdateTeamRequest`: `name?`, `teamLeadId?`, `cadence?`
(same `oneof` list), `distributionListEmail?`. All pointers; omitted means unchanged.

**Response** `200` — a bare `AdminTeamDTO`, re-fetched to resolve `teamLeadName`.

Changing the lead re-derives the supervisor chain, or clears it when the new lead is the
empty string. As with users, `"teamLeadId": null` means "unchanged" — only `""` clears it.

**Errors** — `400 Invalid request body`; `404 Team not found`; `500 Failed to update team`.

### DELETE /api/v1/admin/teams/:id

**Response** `200` — `{"message": "Team deleted successfully"}`. No 404.
**Errors** — `500 Failed to delete team`.

Deleting a team cascades to `team_members`, `team_supervisors`, and `action_items`, but
**not** to `health_check_sessions`, which have no foreign key. Sessions for a deleted team
are orphaned rather than removed.

### GET /api/v1/admin/teams/:id/members

**Response** `200` — `{"members": [{"userId", "userName", "email"}], "total": int}`.
**Errors** — `404 Team not found`; `500 Failed to fetch team members`.

### POST /api/v1/admin/teams/:id/members

**Request** — `{"userId": "..."}`, required.
**Response** `201` — `{"message": "Member added successfully"}`. The member object is not
returned, unlike the other create endpoints.
**Errors** — `400 Invalid request body`; `404 Team not found`; `404 User not found`;
`500 Failed to add team member`. The two 404s share a status and differ only in the
`error` string.

### DELETE /api/v1/admin/teams/:id/members/:userId

**Response** `200` — `{"message": "Member removed successfully"}`.
**Errors** — `404 Team member not found`; `500 Failed to remove team member`.

!!! warning "The 404 is derived from string matching"
    The handler distinguishes "not found" from other failures with
    `strings.Contains(err.Error(), "not found")` on the repository error. A change to that
    error text would silently turn 404s into 500s.

### GET /api/v1/admin/teams/:id/supervisors

**Response** `200` — `dto.SupervisorChainResponse`

```json
{
  "teamId": "platform-squad",
  "supervisors": [
    {"userId": "teamlead1", "userName": "Dana Lee", "levelId": "level-4", "levelName": "Team Lead"},
    {"userId": "manager1", "userName": "Sam Ortiz", "levelId": "level-3", "levelName": "Manager"}
  ]
}
```

The chain is ordered closest-supervisor-first (`position` 1 upward). `userName` and
`levelName` are enrichment lookups; if either fails the field is left as an empty string
rather than failing the request.

**Errors** — `404 Team not found`; `500 Failed to fetch supervisor chain`.

### PUT /api/v1/admin/teams/:id/supervisors

**Request** — `{"supervisors": [{"userId": "...", "levelId": "..."}]}`. Both element
fields are required, and the array itself is `binding:"required"`.

!!! warning "A supervisor chain cannot be cleared through this endpoint"
    Because `required` on a slice rejects an empty array, `{"supervisors": []}` returns
    `400`. The only way to clear a chain is indirectly, by clearing the team lead through
    `PUT /api/v1/admin/teams/:id`.

`position` is assigned server-side from the array index; it is not accepted as input.
There is no validation that the referenced users or levels exist.

**Response** `200` — the handler delegates to `GetSupervisorChain`, so the body is the
enriched `SupervisorChainResponse`.

!!! note "A post-write read failure is reported as the write failing"
    Because the success path re-reads through `GetSupervisorChain`, a 404 or 500 from that
    follow-up read is returned to the client **after** the write has already committed.

**Errors** — `400 Invalid request body`; `404 Team not found`;
`500 Failed to update supervisor chain`; plus the read errors above.

---

## Settings

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/admin/settings/dimensions` | List dimensions |
| POST | `/api/v1/admin/settings/dimensions` | Create a dimension |
| PUT | `/api/v1/admin/settings/dimensions/:id` | Update a dimension |
| DELETE | `/api/v1/admin/settings/dimensions/:id` | Soft-delete a dimension |
| GET | `/api/v1/admin/settings/branding` | Read company branding |
| PUT | `/api/v1/admin/settings/branding` | Update company branding |
| GET | `/api/v1/admin/settings/notifications` | Read notification toggles |
| PUT | `/api/v1/admin/settings/notifications` | Update notification toggles |
| GET | `/api/v1/admin/settings/retention` | Read the retention policy |
| PUT | `/api/v1/admin/settings/retention` | Update the retention policy |

### Dimensions

`HealthDimensionDTO` is
`{id, name, description, goodDescription, badDescription, isActive, weight, createdAt, updatedAt}`.

**GET** returns `{"dimensions": [...]}` — note there is no `total`, unlike the users and
teams lists. Errors: `500 Failed to query dimensions`.

**POST** — `dto.CreateDimensionRequest`:

| Field | Type | Validation |
|-------|------|-----------|
| `id` | string | **required** |
| `name` | string | required |
| `description` | string | — |
| `goodDescription` | string | required |
| `badDescription` | string | required |
| `isActive` | bool | optional, defaults to `true` |
| `weight` | float | optional, 0–10, defaults to `1.0` |

!!! note "Dimensions are the only admin resource without ID generation"
    Hierarchy levels, users, and teams all slug-generate an ID from their name when one is
    not supplied. Dimensions require the client to provide `id`.

!!! warning "A weight of exactly 0 cannot be created"
    `weight: 0` passes the 0–10 range check and is then silently rewritten to `1.0`,
    because the handler cannot distinguish an explicit zero from an omitted field. Update
    does not have this problem and will store 0.

Returns `201` with a bare `HealthDimensionDTO`. Errors: `400 Invalid request body`;
`400 Weight must be between 0 and 10`; `500 Failed to create dimension`.

**PUT** takes all-optional pointer fields (`name`, `description`, `goodDescription`,
`badDescription`, `isActive`, `weight`) and returns `200` with the bare DTO. Errors:
`400 Invalid request body`; `400 Weight must be between 0 and 10` (checked before the
existence lookup, so a bad weight on an unknown ID returns 400 rather than 404);
`404 Dimension not found`; `500 Failed to update dimension`.

**DELETE** returns `200 {"message": "Dimension deleted successfully"}` and
`500 Failed to delete dimension`. There is no 404 and no in-use check.

!!! note "Deleting a dimension is a soft delete"
    The repository issues `UPDATE health_dimensions SET is_active = false`. The row
    remains, which is what allows `health_check_responses.dimension_id` to keep its
    `ON DELETE RESTRICT` foreign key while historical responses stay readable.

### Branding

**GET** returns `{"companyName": "...", "logoURL": "..."}`. Errors:
`500 Failed to fetch branding settings`.

**PUT** takes the same struct; `companyName` is required with a maximum of 100 characters.
`logoURL` in practice holds a base64 data URL — the admin UI uploads an image and inlines
it. The handler rejects payloads over 700000 bytes, which is roughly a 500 KB image after
base64 expansion, with `400 Logo is too large. Maximum size is 500KB.`

Other errors: `400 Invalid request body`; `500 Failed to save branding settings`.

These values are also served publicly by [`GET /api/v1/config`](auth.md#get-apiv1config).

### Notifications

**GET** returns `dto.NotificationSettings`:

```json
{
  "emailEnabled": false,
  "slackEnabled": false,
  "notifyOnSubmission": false,
  "notifyManagers": false,
  "reminderDaysBefore": 7,
  "reminderRecipients": [],
  "smtpConfigured": true
}
```

Only `emailEnabled`, `slackEnabled`, and `notifyOnSubmission` are persisted — the last
maps to the `app_settings.weekly_digest` column. `smtpConfigured` is derived from
`os.Getenv("SMTP_HOST") != ""`.

**PUT** accepts the same struct and returns `200`. Errors: `400 Invalid request body`;
`500 Failed to save settings`.

!!! warning "Four notification fields look writable but are not"
    `notifyManagers`, `reminderDaysBefore`, `reminderRecipients`, and `smtpConfigured` are
    accepted by `PUT` and **echoed back in the response as though saved**, but they are
    never persisted. A subsequent `GET` returns the hardcoded defaults (`false`, `7`,
    `[]`, and the derived SMTP flag). `slackEnabled` is persisted but nothing ever reads
    it — there is no Slack integration in the codebase.

### Retention

**GET** returns `{"keepSessionsMonths": 12, "archiveEnabled": false, "anonymizeAfterDays": 360}`.
`archiveEnabled` is hardcoded `false`; `anonymizeAfterDays` is computed as
`keepSessionsMonths * 30`.

**PUT** accepts the same struct, validates `keepSessionsMonths` between 1 and 120, and
recomputes `anonymizeAfterDays` server-side. Errors: `400 Invalid request body`;
`400 Keep sessions months must be between 1 and 120`; `500 Failed to save retention policy`.

!!! warning "Retention is stored but never enforced"
    No scheduled job, cron, or cleanup routine reads `retention_months`. Nothing is ever
    deleted or anonymized. `archiveEnabled` is accepted and echoed but not persisted at
    all.
