# Organization Snapshot Contract v1.0

The organization snapshot is a complete, provider-neutral representation of externally managed users, teams, memberships, and reporting relationships.

```json
{
  "contractVersion": "1.0",
  "generatedAt": "2026-08-29T12:00:00Z",
  "users": [
    {
      "id": "user-1",
      "username": "alice",
      "displayName": "Alice",
      "email": "alice@example.com",
      "reportsToId": null,
      "hierarchyLevelId": "level-3"
    }
  ],
  "teams": [
    {
      "id": "team-1",
      "name": "Platform",
      "teamLeadId": "user-1",
      "healthCheckEnabled": true
    }
  ],
  "memberships": [
    { "userId": "user-1", "teamId": "team-1" }
  ]
}
```

## Transport

A conforming provider must expose the snapshot at a single, fixed endpoint and
credential scheme -- this is part of the contract, not a per-deployment
setting:

- **Endpoint**: `GET {DATA_PROVIDER_BASE_URL}/org-snapshot`
- **Authentication**: the credential configured in `DATA_PROVIDER_API_TOKEN`
  must be sent in an `x-api-key` request header. No other auth header or auth
  scheme is sent.
- **Response**: `200 OK` with a JSON body matching the shape below. Any other
  status code is treated as a failed fetch.

A deployment points Team Health Check at any conforming provider by setting
only `DATA_PROVIDER_BASE_URL` and `DATA_PROVIDER_API_TOKEN` -- no other
configuration is read. The path and header are fixed so that "provider-neutral"
means "any provider implementing this contract," not "any arbitrary API
shape"; this keeps the integration simple and auditable rather than an
open-ended adapter surface.

## Identity

- Provider user and team IDs are canonical IDs for externally managed records. They must be non-empty, stable, unique within their entity type, and never reused.
- One external provider is active at a time. Provider IDs share Team Health Check's record-ID namespace; switching providers or linking a provider record to an existing local record requires an explicit migration outside v1.0.

## Required and Optional Fields

- `username`, `displayName`, `email`, `hierarchyLevelId`, team `name`, `contractVersion`, and `generatedAt` are required.
- Omit `reportsToId` or use `null` for a root user; otherwise it must reference a user in the snapshot.
- `hierarchyLevelId` must identify a hierarchy level configured in the target Team Health Check deployment. Existence is checked during synchronization because the snapshot does not contain hierarchy-level definitions.
- `teamLeadId` is optional. When present, it must reference a user who is also a member of that team. When omitted or `null`, synchronization preserves the existing Team Health Check value; v1.0 does not define an explicit clear operation.
- `healthCheckEnabled` is optional. Omission preserves the existing Team Health Check value; explicit `true` or `false` replaces it.

## Complete Snapshot Semantics

- `users`, `teams`, and `memberships` should be JSON arrays rather than `null`. `users` and `teams` must each contain at least one record; `memberships` may be empty.
- `memberships` is the complete source of truth for team membership **of every team the snapshot lists**. For each such team, a user-team pair that is not in the snapshot is removed during reconciliation. Membership rows of teams the snapshot does not list are removed together with those teams (see below), and rows held by the protected and skipped users described below are left alone.
- The snapshot is treated as the complete, authoritative list of users and teams. After a successfully validated snapshot is applied, **every user and team that is absent from it is permanently deleted** (not deactivated), including records that were created locally in Team Health Check, unless one of the protections below applies.
- Team Health Check does **not** currently track provider provenance: there is no column that marks a user or team as externally managed rather than local. Reconciliation therefore cannot tell a provider-owned record from a local one, and a provider that omits a record deletes it. Deployments that mix locally created users or teams with a provider must either have the provider list them or accept their deletion. Provenance-aware deactivation (preserving local records and deactivating rather than deleting provider records) is **not implemented**, so it is not part of v1.0 behavior.

### What a sync deletes, and what it never deletes

Deletion is hard and happens in the same database transaction as the rest of the sync, so a failure at any point leaves everything unchanged.

- **Cascades.** Deleting a team also removes its supervisor-chain rows, its team memberships, and its action items. Deleting a user also removes their team memberships, password-reset tokens, and the action items they created, and clears (sets to empty) any `reportsToId` or team lead that pointed at them. Health-check sessions and responses have no foreign key to users or teams and are not removed, so sessions can end up referencing a user who no longer exists.
- **Never deleted: protected records.** The permanent admin, any user at the admin hierarchy level, the fixed demo, test, and E2E fixture users and teams, and any user named in a protected team's supervisor chain. Their team memberships are preserved as well.
- **Never deleted: users the sync could not validate.** A user skipped because their `hierarchyLevelId` is missing or unknown to this deployment is not evidence that the provider removed them, so they are neither deleted nor stripped of existing memberships or manager links in that sync.
- **Mass-deletion guard.** Before anything is written, a sync that would delete more than the configured share of existing, non-protected users or teams is held for administrator review and changes nothing until the administrator confirms the exact counts they reviewed.

## Validation and Synchronization Safety

- `Validate` performs structural checks after decoding. JSON decoders should reject malformed payloads; unknown fields are ignored by Go's default `encoding/json` behavior.
- `generatedAt` records when the provider generated the snapshot. Stale or out-of-order snapshot checks belong to synchronization because they require the last successful sync state.
- Synchronization applies a configured blast-radius guard before deleting a large percentage of existing users or teams. Empty user or team collections are rejected by contract validation, while the guard protects against dangerously incomplete non-empty snapshots.

## Versioning

- Incompatible changes require a new contract version. Consumers of v1.0 reject unsupported versions.
- Additive optional fields may be introduced within v1.0 only when existing consumers can safely ignore them.
