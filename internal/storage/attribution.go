package storage

import (
	"context"
	"fmt"
	"time"
)

// AttributionSource represents a single traffic source with engagement metrics.
type AttributionSource struct {
	Source   string  `json:"source"`
	Channel string  `json:"channel"`
	Sessions int64  `json:"sessions"`
	Users    int64  `json:"users"`
	Bounced  int64  `json:"bounced"`
	AvgPages float64 `json:"avg_pages"`
}

// ChannelSummary aggregates attribution data by channel.
type ChannelSummary struct {
	Channel  string `json:"channel"`
	Sessions int64  `json:"sessions"`
	Users    int64  `json:"users"`
	Bounced  int64  `json:"bounced"`
}

const attributionCTE = `
WITH first_pageviews AS (
    SELECT
        session_id,
        distinct_id,
        referrer,
        CAST(properties AS VARCHAR) AS props_str,
        ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY timestamp) AS rn
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
),
entry AS (
    SELECT session_id, distinct_id, referrer, props_str
    FROM first_pageviews WHERE rn = 1
),
classified AS (
    SELECT
        session_id,
        distinct_id,
        CASE
            WHEN json_extract_string(props_str, '$.ref') IS NOT NULL AND json_extract_string(props_str, '$.ref') != ''
                THEN json_extract_string(props_str, '$.ref')
            WHEN json_extract_string(props_str, '$.utm_source') IS NOT NULL AND json_extract_string(props_str, '$.utm_source') != ''
                THEN json_extract_string(props_str, '$.utm_source') || COALESCE(' / ' || NULLIF(json_extract_string(props_str, '$.utm_medium'), ''), '')
            WHEN referrer IS NOT NULL AND referrer != '' THEN referrer
            ELSE '(direct)'
        END AS source,
        CASE
            WHEN json_extract_string(props_str, '$.ref') IS NOT NULL AND json_extract_string(props_str, '$.ref') != ''
                THEN 'Ref Code'
            WHEN json_extract_string(props_str, '$.utm_campaign') IS NOT NULL AND json_extract_string(props_str, '$.utm_campaign') != ''
                THEN 'UTM Campaign'
            WHEN json_extract_string(props_str, '$.utm_source') IS NOT NULL AND json_extract_string(props_str, '$.utm_source') != ''
                THEN 'UTM Campaign'
            WHEN referrer IS NULL OR referrer = '' THEN 'Direct'
            WHEN referrer LIKE '%google.%' OR referrer LIKE '%bing.%' OR referrer LIKE '%duckduckgo.%'
                 OR referrer LIKE '%yahoo.%' OR referrer LIKE '%baidu.%' OR referrer LIKE '%yandex.%'
                THEN 'Organic Search'
            WHEN referrer LIKE '%twitter.%' OR referrer LIKE '%x.com%' OR referrer LIKE '%facebook.%'
                 OR referrer LIKE '%reddit.%' OR referrer LIKE '%linkedin.%' OR referrer LIKE '%youtube.%'
                 OR referrer LIKE '%instagram.%' OR referrer LIKE '%tiktok.%' OR referrer LIKE '%t.co%'
                THEN 'Social'
            ELSE 'Referral'
        END AS channel
    FROM entry
),
session_pages AS (
    SELECT session_id, COUNT(*) AS page_count
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
    GROUP BY session_id
)
`

// QueryAttribution returns per-source attribution with engagement metrics.
func (d *DuckDB) QueryAttribution(ctx context.Context, projectID string, start, end time.Time, limit int) ([]AttributionSource, error) {
	if limit <= 0 {
		limit = 50
	}

	query := attributionCTE + `
SELECT
    c.source,
    c.channel,
    COUNT(DISTINCT c.session_id) AS sessions,
    COUNT(DISTINCT c.distinct_id) AS users,
    COUNT(DISTINCT CASE WHEN sp.page_count = 1 THEN c.session_id END) AS bounced,
    AVG(sp.page_count) AS avg_pages
FROM classified c
LEFT JOIN session_pages sp ON c.session_id = sp.session_id
GROUP BY c.source, c.channel
ORDER BY sessions DESC
LIMIT ?
`
	rows, err := d.db.QueryContext(ctx, query, projectID, start, end, projectID, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("querying attribution: %w", err)
	}
	defer rows.Close()

	var sources []AttributionSource
	for rows.Next() {
		var s AttributionSource
		if err := rows.Scan(&s.Source, &s.Channel, &s.Sessions, &s.Users, &s.Bounced, &s.AvgPages); err != nil {
			return nil, fmt.Errorf("scanning attribution source: %w", err)
		}
		sources = append(sources, s)
	}
	return sources, rows.Err()
}

// QueryAttributionOverview returns attribution grouped by channel only (for stat cards).
func (d *DuckDB) QueryAttributionOverview(ctx context.Context, projectID string, start, end time.Time) ([]ChannelSummary, error) {
	query := attributionCTE + `
SELECT
    c.channel,
    COUNT(DISTINCT c.session_id) AS sessions,
    COUNT(DISTINCT c.distinct_id) AS users,
    COUNT(DISTINCT CASE WHEN sp.page_count = 1 THEN c.session_id END) AS bounced
FROM classified c
LEFT JOIN session_pages sp ON c.session_id = sp.session_id
GROUP BY c.channel
ORDER BY sessions DESC
`
	rows, err := d.db.QueryContext(ctx, query, projectID, start, end, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying attribution overview: %w", err)
	}
	defer rows.Close()

	var channels []ChannelSummary
	for rows.Next() {
		var ch ChannelSummary
		if err := rows.Scan(&ch.Channel, &ch.Sessions, &ch.Users, &ch.Bounced); err != nil {
			return nil, fmt.Errorf("scanning channel summary: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}
