# Guides

Task-oriented walkthroughs of the workflows that actually exist in the Team360 UI. Each
guide is written from the user's side of the screen; where a workflow has a mechanism
worth understanding, it links across to the [Architecture](../architecture/overview.md) or
[API Reference](../api/index.md) sections rather than restating it.

| Guide | For | Covers |
|-------|-----|--------|
| [Signing In](authentication.md) | Everyone | Username/password login, SSO, password reset, what happens after login |
| [Submitting a Health Check](survey.md) | Team members, team leads | The survey flow, scoring, drafts, post-workshop surveys |
| [Team Lead Dashboard](team-lead-dashboard.md) | Team leads | Radar, distribution, individual responses, trends, Excel export |
| [Manager Dashboard](manager-dashboard.md) | Managers, directors, VPs | Rolled-up team health, hierarchy view, comparison, cross-team actions |
| [Action Items](action-items.md) | Team leads, team members | Turning a red dimension into a tracked follow-up |
| [Administration](administration.md) | Admins | Hierarchy levels, dimensions, users, teams, branding, settings |

## Which screen you land on

Team360 routes you by hierarchy level, both at login and in `frontend/middleware.ts`:

| Level | Landing route | Guide |
|-------|--------------|-------|
| Admin (`level-admin`) | `/admin` | [Administration](administration.md) |
| VP, Director, Manager (`level-1`–`level-3`) | `/manager` | [Manager Dashboard](manager-dashboard.md) |
| Team Lead (`level-4`) | `/dashboard` | [Team Lead Dashboard](team-lead-dashboard.md) |
| Team Member (`level-5`) | `/home` | [Submitting a Health Check](survey.md) |

There is no way to reach another role's landing page by typing its URL — the middleware
redirects you back. What each role can *see* once inside is decided by the permission flags
on their hierarchy level; see [Authentication &
Authorization](../architecture/authentication.md) for how that is enforced.

## A typical cycle

1. A team's cadence comes due (monthly, quarterly, half-yearly or yearly).
2. Each member fills in the [survey](survey.md) — 11 dimensions, red/yellow/green.
3. The team lead reviews the results on the [team dashboard](team-lead-dashboard.md) and
   runs a workshop discussion.
4. The lead records the team's consensus as a **post-workshop survey**, and captures the
   agreed follow-ups as [action items](action-items.md).
5. Managers and above see the roll-up on the [manager dashboard](manager-dashboard.md),
   including which teams have open actions.
