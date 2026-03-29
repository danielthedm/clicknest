-- 024_segments.sql
-- Saved segments: named user groups defined by scoring-rule-style conditions.
CREATE TABLE IF NOT EXISTS segments (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    project_id TEXT NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    conditions TEXT NOT NULL DEFAULT '[]', -- JSON array of {rule_type, config, points}
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_segments_project ON segments(project_id, created_at DESC);
