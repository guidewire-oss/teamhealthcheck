-- Idempotent sample-data seed for teams360_dummy's Survey Completion page.
--
-- SCHEMA NOTES (inspected from backend/infrastructure/persistence/postgres/
-- migrations/ before writing this):
--   - teams(id, name, team_lead_id, health_check_enabled, ...),
--     team_members(team_id, user_id), users(id, full_name, ...) --
--     000005_create_users_and_teams.up.sql / 000021_add_health_check_enabled.up.sql
--   - health_check_sessions(id VARCHAR(100), team_id VARCHAR(50),
--     user_id VARCHAR(50), date, assessment_period VARCHAR(50),
--     completed BOOLEAN, survey_type VARCHAR(20) CHECK IN
--     ('individual','post_workshop')) -- 000002_create_health_check_sessions.up.sql,
--     000014_add_survey_type.up.sql. Note team_id/user_id here are capped at
--     50 chars even though teams.id/users.id allow up to 255 -- a
--     pre-existing schema mismatch this script does not attempt to fix
--     (out of scope: no schema/application changes). If any real team or
--     user id in this database exceeds 50 characters, the INSERTs below
--     will fail with a clear "value too long for type character
--     varying(50)" error and, because of the transaction + ON_ERROR_STOP
--     below, the whole run rolls back cleanly rather than leaving partial
--     data.
--
-- Mirrors deriveSurveyCompletionStatus in
-- backend/interfaces/api/v1/survey_completion_admin_handler.go exactly:
--   !health_check_enabled          -> opted_out
--   member_count=0 OR completed=0  -> not_started
--   completed >= member_count      -> complete
--   else                            -> in_progress
--
-- Confined entirely to sessions this script itself owns (id prefix
-- 'survey-seed-') and to teams.health_check_enabled -- safe to re-run any
-- number of times, and driven purely from whatever teams/team_members
-- currently exist in teams360_dummy (nothing hardcoded). Does not touch
-- users, hierarchy_levels, team_supervisors, or reports_to.
--
-- Bucketing is deterministic: teams with at least one member are ordered
-- by id and cycled through an 8-slot pattern (ROW_NUMBER() mod 8) weighted
-- so roughly half land in_progress (4/8), a quarter complete (2/8), and
-- one-eighth each not_started / opted_out -- so the same team lands in the
-- same bucket on every rerun as long as the roster itself hasn't changed. A
-- team that would land in "in_progress" with fewer than 2 members is
-- downgraded to "not_started", since partial completion isn't
-- representable with only one member (their one survey completing would
-- read as 100%, i.e. "complete", not "in progress"). Teams with zero
-- members are skipped entirely -- they already render as "not_started"
-- (member_count=0) with no rows needed.
--
-- Usage (target teams360_dummy only):
--   docker exec -i teams360-db psql -U postgres -d teams360_dummy -f - \
--     < seed_survey_completion_sample_data.sql

\set ON_ERROR_STOP on

BEGIN;

-- 1. Assessment period: reproduces frontend lib/assessment-period.ts's own
--    rule (Jan-Jun -> previous year's 2nd Half; Jul-Dec -> current year's
--    1st Half) so the seeded data lands in whatever period the dashboard
--    already defaults to today.
CREATE TEMP TABLE seed_period AS
SELECT
  CASE
    WHEN extract(month FROM current_date) BETWEEN 1 AND 6
      THEN (extract(year FROM current_date)::int - 1) || ' - 2nd Half'
    ELSE extract(year FROM current_date)::int || ' - 1st Half'
  END AS period;

-- 2. Clear this script's own previously-seeded rows for this period, so
--    reruns converge to the current bucket assignment rather than
--    accumulating stale rows on top of it.
DELETE FROM health_check_sessions
WHERE id LIKE 'survey-seed-%'
  AND assessment_period = (SELECT period FROM seed_period);

-- 2b. Also clear any earlier ad-hoc "fake-*" seed rows for this same
--     period (from a prior, non-deterministic seeding pass in this same
--     dummy database, if one was ever run). Left in place, those rows
--     would add extra completed users on top of the counts this script is
--     now deterministically controlling, and could silently flip a team
--     out of its intended bucket. This touches only that one known
--     prefix, only for this period, and never touches any row from
--     another period or any table other than health_check_sessions.
DELETE FROM health_check_sessions
WHERE id LIKE 'fake-%'
  AND assessment_period = (SELECT period FROM seed_period);

-- 3. Deterministic bucket assignment, from the real current roster.
CREATE TEMP TABLE team_buckets AS
WITH ranked AS (
  SELECT
    t.id,
    t.team_lead_id,
    COUNT(tm.user_id) AS member_count,
    array_agg(tm.user_id ORDER BY tm.user_id) FILTER (WHERE tm.user_id IS NOT NULL) AS member_ids,
    ROW_NUMBER() OVER (ORDER BY t.id) AS rn
  FROM teams t
  LEFT JOIN team_members tm ON tm.team_id = t.id
  GROUP BY t.id, t.team_lead_id
)
SELECT
  id, team_lead_id, member_count, member_ids,
  CASE
    WHEN member_count = 0 THEN 'skip'
    WHEN (ARRAY['in_progress', 'complete', 'in_progress', 'not_started', 'in_progress', 'opted_out', 'in_progress', 'complete'])[((rn - 1) % 8) + 1] = 'in_progress'
         AND member_count < 2
      THEN 'not_started'
    ELSE (ARRAY['in_progress', 'complete', 'in_progress', 'not_started', 'in_progress', 'opted_out', 'in_progress', 'complete'])[((rn - 1) % 8) + 1]
  END AS bucket
FROM ranked
WHERE member_count > 0;

-- 4. Opt every bucketed team in/out per its bucket. This is the only
--    write against the teams table -- no other column is touched, and no
--    row for a zero-member team is touched at all.
UPDATE teams t
SET health_check_enabled = (b.bucket <> 'opted_out')
FROM team_buckets b
WHERE t.id = b.id;

-- 5a. "complete": every current member completes their individual
--     survey, plus a completed post-workshop session.
INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed, survey_type)
SELECT
  'survey-seed-ind-' || substr(md5(b.id || ':' || m.user_id), 1, 20),
  b.id,
  m.user_id,
  current_date - 3,
  (SELECT period FROM seed_period),
  true,
  'individual'
FROM team_buckets b
CROSS JOIN LATERAL unnest(b.member_ids) AS m(user_id)
WHERE b.bucket = 'complete';

INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed, survey_type)
SELECT
  'survey-seed-pw-' || substr(md5(b.id), 1, 20),
  b.id,
  COALESCE(b.team_lead_id, b.member_ids[1]),
  current_date - 1,
  (SELECT period FROM seed_period),
  true,
  'post_workshop'
FROM team_buckets b
WHERE b.bucket = 'complete';

-- 5b. "in_progress": roughly half of the current members complete their
--     individual survey (strictly between 0 and member_count, guaranteed
--     by the <2 downgrade above); no post-workshop session at all, which
--     the backend already reads as incomplete -- it only ever consults
--     completed=true sessions, so an absent row and a completed=false row
--     have identical effect on the derived status.
INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed, survey_type)
SELECT
  'survey-seed-ind-' || substr(md5(b.id || ':' || m.user_id), 1, 20),
  b.id,
  m.user_id,
  current_date - 3,
  (SELECT period FROM seed_period),
  true,
  'individual'
FROM team_buckets b
CROSS JOIN LATERAL unnest(b.member_ids[1 : GREATEST(1, b.member_count / 2)]) AS m(user_id)
WHERE b.bucket = 'in_progress';

-- 5c. "not_started": no individual sessions and no post-workshop session
--     inserted at all -- member_count > 0 but completed = 0 is exactly
--     what deriveSurveyCompletionStatus reads as "not_started".

-- 5d. "opted_out": no sessions touched either way -- health_check_enabled
--     = false (set in step 4) overrides completion counts entirely.

-- 6. Sanity check before committing -- eyeball this output.
SELECT bucket, count(*) AS teams, sum(member_count) AS total_members
FROM team_buckets
GROUP BY bucket
ORDER BY bucket;

COMMIT;
