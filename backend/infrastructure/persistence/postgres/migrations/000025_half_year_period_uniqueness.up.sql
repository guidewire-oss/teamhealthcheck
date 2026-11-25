-- One submission per period, and no more 'quarterly' cadence.
--
-- Final state of the half-year (H1/H2) assessment-period work, in a single migration:
--
--   1. 'quarterly' is no longer a team cadence. Existing quarterly teams move to
--      'half-yearly', the closest supported cadence, and half-yearly becomes the column
--      default (it was 'quarterly' since migration 000008, so an INSERT that omitted cadence
--      would otherwise violate the new CHECK).
--   2. A completed session must carry an assessment period. Without that, the unique indexes
--      below could not apply: PostgreSQL treats every NULL as distinct, so unlimited completed
--      submissions could share a missing period. The constraint is NOT VALID, so it applies to
--      rows written from now on and does not reject historical rows that never had a period.
--   3. At most one completed submission per user (Individual Survey) or per team
--      (Post-Workshop Survey) for a given assessment period. The application stores periods in
--      one canonical form (see healthcheck.CanonicalPeriod), so equal periods are equal
--      strings. This is the race-safe backstop behind the application's own duplicate check
--      (healthcheck.CheckPeriodEligibility). Which periods are open right now is a policy the
--      application decides; only the duplicate rule belongs in the database.

-- 1. Cadence
UPDATE teams SET cadence = 'half-yearly' WHERE cadence = 'quarterly';

ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;
ALTER TABLE teams
    ADD CONSTRAINT chk_teams_cadence_values
    CHECK (cadence IS NULL OR cadence IN ('monthly', 'half-yearly', 'yearly'));

ALTER TABLE teams ALTER COLUMN cadence SET DEFAULT 'half-yearly';

-- 2. Completed sessions must have a period (new and updated rows only)
ALTER TABLE health_check_sessions
    ADD CONSTRAINT chk_completed_session_has_period
    CHECK (NOT completed OR (assessment_period IS NOT NULL AND assessment_period <> ''))
    NOT VALID;

-- 3. One completed submission per scope and period. Fail with a readable message, rather
--    than a bare unique-violation, if existing history already breaks the rule: those rows
--    are real data and are never deleted or rewritten here.
DO $$
DECLARE
    dup_groups integer;
BEGIN
    SELECT count(*) INTO dup_groups FROM (
        SELECT 1
        FROM health_check_sessions
        WHERE completed = true AND assessment_period IS NOT NULL
          AND survey_type IN ('individual', 'post_workshop')
        GROUP BY survey_type,
                 CASE WHEN survey_type = 'individual' THEN user_id ELSE team_id END,
                 assessment_period
        HAVING count(*) > 1
    ) d;

    IF dup_groups > 0 THEN
        RAISE EXCEPTION
            'Cannot add one-submission-per-period uniqueness: % (scope, survey type, period) group(s) already have more than one completed submission. Reconcile them first, for example by marking the extra rows completed = false, then re-run the migration.',
            dup_groups;
    END IF;
END $$;

CREATE UNIQUE INDEX uniq_individual_period_submission
    ON health_check_sessions (user_id, assessment_period)
    WHERE survey_type = 'individual' AND completed = true;

CREATE UNIQUE INDEX uniq_post_workshop_period_submission
    ON health_check_sessions (team_id, assessment_period)
    WHERE survey_type = 'post_workshop' AND completed = true;
