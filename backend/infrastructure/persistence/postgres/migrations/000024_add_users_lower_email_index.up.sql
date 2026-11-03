-- The organization sync re-keys users by matching on lower(email) or username
-- (findRekeyCandidates). users.email has only a plain UNIQUE constraint, which
-- cannot serve lower(email) = ..., so every lookup scanned the whole table.
-- Not unique: existing rows may differ only by case.
CREATE INDEX IF NOT EXISTS idx_users_lower_email ON users (lower(email));
