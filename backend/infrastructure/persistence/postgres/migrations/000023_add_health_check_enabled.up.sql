-- Track teams.health_check_enabled as a real migration instead of the
-- hand-patched ALTER TABLE some databases had accumulated out of band.
ALTER TABLE teams ADD COLUMN IF NOT EXISTS health_check_enabled BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN teams.health_check_enabled IS 'Whether this team participates in survey-completion tracking; false = opted out of health checks entirely.';
