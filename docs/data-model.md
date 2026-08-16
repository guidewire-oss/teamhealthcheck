# Data Model

The PostgreSQL schema is defined entirely by the 20 `golang-migrate` SQL migrations
under `backend/infrastructure/persistence/postgres/migrations/`. There is no ORM and
no schema-generation step: what the migrations create is what exists.

This page describes the **final state** of each table after all 20 migrations have been
applied. For the migration-by-migration history and rollback behaviour, see
[Migrations & Data](operations/migrations.md).

!!! note "No dynamic or custom-field mechanism exists"
    Every column below is fixed by DDL. There is no EAV table, no JSON/JSONB column, and
    no "custom fields" table anywhere in the schema. The only user-extensible concept is
    **rows**: administrators can add hierarchy levels and health dimensions, but not new
    attributes on them. Extending the model requires a new migration.

## Table overview

| Table | Purpose | Created by |
|-------|---------|-----------|
| `health_dimensions` | The health dimensions teams score themselves against | `000001`, seeded by `000004` |
| `health_check_sessions` | One survey submission by one user (aggregate root) | `000002` |
| `health_check_responses` | One dimension score inside a session (value object) | `000003` |
| `users` | People, their hierarchy level, and their supervisor | `000005` |
| `teams` | Teams, their lead, and their check-in cadence | `000005` |
| `team_members` | User ↔ team membership (many-to-many) | `000005` |
| `team_supervisors` | Denormalised supervisor chain per team | `000005` |
| `hierarchy_levels` | Organisational levels and their permission flags | `000009` |
| `password_reset_tokens` | Short-lived, single-use password reset tokens | `000012` |
| `app_settings` | Singleton row of app-wide settings and branding | `000015` |
| `action_items` | Follow-up actions a team commits to | `000020` |
| `app_config` | Key/value runtime config — **not** created by a migration | `postgres.EnsureAppConfig` |
| `schema_migrations` | `golang-migrate` bookkeeping | the migration library |

---

## `health_dimensions`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `VARCHAR(50)` | no | — | PK. Slug such as `mission`, `value`, `speed` |
| `name` | `VARCHAR(200)` | no | — | Display name |
| `description` | `TEXT` | no | — | |
| `good_description` | `TEXT` | no | — | Shown next to the green option in the survey |
| `bad_description` | `TEXT` | no | — | Shown next to the red option in the survey |
| `is_active` | `BOOLEAN` | yes | `true` | Soft-delete flag |
| `weight` | `NUMERIC(3,2)` | yes | `1.00` | |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | yes | `CURRENT_TIMESTAMP` | System-managed |

**Indexes** — `idx_dimensions_active` on `(is_active) WHERE is_active = true`.

Migration `000004` seeds exactly 11 rows and asserts the count in a `DO $$` block,
raising `Expected 11 health dimensions, but found %` if the insert is incomplete.

Maps to `organization.HealthDimension` (`backend/domain/organization/organization.go:32`).
Admin-editable through `PUT /api/v1/admin/settings/dimensions/:id`
(`settings_admin_handler.go`); `id`, `created_at` and `updated_at` are system-managed —
the admin UI generates the `id` as a slug of the name on create and hides it on edit.

!!! warning "`weight` is stored and editable but never used in any calculation"
    `organization_repository.go` reads and writes `weight` (lines 466–654), but no
    aggregation query multiplies by it. The dashboards compute an unweighted mean per
    dimension, and the frontend's "health score" in the distribution view is
    `(green·3 + yellow·2 + red·1) / total` — a weighting of *scores*, not of dimensions.
    Changing a dimension's weight currently changes nothing.

!!! warning "Deleting a dimension deactivates it"
    `DELETE /api/v1/admin/settings/dimensions/:id` is a soft delete — the admin UI even
    labels the confirm dialog "Deactivate Dimension". This is deliberate: migration
    `000013` gives `health_check_responses.dimension_id` an `ON DELETE RESTRICT` foreign
    key, so a hard delete would fail wherever historical responses exist.

## `health_check_sessions`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `VARCHAR(100)` | no | — | PK. Generated server-side as `session-{unixnano}` |
| `team_id` | `VARCHAR(50)` | no | — | **No foreign key** — see warning below |
| `user_id` | `VARCHAR(50)` | no | — | **No foreign key** |
| `date` | `DATE` | no | — | Submission date; must be RFC3339 and not in the future |
| `assessment_period` | `VARCHAR(50)` | yes | — | e.g. `2026 Q1`, `2026 H1`, `2026 Mar` |
| `completed` | `BOOLEAN` | yes | `false` | |
| `survey_type` | `VARCHAR(20)` | no | `'individual'` | `CHECK IN ('individual','post_workshop')` (`000014`) |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | yes | `CURRENT_TIMESTAMP` | System-managed |

**Indexes** — `(team_id, date DESC)`, `(user_id, date DESC)`, partial indexes on
`assessment_period`, `completed`, `(team_id, completed, date DESC)`, `survey_type`, and
`(team_id, assessment_period, survey_type) WHERE completed = true`.

!!! warning "`team_id` and `user_id` are not foreign keys"
    Migration `000013` deliberately skips them, with the inline comment: *"the demo seed
    data may reference non-existent users… application-level validation ensures
    referential integrity."* A session can therefore outlive the user or team it names,
    and deleting a user does **not** cascade to their submissions.

Maps to `healthcheck.HealthCheckSession` (`backend/domain/healthcheck/healthcheck.go:22`).
Everything except `id`, `created_at` and `updated_at` comes from the survey POST body
(`backend/application/commands/submit_health_check.go`).

## `health_check_responses`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `SERIAL` | no | sequence | PK — the only integer surrogate key in the schema |
| `session_id` | `VARCHAR(100)` | no | — | FK → `health_check_sessions(id)` `ON DELETE CASCADE` |
| `dimension_id` | `VARCHAR(50)` | no | — | FK → `health_dimensions(id)` `ON DELETE RESTRICT` |
| `score` | `SMALLINT` | no | — | `CHECK (score BETWEEN 1 AND 3)` — 1 red, 2 yellow, 3 green |
| `trend` | `VARCHAR(20)` | no | — | `CHECK IN ('improving','stable','declining')` |
| `comment` | `TEXT` | yes | — | `CHECK (length(comment) <= 1000)` (`000013`) |
| `created_at` | `TIMESTAMPTZ` | yes | `CURRENT_TIMESTAMP` | System-managed |

**Uniqueness** — one response per dimension per session, enforced twice: the unique index
`idx_responses_session_dimension` from `000003` and the named constraint
`uq_responses_session_dimension` added by `000013`.

Maps to `healthcheck.HealthCheckResponse`. In DDD terms this is a value object inside the
session aggregate — it has a database identity only so that the `CASCADE` works; nothing
in the API ever addresses a response by its `id`.

## `users`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `VARCHAR(255)` | no | — | PK. The admin UI sets it to the username on create |
| `username` | `VARCHAR(255)` | no | — | `UNIQUE`; `CHECK (username ~ '^[a-zA-Z0-9_-]{2,50}$')` |
| `email` | `VARCHAR(255)` | no | — | `UNIQUE`; `CHECK` against an email regex (`000013`) |
| `full_name` | `VARCHAR(255)` | no | — | |
| `hierarchy_level_id` | `VARCHAR(255)` | no | — | FK → `hierarchy_levels(id)`, added by `000009` |
| `reports_to` | `VARCHAR(255)` | yes | — | Self-FK → `users(id)` `ON DELETE SET NULL` |
| `password_hash` | `VARCHAR(255)` | no | `'demo'` | bcrypt; never serialised to JSON |
| `auth_type` | `VARCHAR(20)` | no | `'local'` | `CHECK IN ('local','sso')` (`000017`) |
| `created_at` / `updated_at` | `TIMESTAMP` | yes | `CURRENT_TIMESTAMP` | Note: **no** time zone |

**Indexes** — `idx_users_reports_to`, `idx_users_hierarchy_level`, `idx_users_username`.

!!! warning "The `password_hash` default is the literal string `demo`"
    Migration `000006` adds the column as `NOT NULL DEFAULT 'demo'` with the comment
    *"For now, we'll use plain text 'demo' for development"*. Any row inserted without an
    explicit hash gets an unusable non-bcrypt value rather than failing loudly. The only
    account created by migration — `admin`, from `000007` — does set a real bcrypt hash.

Maps to `user.User` (`backend/domain/user/user.go:18`). `TeamIDs` and `IsAdmin` on that
struct are **derived, not stored**: team IDs come from `team_members`, and `IsAdmin` is
computed from the hierarchy level.

## `teams`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `VARCHAR(255)` | no | — | PK |
| `name` | `VARCHAR(255)` | no | — | |
| `team_lead_id` | `VARCHAR(255)` | yes | — | FK → `users(id)` `ON DELETE SET NULL` |
| `cadence` | `VARCHAR(50)` | yes | `'quarterly'` | `CHECK IN ('monthly','quarterly','half-yearly','yearly')` |
| `distribution_list_email` | `VARCHAR(255)` | yes | — | Added by `000019`; no format constraint in SQL |
| `created_at` / `updated_at` | `TIMESTAMP` | yes | `CURRENT_TIMESTAMP` | System-managed |

Cadence drives the assessment-period format (see [Submitting a Health
Check](guides/survey.md)). Its allowed values changed in `000016`: `weekly` and `biweekly`
were dropped in favour of `half-yearly` and `yearly`, and the migration notes it is lossy —
existing `weekly`/`biweekly` rows are rewritten to `monthly` and cannot be restored.

The distribution-list address is validated in the admin UI, not the database. It is the
address post-workshop summary emails are sent to.

!!! warning "`Team.Department`, `Team.Division` and `Team.Tags` have no columns"
    These three fields exist on the Go struct (`backend/domain/team/team.go:21-23`) but no
    migration ever creates storage for them. `team_repository.go:614` makes this explicit
    for tags: *"return empty slice since there's no team_tags table in the schema"*. They
    are always empty in API responses.

## `team_members`

| Column | Type | Null | Notes |
|--------|------|------|-------|
| `team_id` | `VARCHAR(255)` | no | FK → `teams(id)` `ON DELETE CASCADE` |
| `user_id` | `VARCHAR(255)` | no | FK → `users(id)` `ON DELETE CASCADE` |
| `joined_at` | `TIMESTAMP` | yes | Defaults to `CURRENT_TIMESTAMP` |

Primary key is `(team_id, user_id)`, so a user can belong to several teams but only once
each. Index `idx_team_members_user` supports the "which teams am I in?" lookup that
populates `teamIds` in the JWT.

## `team_supervisors`

| Column | Type | Null | Notes |
|--------|------|------|-------|
| `team_id` | `VARCHAR(255)` | no | FK → `teams(id)` `ON DELETE CASCADE` |
| `user_id` | `VARCHAR(255)` | no | FK → `users(id)` `ON DELETE CASCADE` |
| `hierarchy_level_id` | `VARCHAR(255)` | no | **Not** a foreign key |
| `position` | `INT` | no | `CHECK (position > 0)`; 1 = closest supervisor |

Primary key `(team_id, user_id)`, plus `UNIQUE (team_id, position)` so two supervisors
cannot occupy the same rung. Described in the migration as *"denormalized for
performance"*: it is the materialised walk up the `users.reports_to` chain from the team
lead, and it is what the manager dashboard joins against to decide which teams a manager
supervises. Maps to `team.SupervisorLink`.

## `hierarchy_levels`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `VARCHAR(50)` | no | — | PK, e.g. `level-1` … `level-5`, `level-admin` |
| `name` | `VARCHAR(100)` | no | — | `UNIQUE` |
| `position` | `INT` | no | — | `UNIQUE`; `CHECK (position >= 0)` — 0 is Admin, 1 is the top of the org |
| `color` | `VARCHAR(20)` | yes | — | Added by `000010` |
| `can_view_all_teams` | `BOOLEAN` | yes | `false` | |
| `can_edit_teams` | `BOOLEAN` | yes | `false` | |
| `can_manage_users` | `BOOLEAN` | yes | `false` | |
| `can_take_survey` | `BOOLEAN` | yes | `true` | The only permission that defaults on |
| `can_view_analytics` | `BOOLEAN` | yes | `false` | |
| `can_configure_system` | `BOOLEAN` | yes | `false` | Added by `000011` |
| `can_view_reports` | `BOOLEAN` | yes | `false` | Added by `000011` |
| `can_export_data` | `BOOLEAN` | yes | `false` | Added by `000011` |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | yes | `CURRENT_TIMESTAMP` | System-managed |

Migration `000009` seeds the six standard levels; `000010` gives them colours and `000011`
adds the last three permission flags. Maps to `organization.HierarchyLevel` and its nested
`Permissions` struct.

!!! warning "Only five of the eight permission flags round-trip through the admin UI"
    `HierarchyConfig.tsx:79-97` maps just `canViewAllTeams`, `canEditTeams`,
    `canManageUsers`, `canTakeSurvey` and `canViewAnalytics` to the backend, and hard-codes
    the other three to `false` on read with the comment *"Frontend-only permissions (not in
    backend yet)"*. Toggling **Configure System**, **View Reports** or **Export Data** in
    the admin UI has no persisted effect, even though the columns exist and `000011`
    populated them. Likewise `color` is a real column but is never sent by the UI
    (*"Backend doesn't support color yet"*).

## `password_reset_tokens`

| Column | Type | Null | Notes |
|--------|------|------|-------|
| `id` | `VARCHAR(36)` | no | PK (UUID-shaped) |
| `user_id` | `VARCHAR(36)` | no | FK → `users(id)` `ON DELETE CASCADE` |
| `token_hash` | `VARCHAR(255)` | no | `UNIQUE`; bcrypt hash of the token, never the token itself |
| `expires_at` | `TIMESTAMPTZ` | no | One hour after issue |
| `used_at` | `TIMESTAMPTZ` | yes | `NULL` until redeemed — enforces single use |
| `created_at` | `TIMESTAMPTZ` | yes | System-managed |

!!! note "`user_id` is `VARCHAR(36)` but `users.id` is `VARCHAR(255)`"
    The foreign key is still valid, but a user whose id exceeds 36 characters cannot have
    a reset token created. Since the admin UI sets `id` to the username, and usernames are
    capped at 50 characters by `chk_users_username_format`, this is reachable.

Nothing purges expired rows automatically. `000012` adds indexes on `expires_at` and
`created_at` with the note that cleanup *"would typically be done by a scheduled job"* —
no such job exists in this codebase.

## `app_settings`

A **singleton** table: `id INTEGER PRIMARY KEY DEFAULT 1` with a named constraint
`singleton CHECK (id = 1)`, and `000015` inserts the one row immediately.

| Column | Type | Null | Default | Exposed as |
|--------|------|------|---------|-----------|
| `email_notifications` | `BOOLEAN` | no | `false` | Notification settings |
| `slack_notifications` | `BOOLEAN` | no | `false` | Notification settings |
| `weekly_digest` | `BOOLEAN` | no | `false` | Notification settings |
| `retention_months` | `INTEGER` | no | `12` | Retention policy |
| `company_name` | `TEXT` | no | `'My Company'` | Branding (`000018`) |
| `logo_url` | `TEXT` | yes | `NULL` | Branding (`000018`) |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | yes | `NOW()` | System-managed |

Maps to `organization.AppSettings`. `company_name` and `logo_url` are served publicly by
`GET /api/v1/config` so the login page can brand itself before anyone authenticates.
Despite the name, `logo_url` usually holds a `data:` URI — the admin UI uploads a file and
base64-encodes it client-side rather than storing a link.

!!! warning "`retention_months` and the notification booleans are stored but not acted on"
    Nothing in the backend reads `retention_months` to prune data, and the three
    notification flags gate no scheduled job. They are persisted preferences awaiting an
    implementation. See [Administration](guides/administration.md).

## `action_items`

| Column | Type | Null | Default | Notes |
|--------|------|------|---------|-------|
| `id` | `VARCHAR(100)` | no | — | PK, generated server-side |
| `team_id` | `VARCHAR(255)` | no | — | FK → `teams(id)` `ON DELETE CASCADE` |
| `dimension_id` | `VARCHAR(50)` | yes | — | FK → `health_dimensions(id)` `ON DELETE SET NULL` |
| `created_by` | `VARCHAR(255)` | no | — | FK → `users(id)` `ON DELETE CASCADE` — taken from JWT claims |
| `assigned_to` | `VARCHAR(255)` | yes | — | FK → `users(id)` `ON DELETE SET NULL` |
| `title` | `VARCHAR(500)` | no | — | Also `binding:"required,max=500"` at the API |
| `description` | `TEXT` | yes | — | |
| `status` | `VARCHAR(20)` | no | `'open'` | `CHECK IN ('open','in_progress','done')` |
| `due_date` | `DATE` | yes | — | API accepts `YYYY-MM-DD` only |
| `assessment_period` | `VARCHAR(50)` | yes | — | Ties an action to the check-in that prompted it |
| `created_at` / `updated_at` | `TIMESTAMPTZ` | yes | `NOW()` | System-managed |

**Indexes** — `idx_action_items_team_id`, `idx_action_items_team_status`.

`status` is system-managed on create: the `INSERT` hard-codes `'open'` and the create
endpoint has no `status` field at all. It becomes user-editable only through
`PATCH`. `created_by` comes from the authenticated token and cannot be supplied by the
client. See [Action Items](guides/action-items.md) and
[Action Items API](api/action-items.md).

## `app_config` — created outside the migration system

`postgres.EnsureAppConfig` (`backend/infrastructure/persistence/postgres/seed.go:13`) runs
a `CREATE TABLE IF NOT EXISTS app_config (key TEXT PRIMARY KEY, value TEXT NOT NULL)`
**before** the migration engine starts, and upserts `APP_ENV` into it. The comment explains
the ordering: *"This runs BEFORE migrations so that migration scripts can conditionally
execute based on the environment."*

Because it is not a migration, it has no down step and does not appear in
`schema_migrations`. Dropping the database is the only way to remove it.

## Domain mapping summary

| Aggregate root | Go type | Backing tables |
|----------------|---------|----------------|
| User | `user.User` | `users` (+ `team_members` for `TeamIDs`) |
| Team | `team.Team` | `teams`, `team_members`, `team_supervisors` |
| Health check session | `healthcheck.HealthCheckSession` | `health_check_sessions`, `health_check_responses` |
| Organization config | `organization.OrganizationConfig` | `hierarchy_levels`, `health_dimensions`, `app_settings` |

Each aggregate has one `Repository` interface in its domain package, implemented once in
`backend/infrastructure/persistence/postgres/`. The domain packages import no SQL — the
mapping direction is always infrastructure → domain.
