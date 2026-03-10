CREATE TABLE IF NOT EXISTS campaign_posts (
    id TEXT PRIMARY KEY,
    campaign_id TEXT NOT NULL REFERENCES campaigns(id),
    project_id TEXT NOT NULL REFERENCES projects(id),
    connector_name TEXT NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    external_url TEXT NOT NULL DEFAULT '',
    posted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_engagement TEXT NOT NULL DEFAULT '{}', -- JSON: {views, likes, comments, shares, clicks}
    last_fetched_at DATETIME
);
