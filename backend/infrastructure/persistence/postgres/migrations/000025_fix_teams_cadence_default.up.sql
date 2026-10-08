-- Migration 000023 removed 'quarterly' from the teams.cadence CHECK constraint and backfilled
-- existing rows, but never changed the column's DEFAULT, which migration 000008 set to
-- 'quarterly'. Any INSERT that omits cadence (e.g. the demo-data seeder) still fell back to
-- that now-invalid default and violated chk_teams_cadence_values. Point the default at
-- half-yearly, the same value existing quarterly-cadence teams were migrated to.
ALTER TABLE teams ALTER COLUMN cadence SET DEFAULT 'half-yearly';
