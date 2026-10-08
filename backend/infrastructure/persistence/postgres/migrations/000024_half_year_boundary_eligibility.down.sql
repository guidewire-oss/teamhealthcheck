ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS excl_individual_six_month_gap;
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS excl_post_workshop_six_month_gap;

ALTER TABLE health_check_sessions
ADD CONSTRAINT excl_individual_six_month_gap
EXCLUDE USING gist (
    user_id WITH =,
    daterange(date, (date + INTERVAL '6 months')::date, '[)') WITH &&
)
WHERE (survey_type = 'individual' AND completed = true);

ALTER TABLE health_check_sessions
ADD CONSTRAINT excl_post_workshop_six_month_gap
EXCLUDE USING gist (
    team_id WITH =,
    daterange(date, (date + INTERVAL '6 months')::date, '[)') WITH &&
)
WHERE (survey_type = 'post_workshop' AND completed = true);

DROP FUNCTION IF EXISTS next_eligible_submission_date(date);
