CREATE TABLE IF NOT EXISTS identity_aliases (
    project_id TEXT NOT NULL,
    anonymous_id TEXT NOT NULL,
    identified_id TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, anonymous_id)
);
CREATE INDEX IF NOT EXISTS idx_identity_aliases_identified ON identity_aliases(project_id, identified_id);
