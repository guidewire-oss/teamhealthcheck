-- Queries for UserRepository (backend/infrastructure/persistence/postgres/user_repository.go)
-- Moved verbatim from the hand-written SQL that lived inline in that file.
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).

-- name: FindUserByID :one
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: FindUserByUsername :one
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM users
WHERE username = sqlc.arg(username) AND is_active;

-- name: FindUserByEmail :one
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM users
WHERE email = sqlc.arg(email) AND is_active;

-- name: FindAllUsers :many
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM users
ORDER BY username;

-- name: FindUsersByHierarchyLevel :many
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM users
WHERE hierarchy_level_id = sqlc.arg(hierarchy_level_id)
ORDER BY username;

-- name: FindSubordinatesRecursive :many
-- Recursive CTE with cycle protection (visited array) and depth limit (20)
-- to prevent infinite recursion from bad data in reports_to.
WITH RECURSIVE subordinates AS (
	SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
	       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at,
	       1 AS depth, ARRAY[sqlc.arg(supervisor_id)::text, id::text] AS visited
	FROM users
	WHERE reports_to = sqlc.arg(supervisor_id)

	UNION ALL

	SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
	       u.password_hash, u.auth_type, u.created_at, u.updated_at, u.is_active, u.deactivated_at,
	       s.depth + 1, s.visited || u.id::text
	FROM users u
	INNER JOIN subordinates s ON u.reports_to = s.id
	WHERE s.depth < 20
	  AND NOT (u.id::text = ANY(s.visited))
)
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM subordinates
ORDER BY username;

-- name: FindSupervisorChainUpRecursive :many
-- Walks UP the reports_to chain from a user: direct manager first, then
-- their manager, etc. Same cycle protection and depth limit as above.
WITH RECURSIVE supervisors AS (
	SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
	       u.password_hash, u.auth_type, u.created_at, u.updated_at, u.is_active, u.deactivated_at,
	       1 AS depth, ARRAY[sqlc.arg(user_id)::text, u.id::text] AS visited
	FROM users u
	WHERE u.id = (SELECT reports_to FROM users WHERE id = sqlc.arg(user_id))

	UNION ALL

	SELECT u.id, u.username, u.full_name, u.email, u.hierarchy_level_id, u.reports_to,
	       u.password_hash, u.auth_type, u.created_at, u.updated_at, u.is_active, u.deactivated_at,
	       s.depth + 1, s.visited || u.id::text
	FROM users u
	INNER JOIN supervisors s ON u.id = s.reports_to
	WHERE s.depth < 20 AND NOT (u.id::text = ANY(s.visited))
)
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM supervisors
ORDER BY depth;

-- name: InsertUser :exec
INSERT INTO users (id, username, full_name, email, hierarchy_level_id, reports_to, password_hash, auth_type, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- InsertTeamMember already exists in team.sql - reused by UserRepository.Save/Update.

-- name: UpdateUser :execrows
UPDATE users SET
	username = sqlc.arg(username),
	full_name = sqlc.arg(full_name),
	email = sqlc.arg(email),
	hierarchy_level_id = sqlc.arg(hierarchy_level_id),
	reports_to = sqlc.arg(reports_to),
	auth_type = COALESCE(NULLIF(sqlc.arg(auth_type)::text, ''), auth_type),
	updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeactivateTeamMembershipsNotIn :exec
-- Reconcile a user's memberships without destroying history: a team the person
-- was taken out of becomes an inactive membership (kept).
UPDATE team_members
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
WHERE user_id = sqlc.arg(user_id) AND is_active AND team_id <> ALL(sqlc.arg(team_ids)::text[]);

-- name: DeactivateUser :execrows
-- deactivated_at keeps its original value if the person was already inactive.
UPDATE users
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeactivateTeamMembershipsForUser :exec
UPDATE team_members
SET is_active = false, deactivated_at = COALESCE(deactivated_at, CURRENT_TIMESTAMP)
WHERE user_id = sqlc.arg(user_id) AND is_active;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = sqlc.arg(id);

-- name: FindTeamsWhereUserIsLead :many
SELECT id
FROM teams
WHERE team_lead_id = sqlc.arg(user_id)
ORDER BY id;

-- name: FindTeamIDsForUser :many
SELECT team_id
FROM team_members
WHERE user_id = sqlc.arg(user_id) AND is_active
ORDER BY team_id;

-- name: UpdateUserPassword :execrows
UPDATE users
SET password_hash = sqlc.arg(password_hash), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: FindAllUsersLite :many
-- Minimal columns for dropdowns/pickers (no team memberships, no password hash).
SELECT id, username, full_name, hierarchy_level_id
FROM users
ORDER BY username;

-- name: FindTeamIDsForUsers :many
-- Batch-loads active team memberships for many users in one query (avoids N+1).
SELECT user_id, team_id
FROM team_members
WHERE user_id = ANY(sqlc.arg(user_ids)::text[]) AND is_active
ORDER BY user_id, team_id;

-- name: CountUsersPage :one
SELECT COUNT(*)
FROM users
WHERE (sqlc.arg(hierarchy_level)::text = '' OR hierarchy_level_id = sqlc.arg(hierarchy_level)::text)
  AND (sqlc.arg(search)::text = '' OR username ILIKE '%' || sqlc.arg(search)::text || '%'
            OR full_name ILIKE '%' || sqlc.arg(search)::text || '%'
            OR email ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.arg(status)::text = ''
       OR (sqlc.arg(status)::text = 'active' AND is_active)
       OR (sqlc.arg(status)::text = 'inactive' AND NOT is_active));

-- name: FindUsersPage :many
SELECT id, username, full_name, email, hierarchy_level_id, reports_to,
       password_hash, auth_type, created_at, updated_at, is_active, deactivated_at
FROM users
WHERE (sqlc.arg(hierarchy_level)::text = '' OR hierarchy_level_id = sqlc.arg(hierarchy_level)::text)
  AND (sqlc.arg(search)::text = '' OR username ILIKE '%' || sqlc.arg(search)::text || '%'
            OR full_name ILIKE '%' || sqlc.arg(search)::text || '%'
            OR email ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.arg(status)::text = ''
       OR (sqlc.arg(status)::text = 'active' AND is_active)
       OR (sqlc.arg(status)::text = 'inactive' AND NOT is_active))
ORDER BY username
LIMIT sqlc.arg(page_limit)::bigint OFFSET sqlc.arg(page_offset)::bigint;
