CREATE TABLE IF NOT EXISTS mentions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    source_name TEXT NOT NULL,
    external_id TEXT NOT NULL,
    external_url TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    relevance_score REAL NOT NULL DEFAULT 0.0,
    status TEXT NOT NULL DEFAULT 'new',
    suggested_reply TEXT NOT NULL DEFAULT '',
    parent_id TEXT NOT NULL DEFAULT '',
    metadata TEXT NOT NULL DEFAULT '{}',
    posted_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mentions_project_status ON mentions(project_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_mentions_project_external ON mentions(project_id, source_name, external_id);
