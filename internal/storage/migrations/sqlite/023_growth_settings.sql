-- 023_growth_settings.sql
-- Generic key-value settings table for growth features (ICP auto-refresh schedules, etc.)
CREATE TABLE IF NOT EXISTS growth_settings (
    project_id TEXT NOT NULL REFERENCES projects(id),
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, key)
);
