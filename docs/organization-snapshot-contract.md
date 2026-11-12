# Organization Snapshot Contract v1.0 and v1.1

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

Version 1.1 adds one optional user field, `podiqRole`. See [PodIQ Role (v1.1)](#podiq-role-v11). Every other rule in this document applies to both versions.

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
- `snapshotType` is optional and is either `"COMPLETE"` or `"PARTIAL"`. Omission means `"COMPLETE"`. Any other value is rejected by validation. See [Partial Snapshots](#partial-snapshots).

## PodIQ Role (v1.1)

A v1.1 snapshot may include an optional `podiqRole` on each user:

```json
{ "id": "user-1", "username": "alice", "displayName": "Alice", "email": "alice@example.com", "hierarchyLevelId": "level-5", "podiqRole": "L1" }
```

- **Versions.** `contractVersion` is `"1.0"` or `"1.1"`; both are accepted. `podiqRole` is only valid when `contractVersion` is `"1.1"`.
- **Valid values.** Exactly `"L2"`, `"L1"` and `"Developer"` (case-sensitive). The provider has already resolved multi-role precedence; Team Health Check maps the value it receives and never calculates a role.
- **Mapping.** The role is the final level decision of the sync transaction, applied after users, teams, memberships and active state are processed: `L2` is stored as `level-3`, `L1` as `level-4`, `Developer` as `level-5`. It overrides the user's `hierarchyLevelId`, any grade-derived level, `teamLeadId`, and the pod-lead promotion. A role-mapped user whose `hierarchyLevelId` is unknown to the deployment is still imported, at the role's level. Protected users (see below) are never changed. A second identical sync writes no level update.
- **Missing role (v1.0 fallback).** A user without `podiqRole`, whether the snapshot is v1.0 or v1.1, keeps the legacy behavior: the provider's `hierarchyLevelId` is stored, with the existing pod-lead promotion of `level-5` team leads to `level-4`.
- **Invalid role.** Any other value, including `""`, a different case, or an unknown role, fails validation. So does `podiqRole` in a v1.0 snapshot. The whole snapshot is rejected and nothing is written; no level is assigned. A user that has a `podiqRole` but a missing `hierarchyLevelId` also rejects the whole snapshot, because `hierarchyLevelId` is required and the user is not skipped. A JSON `null` is treated as absent.
- **Missing level.** If a role maps to a level this deployment does not have, the sync is rejected before anything is written.

## Complete Snapshot Semantics

- `users`, `teams`, and `memberships` should be JSON arrays rather than `null`. `users` and `teams` must each contain at least one record; `memberships` may be empty.
- `memberships` is the complete source of truth for team membership **of every team the snapshot lists**. For each such team, a user-team pair that is not in the snapshot is removed during reconciliation. Membership rows of teams the snapshot does not list are removed together with those teams; rows held by the protected and skipped users described below are preserved only on teams that remain.
- The snapshot is treated as the complete, authoritative list of users and teams. After a successfully validated snapshot is applied, **every user and team that is absent from it is permanently deleted** (not deactivated), including records that were created locally in Team Health Check, unless one of the protections below applies.
- Team Health Check does **not** currently track provider provenance: there is no column that marks a user or team as externally managed rather than local. Reconciliation therefore cannot tell a provider-owned record from a local one, and a provider that omits a record deletes it. Deployments that mix locally created users or teams with a provider must either have the provider list them or accept their deletion. Provenance-aware deactivation (preserving local records and deactivating rather than deleting provider records) is **not implemented**, so it is not part of v1.0 behavior.

### What a sync deletes, and what it never deletes

Deletion is hard. The persistence writes (upserts, membership reconciliation, and deletions) happen in one database transaction, so a failure before commit leaves them unchanged. Rebuilding the derived supervisor chains runs after that commit; if it fails, the user and team changes stay committed and the sync reports `completed_with_warnings`.

- **Cascades.** Deleting a team also removes its supervisor-chain rows, its team memberships, and its action items. Deleting a user also removes their team memberships, password-reset tokens, and the action items they created, and clears (sets to empty) any `reportsToId` or team lead that pointed at them. Health-check sessions and responses have no foreign key to users or teams and are not removed, so sessions can end up referencing a user who no longer exists.
- **Never deleted: protected records.** The permanent admin, any user at the admin hierarchy level, the fixed demo, test, and E2E fixture users and teams, and any user named in a protected team's supervisor chain. Their team memberships are preserved on teams that remain; a team that is deleted takes all of its membership rows with it.
- **Never deleted: users the sync could not validate.** A user skipped because their `hierarchyLevelId` is missing, or unknown to this deployment, and who carries no `podiqRole`, is not evidence that the provider removed them, so they are neither deleted nor stripped of existing manager links in that sync, and their existing memberships are kept on teams that remain (a deleted team still takes its membership rows with it).
- **Mass-deletion guard.** Before anything is written, a sync that would delete more than the configured share of existing, non-protected users or teams is held for administrator review and changes nothing until the administrator confirms the exact counts they reviewed.

## Partial Snapshots

A provider that can only send part of its organization sets `"snapshotType": "PARTIAL"`:

```json
{
  "contractVersion": "1.0",
  "generatedAt": "2026-08-29T12:00:00Z",
  "snapshotType": "PARTIAL",
  "users": [
    {
      "id": "user-1",
      "username": "alice",
      "displayName": "Alice",
      "email": "alice@example.com",
      "hierarchyLevelId": "level-3"
    }
  ],
  "teams": [
    { "id": "team-1", "name": "Platform", "teamLeadId": "user-1" }
  ],
  "memberships": [
    { "userId": "user-1", "teamId": "team-1" }
  ]
}
```

- `"COMPLETE"`, or omitting `snapshotType`, states that the snapshot is complete for its provider. Synchronization may mark externally managed records that are absent from the payload as inactive.
- `"PARTIAL"` states that the payload is only a subset. Synchronization adds and updates the records it contains, and never deactivates a record or ends a membership because it is absent. Memberships omitted from a partial snapshot are left unchanged.
- Non-empty is never treated as evidence of completeness. Only an omitted or explicit `"COMPLETE"` value allows deactivation.
- Empty `users` or `teams` are still rejected for a `PARTIAL` snapshot, as for any other snapshot.

## Validation and Synchronization Safety

- `Validate` performs structural checks after decoding. JSON decoders should reject malformed payloads; unknown fields are ignored by Go's default `encoding/json` behavior.
- `generatedAt` records when the provider generated the snapshot. Stale or out-of-order snapshot checks belong to synchronization because they require the last successful sync state.
- Synchronization applies a configured blast-radius guard before deleting a large percentage of existing users or teams. Empty user or team collections are rejected by contract validation, while the guard protects against dangerously incomplete non-empty snapshots.

## Versioning

- Incompatible changes require a new contract version. Consumers reject unsupported versions; Team Health Check accepts `"1.0"` and `"1.1"`.
- Additive optional fields may be introduced within v1.0 only when existing consumers can safely ignore them. `podiqRole` is not one of those: it is v1.1-only and rejected in v1.0 payloads.
- `snapshotType` is the one exception, and it is safe only because of how v1.0 is deployed: the sole consumer is the Team Health Check synchronization service, which is released together with this document. A provider must not send `"PARTIAL"` to a deployment that predates the field, because such a consumer would ignore it and treat the payload as complete.
