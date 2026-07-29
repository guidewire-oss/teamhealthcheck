-- SPDX-License-Identifier: Apache-2.0

-- Drop health_check_responses table
DROP INDEX IF EXISTS idx_responses_session_dimension;
DROP INDEX IF EXISTS idx_responses_dimension_score;
DROP INDEX IF EXISTS idx_responses_session;
DROP TABLE IF EXISTS health_check_responses CASCADE;
