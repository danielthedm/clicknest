-- 022_lead_score_snapshots.sql
-- Daily score snapshots for trend visualization (score delta) and history charts.
CREATE TABLE IF NOT EXISTS lead_score_snapshots (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    project_id TEXT NOT NULL REFERENCES projects(id),
    distinct_id TEXT NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    raw_score INTEGER NOT NULL DEFAULT 0,
    snapshot_date TEXT NOT NULL, -- YYYY-MM-DD
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project_id, distinct_id, snapshot_date)
);
CREATE INDEX IF NOT EXISTS idx_score_snapshots_lead ON lead_score_snapshots(project_id, distinct_id, snapshot_date DESC);
CREATE INDEX IF NOT EXISTS idx_score_snapshots_date ON lead_score_snapshots(project_id, snapshot_date DESC);
