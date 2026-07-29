-- SPDX-License-Identifier: Apache-2.0

CREATE TABLE IF NOT EXISTS app_settings (
    id INTEGER PRIMARY KEY DEFAULT 1 CONSTRAINT singleton CHECK (id = 1),
    email_notifications BOOLEAN NOT NULL DEFAULT false,
    slack_notifications BOOLEAN NOT NULL DEFAULT false,
    weekly_digest BOOLEAN NOT NULL DEFAULT false,
    retention_months INTEGER NOT NULL DEFAULT 12,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

INSERT INTO app_settings (id) VALUES (1) ON CONFLICT DO NOTHING;
