-- Per-project OAuth credentials for social platform sources and publishers.
-- Credentials are stored encrypted using the project-level encryption key.
CREATE TABLE source_credentials (
    id           TEXT PRIMARY KEY,
    project_id   TEXT NOT NULL,
    source_name  TEXT NOT NULL,        -- e.g. 'reddit', 'twitter'
    access_token TEXT NOT NULL DEFAULT '',   -- AES-256-GCM encrypted
    refresh_token TEXT NOT NULL DEFAULT '',  -- AES-256-GCM encrypted
    token_expiry DATETIME,
    username     TEXT NOT NULL DEFAULT '',   -- display name / handle, unencrypted
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(project_id, source_name)
);

-- Extend oauth_state to carry source-specific extra data (e.g. Twitter PKCE code verifier).
ALTER TABLE oauth_state ADD COLUMN extra TEXT NOT NULL DEFAULT '';
