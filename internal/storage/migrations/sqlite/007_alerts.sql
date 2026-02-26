CREATE TABLE IF NOT EXISTS alerts (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  metric TEXT NOT NULL,
  event_name TEXT,
  threshold INTEGER NOT NULL,
  window_minutes INTEGER NOT NULL DEFAULT 60,
  webhook_url TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  last_triggered_at DATETIME,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
