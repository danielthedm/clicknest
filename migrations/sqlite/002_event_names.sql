CREATE TABLE IF NOT EXISTS event_names (
    fingerprint TEXT NOT NULL,
    project_id  TEXT NOT NULL,
    ai_name     TEXT NOT NULL,
    user_name   TEXT,
    source_file TEXT,
    confidence  REAL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (fingerprint, project_id)
);
