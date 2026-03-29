package storage

import (
	"context"
	"fmt"
	"time"
)

// LeadAttribution represents a single traffic source contributing to a lead's sessions.
type LeadAttribution struct {
	Source     string    `json:"source"`
	Channel    string    `json:"channel"`
	Campaign   string    `json:"campaign,omitempty"`
	Sessions   int       `json:"sessions"`
	FirstTouch time.Time `json:"first_touch"`
	LastTouch  time.Time `json:"last_touch"`
}

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

// CampaignStats holds aggregated performance metrics for a campaign ref code.
type CampaignStats struct {
	Sessions   int64   `json:"sessions"`
	Users      int64   `json:"users"`
	Bounced    int64   `json:"bounced"`
	AvgPages   float64 `json:"avg_pages"`
	EventCount int64   `json:"event_count"`
}

// CampaignDailyStats holds per-day metrics for a campaign.
type CampaignDailyStats struct {
	Date     string `json:"date"`
	Sessions int64  `json:"sessions"`
	Users    int64  `json:"users"`
}

// QueryCampaignStats returns attribution metrics for a specific ref code.
func (d *DuckDB) QueryCampaignStats(ctx context.Context, projectID, refCode string, start, end time.Time) (*CampaignStats, error) {
	query := `
WITH ref_sessions AS (
    SELECT DISTINCT session_id
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
        AND CAST(properties AS VARCHAR) LIKE '%"ref":"` + refCode + `"%'
),
session_pages AS (
    SELECT e.session_id, COUNT(*) AS page_count
    FROM events e
    INNER JOIN ref_sessions rs ON e.session_id = rs.session_id
    WHERE e.project_id = ? AND e.event_type = 'pageview'
        AND e.timestamp BETWEEN ? AND ?
    GROUP BY e.session_id
)
SELECT
    COUNT(DISTINCT rs.session_id) AS sessions,
    (SELECT COUNT(DISTINCT e.distinct_id) FROM events e INNER JOIN ref_sessions rs2 ON e.session_id = rs2.session_id WHERE e.project_id = ? AND e.timestamp BETWEEN ? AND ?) AS users,
    COUNT(DISTINCT CASE WHEN sp.page_count = 1 THEN rs.session_id END) AS bounced,
    COALESCE(AVG(sp.page_count), 0) AS avg_pages,
    (SELECT COUNT(*) FROM events e INNER JOIN ref_sessions rs3 ON e.session_id = rs3.session_id WHERE e.project_id = ? AND e.timestamp BETWEEN ? AND ?) AS event_count
FROM ref_sessions rs
LEFT JOIN session_pages sp ON rs.session_id = sp.session_id
`
	var s CampaignStats
	err := d.db.QueryRowContext(ctx, query,
		projectID, start, end,
		projectID, start, end,
		projectID, start, end,
		projectID, start, end,
	).Scan(&s.Sessions, &s.Users, &s.Bounced, &s.AvgPages, &s.EventCount)
	if err != nil {
		return nil, fmt.Errorf("querying campaign stats: %w", err)
	}
	return &s, nil
}

// QueryRefCodeStatsBatch returns session metrics for all ref codes in one DuckDB pass.
// The result map is keyed by ref code string (the value, not the ID).
func (d *DuckDB) QueryRefCodeStatsBatch(ctx context.Context, projectID string, start, end time.Time) (map[string]CampaignStats, error) {
	query := `
WITH ref_tagged AS (
    SELECT
        session_id,
        distinct_id,
        json_extract_string(CAST(properties AS VARCHAR), '$.ref') AS ref_code
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
        AND json_extract_string(CAST(properties AS VARCHAR), '$.ref') IS NOT NULL
        AND json_extract_string(CAST(properties AS VARCHAR), '$.ref') != ''
),
ref_sessions AS (
    SELECT session_id, MIN(ref_code) AS ref_code, MIN(distinct_id) AS distinct_id
    FROM ref_tagged
    GROUP BY session_id
),
session_pages AS (
    SELECT e.session_id, COUNT(*) AS page_count
    FROM events e
    INNER JOIN ref_sessions rs ON e.session_id = rs.session_id
    WHERE e.project_id = ? AND e.event_type = 'pageview'
        AND e.timestamp BETWEEN ? AND ?
    GROUP BY e.session_id
)
SELECT
    rs.ref_code,
    COUNT(DISTINCT rs.session_id) AS sessions,
    COUNT(DISTINCT rs.distinct_id) AS users,
    COUNT(DISTINCT CASE WHEN sp.page_count = 1 THEN rs.session_id END) AS bounced,
    COALESCE(AVG(sp.page_count), 0) AS avg_pages
FROM ref_sessions rs
LEFT JOIN session_pages sp ON rs.session_id = sp.session_id
GROUP BY rs.ref_code
`
	rows, err := d.db.QueryContext(ctx, query, projectID, start, end, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying ref code stats batch: %w", err)
	}
	defer rows.Close()

	result := make(map[string]CampaignStats)
	for rows.Next() {
		var refCode string
		var s CampaignStats
		if err := rows.Scan(&refCode, &s.Sessions, &s.Users, &s.Bounced, &s.AvgPages); err != nil {
			return nil, fmt.Errorf("scanning ref code stats: %w", err)
		}
		result[refCode] = s
	}
	return result, rows.Err()
}

// CampaignChannelBreakdown holds session counts per channel for a campaign.
type CampaignChannelBreakdown struct {
	Channel  string `json:"channel"`
	Sessions int64  `json:"sessions"`
	Users    int64  `json:"users"`
}

// QueryCampaignChannelBreakdown returns per-channel traffic breakdown for a specific ref code.
func (d *DuckDB) QueryCampaignChannelBreakdown(ctx context.Context, projectID, refCode string, start, end time.Time) ([]CampaignChannelBreakdown, error) {
	query := `
WITH ref_sessions AS (
    SELECT DISTINCT session_id, distinct_id
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
        AND CAST(properties AS VARCHAR) LIKE '%"ref":"` + refCode + `"%'
),
first_entry AS (
    SELECT e.session_id, CAST(e.properties AS VARCHAR) AS props_str, e.referrer,
        ROW_NUMBER() OVER (PARTITION BY e.session_id ORDER BY e.timestamp) AS rn
    FROM events e
    INNER JOIN ref_sessions rs ON e.session_id = rs.session_id
    WHERE e.project_id = ? AND e.event_type = 'pageview'
        AND e.timestamp BETWEEN ? AND ?
),
classified AS (
    SELECT
        session_id,
        CASE
            WHEN json_extract_string(props_str, '$.utm_medium') IS NOT NULL
                 AND json_extract_string(props_str, '$.utm_medium') != ''
                THEN json_extract_string(props_str, '$.utm_medium')
            WHEN referrer IS NULL OR referrer = '' THEN 'direct'
            WHEN referrer LIKE '%google.%' OR referrer LIKE '%bing.%'
                 OR referrer LIKE '%duckduckgo.%' OR referrer LIKE '%yahoo.%'
                THEN 'organic'
            WHEN referrer LIKE '%twitter.%' OR referrer LIKE '%x.com%'
                 OR referrer LIKE '%reddit.%' OR referrer LIKE '%linkedin.%'
                 OR referrer LIKE '%facebook.%' OR referrer LIKE '%instagram.%'
                THEN 'social'
            ELSE 'referral'
        END AS channel
    FROM first_entry WHERE rn = 1
)
SELECT
    c.channel,
    COUNT(DISTINCT c.session_id) AS sessions,
    COUNT(DISTINCT rs.distinct_id) AS users
FROM classified c
LEFT JOIN ref_sessions rs ON c.session_id = rs.session_id
GROUP BY c.channel
ORDER BY sessions DESC
`
	rows, err := d.db.QueryContext(ctx, query, projectID, start, end, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying campaign channel breakdown: %w", err)
	}
	defer rows.Close()

	var result []CampaignChannelBreakdown
	for rows.Next() {
		var b CampaignChannelBreakdown
		if err := rows.Scan(&b.Channel, &b.Sessions, &b.Users); err != nil {
			return nil, fmt.Errorf("scanning channel breakdown: %w", err)
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// QueryCampaignConversions counts sessions (via a ref code) that also had a specific event type.
func (d *DuckDB) QueryCampaignConversions(ctx context.Context, projectID, refCode, conversionEvent string, start, end time.Time) (int64, error) {
	query := `
WITH ref_sessions AS (
    SELECT DISTINCT session_id
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
        AND CAST(properties AS VARCHAR) LIKE '%"ref":"` + refCode + `"%'
)
SELECT COUNT(DISTINCT e.session_id)
FROM events e
INNER JOIN ref_sessions rs ON e.session_id = rs.session_id
WHERE e.project_id = ? AND e.event_type = ?
    AND e.timestamp BETWEEN ? AND ?
`
	var count int64
	err := d.db.QueryRowContext(ctx, query,
		projectID, start, end,
		projectID, conversionEvent, start, end,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("querying campaign conversions: %w", err)
	}
	return count, nil
}

// QueryCampaignTimeSeries returns daily session/user counts for a specific ref code.
func (d *DuckDB) QueryCampaignTimeSeries(ctx context.Context, projectID, refCode string, start, end time.Time) ([]CampaignDailyStats, error) {
	query := `
WITH ref_sessions AS (
    SELECT DISTINCT session_id, CAST(timestamp AS DATE) AS day
    FROM events
    WHERE project_id = ? AND event_type = 'pageview'
        AND timestamp BETWEEN ? AND ?
        AND CAST(properties AS VARCHAR) LIKE '%"ref":"` + refCode + `"%'
)
SELECT
    CAST(day AS VARCHAR) AS date,
    COUNT(DISTINCT session_id) AS sessions,
    0 AS users
FROM ref_sessions
GROUP BY day
ORDER BY day
`
	rows, err := d.db.QueryContext(ctx, query, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying campaign time series: %w", err)
	}
	defer rows.Close()

	var result []CampaignDailyStats
	for rows.Next() {
		var s CampaignDailyStats
		if err := rows.Scan(&s.Date, &s.Sessions, &s.Users); err != nil {
			return nil, fmt.Errorf("scanning campaign daily stats: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// ConversionAttribution represents a single traffic source with conversion and revenue data.
type ConversionAttribution struct {
	Source      string  `json:"source"`
	Channel     string  `json:"channel"`
	Campaign    string  `json:"campaign,omitempty"`
	Conversions int64   `json:"conversions"`
	Revenue     float64 `json:"revenue"`
	Users       int64   `json:"users"`
}

// RevenueOverview contains aggregated conversion and revenue metrics.
type RevenueOverview struct {
	TotalConversions int64                  `json:"total_conversions"`
	TotalRevenue     float64                `json:"total_revenue"`
	ByChannel        []ConversionAttribution `json:"by_channel"`
}

// GoalCriteria defines how to match conversion events for a goal.
type GoalCriteria struct {
	EventType     string
	EventName     string
	URLPattern    string
	ValueProperty string // JSON property key for monetary value (e.g. "$value")
}

// QueryConversionsByGoal returns per-source conversion and revenue attribution for a conversion goal.
// The model parameter selects the attribution model: "first_touch", "last_touch", or "linear".
func (d *DuckDB) QueryConversionsByGoal(ctx context.Context, projectID string, goal GoalCriteria, model string, start, end time.Time) ([]ConversionAttribution, error) {
	if goal.ValueProperty == "" {
		goal.ValueProperty = "$value"
	}

	// Build conversion event filter.
	convFilter := "e2.project_id = ? AND e2.timestamp BETWEEN ? AND ?"
	convArgs := []any{projectID, start, end}
	if goal.EventType != "" {
		convFilter += " AND e2.event_type = ?"
		convArgs = append(convArgs, goal.EventType)
	}
	if goal.EventName != "" {
		convFilter += " AND e2.event_name = ?"
		convArgs = append(convArgs, goal.EventName)
	}
	if goal.URLPattern != "" {
		convFilter += " AND e2.url_path LIKE ?"
		convArgs = append(convArgs, goal.URLPattern)
	}

	// Determine the touch attribution ordering.
	touchOrder := "MIN(fp.timestamp)" // first_touch
	if model == "last_touch" {
		touchOrder = "MAX(fp.timestamp)"
	}

	if model == "linear" {
		return d.queryLinearAttribution(ctx, projectID, goal, convFilter, convArgs, start, end)
	}

	// First-touch or last-touch query.
	query := `
WITH conversions AS (
    SELECT
        e2.distinct_id,
        COUNT(*) AS conv_count,
        COALESCE(SUM(TRY_CAST(json_extract_string(CAST(e2.properties AS VARCHAR), '$.' || ?) AS DOUBLE)), 0) AS revenue
    FROM events e2
    WHERE ` + convFilter + `
        AND e2.distinct_id IS NOT NULL AND e2.distinct_id != ''
    GROUP BY e2.distinct_id
),
user_sessions AS (
    SELECT
        fp.distinct_id,
        fp.session_id,
        fp.referrer,
        CAST(fp.properties AS VARCHAR) AS props_str,
        ROW_NUMBER() OVER (PARTITION BY fp.distinct_id ORDER BY ` + touchOrder + `) AS rn
    FROM events fp
    WHERE fp.project_id = ? AND fp.event_type = 'pageview'
        AND fp.timestamp BETWEEN ? AND ?
        AND fp.distinct_id IS NOT NULL AND fp.distinct_id != ''
    GROUP BY fp.distinct_id, fp.session_id, fp.referrer, CAST(fp.properties AS VARCHAR)
),
attributed AS (
    SELECT
        us.distinct_id,
        CASE
            WHEN json_extract_string(us.props_str, '$.ref') IS NOT NULL AND json_extract_string(us.props_str, '$.ref') != ''
                THEN json_extract_string(us.props_str, '$.ref')
            WHEN json_extract_string(us.props_str, '$.utm_source') IS NOT NULL AND json_extract_string(us.props_str, '$.utm_source') != ''
                THEN json_extract_string(us.props_str, '$.utm_source')
            WHEN us.referrer IS NOT NULL AND us.referrer != '' THEN us.referrer
            ELSE '(direct)'
        END AS source,
        CASE
            WHEN json_extract_string(us.props_str, '$.ref') IS NOT NULL AND json_extract_string(us.props_str, '$.ref') != '' THEN 'Ref Code'
            WHEN json_extract_string(us.props_str, '$.utm_source') IS NOT NULL AND json_extract_string(us.props_str, '$.utm_source') != '' THEN 'UTM Campaign'
            WHEN us.referrer IS NULL OR us.referrer = '' THEN 'Direct'
            WHEN us.referrer LIKE '%google.%' OR us.referrer LIKE '%bing.%' OR us.referrer LIKE '%duckduckgo.%'
                 OR us.referrer LIKE '%yahoo.%' THEN 'Organic Search'
            WHEN us.referrer LIKE '%twitter.%' OR us.referrer LIKE '%x.com%' OR us.referrer LIKE '%facebook.%'
                 OR us.referrer LIKE '%reddit.%' OR us.referrer LIKE '%linkedin.%' THEN 'Social'
            ELSE 'Referral'
        END AS channel,
        COALESCE(json_extract_string(us.props_str, '$.utm_campaign'), '') AS campaign
    FROM user_sessions us
    WHERE us.rn = 1
)
SELECT
    a.source,
    a.channel,
    a.campaign,
    SUM(c.conv_count) AS conversions,
    SUM(c.revenue) AS revenue,
    COUNT(DISTINCT a.distinct_id) AS users
FROM attributed a
INNER JOIN conversions c ON a.distinct_id = c.distinct_id
GROUP BY a.source, a.channel, a.campaign
ORDER BY conversions DESC
LIMIT 50
`
	allArgs := []any{goal.ValueProperty}
	allArgs = append(allArgs, convArgs...)
	allArgs = append(allArgs, projectID, start, end)

	rows, err := d.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying conversions by goal: %w", err)
	}
	defer rows.Close()

	var results []ConversionAttribution
	for rows.Next() {
		var ca ConversionAttribution
		if err := rows.Scan(&ca.Source, &ca.Channel, &ca.Campaign, &ca.Conversions, &ca.Revenue, &ca.Users); err != nil {
			return nil, fmt.Errorf("scanning conversion attribution: %w", err)
		}
		results = append(results, ca)
	}
	return results, rows.Err()
}

// queryLinearAttribution distributes each user's conversions evenly across all their sessions' sources.
func (d *DuckDB) queryLinearAttribution(ctx context.Context, projectID string, goal GoalCriteria, convFilter string, convArgs []any, start, end time.Time) ([]ConversionAttribution, error) {
	query := `
WITH conversions AS (
    SELECT
        e2.distinct_id,
        COUNT(*) AS conv_count,
        COALESCE(SUM(TRY_CAST(json_extract_string(CAST(e2.properties AS VARCHAR), '$.' || ?) AS DOUBLE)), 0) AS revenue
    FROM events e2
    WHERE ` + convFilter + `
        AND e2.distinct_id IS NOT NULL AND e2.distinct_id != ''
    GROUP BY e2.distinct_id
),
session_sources AS (
    SELECT
        fp.distinct_id,
        fp.session_id,
        CASE
            WHEN json_extract_string(CAST(fp.properties AS VARCHAR), '$.ref') IS NOT NULL AND json_extract_string(CAST(fp.properties AS VARCHAR), '$.ref') != ''
                THEN json_extract_string(CAST(fp.properties AS VARCHAR), '$.ref')
            WHEN json_extract_string(CAST(fp.properties AS VARCHAR), '$.utm_source') IS NOT NULL AND json_extract_string(CAST(fp.properties AS VARCHAR), '$.utm_source') != ''
                THEN json_extract_string(CAST(fp.properties AS VARCHAR), '$.utm_source')
            WHEN fp.referrer IS NOT NULL AND fp.referrer != '' THEN fp.referrer
            ELSE '(direct)'
        END AS source,
        CASE
            WHEN json_extract_string(CAST(fp.properties AS VARCHAR), '$.ref') IS NOT NULL AND json_extract_string(CAST(fp.properties AS VARCHAR), '$.ref') != '' THEN 'Ref Code'
            WHEN json_extract_string(CAST(fp.properties AS VARCHAR), '$.utm_source') IS NOT NULL AND json_extract_string(CAST(fp.properties AS VARCHAR), '$.utm_source') != '' THEN 'UTM Campaign'
            WHEN fp.referrer IS NULL OR fp.referrer = '' THEN 'Direct'
            WHEN fp.referrer LIKE '%google.%' OR fp.referrer LIKE '%bing.%' OR fp.referrer LIKE '%duckduckgo.%'
                 OR fp.referrer LIKE '%yahoo.%' THEN 'Organic Search'
            WHEN fp.referrer LIKE '%twitter.%' OR fp.referrer LIKE '%x.com%' OR fp.referrer LIKE '%facebook.%'
                 OR fp.referrer LIKE '%reddit.%' OR fp.referrer LIKE '%linkedin.%' THEN 'Social'
            ELSE 'Referral'
        END AS channel,
        COALESCE(json_extract_string(CAST(fp.properties AS VARCHAR), '$.utm_campaign'), '') AS campaign,
        ROW_NUMBER() OVER (PARTITION BY fp.distinct_id, fp.session_id ORDER BY fp.timestamp) AS rn
    FROM events fp
    WHERE fp.project_id = ? AND fp.event_type = 'pageview'
        AND fp.timestamp BETWEEN ? AND ?
        AND fp.distinct_id IS NOT NULL AND fp.distinct_id != ''
),
unique_sessions AS (
    SELECT distinct_id, source, channel, campaign
    FROM session_sources WHERE rn = 1
),
session_counts AS (
    SELECT distinct_id, COUNT(*) AS total_sessions
    FROM unique_sessions GROUP BY distinct_id
),
linear_credit AS (
    SELECT
        us.source,
        us.channel,
        us.campaign,
        us.distinct_id,
        1.0 / sc.total_sessions AS credit
    FROM unique_sessions us
    INNER JOIN session_counts sc ON us.distinct_id = sc.distinct_id
)
SELECT
    lc.source,
    lc.channel,
    lc.campaign,
    CAST(ROUND(SUM(lc.credit * c.conv_count)) AS BIGINT) AS conversions,
    SUM(lc.credit * c.revenue) AS revenue,
    COUNT(DISTINCT lc.distinct_id) AS users
FROM linear_credit lc
INNER JOIN conversions c ON lc.distinct_id = c.distinct_id
GROUP BY lc.source, lc.channel, lc.campaign
ORDER BY conversions DESC
LIMIT 50
`
	allArgs := []any{goal.ValueProperty}
	allArgs = append(allArgs, convArgs...)
	allArgs = append(allArgs, projectID, start, end)

	rows, err := d.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying linear attribution: %w", err)
	}
	defer rows.Close()

	var results []ConversionAttribution
	for rows.Next() {
		var ca ConversionAttribution
		if err := rows.Scan(&ca.Source, &ca.Channel, &ca.Campaign, &ca.Conversions, &ca.Revenue, &ca.Users); err != nil {
			return nil, fmt.Errorf("scanning linear attribution: %w", err)
		}
		results = append(results, ca)
	}
	return results, rows.Err()
}

// QueryRevenueOverview returns total conversions, revenue, and per-channel breakdown for a goal.
func (d *DuckDB) QueryRevenueOverview(ctx context.Context, projectID string, goal GoalCriteria, start, end time.Time) (*RevenueOverview, error) {
	if goal.ValueProperty == "" {
		goal.ValueProperty = "$value"
	}

	convFilter := "e.project_id = ? AND e.timestamp BETWEEN ? AND ?"
	convArgs := []any{projectID, start, end}
	if goal.EventType != "" {
		convFilter += " AND e.event_type = ?"
		convArgs = append(convArgs, goal.EventType)
	}
	if goal.EventName != "" {
		convFilter += " AND e.event_name = ?"
		convArgs = append(convArgs, goal.EventName)
	}
	if goal.URLPattern != "" {
		convFilter += " AND e.url_path LIKE ?"
		convArgs = append(convArgs, goal.URLPattern)
	}

	// Total conversions and revenue.
	totalQuery := `SELECT COUNT(*), COALESCE(SUM(TRY_CAST(json_extract_string(CAST(e.properties AS VARCHAR), '$.' || ?) AS DOUBLE)), 0)
		FROM events e WHERE ` + convFilter
	totalArgs := append([]any{goal.ValueProperty}, convArgs...)

	overview := &RevenueOverview{}
	if err := d.db.QueryRowContext(ctx, totalQuery, totalArgs...).Scan(&overview.TotalConversions, &overview.TotalRevenue); err != nil {
		return nil, fmt.Errorf("querying revenue totals: %w", err)
	}

	// Per-channel breakdown using first-touch attribution.
	byChannel, err := d.QueryConversionsByGoal(ctx, projectID, goal, "first_touch", start, end)
	if err != nil {
		return nil, err
	}

	// Aggregate by channel only.
	chanMap := make(map[string]*ConversionAttribution)
	for _, ca := range byChannel {
		if existing, ok := chanMap[ca.Channel]; ok {
			existing.Conversions += ca.Conversions
			existing.Revenue += ca.Revenue
			existing.Users += ca.Users
		} else {
			copy := ca
			copy.Source = ""
			copy.Campaign = ""
			chanMap[ca.Channel] = &copy
		}
	}
	for _, ca := range chanMap {
		overview.ByChannel = append(overview.ByChannel, *ca)
	}

	return overview, nil
}

// QueryLeadAttribution returns the traffic source breakdown for a specific user,
// showing which sources drove their sessions and when they first/last appeared.
func (d *DuckDB) QueryLeadAttribution(ctx context.Context, projectID, distinctID string, days int) ([]LeadAttribution, error) {
	if days <= 0 {
		days = 90
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	const q = `
WITH session_entry AS (
    SELECT
        session_id,
        arg_min(CAST(properties AS VARCHAR), timestamp) AS props_str,
        arg_min(referrer, timestamp) AS referrer,
        MIN(timestamp) AS first_touch,
        MAX(timestamp) AS last_touch
    FROM events
    WHERE project_id = ? AND distinct_id = ? AND timestamp >= ?
    GROUP BY session_id
),
classified AS (
    SELECT
        session_id,
        first_touch,
        last_touch,
        CASE
            WHEN json_extract_string(props_str, '$.ref') IS NOT NULL
                 AND json_extract_string(props_str, '$.ref') != ''
                THEN json_extract_string(props_str, '$.ref')
            WHEN json_extract_string(props_str, '$.utm_source') IS NOT NULL
                 AND json_extract_string(props_str, '$.utm_source') != ''
                THEN json_extract_string(props_str, '$.utm_source')
            WHEN referrer IS NOT NULL AND referrer != ''
                THEN split_part(split_part(referrer, '://', 2), '/', 1)
            ELSE '(direct)'
        END AS source,
        CASE
            WHEN json_extract_string(props_str, '$.ref') IS NOT NULL
                 AND json_extract_string(props_str, '$.ref') != ''
                THEN 'Ref Code'
            WHEN json_extract_string(props_str, '$.utm_medium') IN ('cpc','ppc','paid','paidsearch')
                THEN 'Paid'
            WHEN json_extract_string(props_str, '$.utm_medium') = 'email'
                THEN 'Email'
            WHEN json_extract_string(props_str, '$.utm_medium') IN ('social','socialmedia')
                THEN 'Social'
            WHEN json_extract_string(props_str, '$.utm_source') IS NOT NULL
                 AND json_extract_string(props_str, '$.utm_source') != ''
                THEN 'UTM'
            WHEN referrer IS NOT NULL AND referrer != ''
                THEN 'Referral'
            ELSE 'Direct'
        END AS channel,
        COALESCE(json_extract_string(props_str, '$.utm_campaign'), '') AS campaign
    FROM session_entry
)
SELECT
    source,
    channel,
    campaign,
    COUNT(*) AS sessions,
    MIN(first_touch) AS first_touch,
    MAX(last_touch) AS last_touch
FROM classified
GROUP BY source, channel, campaign
ORDER BY sessions DESC
LIMIT 20`

	rows, err := d.db.QueryContext(ctx, q, projectID, distinctID, since)
	if err != nil {
		return nil, fmt.Errorf("query lead attribution: %w", err)
	}
	defer rows.Close()

	var out []LeadAttribution
	for rows.Next() {
		var a LeadAttribution
		if err := rows.Scan(&a.Source, &a.Channel, &a.Campaign, &a.Sessions, &a.FirstTouch, &a.LastTouch); err != nil {
			return nil, fmt.Errorf("scan lead attribution: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
