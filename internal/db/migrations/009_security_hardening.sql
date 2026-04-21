-- +goose Up
ALTER TABLE users ADD COLUMN is_super_admin INTEGER NOT NULL DEFAULT 0;

UPDATE users
SET is_super_admin = 1
WHERE id = (
    SELECT id
    FROM users
    ORDER BY created_at ASC, rowid ASC
    LIMIT 1
)
AND NOT EXISTS (
    SELECT 1
    FROM users
    WHERE is_super_admin = 1
);

CREATE TABLE IF NOT EXISTS user_sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    is_super_admin INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_user_sessions_expires_at;
DROP INDEX IF EXISTS idx_user_sessions_user_id;
DROP TABLE IF EXISTS user_sessions;
