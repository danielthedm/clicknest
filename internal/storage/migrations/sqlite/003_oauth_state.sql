CREATE TABLE IF NOT EXISTS oauth_state (
    state       TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);
