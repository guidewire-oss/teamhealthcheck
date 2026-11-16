-- Read-only verification for seed_survey_completion_sample_data.sql.
-- Reproduces deriveSurveyCompletionStatus's exact branching (in the exact
-- same order: opted_out first, then not_started, then complete, else
-- in_progress -- see backend/interfaces/api/v1/survey_completion_admin_handler.go)
-- in pure SQL, per pod, so the seed can be confirmed correct before ever
-- opening a browser.
--
-- Usage (target teams360_dummy only):
--   docker exec -i teams360-db psql -U postgres -d teams360_dummy -f - \
--     < verify_survey_completion_seed.sql

\set ON_ERROR_STOP on

WITH seed_period AS (
  SELECT
    CASE
      WHEN extract(month FROM current_date) BETWEEN 1 AND 6
        THEN (extract(year FROM current_date)::int - 1) || ' - 2nd Half'
      ELSE extract(year FROM current_date)::int || ' - 1st Half'
    END AS period
)
SELECT
  t.id AS pod_id,
  t.name AS pod_name,
  (SELECT period FROM seed_period) AS assessment_period,
  t.health_check_enabled AS opted_in,
  COUNT(DISTINCT tm.user_id) AS member_count,
  COUNT(DISTINCT s_ind.user_id) AS individual_completed_count,
  BOOL_OR(s_pw.id IS NOT NULL) AS post_workshop_completed,
  CASE
    WHEN NOT t.health_check_enabled THEN 'opted_out'
    WHEN COUNT(DISTINCT tm.user_id) = 0 OR COUNT(DISTINCT s_ind.user_id) = 0 THEN 'not_started'
    WHEN COUNT(DISTINCT s_ind.user_id) >= COUNT(DISTINCT tm.user_id) THEN 'complete'
    ELSE 'in_progress'
  END AS derived_status
FROM teams t
LEFT JOIN team_members tm ON tm.team_id = t.id
LEFT JOIN health_check_sessions s_ind
  ON s_ind.team_id = t.id
 AND s_ind.completed = true
 AND s_ind.survey_type = 'individual'
 AND s_ind.assessment_period = (SELECT period FROM seed_period)
LEFT JOIN health_check_sessions s_pw
  ON s_pw.team_id = t.id
 AND s_pw.completed = true
 AND s_pw.survey_type = 'post_workshop'
 AND s_pw.assessment_period = (SELECT period FROM seed_period)
GROUP BY t.id, t.name, t.health_check_enabled
ORDER BY derived_status, t.name;
