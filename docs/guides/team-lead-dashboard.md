# Team Lead Dashboard

`/dashboard` is where a Team Lead (`level-4`) reads their team's health check results. It
is the only screen that shows individual responses, and the only place action items can be
created.

The header carries the team name, a **Take Survey** button, the post-workshop control, and
— if you lead more than one team — a **team selector**. Switching teams clears every chart
and refetches from scratch.

## Filtering by assessment period

A single `Assessment Period` dropdown at the top governs the page. Its options come from
`GET /api/v1/assessment-periods` (every period that has submissions), and it defaults to
**All Periods**.

!!! warning "The period filter does not apply to the Trends tab"
    Health summary, response distribution and individual responses all take
    `?assessmentPeriod=`, but the trends fetch deliberately does not — the comment at
    `frontend/app/dashboard/page.tsx:242` reads *"trends don't filter by period - they show
    all periods"*. Trends are always the full history, whatever the dropdown says.

## The five tabs

### Radar Chart

A radar plot of the average score per dimension on a 0–3 axis — the classic squad health
shape. Above it sits a score-band legend that is used consistently across the page:

| Band | Range |
|------|-------|
| Excellent | 2.7 – 3.0 |
| Good | 2.3 – 2.6 |
| Fair | 1.7 – 2.2 |
| Poor | 1.0 – 1.6 |

### Response Distribution

Two views, toggled by **By Dimension** / **Chart**:

- **By Dimension** (the default) is a stacked green/yellow/red bar per dimension, sorted
  worst-first — *"Sorted by health score — most attention needed first"* — with each
  dimension's response count. The health score behind the sort is
  `(green·3 + yellow·2 + red·1) / total`.
- **Chart** is a grouped bar chart with one bar per colour per dimension.

### Individual Responses

This is the tab that makes a workshop discussion possible, and it has two views:

- **Matrix** (default) — a table of members × dimensions, each cell a coloured dot plus a
  trend icon, with a small indigo dot marking cells that carry a comment. Hovering a cell
  shows the dimension, score, trend and full comment. The last row is the **Team Average**
  per dimension. Dimension names are abbreviated in the header (`Delivering Value` →
  `D.Value`, `Pawns or Players` → `Autonomy`, and so on).
- **Cards** — one collapsible card per member, tagged **Post-Workshop** (amber) or
  **Individual** (blue), with `Collapse all` / `Expand all`.

!!! note "Individual responses are not anonymous to the team lead"
    Every score is attributed by name here. That is intentional for a workshop-style
    review, but it is worth being explicit about with the team beforehand — the model
    depends on honest answers, and Team360's design principles treat health checks as a
    support tool rather than an evaluation.

### Trends

Also two views:

- **By Dimension** (default) — a grid of sparkline cards, one per dimension, each showing
  the latest score, a direction label (Improving / Declining / Stable, using a ±0.1
  threshold) and a mini line chart. Cards with no data are dropped; a single data point is
  labelled `Single period`.
- **Overview** — all 11 dimensions on one line chart, y-axis 1–3 with the ticks labelled
  Red / Yellow / Green.

### Actions

Embeds the shared action items board. See [Action Items](action-items.md) for what it does.

The tab is pre-wired to the team's problem area: the dashboard passes the id of the
**worst-scoring dimension** as `defaultDimensionId`, so a new action item is linked to it
by default. It also passes the current period filter and the team's member list for the
assignee picker.

!!! warning "`canEdit` is hard-coded to `true`"
    `dashboard/page.tsx:1475` passes `canEdit={true}` unconditionally, even though the
    prop is documented as *"Team Lead and above"*. Anyone who can load this page can create
    and delete action items. The real guard is the API's team-membership middleware — see
    [Action Items](action-items.md#who-can-do-what).

## Post-workshop status

Next to **Take Survey** the header shows one of two things, driven by `postWorkshopExists`
from `GET /api/v1/teams/{teamId}/submission-status`:

- an amber **Post-Workshop Survey** button linking to
  `/survey?type=post_workshop&team={teamId}`, or
- a green **Workshop Submitted** badge, once a consensus submission exists for the current
  period.

The period used for that check is the team's *current* period derived from its cadence, not
whatever the page filter is set to.

!!! note "Participation counts are fetched but not displayed"
    The same endpoint returns `totalMembers`, `submittedMembers` and `allSubmitted`, but
    the page renders only `postWorkshopExists`. There is no "6 of 8 submitted" indicator
    anywhere on this dashboard; to see who has responded, open the Individual Responses
    tab.

## Export to Excel

The **Export to Excel** button appears when you have the `canExportData` permission and
there is health data to export. It builds the workbook in the browser with
[SheetJS](https://sheetjs.com/) (`xlsx`) — nothing is generated server-side — and saves
`health-check-{team}-{period}.xlsx` with four sheets:

| Sheet | Columns |
|-------|---------|
| `Summary` | Dimension, Average Score, Band |
| `Individual Responses` | Member, Survey Type, Date, Dimension, Score, Score Label, Trend, Comment |
| `Distribution` | Dimension, Green, Yellow, Red, Total, Health Score, Band |
| `Action Items` | Title, Dimension, Status, Assigned To, Due Date, Description, Created By, Created At |

The action-items sheet is fetched on demand and falls back to a single `No action items`
row when the team has none.

!!! note "This is export only — there is no import"
    Despite the `xlsx` dependency being a read/write library, the dashboard only ever calls
    `XLSX.writeFile`. There is no path for loading a spreadsheet back into Team360.

The `canExportData` permission comes from `frontend/lib/org-config.ts`, which reads a
hardcoded default config (optionally overridden from `localStorage`), **not** from the
hierarchy levels stored in the database. In the default config `level-5` has no export
permission and `level-4` does.

## Endpoints this page uses

| Purpose | Endpoint |
|---------|----------|
| Branding | `GET /api/v1/config` |
| Radar data | `GET /api/v1/teams/{teamId}/dashboard/health-summary` |
| Distribution | `GET /api/v1/teams/{teamId}/dashboard/response-distribution` |
| Matrix / cards | `GET /api/v1/teams/{teamId}/dashboard/individual-responses` |
| Trends | `GET /api/v1/teams/{teamId}/dashboard/trends` |
| Period options | `GET /api/v1/assessment-periods` |
| Workshop status | `GET /api/v1/teams/{teamId}/submission-status` |
| Actions tab & export | `GET /api/v1/teams/{teamId}/action-items` |

Full request and response shapes are in the [Team Dashboard API](../api/team-dashboard.md)
reference. Any 5xx surfaces as the banner *"Unable to load dashboard data. Please refresh
the page."*
