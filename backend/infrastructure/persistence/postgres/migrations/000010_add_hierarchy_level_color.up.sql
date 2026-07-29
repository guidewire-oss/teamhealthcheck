-- SPDX-License-Identifier: Apache-2.0

-- Add color column to hierarchy_levels table
ALTER TABLE hierarchy_levels ADD COLUMN IF NOT EXISTS color VARCHAR(20);

-- Update existing levels with default colors
UPDATE hierarchy_levels SET color = '#9333EA' WHERE id = 'level-1';  -- VP - Purple
UPDATE hierarchy_levels SET color = '#2563EB' WHERE id = 'level-2';  -- Director - Blue
UPDATE hierarchy_levels SET color = '#059669' WHERE id = 'level-3';  -- Manager - Green
UPDATE hierarchy_levels SET color = '#D97706' WHERE id = 'level-4';  -- Team Lead - Amber
UPDATE hierarchy_levels SET color = '#6B7280' WHERE id = 'level-5';  -- Team Member - Gray
UPDATE hierarchy_levels SET color = '#DC2626' WHERE id = 'level-admin';  -- Admin - Red
