CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    api_key     TEXT UNIQUE NOT NULL,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS github_connections (
    project_id      TEXT PRIMARY KEY,
    repo_owner      TEXT NOT NULL,
    repo_name       TEXT NOT NULL,
    access_token    TEXT NOT NULL,
    default_branch  TEXT DEFAULT 'main',
    last_synced_at  DATETIME,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE TABLE IF NOT EXISTS source_index (
    project_id      TEXT NOT NULL,
    file_path       TEXT NOT NULL,
    component_name  TEXT,
    selectors       TEXT,
    content_hash    TEXT,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, file_path)
);

CREATE TABLE IF NOT EXISTS llm_config (
    project_id  TEXT PRIMARY KEY,
    provider    TEXT NOT NULL DEFAULT 'openai',
    api_key     TEXT,
    model       TEXT NOT NULL DEFAULT 'gpt-4o-mini',
    base_url    TEXT,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);
