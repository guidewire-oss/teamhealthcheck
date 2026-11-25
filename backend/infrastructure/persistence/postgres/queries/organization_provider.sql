-- Queries for OrganizationProviderRepository
-- (backend/infrastructure/persistence/postgres/organization_provider_repository.go)
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: GetHierarchyLevelIDs :many
SELECT id FROM hierarchy_levels;

-- name: LockLastAppliedSnapshotTime :one
-- Locks the singleton settings row for the rest of the transaction, so two
-- applies serialize and an older snapshot cannot overwrite a newer one.
SELECT org_sync_last_source_generated_at FROM app_settings WHERE id = 1 FOR UPDATE;

-- name: RecordAppliedSnapshotTime :exec
UPDATE app_settings SET org_sync_last_source_generated_at = sqlc.arg(generated_at) WHERE id = 1;

-- name: FindActiveUserIDsAndLevels :many
-- Already-inactive rows are neither deactivation candidates nor part of the
-- mass-deletion guard's denominator.
SELECT id, hierarchy_level_id FROM users WHERE is_active;

-- name: FindActiveTeamIDs :many
SELECT id FROM teams WHERE is_active;

-- name: CountAllTeamMemberships :one
SELECT COUNT(*) FROM team_members;

-- name: CountCascadedTeamMemberships :one
SELECT COUNT(*) FROM team_members
WHERE team_id = ANY(sqlc.arg(missing_team_ids)::text[])
   OR (user_id = ANY(sqlc.arg(missing_user_ids)::text[]) AND team_id <> ALL(sqlc.arg(reconciled_team_ids)::text[]));

-- name: CountTeamsDisablingHealthCheck :one
SELECT COUNT(*) FROM teams WHERE id = ANY(sqlc.arg(team_ids)::text[]) AND health_check_enabled = true;

-- name: CountTeamsEnablingHealthCheck :one
SELECT COUNT(*) FROM teams WHERE id = ANY(sqlc.arg(team_ids)::text[]) AND health_check_enabled = false;

-- name: FindExistingUserIDs :many
SELECT id FROM users WHERE id = ANY(sqlc.arg(ids)::text[]);

-- name: FindSSOUserIDsByEmailOrUsername :many
SELECT id FROM users
WHERE auth_type = 'sso' AND (lower(email) = lower(sqlc.arg(email)::text) OR username = sqlc.arg(username)::text);

-- name: FreeUserCredentialsForRekey :exec
-- The placeholders must satisfy chk_users_username_format and
-- chk_users_email_format (migration 000013); ids can be too long or contain
-- characters those checks reject, so they are derived from a hash.
UPDATE users SET username = 'rk_' || left(md5(id), 20), email = 'rk_' || md5(id) || '@rekey.invalid'
WHERE id = sqlc.arg(id);

-- name: CopyUserUnderNewID :exec
-- Keeps the stored hierarchy level, reports_to, password_hash, auth_type and created_at.
INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash, auth_type, created_at, updated_at)
SELECT sqlc.arg(new_id)::text, sqlc.arg(username)::text, sqlc.arg(email)::text, sqlc.arg(full_name)::text,
       src.hierarchy_level_id, src.reports_to, src.password_hash, src.auth_type, src.created_at, CURRENT_TIMESTAMP
FROM users src WHERE src.id = sqlc.arg(old_id);

-- name: RepointTeamMembersUser :exec
UPDATE team_members SET user_id = sqlc.arg(new_id) WHERE user_id = sqlc.arg(old_id);

-- name: RepointTeamSupervisorsUser :exec
UPDATE team_supervisors SET user_id = sqlc.arg(new_id) WHERE user_id = sqlc.arg(old_id);

-- name: RepointActionItemsCreatedBy :exec
UPDATE action_items SET created_by = sqlc.arg(new_id) WHERE created_by = sqlc.arg(old_id);

-- name: RepointActionItemsAssignedTo :exec
UPDATE action_items SET assigned_to = sqlc.arg(new_id) WHERE assigned_to = sqlc.arg(old_id);

-- name: RepointPasswordResetTokensUser :exec
UPDATE password_reset_tokens SET user_id = sqlc.arg(new_id) WHERE user_id = sqlc.arg(old_id);

-- name: RepointTeamsLead :exec
UPDATE teams SET team_lead_id = sqlc.arg(new_id) WHERE team_lead_id = sqlc.arg(old_id);

-- name: RepointUsersReportsTo :exec
UPDATE users SET reports_to = sqlc.arg(new_id) WHERE reports_to = sqlc.arg(old_id);

-- name: RepointHealthCheckSessionsUser :exec
UPDATE health_check_sessions SET user_id = sqlc.arg(new_id) WHERE user_id = sqlc.arg(old_id);

-- name: UpsertSnapshotUser :one
-- Writes an existing row only when a normalized business field differs, so
-- re-syncing identical data touches nothing. RETURNING yields a row only when
-- the statement inserted or updated; xmax = 0 tells an insert from an update.
-- is_active, password_hash and auth_type are never overwritten on conflict.
-- pod_lead_level / pod_source_level are empty strings when the pod-lead mapping
-- is off, which match no real level.
INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash, auth_type, updated_at)
VALUES (sqlc.arg(id), btrim(sqlc.arg(username)::text), btrim(sqlc.arg(email)::text), btrim(sqlc.arg(full_name)::text), sqlc.arg(level), NULL, '', 'sso', CURRENT_TIMESTAMP)
ON CONFLICT (id) DO UPDATE SET
	username = EXCLUDED.username,
	email = EXCLUDED.email,
	full_name = EXCLUDED.full_name,
	hierarchy_level_id = CASE
		WHEN users.hierarchy_level_id = sqlc.arg(pod_lead_level)::text AND EXCLUDED.hierarchy_level_id = sqlc.arg(pod_source_level)::text THEN users.hierarchy_level_id
		ELSE EXCLUDED.hierarchy_level_id
	END,
	updated_at = CURRENT_TIMESTAMP
WHERE btrim(users.username) IS DISTINCT FROM EXCLUDED.username
   OR lower(btrim(users.email)) IS DISTINCT FROM lower(EXCLUDED.email)
   OR btrim(users.full_name) IS DISTINCT FROM EXCLUDED.full_name
   OR (users.hierarchy_level_id IS DISTINCT FROM EXCLUDED.hierarchy_level_id
       AND NOT (users.hierarchy_level_id = sqlc.arg(pod_lead_level)::text AND EXCLUDED.hierarchy_level_id = sqlc.arg(pod_source_level)::text))
RETURNING (xmax = 0)::boolean AS inserted;

-- name: PromotePodLeads :many
-- Users in source_ids who lead an active team they are an active member of.
UPDATE users SET hierarchy_level_id = sqlc.arg(lead_level), updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(source_ids)::text[]) AND hierarchy_level_id = sqlc.arg(source_level)
  AND id IN (
      SELECT t.team_lead_id
      FROM teams t
      JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = t.team_lead_id AND tm.is_active
      WHERE t.is_active AND t.team_lead_id IS NOT NULL)
RETURNING id;

-- name: ReturnFormerPodLeads :many
-- Users in source_ids who no longer lead an active team go back to the provider's level.
UPDATE users SET hierarchy_level_id = sqlc.arg(source_level), updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(source_ids)::text[]) AND hierarchy_level_id = sqlc.arg(lead_level)
  AND id NOT IN (
      SELECT t.team_lead_id
      FROM teams t
      JOIN team_members tm ON tm.team_id = t.id AND tm.user_id = t.team_lead_id AND tm.is_active
      WHERE t.is_active AND t.team_lead_id IS NOT NULL)
RETURNING id;

-- name: SetUserLevelForIDs :many
UPDATE users SET hierarchy_level_id = sqlc.arg(level), updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(ids)::text[]) AND hierarchy_level_id IS DISTINCT FROM sqlc.arg(level)
RETURNING id;

-- name: SetUserReportsToIfChanged :execrows
-- Only a real manager change writes (and bumps updated_at). A NULL reports_to
-- clears the manager.
UPDATE users SET reports_to = sqlc.narg(reports_to)::varchar, updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id) AND reports_to IS DISTINCT FROM sqlc.narg(reports_to)::varchar;

-- name: UpsertSnapshotTeam :one
-- team_lead_id and health_check_enabled use the bound parameters rather than
-- EXCLUDED so a NULL (field omitted by the provider) preserves the existing
-- value. An existing team is only written when a business field differs.
-- is_active is decided afterwards by ReactivateTeams / DeactivateTeamsByIDs.
INSERT INTO teams (id, name, team_lead_id, health_check_enabled, updated_at)
VALUES (sqlc.arg(id), btrim(sqlc.arg(name)::text), sqlc.narg(team_lead_id)::varchar, COALESCE(sqlc.narg(health_check_enabled)::boolean, true), CURRENT_TIMESTAMP)
ON CONFLICT (id) DO UPDATE SET
	name = EXCLUDED.name,
	team_lead_id = COALESCE(sqlc.narg(team_lead_id)::varchar, teams.team_lead_id),
	health_check_enabled = COALESCE(sqlc.narg(health_check_enabled)::boolean, teams.health_check_enabled),
	updated_at = CURRENT_TIMESTAMP
WHERE btrim(teams.name) IS DISTINCT FROM EXCLUDED.name
   OR teams.team_lead_id IS DISTINCT FROM COALESCE(sqlc.narg(team_lead_id)::varchar, teams.team_lead_id)
   OR teams.health_check_enabled IS DISTINCT FROM COALESCE(sqlc.narg(health_check_enabled)::boolean, teams.health_check_enabled)
RETURNING (xmax = 0)::boolean AS inserted;

-- name: DeactivateTeamMembersNotKept :execrows
-- Complete snapshot: an active row for a reported team that the snapshot no
-- longer lists becomes inactive (kept as history).
UPDATE team_members SET is_active = false, deactivated_at = CURRENT_TIMESTAMP
WHERE team_id = sqlc.arg(team_id) AND is_active AND user_id <> ALL(sqlc.arg(keep_user_ids)::text[]);

-- name: ActivateSnapshotTeamMember :execrows
-- Inserts a new row or flips an inactive one back on. The DO UPDATE WHERE leaves
-- an already-active row alone, so the affected count is 1 only for a real change.
INSERT INTO team_members (team_id, user_id, is_active) VALUES (sqlc.arg(team_id), sqlc.arg(user_id), true)
ON CONFLICT (team_id, user_id) DO UPDATE
	SET is_active = true, deactivated_at = NULL
	WHERE team_members.is_active = false;

-- name: DeactivateTeamsByIDs :execrows
UPDATE teams SET is_active = false, deactivated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(ids)::text[]) AND is_active;

-- name: DeactivateMembershipsOfTeams :execrows
UPDATE team_members SET is_active = false, deactivated_at = CURRENT_TIMESTAMP
WHERE team_id = ANY(sqlc.arg(team_ids)::text[]) AND is_active;

-- name: DeactivateMissingUsers :execrows
-- hierarchy_level_id and the protected-supervisor exemption are re-checked in
-- the statement itself so a concurrent change cannot slip past the earlier read.
UPDATE users SET is_active = false, deactivated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(ids)::text[]) AND is_active
  AND hierarchy_level_id IS DISTINCT FROM sqlc.arg(protected_level)::text
  AND NOT EXISTS (
      SELECT 1 FROM team_supervisors ts
      WHERE ts.user_id = users.id AND ts.team_id = ANY(sqlc.arg(protected_team_ids)::text[])
  );

-- name: DeactivateMembershipsOfInactiveUsers :execrows
UPDATE team_members SET is_active = false, deactivated_at = CURRENT_TIMESTAMP
WHERE user_id = ANY(sqlc.arg(user_ids)::text[]) AND is_active
  AND user_id IN (SELECT id FROM users WHERE is_active = false);

-- name: DeactivateUsersWithoutActiveMembership :many
-- A person with membership rows but none active has no active organization left.
UPDATE users SET is_active = false, deactivated_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(ids)::text[]) AND is_active
  AND EXISTS (SELECT 1 FROM team_members tm WHERE tm.user_id = users.id)
  AND NOT EXISTS (SELECT 1 FROM team_members tm WHERE tm.user_id = users.id AND tm.is_active)
RETURNING id;

-- name: ReactivateUsers :many
-- An inactive person who now has an active membership, or never had any
-- membership (a VP, say), is reactivated.
UPDATE users SET is_active = true, deactivated_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(ids)::text[]) AND NOT is_active
  AND (NOT EXISTS (SELECT 1 FROM team_members tm WHERE tm.user_id = users.id)
       OR EXISTS (SELECT 1 FROM team_members tm WHERE tm.user_id = users.id AND tm.is_active))
RETURNING id;

-- name: ReactivateTeams :many
UPDATE teams SET is_active = true, deactivated_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = ANY(sqlc.arg(ids)::text[]) AND NOT is_active
RETURNING id;

-- name: FindDistinctTeamSupervisors :many
SELECT DISTINCT team_id, user_id FROM team_supervisors;

-- name: MarkOrgSyncRunSucceeded :execrows
-- Only a RUNNING run can succeed (the stale-worker guard): if a recovery sweep
-- already failed it, no row matches and the caller rolls the whole apply back.
UPDATE org_sync_runs SET
	status = 'SUCCESS', stage = sqlc.arg(stage), completed_at = CURRENT_TIMESTAMP, heartbeat_at = CURRENT_TIMESTAMP,
	source_generated_at = sqlc.arg(source_generated_at),
	inserted_users = sqlc.arg(inserted_users), updated_users = sqlc.arg(updated_users),
	inserted_teams = sqlc.arg(inserted_teams), updated_teams = sqlc.arg(updated_teams),
	membership_changes = sqlc.arg(membership_changes), deactivated_records = sqlc.arg(deactivated_records),
	unchanged_users = sqlc.arg(unchanged_users), unchanged_teams = sqlc.arg(unchanged_teams), unchanged_recorded = TRUE,
	health_checks_disabled = sqlc.arg(health_checks_disabled), health_checks_enabled = sqlc.arg(health_checks_enabled),
	mass_deletion_overridden = sqlc.arg(mass_deletion_overridden),
	snapshot_type = sqlc.arg(snapshot_type)
WHERE id = sqlc.arg(id) AND status = ANY(sqlc.arg(allowed_statuses)::text[]);
