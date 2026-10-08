-- Extend the submission-cooldown exclusion constraints to also honor the calendar half-year
-- boundary rule: a submission becomes eligible as soon as EITHER six calendar months have
-- elapsed since the prior submission (the original rule, preserved) OR the calendar has
-- moved into a later half-year period than the prior submission's (new rule) -- whichever
-- comes first. This mirrors healthcheck.NextEligibleDate in the Go domain layer exactly, so
-- the database's race-safe guard never disagrees with the application-level pre-check.
--
-- H1 = January 1 - June 30, H2 = July 1 - December 31. The year is always derived from the
-- submission date, never hardcoded.
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

ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS excl_individual_six_month_gap;
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS excl_post_workshop_six_month_gap;

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
