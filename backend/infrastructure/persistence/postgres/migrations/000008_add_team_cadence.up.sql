-- SPDX-License-Identifier: Apache-2.0

-- Add cadence column to teams table
ALTER TABLE teams ADD COLUMN cadence VARCHAR(50) DEFAULT 'quarterly';

-- Update demo teams with cadence values
UPDATE teams SET cadence = 'quarterly' WHERE id = 'team-phoenix';
UPDATE teams SET cadence = 'monthly' WHERE id = 'team-dragon';
UPDATE teams SET cadence = 'quarterly' WHERE id = 'team-titan';
UPDATE teams SET cadence = 'biweekly' WHERE id = 'team-falcon';
UPDATE teams SET cadence = 'quarterly' WHERE id = 'team-eagle';
