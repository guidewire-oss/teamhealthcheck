-- Replace the calendar-quarter duplicate-submission indexes with a rolling six-month
-- submission cooldown, enforced as a Postgres exclusion constraint: two completed
-- submissions of the same survey type in the same scope (per user for individual, per team
-- for post_workshop) may never have overlapping [date, date + 6 months) windows. This
-- removes the calendar-quarter concept entirely from duplicate-submission enforcement.
CREATE EXTENSION IF NOT EXISTS btree_gist;

DROP INDEX IF EXISTS idx_unique_individual_quarter_submission;
DROP INDEX IF EXISTS idx_unique_post_workshop_quarter_submission;

ALTER TABLE health_check_sessions
DROP COLUMN IF EXISTS quarter_year,
DROP COLUMN IF EXISTS quarter_number;

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
