CREATE TABLE IF NOT EXISTS icp_analyses (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    conversion_pages TEXT NOT NULL DEFAULT '[]',
    summary TEXT NOT NULL DEFAULT '',
    traits TEXT NOT NULL DEFAULT '[]',
    channels TEXT NOT NULL DEFAULT '[]',
    recommendations TEXT NOT NULL DEFAULT '[]',
    profile_count INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_icp_analyses_project ON icp_analyses(project_id, created_at DESC);
