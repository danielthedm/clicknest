CREATE TABLE IF NOT EXISTS conversion_goals (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    event_type TEXT NOT NULL DEFAULT 'custom',
    event_name TEXT NOT NULL DEFAULT '',
    url_pattern TEXT NOT NULL DEFAULT '',
    value_property TEXT NOT NULL DEFAULT '$value',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, name)
);
CREATE INDEX IF NOT EXISTS idx_conversion_goals_project ON conversion_goals(project_id);

ALTER TABLE campaigns ADD COLUMN cost REAL NOT NULL DEFAULT 0;
