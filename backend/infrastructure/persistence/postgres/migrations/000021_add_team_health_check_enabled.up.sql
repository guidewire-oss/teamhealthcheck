-- Whether a team participates in health checks.
-- Defaults to true so teams that predate provider sync keep their current behaviour;
-- an external organization snapshot may override it per team.
ALTER TABLE teams ADD COLUMN IF NOT EXISTS health_check_enabled BOOLEAN NOT NULL DEFAULT true;
