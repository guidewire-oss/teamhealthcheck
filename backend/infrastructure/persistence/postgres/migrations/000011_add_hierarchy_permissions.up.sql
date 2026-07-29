-- SPDX-License-Identifier: Apache-2.0

-- Add additional permission columns to hierarchy_levels table
ALTER TABLE hierarchy_levels ADD COLUMN IF NOT EXISTS can_configure_system BOOLEAN DEFAULT false;
ALTER TABLE hierarchy_levels ADD COLUMN IF NOT EXISTS can_view_reports BOOLEAN DEFAULT false;
ALTER TABLE hierarchy_levels ADD COLUMN IF NOT EXISTS can_export_data BOOLEAN DEFAULT false;

-- Update existing levels with appropriate permissions
UPDATE hierarchy_levels SET can_configure_system = true WHERE id = 'level-admin';  -- Admin can configure system
UPDATE hierarchy_levels SET can_view_reports = true WHERE id IN ('level-1', 'level-2', 'level-3', 'level-admin');  -- VP, Director, Manager, Admin can view reports
UPDATE hierarchy_levels SET can_export_data = true WHERE id IN ('level-1', 'level-2', 'level-admin');  -- VP, Director, Admin can export data
