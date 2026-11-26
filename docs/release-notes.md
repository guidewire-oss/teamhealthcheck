# Release Notes

Release notes for Team360, newest first. Each version matches a git tag. Versions v0.1.0 to v0.3.0 were deployed to the dev environment.

Each entry uses these headings:

- **Features**: what changed for users and admins.
- **Improvements**: smaller changes to existing behavior.
- **Fixes**: bugs that were corrected.

---

## v0.3.0

Released 2026-11-17

### Features

- **Sync History tab for admins.** Admins can now see every organization sync run in one place, with the number of users and teams created, updated, and unchanged. When a run did not record its unchanged counts, they show as unknown instead of a misleading zero. Secrets never appear in the history.
- **Team opt-in for health checks.** A team that has opted out of health checks can no longer receive surveys. Members see a message as soon as they open the survey, instead of failing partway through.
- **Action item updates.** Action items show counts by status and have a new **On hold** status. The workflow and visibility were reworked (PIT-134), and counts load in batches so the page stays fast.
- **PodIQ roles in organization sync.** The `podiqRole` value from the provider snapshot now maps to hierarchy levels. Pod-lead promotions are reported in the sync result.

### Improvements

- The survey completion dashboard excludes inactive teams, shows half-year periods only, and lists the pods that are fully complete.
- Legacy half-year period labels stay selectable.
- A pod shows a completion badge only when its backend status is complete. Partial owners no longer get one.

### Fixes

- A team is now marked **Fully complete** once its post-workshop survey is submitted.
- A failed enrollment lookup now returns `500`. A null `healthCheckEnabled` value is rejected.
- Users with no level but a `podiqRole` are rejected, and duplicate role levels are removed.
- A stored pod lead stays at level-4 when the snapshot omits `teamLeadId`. Valid pod leads sent as level-5 are stored at level-4.
- Pagination now holds when a page fails to load, and the page number is clamped so the offset cannot overflow.
- Action item responses that arrive after the team was changed are ignored.

---

## v0.2.0

Released 2026-11-04

This release also includes v0.1.1 (2026-10-22), which added the Swagger documentation and the user-id column fix listed below.

### Features

- **Scheduled organization sync.** Syncs can run on a schedule. Existing records are updated in place (upsert) instead of being recreated.
- **Survey reminder emails.** The **Remind** buttons on the survey completion dashboard now send emails. Each email includes a per-survey progress table and uses the subject `Your Pod Health Check Survey Reminder: <period>`. In test setups, redirected emails are sent unchanged and the intended recipient is logged.
- **Half-year assessment periods (H1/H2).** Surveys now run per half-year. Admins can pick the assessment period manually, and the UI lets users choose the survey type. Cadence configuration was removed.
- **Swagger API documentation.** The API is documented code-first with default examples. It also lists the `401` and `403` responses for team session and info routes. (v0.1.1)

### Improvements

- Organization sync deactivates users softly instead of deleting them, and the result panel shows what happened to each record.
- The member dashboard's **Take Survey** button goes straight to the survey, without the extra confirmation popup.
- A survey is not enabled until the next survey period opens.
- Admin settings were cleaned up and company branding is the same on every page.

### Fixes

- Half-year survey eligibility and duplicate-submission checks were corrected.
- Organization sync now handles users stored under a different id than the provider's. User id reference columns were widened so long provider ids can be re-keyed. (v0.1.1)
- Sync fails closed when the hold check errors, and returns generic error messages instead of internal details.
- A hung sync request in the UI is released when its lease expires.
- Snapshots must now carry explicit empty memberships, and snapshot field lengths are capped.
- Health dimensions keep their presentation order.

---

## v0.1.0

Released 2026-10-08

### Features

- **Provider-neutral organization sync.** Team360 can pull users, teams, and memberships from an external provider snapshot. See the [Organization Snapshot Contract](./organization-snapshot-contract.md). Provider calls authenticate with an API token.
- **Survey completion dashboard.** Managers can see how many members in each pod have completed the survey, and send reminders to pods that are still pending.
- **Sync safety controls.** Admins can set a deletion threshold for organization sync. Before a sync runs, a blast-radius check shows how many records it would remove.
- **Documentation site.** The docs are published with GitHub Pages and the `docs/` structure was cleaned up.

### Improvements

- User lists are paginated, so large organizations load faster.
- The survey dashboard loads faster.
- Sync status is kept when you navigate away or switch tabs.
- Removed an insecure, obsolete dev startup script.
- Open-source housekeeping: Apache-2.0 `LICENSE` and `NOTICE`, `CONTRIBUTING.md`, GitHub issue templates, and README status badges.

### Fixes

- The mass-deletion guard now counts the same records the atomic delete removes. A race where a concurrent supervisor-chain edit could survive the fix and still be cascade-deleted was closed.
- Sync no longer wipes the supervisor chain of a protected team.
- Provider sync hardening: protected-user bypass, API token leak, NaN/Inf guard, nil snapshot handling, and correct `502`/`500` responses.
- The pending-pods reminder is visible again.
- Added the missing down migration for `000006_add_password_hash`.
- CI fixes: upgraded `playwright-go` after the old CDN went offline, corrected the Trivy scan image names, and removed a duplicate `postcss.config.mjs`.
