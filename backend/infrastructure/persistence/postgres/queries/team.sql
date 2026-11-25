-- Queries for TeamRepository (backend/infrastructure/persistence/postgres/team_repository.go)
-- Moved verbatim from the hand-written SQL that lived inline in that file.
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: FindTeamByID :one
SELECT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, COALESCE(t.health_check_enabled, true)::boolean AS health_check_enabled, u.full_name AS team_lead_name, t.is_active, t.deactivated_at
FROM teams t
LEFT JOIN users u ON t.team_lead_id = u.id
WHERE t.id = sqlc.arg(id);

-- name: FindTeamsByStatus :many
-- status is the empty string (all teams), 'active' or 'inactive'; the filter is applied in the
-- query so rows the caller does not want are never read.
SELECT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, COALESCE(t.health_check_enabled, true)::boolean AS health_check_enabled, u.full_name AS team_lead_name, t.is_active, t.deactivated_at
FROM teams t
LEFT JOIN users u ON t.team_lead_id = u.id
WHERE (sqlc.arg(status)::text = ''
       OR (sqlc.arg(status)::text = 'active' AND t.is_active)
       OR (sqlc.arg(status)::text = 'inactive' AND NOT t.is_active))
ORDER BY t.name;

-- name: FindTeamsByLeadID :many
SELECT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, COALESCE(t.health_check_enabled, true)::boolean AS health_check_enabled, u.full_name AS team_lead_name, t.is_active, t.deactivated_at
FROM teams t
LEFT JOIN users u ON t.team_lead_id = u.id
WHERE t.team_lead_id = sqlc.arg(lead_id)
ORDER BY t.name;

-- name: FindTeamsBySupervisorID :many
SELECT DISTINCT t.id, t.name, t.team_lead_id, t.cadence, t.created_at, t.updated_at, t.distribution_list_email, COALESCE(t.health_check_enabled, true)::boolean AS health_check_enabled, u.full_name AS team_lead_name, t.is_active, t.deactivated_at
FROM teams t
INNER JOIN team_supervisors ts ON t.id = ts.team_id
LEFT JOIN users u ON t.team_lead_id = u.id
WHERE ts.user_id = sqlc.arg(supervisor_id)
ORDER BY t.name;

-- name: FindTeamMemberUserIDs :many
SELECT user_id
FROM team_members
WHERE team_id = sqlc.arg(team_id) AND is_active
ORDER BY user_id;

-- name: FindTeamMembersDetailed :many
SELECT u.id, u.username, u.full_name, u.email
FROM users u
INNER JOIN team_members tm ON u.id = tm.user_id
WHERE tm.team_id = sqlc.arg(team_id) AND tm.is_active
ORDER BY u.username;

-- name: CountTeamMembers :one
SELECT COUNT(*)
FROM team_members
WHERE team_id = sqlc.arg(team_id) AND is_active;

-- name: FindTeamSupervisorChain :many
SELECT user_id, hierarchy_level_id
FROM team_supervisors
WHERE team_id = sqlc.arg(team_id)
ORDER BY position;

-- name: InsertTeam :exec
INSERT INTO teams (id, name, team_lead_id, cadence, distribution_list_email, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(team_lead_id), sqlc.arg(cadence), sqlc.arg(distribution_list_email), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: UpdateTeam :execrows
UPDATE teams SET
    name = sqlc.arg(name),
    team_lead_id = sqlc.arg(team_lead_id),
    cadence = sqlc.arg(cadence),
    distribution_list_email = sqlc.arg(distribution_list_email),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeactivateTeam :execrows
-- deactivated_at keeps its original value if the team was already inactive.
UPDATE teams
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeactivateTeamMembersByTeamID :exec
UPDATE team_members
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
WHERE team_id = sqlc.arg(team_id) AND is_active;

-- name: DeactivateTeamMembersNotIn :exec
-- Reconcile a team's members without destroying history: anyone not in
-- user_ids becomes an inactive membership (kept).
UPDATE team_members
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
WHERE team_id = sqlc.arg(team_id) AND is_active AND user_id <> ALL(sqlc.arg(user_ids)::text[]);

-- name: AddTeamMember :exec
-- Adds a member, or reactivates an inactive membership (keeping its history row).
INSERT INTO team_members (team_id, user_id)
VALUES (sqlc.arg(team_id), sqlc.arg(user_id))
ON CONFLICT (team_id, user_id) DO UPDATE
    SET is_active = true, deactivated_at = NULL
    WHERE team_members.is_active = false;

-- name: InsertTeamMember :exec
INSERT INTO team_members (team_id, user_id)
VALUES (sqlc.arg(team_id), sqlc.arg(user_id));

-- name: RemoveTeamMember :execrows
-- Ends the membership but keeps the row as inactive history.
UPDATE team_members
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
WHERE team_id = sqlc.arg(team_id) AND user_id = sqlc.arg(user_id) AND is_active;

-- name: DeleteTeamSupervisorsByTeamID :exec
DELETE FROM team_supervisors WHERE team_id = sqlc.arg(team_id);

-- name: InsertTeamSupervisor :exec
INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
VALUES (sqlc.arg(team_id), sqlc.arg(user_id), sqlc.arg(hierarchy_level_id), sqlc.arg(position));

-- name: FindSurveyCompletionTeams :many
-- See team_repository.go's FindSurveyCompletionTeams doc comment for the
-- full rationale (closest-supervisor selection, no member count here).
SELECT
    t.id,
    t.name,
    COALESCE(t.health_check_enabled, true)::boolean AS health_check_enabled,
    COALESCE(sup.user_id, '') AS supervisor_id,
    COALESCE(t.team_lead_id, '') AS team_lead_id,
    COALESCE(tl.full_name, '') AS team_lead_name,
    COALESCE(tl.email, '') AS team_lead_email
FROM teams t
LEFT JOIN LATERAL (
    SELECT ts.user_id
    FROM team_supervisors ts
    WHERE ts.team_id = t.id
    ORDER BY ts.position ASC
    LIMIT 1
) sup ON true
LEFT JOIN users tl ON tl.id = t.team_lead_id
WHERE t.is_active
ORDER BY t.name;

-- name: FindEligibleMemberIDs :many
-- See team_repository.go's FindEligibleMemberIDs doc comment for the full
-- rationale (why team_members is authoritative, why position 4/5 only).
SELECT DISTINCT tm.team_id, tm.user_id
FROM team_members tm
JOIN teams t ON t.id = tm.team_id
JOIN users u ON u.id = tm.user_id
JOIN hierarchy_levels hl ON hl.id = u.hierarchy_level_id
WHERE hl.position IN (4, 5) AND tm.is_active AND u.is_active AND t.is_active;
