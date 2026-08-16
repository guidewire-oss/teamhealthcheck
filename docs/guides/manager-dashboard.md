# Manager Dashboard

`/manager` is the roll-up view for anyone above a team lead — Managers (`level-3`),
Directors (`level-2`) and VPs (`level-1`) all land here. It answers "which of my teams need
support?" rather than "what did each person say?".

## How your teams are scoped

Scoping happens **server-side, by your own user id**. The page calls
`GET /api/v1/managers/{yourId}/teams/health` and renders whatever comes back; it does not
filter client-side.

!!! note "This page does not use `canUserAccessTeam` or `getSubordinates`"
    Those helpers exist in `frontend/lib/org-config.ts` and implement the documented rules
    — `canViewAllTeams` short-circuits, then team membership, then presence in the team's
    `supervisorChain`; `getSubordinates` walks `reportsTo` transitively — but
    `frontend/app/manager/page.tsx` imports none of them. Access is decided by the
    backend's join against `team_supervisors`, which is the materialised supervisor chain
    (see [Data Model](../data-model.md#team_supervisors)). If a team is missing from your
    dashboard, the fix is the team lead's **Reports To** field, not a frontend setting.

If nothing comes back you see *"You are not currently supervising any teams"*, or — when a
period filter is applied — *"No teams have submitted health checks for {period}"*.

## The seven tabs

**Team Cards** (default) — one card per team showing overall health out of 3, the same
value as a percentage, the submission count, and a workshop badge: green **Workshop
Complete**, amber **Pending Workshop**, or nothing when the team has no status. Each card
expands with **View Dimension Details** to show per-dimension averages and response counts.
Below the cards sits a summary block: Total Teams, Total Submissions, Average Health.

**Radar** — a single aggregated radar across all your teams, from
`GET /api/v1/managers/{id}/dashboard/radar`.

**Trends** — sparkline cards per dimension (default) or one multi-line overview chart. A
row of team pills lets you narrow from **All Teams** to a single team, which switches the
fetch to that team's own trends endpoint.

**Hierarchy View** — a recursive org tree rooted at you, built from your subordinates'
`reportsTo` values (`GET /api/v1/managers/{id}/subordinates`), with each person's level and
team names.

**Summary View** — three stat tiles (Overall Health Score, Total Teams, Total Submissions)
plus a **Recent Activity** list of your five most active teams by submission count.

**Comparison** — a multi-select team picker (Ctrl/Cmd-click) that renders a side-by-side
table of Overall Health, Submissions and Dimensions Tracked.

**Actions** — see below.

## Action items across teams

The Actions tab lazily calls `GET /api/v1/managers/{id}/teams/action-items` and renders one
row per supervised team: team name, the count of items that are **open or in progress**,
and a status badge:

| Open count | Badge |
|-----------|-------|
| 0 | green — All done |
| 1–3 | yellow — In progress |
| 4+ | red — Needs attention |

The empty state reads *"Team Leads create action items on their team dashboards"*, which
is exactly right: this view is read-only. There is no way to create, edit or open an
individual action item from the manager dashboard — see [Action Items](action-items.md).

The underlying query counts rows where `status != 'done'`, joined through
`team_supervisors`, and the handler rejects any request where the path's `managerId` is not
your own user id. You cannot read another manager's summary.

## Period filter and export

The `Assessment Period` dropdown works the same way as on the team dashboard: options come
from `GET /api/v1/assessment-periods`, the default is **All Periods**, and — as there —
**it is not applied to the Trends tab**.

**Export to Excel** produces `health-check-manager-{period}.xlsx` with three sheets:

| Sheet | Columns |
|-------|---------|
| `Team Summary` | Team, Overall Health Score, Band, Submission Count |
| `Dimension Breakdown` | Team, Dimension, Average Score, Band, Response Count |
| `Action Items` | Team, Title, Dimension, Status, Assigned To, Due Date, Description, Created By, Created At |

The action-items sheet fans out one request per team; a team whose request fails
contributes no rows rather than failing the export.

!!! warning "The manager export ignores the `canExportData` permission"
    The team lead dashboard hides its export button behind
    `userPermissions.canExportData`; the manager dashboard shows its button whenever there
    is at least one team (`manager/page.tsx:502-511`). Any user who can reach `/manager`
    can export.

## Drilling into a team

There are three ways down, and none of them is a link to `/dashboard`:

1. **View Dimension Details** on a team card.
2. The **team pills** on the Trends tab.
3. The **Comparison** tab's side-by-side table.

Individual responses and comments are not visible from this page at all — that data stays
on the team lead's dashboard.

!!! note "Dimension names are shown raw in the card drill-down"
    The expanded card lists dimension ids (`speed`, `pawns`, `release`, …) rather than
    display names, with `value` → `Delivering Value` special-cased
    (`manager/page.tsx:982-984`). Other tabs resolve names properly against
    `HEALTH_DIMENSIONS`.

## Endpoints this page uses

| Purpose | Endpoint |
|---------|----------|
| Branding | `GET /api/v1/config` |
| Team cards / summary | `GET /api/v1/managers/{id}/teams/health` |
| Hierarchy tree | `GET /api/v1/managers/{id}/subordinates` |
| Radar | `GET /api/v1/managers/{id}/dashboard/radar` |
| Trends | `GET /api/v1/managers/{id}/dashboard/trends` or `GET /api/v1/teams/{teamId}/dashboard/trends` |
| Actions tab | `GET /api/v1/managers/{id}/teams/action-items` |
| Period options | `GET /api/v1/assessment-periods` |

See the [Managers API](../api/managers.md) reference for shapes and guards.
