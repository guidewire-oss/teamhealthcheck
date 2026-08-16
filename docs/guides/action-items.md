# Action Items

An action item is a follow-up a team commits to after looking at its health check results:
a short title, optionally linked to the dimension that prompted it, optionally assigned to
a team member with a due date, moving through three states.

They exist because a health check on its own changes nothing. The intended loop is: score →
discuss → agree on one or two concrete changes → track them until the next check-in.

## Where they live

The board is the **Actions** tab on the [Team Lead Dashboard](team-lead-dashboard.md). It
is a three-column Kanban:

| Column | Status value | Icon |
|--------|--------------|------|
| Open | `open` | clock |
| In Progress | `in_progress` | arrow |
| Done | `done` | check |

Above the columns is a counter reading `{open + in progress} active · {done} done`. The
empty state nudges toward the workflow: *"After reviewing the dashboard, click + Add action
to track improvements."*

Managers see a different, read-only view — a per-team count of open items on the
[Manager Dashboard](manager-dashboard.md) Actions tab. There is no drill-down from there
into the items themselves.

## Creating one

**+ Add action** opens a modal with:

| Field | Notes |
|-------|-------|
| **Title** (required) | Max 500 characters. Placeholder: *"e.g. Run daily standups to improve Speed"* |
| **Dimension** | Optional. Defaults to your team's **worst-scoring dimension** |
| **Description** | Free text |
| **Assign to** | Optional, chosen from your team's members |
| **Due date** | Optional date picker |

The assessment period currently selected on the dashboard is attached automatically, which
is what ties an action to the check-in that produced it.

The worst-dimension default is the small design touch that makes the tab useful: open the
Actions tab straight after reading the radar chart and the form is already pointed at the
thing most in need of attention.

You cannot set a status on create. The API has no `status` field on the create request and
the insert hard-codes `'open'`.

!!! warning "The dimension picker uses the hardcoded dimension list"
    `ActionItemModal.tsx` populates its dropdown from the `HEALTH_DIMENSIONS` constant in
    `frontend/lib/data.ts`, not from `GET /api/v1/admin/settings/dimensions`. This is the
    same limitation as the survey — see the
    [dimensions caveat](../index.md#health-dimensions). Dimensions added through the admin
    UI cannot be linked to an action item.

## Moving one along

Each card carries a single advance button rather than a status dropdown: **Start** on an
open item, **Mark done** on one in progress, and nothing on a done item. Delete asks for
confirmation (`Delete "{title}"?`).

Overdue items — a due date in the past on a non-done item — render red with a `⚠` marker.
The date is parsed as a *local* date deliberately, to avoid an off-by-one day in non-UTC
time zones.

!!! note "There are no filters on the board"
    The API's list endpoint accepts optional `status` and `period` query parameters, but
    the tab never sends them: it fetches every action item for the team and splits them
    into columns client-side. On a team with a long history, the Done column grows without
    bound.

## Who can do what

Authorization is by **team membership**, not by role or ownership. All four team-scoped
routes sit behind `JWTAuthMiddleware` plus `TeamMembershipMiddleware("teamId")`:

- Users at `level-1`, `level-2`, `level-3` or `level-admin` pass for **any** team.
- Everyone else must have that team id in their JWT's team list.

`createdBy` is taken from the token claims and cannot be spoofed by the client, and
`assignedTo` is validated to be an actual member of the team (`assignedTo user is not a
member of this team`). Update and delete are scoped by `WHERE id = $1 AND team_id = $2`, so
you cannot touch another team's items by guessing an id.

!!! warning "Any team member can edit or delete any of their team's action items"
    There is no ownership or role check inside the handlers — not on the creator, not on
    the assignee. A `level-5` team member can delete an item their lead created. Combined
    with the dashboard passing `canEdit={true}` unconditionally, the practical rule is:
    **if you can see the board, you can change everything on it.** Treat action items as
    shared team state, not as assignments handed down.

The manager summary endpoint is the exception: it checks that the `managerId` in the path
matches your own user id and returns 403 otherwise.

## API reference

Route table, request and response shapes, validation rules and error codes are in the
[Action Items API](../api/action-items.md) reference. In short: `title` is required and
capped at 500 characters, `dueDate` must be `YYYY-MM-DD`, and `status` on update must be
one of `open`, `in_progress`, `done`.

The storage schema — including which columns are system-managed — is documented under
[`action_items`](../data-model.md#action_items).
