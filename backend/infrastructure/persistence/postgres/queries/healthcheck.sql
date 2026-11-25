-- Queries for HealthCheckRepository (backend/infrastructure/persistence/postgres/health_check_repository.go)
-- Moved verbatim from the hand-written SQL that lived inline in that file.
-- Do not hand-edit sqlcgen/*.go - regenerate with `sqlc generate` (see Makefile).
--
-- FindTeamHealthByManager and FindAggregatedDimensionsByManager each had two
-- runtime branches (with/without an assessmentPeriod filter) in the original
-- code. They are kept as two SEPARATE named queries here (not merged) so the
-- exact filter/behavior of each branch is preserved unchanged.

-- name: UpsertHealthCheckSession :exec
INSERT INTO health_check_sessions (
    id, team_id, user_id, date, assessment_period, survey_type, completed, updated_at
) VALUES (sqlc.arg(id), sqlc.arg(team_id), sqlc.arg(user_id), sqlc.arg(date), sqlc.arg(assessment_period), sqlc.arg(survey_type), sqlc.arg(completed), CURRENT_TIMESTAMP)
ON CONFLICT (id) DO UPDATE SET
    team_id = EXCLUDED.team_id,
    user_id = EXCLUDED.user_id,
    date = EXCLUDED.date,
    assessment_period = EXCLUDED.assessment_period,
    survey_type = EXCLUDED.survey_type,
    completed = EXCLUDED.completed,
    updated_at = CURRENT_TIMESTAMP;

-- name: DeleteHealthCheckResponsesBySessionID :exec
DELETE FROM health_check_responses WHERE session_id = sqlc.arg(session_id);

-- name: InsertHealthCheckResponse :exec
INSERT INTO health_check_responses (
    session_id, dimension_id, score, trend, comment
) VALUES (sqlc.arg(session_id), sqlc.arg(dimension_id), sqlc.arg(score), sqlc.arg(trend), sqlc.arg(comment));

-- name: FindSessionRowsByID :many
SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
       r.dimension_id, r.score, r.trend, r.comment
FROM health_check_sessions s
LEFT JOIN health_check_responses r ON s.id = r.session_id
WHERE s.id = sqlc.arg(id)
ORDER BY r.dimension_id;

-- name: FindSessionRowsByTeamID :many
SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
       r.dimension_id, r.score, r.trend, r.comment
FROM health_check_sessions s
LEFT JOIN health_check_responses r ON s.id = r.session_id
WHERE s.team_id = sqlc.arg(team_id)
ORDER BY s.date DESC, r.dimension_id;

-- name: FindSessionRowsByUserID :many
SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
       r.dimension_id, r.score, r.trend, r.comment
FROM health_check_sessions s
LEFT JOIN health_check_responses r ON s.id = r.session_id
WHERE s.user_id = sqlc.arg(user_id)
ORDER BY s.date DESC, r.dimension_id;

-- name: FindSessionRowsByAssessmentPeriod :many
SELECT s.id, s.team_id, s.user_id, s.date, s.assessment_period, s.survey_type, s.completed,
       r.dimension_id, r.score, r.trend, r.comment
FROM health_check_sessions s
LEFT JOIN health_check_responses r ON s.id = r.session_id
WHERE s.assessment_period = sqlc.arg(assessment_period)
ORDER BY s.date DESC, r.dimension_id;

-- name: FindTeamHealthByManagerWithPeriod :many
WITH post_workshop_teams AS (
    SELECT DISTINCT s.team_id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.survey_type = 'post_workshop'
        AND s.completed = true AND s.assessment_period = sqlc.arg(assessment_period)
),
effective_sessions AS (
    SELECT s.id, s.team_id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.completed = true AND s.assessment_period = sqlc.arg(assessment_period)
        AND (
            s.survey_type = 'post_workshop'
            OR (s.survey_type = 'individual'
                AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
        )
),
team_overall AS (
    SELECT
        t.id AS team_id,
        t.name AS team_name,
        COUNT(DISTINCT es.id) AS submission_count,
        AVG(r.score) AS overall_health,
        CASE WHEN EXISTS (
            SELECT 1 FROM post_workshop_teams pw WHERE pw.team_id = t.id
        ) THEN 'submitted' ELSE 'pending' END AS post_workshop_status
    FROM teams t
    INNER JOIN team_supervisors ts ON t.id = ts.team_id
    LEFT JOIN effective_sessions es ON t.id = es.team_id
    LEFT JOIN health_check_responses r ON es.id = r.session_id
    WHERE ts.user_id = sqlc.arg(manager_id)
    GROUP BY t.id, t.name
),
team_dimensions AS (
    SELECT
        es.team_id,
        r.dimension_id,
        AVG(r.score) AS avg_score,
        COUNT(r.dimension_id) AS response_count
    FROM effective_sessions es
    INNER JOIN health_check_responses r ON es.id = r.session_id
    GROUP BY es.team_id, r.dimension_id
)
SELECT
    o.team_id,
    o.team_name,
    o.submission_count,
    o.overall_health,
    o.post_workshop_status,
    d.dimension_id,
    d.avg_score,
    d.response_count
FROM team_overall o
LEFT JOIN team_dimensions d ON o.team_id = d.team_id
ORDER BY o.overall_health ASC NULLS LAST, o.team_name, d.dimension_id;

-- name: FindTeamHealthByManagerAllPeriods :many
WITH post_workshop_teams AS (
    SELECT DISTINCT s.team_id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.survey_type = 'post_workshop'
        AND s.completed = true
),
effective_sessions AS (
    SELECT s.id, s.team_id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.completed = true
        AND (
            s.survey_type = 'post_workshop'
            OR (s.survey_type = 'individual'
                AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
        )
),
team_overall AS (
    SELECT
        t.id AS team_id,
        t.name AS team_name,
        COUNT(DISTINCT es.id) AS submission_count,
        AVG(r.score) AS overall_health,
        CASE WHEN EXISTS (
            SELECT 1 FROM post_workshop_teams pw WHERE pw.team_id = t.id
        ) THEN 'submitted' ELSE 'pending' END AS post_workshop_status
    FROM teams t
    INNER JOIN team_supervisors ts ON t.id = ts.team_id
    LEFT JOIN effective_sessions es ON t.id = es.team_id
    LEFT JOIN health_check_responses r ON es.id = r.session_id
    WHERE ts.user_id = sqlc.arg(manager_id)
    GROUP BY t.id, t.name
),
team_dimensions AS (
    SELECT
        es.team_id,
        r.dimension_id,
        AVG(r.score) AS avg_score,
        COUNT(r.dimension_id) AS response_count
    FROM effective_sessions es
    INNER JOIN health_check_responses r ON es.id = r.session_id
    GROUP BY es.team_id, r.dimension_id
)
SELECT
    o.team_id,
    o.team_name,
    o.submission_count,
    o.overall_health,
    o.post_workshop_status,
    d.dimension_id,
    d.avg_score,
    d.response_count
FROM team_overall o
LEFT JOIN team_dimensions d ON o.team_id = d.team_id
ORDER BY o.overall_health ASC NULLS LAST, o.team_name, d.dimension_id;

-- name: FindAggregatedDimensionsByManagerWithPeriod :many
WITH post_workshop_teams AS (
    SELECT DISTINCT s.team_id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.survey_type = 'post_workshop'
        AND s.completed = true AND s.assessment_period = sqlc.arg(assessment_period)
),
effective_sessions AS (
    SELECT s.id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.completed = true AND s.assessment_period = sqlc.arg(assessment_period)
        AND (
            s.survey_type = 'post_workshop'
            OR (s.survey_type = 'individual'
                AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
        )
)
SELECT
    r.dimension_id,
    AVG(r.score) AS avg_score,
    COUNT(r.dimension_id) AS response_count
FROM effective_sessions es
INNER JOIN health_check_responses r ON es.id = r.session_id
GROUP BY r.dimension_id
ORDER BY r.dimension_id;

-- name: FindAggregatedDimensionsByManagerAllPeriods :many
WITH post_workshop_teams AS (
    SELECT DISTINCT s.team_id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.survey_type = 'post_workshop'
        AND s.completed = true
),
effective_sessions AS (
    SELECT s.id
    FROM health_check_sessions s
    INNER JOIN team_supervisors ts ON s.team_id = ts.team_id
    WHERE ts.user_id = sqlc.arg(manager_id) AND s.completed = true
        AND (
            s.survey_type = 'post_workshop'
            OR (s.survey_type = 'individual'
                AND s.team_id NOT IN (SELECT pw.team_id FROM post_workshop_teams pw))
        )
)
SELECT
    r.dimension_id,
    AVG(r.score) AS avg_score,
    COUNT(r.dimension_id) AS response_count
FROM effective_sessions es
INNER JOIN health_check_responses r ON es.id = r.session_id
GROUP BY r.dimension_id
ORDER BY r.dimension_id;

-- name: GetTeamSubmissionStatus :one
SELECT
    COUNT(DISTINCT tm.user_id) AS total_members,
    COUNT(DISTINCT CASE WHEN hcs.completed = true THEN hcs.user_id END) AS submitted_members,
    EXISTS(
        SELECT 1 FROM health_check_sessions hcs2
        WHERE hcs2.team_id = sqlc.arg(team_id) AND hcs2.assessment_period = sqlc.arg(assessment_period)
            AND hcs2.survey_type = 'post_workshop' AND hcs2.completed = true
    ) AS post_workshop_exists
FROM team_members tm
LEFT JOIN health_check_sessions hcs ON tm.user_id = hcs.user_id AND tm.team_id = hcs.team_id
    AND hcs.assessment_period = sqlc.arg(assessment_period) AND hcs.survey_type = 'individual'
WHERE tm.team_id = sqlc.arg(team_id) AND tm.is_active;

-- name: DeleteHealthCheckSession :execrows
DELETE FROM health_check_sessions WHERE id = sqlc.arg(id);

-- name: FindDistinctAssessmentPeriods :many
SELECT DISTINCT assessment_period FROM health_check_sessions
WHERE assessment_period IS NOT NULL AND assessment_period != '' AND completed = true
ORDER BY assessment_period DESC;

-- name: LockTeamHealthCheckEnabled :one
-- FOR SHARE locks the team row until the caller's transaction ends: a concurrent
-- sync UPDATE of health_check_enabled needs a conflicting lock, so it either
-- commits first (this read sees false) or waits for the save to finish.
SELECT health_check_enabled FROM teams WHERE id = sqlc.arg(team_id) FOR SHARE;

-- name: GetTeamHealthCheckEnabled :one
SELECT health_check_enabled FROM teams WHERE id = sqlc.arg(team_id);

-- name: FindSubmittedPeriodsByUser :many
-- Completed submissions of one survey type for one user (individual scope).
-- Sessions that never recorded a period are left out.
SELECT assessment_period
FROM health_check_sessions
WHERE user_id = sqlc.arg(user_id) AND survey_type = sqlc.arg(survey_type) AND completed = true
  AND assessment_period IS NOT NULL AND assessment_period <> '';

-- name: FindSubmittedPeriodsByTeam :many
-- Completed submissions of one survey type for one team (post_workshop scope).
SELECT assessment_period
FROM health_check_sessions
WHERE team_id = sqlc.arg(team_id) AND survey_type = sqlc.arg(survey_type) AND completed = true
  AND assessment_period IS NOT NULL AND assessment_period <> '';
