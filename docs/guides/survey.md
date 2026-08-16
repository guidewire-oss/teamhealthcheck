# Submitting a Health Check

The survey at `/survey` is the one screen every Team360 user touches. It walks through the
health dimensions one at a time, collects a red/yellow/green score for each, and submits
them as a single session.

Reaching it requires `canTakeSurvey` on your hierarchy level — `frontend/middleware.ts`
redirects anyone else back to their landing page. In the seeded hierarchy that means Team
Leads (`level-4`) and Team Members (`level-5`).

## Which team you are scored against

The page picks a team in this order (`frontend/app/survey/page.tsx:70-72`):

1. the `?team=` query parameter, **but only if** you are actually a member of that team;
2. otherwise your first team;
3. otherwise nothing — you get *"No team assigned to your account. Please contact your
   administrator."*

If you belong to more than one team a **team selector** appears in the header. Switching
teams saves your progress on the old team, then resets the form and loads any draft for
the new one.

## The assessment period is automatic

You never choose a period. It is computed from today's date and your **team's cadence**
(`frontend/lib/assessment-period.ts`), and shown in the header as `Period: …`:

| Team cadence | Period format | Example |
|--------------|---------------|---------|
| `monthly` | `YYYY Mon` | `2026 Mar` |
| `quarterly` | `YYYY Qn` | `2026 Q1` |
| `half-yearly` | `YYYY Hn` | `2026 H1` (Jan–Jun) / `2026 H2` (Jul–Dec) |
| `yearly` | `YYYY` | `2026` |

Any unrecognised cadence falls back to `half-yearly`. This is why a team's cadence is worth
setting correctly in the admin UI: it decides how submissions are bucketed for every trend
line and dashboard filter downstream.

## Answering a dimension

Each screen shows one dimension with a progress bar (`Question n of 11`) and three score
buttons:

| Button | Score stored | Caption |
|--------|--------------|---------|
| **Red** | `1` | The dimension's `badDescription` |
| **Yellow** | `2` | "Some problems, but we are working on it" |
| **Green** | `3` | The dimension's `goodDescription` |

Below the score, when applicable, is a **trend** — Improving, Stable or Declining — and an
optional **Comments** box capped at 1000 characters with a live counter that turns red past
950. The trend selector appears only after you have chosen a score.

!!! note "Team members do not see the trend selector on an individual survey"
    The trend block is rendered only when `!isTeamMember || isPostWorkshop`
    (`survey/page.tsx:627`). For a `level-5` user filling in a normal survey it is hidden
    and the response is stored with `trend: 'stable'`. Everyone else — and everyone on a
    post-workshop survey — must pick one before advancing.

**Next** is blocked until the required fields are set (*"Please select a score (Red,
Yellow, or Green) before continuing."*), and **Submit Responses** on the last dimension
re-checks that all 11 are complete.

First-time team members get a **scoring guide** panel that opens itself for three seconds
on the first question; after that it is behind a toggle. The "seen" flag lives in
`localStorage` under `survey_help_seen:{userId}`.

## Your progress is saved as you go

Every change is written, after a 300 ms debounce, to `localStorage` under
`surveyDraft:{userId}:{teamId}`. Returning to the survey restores it — but **only if the
draft's assessment period matches the current one**, so a stale draft from last quarter is
ignored rather than silently mixed in. When a draft is restored a dismissible banner says
so. Leaving the page with unsaved answers triggers the browser's "are you sure?" prompt.
The draft is deleted on successful submission.

## Submitting

Submission POSTs to `/api/v1/health-checks` with the team, user, date, computed assessment
period, survey type, and the array of `{ dimensionId, score, trend, comment }`. The server
re-validates everything: the date must be RFC3339 and not in the future, the assessment
period must match one of the recognised formats, scores must be 1–3, and the survey type
must be `individual` or `post_workshop`. See the [Health Checks API](../api/health-checks.md).

After a successful submit you are redirected to `/home` (or to `/dashboard` after a
post-workshop survey).

!!! warning "There is no duplicate-submission guard"
    The survey page never checks whether you have already submitted for this period, and
    the repository upserts on session `id` only — but the client never sends an `id`, so
    the server mints a fresh `session-{unixnano}` every time
    (`backend/application/commands/submit_health_check.go:45-47`). Submitting twice
    produces two sessions and both are counted in the averages. The only place duplicate
    suppression exists is the post-workshop button on the team dashboard, which is replaced
    by a "Workshop Submitted" badge once one exists.

## Post-workshop surveys

Team360 supports the Spotify workflow where individuals score privately, the team then
discusses the results together, and the team records its **consensus** as a single
submission. That consensus is a post-workshop survey.

You reach it from the team lead dashboard's amber **Post-Workshop Survey** button, which
links to `/survey?type=post_workshop&team={teamId}`. The page reads the `type` parameter and
changes accordingly:

- the title becomes **Post-Workshop Survey** and the header turns amber;
- a banner reads *"Record your team's consensus from the workshop discussion"*;
- the trend selector is required for everyone, including team members;
- the payload carries `surveyType: 'post_workshop'`, which is what the dashboards use to
  distinguish consensus results from individual ones;
- submitting returns you to `/dashboard` rather than `/home`.

Any other value of `type` — or none — is treated as `individual`.

!!! warning "The post-workshop survey has no role gate of its own"
    Nothing in `survey/page.tsx` restricts the `post_workshop` type to team leads. Anyone
    who can take a survey at all can submit a consensus record by visiting the URL
    directly. The distinction is enforced only by where the button is placed in the UI.

## Which dimensions you see

The survey renders the hardcoded `HEALTH_DIMENSIONS` constant in `frontend/lib/data.ts` —
the same 11 dimensions that migration `000004` seeds. It never calls
`GET /api/v1/health-dimensions`. This is the caveat described on the
[home page](../index.md#health-dimensions): dimensions added, renamed, reweighted or
deactivated through the admin UI will not appear here.

Because responses are stored against `dimension_id`, a dimension added only in the database
also has no way to receive answers, and one removed from the constant would leave its
historical responses in place but uncollected going forward.
