CREATE TABLE IF NOT EXISTS events (
    id              VARCHAR DEFAULT gen_random_uuid(),
    project_id      VARCHAR NOT NULL,
    session_id      VARCHAR NOT NULL,
    distinct_id     VARCHAR,
    event_type      VARCHAR NOT NULL,
    fingerprint     VARCHAR NOT NULL,
    event_name      VARCHAR,

    -- DOM context
    element_tag     VARCHAR,
    element_id      VARCHAR,
    element_classes VARCHAR,
    element_text    VARCHAR,
    aria_label      VARCHAR,
    data_attributes JSON,
    parent_path     VARCHAR,

    -- Page context
    url             VARCHAR NOT NULL,
    url_path        VARCHAR NOT NULL,
    page_title      VARCHAR,
    referrer        VARCHAR,

    -- Device/browser
    screen_width    INTEGER,
    screen_height   INTEGER,
    user_agent      VARCHAR,

    -- Timing
    timestamp       TIMESTAMPTZ NOT NULL,
    received_at     TIMESTAMPTZ DEFAULT now(),

    -- Custom properties
    properties      JSON
);

CREATE INDEX IF NOT EXISTS idx_events_project_ts ON events (project_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_project_fp ON events (project_id, fingerprint);
CREATE INDEX IF NOT EXISTS idx_events_project_name ON events (project_id, event_name);
CREATE INDEX IF NOT EXISTS idx_events_session ON events (project_id, session_id);
