-- Replace the six-month/half-year-boundary cooldown exclusion constraints with a direct
-- one-submission-per-period rule: at most one completed submission per user (for the
-- Individual Survey) or per team (for the Post-Workshop Survey), per survey type and exact
-- assessment period ("YYYY H1"/"YYYY H2"). Eligibility windows (which periods are open right
-- now) are a business-hours policy decided by the application
-- (healthcheck.CheckPeriodEligibility), not a data-integrity constraint, so only the
-- duplicate rule needs to live in the database -- this is the authoritative, race-safe guard
-- behind that application-level check.
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS excl_individual_six_month_gap;
ALTER TABLE health_check_sessions DROP CONSTRAINT IF EXISTS excl_post_workshop_six_month_gap;

DROP FUNCTION IF EXISTS next_eligible_submission_date(date);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_individual_period_submission
    ON health_check_sessions (user_id, assessment_period)
    WHERE (survey_type = 'individual' AND completed = true);

CREATE UNIQUE INDEX IF NOT EXISTS uniq_post_workshop_period_submission
    ON health_check_sessions (team_id, assessment_period)
    WHERE (survey_type = 'post_workshop' AND completed = true);
