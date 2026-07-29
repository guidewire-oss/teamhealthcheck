-- SPDX-License-Identifier: Apache-2.0

-- Revert cadence constraint (lossy: cannot restore original weekly/biweekly values)
ALTER TABLE teams DROP CONSTRAINT IF EXISTS chk_teams_cadence_values;

-- Re-map half-yearly/yearly back to quarterly as closest equivalent
UPDATE teams SET cadence = 'quarterly' WHERE cadence IN ('half-yearly', 'yearly');

ALTER TABLE teams
ADD CONSTRAINT chk_teams_cadence_values
CHECK (cadence IS NULL OR cadence IN ('weekly', 'biweekly', 'monthly', 'quarterly'));
