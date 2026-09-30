# Database Schema

The backend uses **PostgreSQL**. The schema is defined by ordered migrations in
`backend/infrastructure/persistence/postgres/migrations/` (`000001` … `000020`) and is
applied automatically on backend startup. The definitions below reflect the final state
after all migrations have run.

> The frontend demo can also run without a backend, persisting to browser `localStorage`
> (`healthCheckSessions`, `orgConfig`) with auth held in a cookie. That mode is for the
> standalone demo only; the tables below are the source of truth for the backend.

## Entity Overview

- **health_dimensions** — the 11 health-check dimensions (seeded)
- **health_check_sessions** — a user's survey submission for a team (aggregate root)
- **health_check_responses** — per-dimension scores within a session
- **users** — accounts and their place in the org hierarchy
- **teams** — teams being assessed
- **team_members** — user↔team membership (many-to-many)
- **team_supervisors** — denormalized supervisor chain per team
- **hierarchy_levels** — org levels and their role-based permissions (seeded)
- **password_reset_tokens** — short-lived, single-use reset tokens
- **app_settings** — singleton row of system/branding settings
- **action_items** — follow-up items raised against a team/dimension

## health_dimensions

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(50) | PK (e.g. `mission`, `value`, `speed`) |
| name | VARCHAR(200) | NOT NULL |
| description | TEXT | NOT NULL |
| good_description | TEXT | NOT NULL |
| bad_description | TEXT | NOT NULL |
| is_active | BOOLEAN | DEFAULT true |
| weight | NUMERIC(3,2) | DEFAULT 1.00 (aggregated-scoring weight) |
| created_at / updated_at | TIMESTAMPTZ | DEFAULT CURRENT_TIMESTAMP |

Index: `idx_dimensions_active` (partial, `WHERE is_active = true`). Seeded with 11 dimensions
(mission, value, speed, fun, health, learning, support, pawns, release, process, teamwork).

## health_check_sessions

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(100) | PK |
| team_id | VARCHAR(50) | NOT NULL |
| user_id | VARCHAR(50) | NOT NULL |
| date | DATE | NOT NULL |
| assessment_period | VARCHAR(50) | e.g. `2024 - 2nd Half` |
| completed | BOOLEAN | DEFAULT false |
| survey_type | VARCHAR(20) | NOT NULL DEFAULT `individual`; CHECK in (`individual`, `post_workshop`) |
| created_at / updated_at | TIMESTAMPTZ | DEFAULT CURRENT_TIMESTAMP |

Indexes: `(team_id, date DESC)`, `(user_id, date DESC)`, `assessment_period` (partial),
`completed` (partial), `(team_id, completed, date DESC)` (partial), `survey_type`,
`(team_id, assessment_period, survey_type)` (partial, completed only).

## health_check_responses

| Column | Type | Notes |
|--------|------|-------|
| id | SERIAL | PK |
| session_id | VARCHAR(100) | NOT NULL → `health_check_sessions(id)` ON DELETE CASCADE |
| dimension_id | VARCHAR(50) | NOT NULL → `health_dimensions(id)` ON DELETE RESTRICT |
| score | SMALLINT | NOT NULL; CHECK 1–3 (1=red, 2=yellow, 3=green) |
| trend | VARCHAR(20) | NOT NULL; CHECK in (`improving`, `stable`, `declining`) |
| comment | TEXT | CHECK length ≤ 1000 |
| created_at | TIMESTAMPTZ | DEFAULT CURRENT_TIMESTAMP |

Constraints/indexes: UNIQUE `(session_id, dimension_id)` (one response per dimension per
session), `(dimension_id, score)`, `session_id`.

## users

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(255) | PK |
| username | VARCHAR(255) | UNIQUE NOT NULL; CHECK `^[a-zA-Z0-9_-]{2,50}$` |
| email | VARCHAR(255) | UNIQUE NOT NULL; CHECK email-format |
| full_name | VARCHAR(255) | NOT NULL |
| hierarchy_level_id | VARCHAR(255) | NOT NULL → `hierarchy_levels(id)` |
| reports_to | VARCHAR(255) | → `users(id)` ON DELETE SET NULL (self-reference) |
| password_hash | VARCHAR(255) | NOT NULL DEFAULT `demo` (bcrypt in real use) |
| auth_type | VARCHAR(20) | NOT NULL DEFAULT `local`; CHECK in (`local`, `sso`) |
| created_at / updated_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP |

Indexes: `reports_to`, `hierarchy_level_id`, `username`.

## teams

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(255) | PK |
| name | VARCHAR(255) | NOT NULL |
| team_lead_id | VARCHAR(255) | → `users(id)` ON DELETE SET NULL |
| cadence | VARCHAR(50) | DEFAULT `quarterly`; CHECK in (`monthly`, `quarterly`, `half-yearly`, `yearly`) |
| distribution_list_email | VARCHAR(255) | nullable |
| created_at / updated_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP |

## team_members

Junction table (users can belong to multiple teams).

| Column | Type | Notes |
|--------|------|-------|
| team_id | VARCHAR(255) | → `teams(id)` ON DELETE CASCADE |
| user_id | VARCHAR(255) | → `users(id)` ON DELETE CASCADE |
| joined_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP |

PK `(team_id, user_id)`. Index: `user_id`.

## team_supervisors

Denormalized supervisor chain per team (for query performance).

| Column | Type | Notes |
|--------|------|-------|
| team_id | VARCHAR(255) | → `teams(id)` ON DELETE CASCADE |
| user_id | VARCHAR(255) | → `users(id)` ON DELETE CASCADE |
| hierarchy_level_id | VARCHAR(255) | NOT NULL |
| position | INT | NOT NULL; CHECK > 0 (1 = closest supervisor) |

PK `(team_id, user_id)`, UNIQUE `(team_id, position)`. Indexes: `user_id`, `(team_id, position)`.

## hierarchy_levels

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(50) | PK |
| name | VARCHAR(100) | NOT NULL UNIQUE |
| position | INT | NOT NULL UNIQUE; CHECK ≥ 0 (0 = Admin, 1 = VP … 5 = Team Member) |
| can_view_all_teams | BOOLEAN | DEFAULT false |
| can_edit_teams | BOOLEAN | DEFAULT false |
| can_manage_users | BOOLEAN | DEFAULT false |
| can_take_survey | BOOLEAN | DEFAULT true |
| can_view_analytics | BOOLEAN | DEFAULT false |
| can_configure_system | BOOLEAN | DEFAULT false |
| can_view_reports | BOOLEAN | DEFAULT false |
| can_export_data | BOOLEAN | DEFAULT false |
| color | VARCHAR(20) | UI accent color |
| created_at / updated_at | TIMESTAMPTZ | DEFAULT CURRENT_TIMESTAMP |

Index: `position`. Seeded with `level-1` (VP) … `level-5` (Team Member) and `level-admin`.
`users.hierarchy_level_id` references this table.

## password_reset_tokens

Short-lived (≈1 hour), single-use tokens.

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(36) | PK |
| user_id | VARCHAR(36) | NOT NULL → `users(id)` ON DELETE CASCADE |
| token_hash | VARCHAR(255) | NOT NULL UNIQUE (bcrypt hash of the token) |
| expires_at | TIMESTAMPTZ | NOT NULL |
| used_at | TIMESTAMPTZ | NULL until used |
| created_at | TIMESTAMPTZ | DEFAULT CURRENT_TIMESTAMP |

Indexes: `user_id`, `expires_at`, `created_at`.

## app_settings

Singleton row (`id` is constrained to `1`).

| Column | Type | Notes |
|--------|------|-------|
| id | INTEGER | PK DEFAULT 1; CHECK (id = 1) |
| email_notifications | BOOLEAN | NOT NULL DEFAULT false |
| slack_notifications | BOOLEAN | NOT NULL DEFAULT false |
| weekly_digest | BOOLEAN | NOT NULL DEFAULT false |
| retention_months | INTEGER | NOT NULL DEFAULT 12 |
| company_name | TEXT | NOT NULL DEFAULT `My Company` |
| logo_url | TEXT | nullable |
| created_at / updated_at | TIMESTAMPTZ | DEFAULT NOW() |

## action_items

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(100) | PK |
| team_id | VARCHAR(255) | NOT NULL → `teams(id)` ON DELETE CASCADE |
| dimension_id | VARCHAR(50) | → `health_dimensions(id)` ON DELETE SET NULL |
| created_by | VARCHAR(255) | NOT NULL → `users(id)` ON DELETE CASCADE |
| assigned_to | VARCHAR(255) | → `users(id)` ON DELETE SET NULL |
| title | VARCHAR(500) | NOT NULL |
| description | TEXT | nullable |
| status | VARCHAR(20) | NOT NULL DEFAULT `open`; CHECK in (`open`, `in_progress`, `done`) |
| due_date | DATE | nullable |
| assessment_period | VARCHAR(50) | nullable |
| created_at / updated_at | TIMESTAMPTZ | DEFAULT NOW() |

Indexes: `team_id`, `(team_id, status)`.
