CREATE TABLE IF NOT EXISTS experiments (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    flag_key TEXT NOT NULL,
    variants TEXT NOT NULL DEFAULT '[]',
    conversion_goal_id TEXT DEFAULT '' REFERENCES conversion_goals(id),
    status TEXT NOT NULL DEFAULT 'running',
    auto_stop INTEGER NOT NULL DEFAULT 0,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at DATETIME,
    winner_variant TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_experiments_project ON experiments(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_experiments_flag ON experiments(project_id, flag_key);
