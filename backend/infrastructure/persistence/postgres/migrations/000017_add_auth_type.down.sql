-- SPDX-License-Identifier: Apache-2.0

ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_auth_type;
ALTER TABLE users DROP COLUMN IF EXISTS auth_type;
