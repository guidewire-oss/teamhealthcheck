-- SPDX-License-Identifier: Apache-2.0

-- Remove additional permission columns from hierarchy_levels table
ALTER TABLE hierarchy_levels DROP COLUMN IF EXISTS can_configure_system;
ALTER TABLE hierarchy_levels DROP COLUMN IF EXISTS can_view_reports;
ALTER TABLE hierarchy_levels DROP COLUMN IF EXISTS can_export_data;
