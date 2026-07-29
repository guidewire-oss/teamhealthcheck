-- SPDX-License-Identifier: Apache-2.0

-- Update cadence values: remove weekly/biweekly, add half-yearly/yearly
-- This migration is lossy: weekly/biweekly are mapped to monthly and cannot be restored.

-- Drop old CHECK constraint
ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;

-- Migrate weekly/biweekly → monthly (nearest remaining frequent cadence)
UPDATE teams SET cadence = 'monthly' WHERE cadence IN ('weekly', 'biweekly');

-- Add new CHECK constraint with updated values
ALTER TABLE teams
ADD CONSTRAINT chk_teams_cadence_values
CHECK (cadence IS NULL OR cadence IN ('monthly', 'quarterly', 'half-yearly', 'yearly'));
