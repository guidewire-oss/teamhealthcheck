DROP INDEX IF EXISTS uniq_individual_period_submission;
DROP INDEX IF EXISTS uniq_post_workshop_period_submission;

-- Restore the half-year-boundary exclusion constraints this migration replaced (see
-- 000024_half_year_boundary_eligibility.up.sql).
CREATE OR REPLACE FUNCTION next_eligible_submission_date(submission_date date)
RETURNS date
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    cooldown_end date := submission_date + INTERVAL '6 months';
    next_half_start date;
BEGIN
    IF EXTRACT(MONTH FROM submission_date) <= 6 THEN
        next_half_start := make_date(EXTRACT(YEAR FROM submission_date)::int, 7, 1);
    ELSE
        next_half_start := make_date(EXTRACT(YEAR FROM submission_date)::int + 1, 1, 1);
    END IF;

    IF next_half_start < cooldown_end THEN
        RETURN next_half_start;
    END IF;
    RETURN cooldown_end;
END;
$$;

ALTER TABLE health_check_sessions
ADD CONSTRAINT excl_individual_six_month_gap
EXCLUDE USING gist (
    user_id WITH =,
    daterange(date, next_eligible_submission_date(date), '[)') WITH &&
)
WHERE (survey_type = 'individual' AND completed = true);

ALTER TABLE health_check_sessions
ADD CONSTRAINT excl_post_workshop_six_month_gap
EXCLUDE USING gist (
    team_id WITH =,
    daterange(date, next_eligible_submission_date(date), '[)') WITH &&
)
WHERE (survey_type = 'post_workshop' AND completed = true);
