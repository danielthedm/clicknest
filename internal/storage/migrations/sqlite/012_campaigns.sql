CREATE TABLE IF NOT EXISTS campaigns (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    channel TEXT NOT NULL, -- reddit, linkedin, twitter, youtube, blog
    ref_code_id TEXT REFERENCES ref_codes(id),
    status TEXT NOT NULL DEFAULT 'draft', -- draft, published, archived
    content TEXT NOT NULL DEFAULT '{}', -- JSON: {title, body, url, tags}
    ai_prompt TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
