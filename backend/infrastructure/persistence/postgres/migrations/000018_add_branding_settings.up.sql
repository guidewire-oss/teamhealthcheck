-- SPDX-License-Identifier: Apache-2.0

ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS company_name TEXT NOT NULL DEFAULT 'My Company';
ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS logo_url TEXT DEFAULT NULL;
