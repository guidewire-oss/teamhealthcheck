# Administration

`/admin` is reachable only by users whose hierarchy level is `level-1` or `level-admin`.
The page redirects non-admins client-side, and every route under `/api/v1/admin` is guarded
by `JWTAuthMiddleware` followed by `AdminOnlyMiddleware`, which rejects anything else with
*"Access denied: admin privileges required"*.

There are four tabs: **Hierarchy**, **Teams**, **Users**, **Settings**. Each loads its data
on first visit.

## Hierarchy

Define the organisational levels every other permission decision hangs off. **Add Level**
creates one; each row can be renamed, reordered with the up/down arrows, or deleted (with a
confirmation — this is a hard delete).

The permission checkboxes on a level are:

| Permission | Persisted? |
|-----------|-----------|
| View All Teams | yes |
| Edit Teams | yes |
| Manage Users | yes |
| Take Survey | yes |
| View Analytics | yes |
| Configure System | **no** |
| View Reports | **no** |
| Export Data | **no** |

!!! warning "Three permission toggles are inert, and so is the colour picker"
    `HierarchyConfig.tsx:79-97` sends only the first five permissions to the backend and
    hard-codes the last three to `false` when reading, with the comment *"Frontend-only
    permissions (not in backend yet)"*. The columns exist in the database and migration
    `000011` seeded sensible values, but the admin UI cannot change them and will always
    display them as off. The colour field is the same story — the level list renders a
    fixed indigo swatch and the request omits `color` entirely (*"Backend doesn't support
    color yet"*). See [`hierarchy_levels`](../data-model.md#hierarchy_levels).

**Position** is assigned automatically on create (next free slot) and changed only via the
reorder arrows, which swap positions with the neighbouring level. Position 0 is the admin
level; 1 is the top of the org chart.

Because `users.hierarchy_level_id` is a foreign key, deleting a level that still has users
assigned will fail at the database.

## Teams

Create, edit and delete teams. The form has exactly four fields:

- **Team Name** (required).
- **Team Lead** — a dropdown of users at `level-2`, `level-3` or `level-4`.
- **Cadence** (required) — Monthly, Quarterly, Half-Yearly or Yearly, defaulting to
  Monthly. This is the field that decides how the survey buckets submissions into
  assessment periods; see [Submitting a Health Check](survey.md#the-assessment-period-is-automatic).
- **Distribution List Email** — where post-workshop survey summaries are sent. Validated
  against an email regex in the browser only; the column has no SQL constraint.

The team list is searchable by name or team lead. Each row offers **Edit**, **Manage
members**, **View hierarchy** and **Delete**.

**Manage members** adds and removes users on the team — nothing else. The search list is
capped at ten results at a time, so type enough to narrow it.

**View hierarchy** is read-only. It shows the team's supervisor chain and explains why
there is nothing to edit: *"This hierarchy is automatically derived from the team lead's
reporting chain. To change it, update the 'Reports To' field on the relevant users."* If it
is empty, the team lead has no supervisor set.

!!! note "`PUT /api/v1/admin/teams/:id/supervisors` exists but nothing calls it"
    The backend can accept an explicitly-set supervisor chain
    (`admin_routes.go:52`), but no frontend code invokes it. In the UI the chain is always
    derived. This matters because the manager dashboard scopes teams by joining against
    `team_supervisors` — a manager missing a team is almost always a missing **Reports To**
    somewhere up the chain.

## Users

Create, edit and delete users. Fields:

- **Full Name**, **Username**, **Email** — all required. Username and email are both unique
  and both have `CHECK` constraints in the database.
- **Authentication Type** — a radio pair, **Local (Password)** or **SSO (Identity
  Provider)**. Choosing SSO hides and clears the password field. A user must be set to SSO
  here before they can sign in through an identity provider; SSO does **not** auto-provision
  accounts. See [Signing In](authentication.md#single-sign-on).
- **Password** — shown only for local users. Required on create; on edit the label reads
  *"leave blank to keep current"*, which makes this form the de-facto password reset tool
  given there is no self-service reset UI.
- **Role** — the hierarchy level.
- **Reports To** — restricted to users at a strictly higher level than the one selected,
  and disabled until a role is chosen. This is what builds the supervisor chains.

On create the user's `id` is set to their username.

The list is searchable and filterable by role, shows an **SSO** badge on federated
accounts, and blocks two deletions outright: your own account (*"You cannot delete your own
account"*) and the built-in `admin` (*"Cannot delete the admin user"*).

## Settings

### Health Dimensions

Full CRUD over the dimensions: name, an auto-generated slug id (editable on create only),
description, good-state description, bad-state description, weight, and an active flag.
Name and both state descriptions are required.

The delete button is a **deactivation** — the dialog is titled "Deactivate Dimension" and
explains that *"the dimension will be hidden from new surveys but historical data will be
preserved."* That is the correct behaviour given the `ON DELETE RESTRICT` foreign key from
`health_check_responses`.

!!! warning "Dimension changes do not reach the survey"
    This is the single most important caveat on this page. The survey renders the
    hardcoded `HEALTH_DIMENSIONS` constant in `frontend/lib/data.ts`, so a dimension you
    add here will never be asked, one you deactivate will still be asked, and a renamed one
    will keep its old wording. See [the note on the home page](../index.md#health-dimensions).

!!! warning "`weight` is stored but never used"
    No aggregation query multiplies by it. Changing a dimension's weight has no effect on
    any dashboard number today. See [`health_dimensions`](../data-model.md#health_dimensions).

### Company Branding

**Company Name** (max 100 characters) and **Company Logo**. The logo is a file upload, not
a URL: PNG, JPEG or WebP under 500 KB, read client-side into a data URI and stored in
`app_settings.logo_url`. Both are served publicly by `GET /api/v1/config` so the login page
can brand itself before anyone has authenticated.

### Email and distribution lists

A read-only banner appears when SMTP is not configured: *"Email notifications are disabled.
Set SMTP_HOST and related environment variables to enable email delivery."* Email delivery
is configured by environment variable only — see
[Configuration](../operations/configuration.md).

Below it, a read-only table lists each team's distribution list email. Editing happens on
the Teams tab.

### Notification Settings

Three checkboxes, persisted to `app_settings`.

!!! warning "The notification checkbox labels do not match the fields they set"
    `admin/page.tsx:1733-1769` wires the checkbox labelled *"Send email reminders for
    upcoming health checks"* to `emailEnabled`, *"Notify managers when team health
    declines"* to `slackEnabled`, and *"Send weekly summary reports"* to
    `notifyOnSubmission`. The stored columns are `email_notifications`,
    `slack_notifications` and `weekly_digest`. Beyond the mislabelling, no scheduled job in
    this codebase reads any of the three — they are preferences awaiting an implementation.

### Data Retention Policy

**Keep health check data for** — 6 months, 1 year, 2 years or Forever, persisted to
`app_settings.retention_months`.

!!! warning "Nothing enforces the retention policy, and the export dropdown is dead"
    No code reads `retention_months` to delete anything. The adjacent **Export format**
    dropdown (CSV / JSON / Excel) has no value binding, no change handler and no state — it
    is never read when settings are saved.

## What the admin UI does *not* have

Three things are worth stating plainly, because they are commonly assumed:

- **No CSV or Excel export.** The only occurrence of the word "export" on this page is the
  inert dropdown described above. Export exists, but on the [team lead](team-lead-dashboard.md#export-to-excel)
  and [manager](manager-dashboard.md#period-filter-and-export) dashboards, generated in the
  browser.
- **No way to send an email.** The page toggles notification booleans and shows SMTP
  status; there is no "send test email" or "notify now" action.
- **No audit history.** There is no change log, no audit tab, and no UI over who changed
  what. Authorization failures are written to the backend log by `AdminOnlyMiddleware`, but
  nothing surfaces them.

## Admin API routes

| Group | Routes |
|-------|--------|
| Hierarchy levels | `GET`/`POST` `/hierarchy-levels`, `PUT`/`DELETE` `/hierarchy-levels/:id`, `PUT /hierarchy-levels/:id/position` |
| Users | `GET`/`POST` `/users`, `PUT`/`DELETE` `/users/:id` |
| Teams | `GET`/`POST` `/teams`, `PUT`/`DELETE` `/teams/:id`, `GET`/`POST` `/teams/:id/members`, `DELETE /teams/:id/members/:userId`, `GET`/`PUT` `/teams/:id/supervisors` |
| Settings | `GET`/`POST` `/settings/dimensions`, `PUT`/`DELETE` `/settings/dimensions/:id`, `GET`/`PUT` `/settings/branding`, `/settings/notifications`, `/settings/retention` |

All are prefixed `/api/v1/admin` and carry the same two guards; none has additional
per-route middleware. Full details in the [Admin API](../api/admin.md) reference.
