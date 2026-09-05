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
- `memberships` is the complete source of truth for externally managed team membership. Omitting a user-team pair removes that external membership during reconciliation.
- The snapshot is complete for its configured provider. A later reconciliation service deactivates externally managed records absent from a successfully validated snapshot, but does not alter local records or Team Health Check-owned data.
- Synchronization must track provider provenance so externally managed records can be distinguished from local records.

## Validation and Synchronization Safety

- `Validate` performs structural checks after decoding. JSON decoders should reject malformed payloads; unknown fields are ignored by Go's default `encoding/json` behavior.
- `generatedAt` records when the provider generated the snapshot. Stale or out-of-order snapshot checks belong to synchronization because they require the last successful sync state.
- Synchronization must apply a configured blast-radius guard before deactivating a large percentage of existing external records. Empty user or team collections are rejected by contract validation, while the guard protects against dangerously incomplete non-empty snapshots.

## Versioning

- Incompatible changes require a new contract version. Consumers of v1.0 reject unsupported versions.
- Additive optional fields may be introduced within v1.0 only when existing consumers can safely ignore them.
