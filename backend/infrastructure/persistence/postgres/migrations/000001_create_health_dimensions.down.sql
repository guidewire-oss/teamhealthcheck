-- SPDX-License-Identifier: Apache-2.0

-- Drop health_dimensions table
DROP INDEX IF EXISTS idx_dimensions_active;
DROP TABLE IF EXISTS health_dimensions CASCADE;
