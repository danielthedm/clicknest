package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

type Event struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"project_id"`
	SessionID     string            `json:"session_id"`
	DistinctID    string            `json:"distinct_id,omitempty"`
	EventType     string            `json:"event_type"`
	Fingerprint   string            `json:"fingerprint"`
	EventName     *string           `json:"event_name,omitempty"`
	ElementTag    string            `json:"element_tag,omitempty"`
	ElementID     string            `json:"element_id,omitempty"`
	ElementClasses string           `json:"element_classes,omitempty"`
	ElementText   string            `json:"element_text,omitempty"`
	AriaLabel     string            `json:"aria_label,omitempty"`
	DataAttributes map[string]string `json:"data_attributes,omitempty"`
	ParentPath    string            `json:"parent_path,omitempty"`
	URL           string            `json:"url"`
	URLPath       string            `json:"url_path"`
	PageTitle     string            `json:"page_title,omitempty"`
	Referrer      string            `json:"referrer,omitempty"`
	ScreenWidth   int               `json:"screen_width,omitempty"`
	ScreenHeight  int               `json:"screen_height,omitempty"`
	UserAgent     string            `json:"user_agent,omitempty"`
	Timestamp     time.Time         `json:"timestamp"`
	ReceivedAt    time.Time         `json:"received_at"`
	Properties    map[string]any    `json:"properties,omitempty"`
}

type EventFilter struct {
	ProjectID     string
	EventType     string
	EventName     string
	Fingerprint   string
	SessionID     string
	DistinctID    string
	PropertyKey   string
	PropertyValue string
	StartTime     time.Time
	EndTime       time.Time
	Limit         int
	Offset        int
}

type TrendPoint struct {
	Bucket string `json:"bucket"`
	Count  int64  `json:"count"`
}

type DuckDB struct {
	db *sql.DB
}

func NewDuckDB(path string) (*DuckDB, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("opening duckdb: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging duckdb: %w", err)
	}

	if err := RunMigrations(db, duckdbMigrations, "migrations/duckdb"); err != nil {
		return nil, fmt.Errorf("running duckdb migrations: %w", err)
	}

	return &DuckDB{db: db}, nil
}

func (d *DuckDB) InsertEvents(ctx context.Context, events []Event) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (
			project_id, session_id, distinct_id, event_type, fingerprint, event_name,
			element_tag, element_id, element_classes, element_text, aria_label,
			data_attributes, parent_path,
			url, url_path, page_title, referrer,
			screen_width, screen_height, user_agent,
			timestamp, received_at, properties
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()

	for _, e := range events {
		dataAttrs, _ := json.Marshal(e.DataAttributes)
		props, _ := json.Marshal(e.Properties)

		_, err := stmt.ExecContext(ctx,
			e.ProjectID, e.SessionID, e.DistinctID, e.EventType, e.Fingerprint, e.EventName,
			e.ElementTag, e.ElementID, e.ElementClasses, e.ElementText, e.AriaLabel,
			string(dataAttrs), e.ParentPath,
			e.URL, e.URLPath, e.PageTitle, e.Referrer,
			e.ScreenWidth, e.ScreenHeight, e.UserAgent,
			e.Timestamp, now, string(props),
		)
		if err != nil {
			return fmt.Errorf("inserting event: %w", err)
		}
	}

	return tx.Commit()
}

func (d *DuckDB) QueryEvents(ctx context.Context, f EventFilter) ([]Event, error) {
	query := `SELECT
		id, project_id, session_id, distinct_id, event_type, fingerprint, event_name,
		element_tag, element_id, element_classes, element_text, aria_label,
		CAST(data_attributes AS VARCHAR), parent_path,
		url, url_path, page_title, referrer,
		screen_width, screen_height, user_agent,
		timestamp, received_at, CAST(properties AS VARCHAR)
		FROM events WHERE project_id = ?`

	args := []any{f.ProjectID}

	if f.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, f.EventType)
	}
	if f.EventName != "" {
		query += " AND event_name = ?"
		args = append(args, f.EventName)
	}
	if f.Fingerprint != "" {
		query += " AND fingerprint = ?"
		args = append(args, f.Fingerprint)
	}
	if f.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, f.SessionID)
	}
	if f.DistinctID != "" {
		query += " AND distinct_id = ?"
		args = append(args, f.DistinctID)
	}
	if f.PropertyKey != "" && f.PropertyValue != "" {
		query += " AND json_extract_string(properties, '$.' || ?) = ?"
		args = append(args, f.PropertyKey, f.PropertyValue)
	}
	if !f.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, f.StartTime)
	}
	if !f.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, f.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if f.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, f.Offset)
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var eventName, distinctID sql.NullString
		var elementTag, elementID, elementClasses, elementText, ariaLabel sql.NullString
		var dataAttrsJSON, parentPath sql.NullString
		var pageTitle, referrer, userAgent sql.NullString
		var propsJSON sql.NullString
		var screenWidth, screenHeight sql.NullInt32

		err := rows.Scan(
			&e.ID, &e.ProjectID, &e.SessionID, &distinctID, &e.EventType, &e.Fingerprint, &eventName,
			&elementTag, &elementID, &elementClasses, &elementText, &ariaLabel,
			&dataAttrsJSON, &parentPath,
			&e.URL, &e.URLPath, &pageTitle, &referrer,
			&screenWidth, &screenHeight, &userAgent,
			&e.Timestamp, &e.ReceivedAt, &propsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning event row: %w", err)
		}

		if eventName.Valid {
			e.EventName = &eventName.String
		}
		if distinctID.Valid {
			e.DistinctID = distinctID.String
		}
		e.ElementTag = elementTag.String
		e.ElementID = elementID.String
		e.ElementClasses = elementClasses.String
		e.ElementText = elementText.String
		e.AriaLabel = ariaLabel.String
		e.ParentPath = parentPath.String
		e.PageTitle = pageTitle.String
		e.Referrer = referrer.String
		e.UserAgent = userAgent.String
		if screenWidth.Valid {
			e.ScreenWidth = int(screenWidth.Int32)
		}
		if screenHeight.Valid {
			e.ScreenHeight = int(screenHeight.Int32)
		}
		if dataAttrsJSON.Valid {
			if err := json.Unmarshal([]byte(dataAttrsJSON.String), &e.DataAttributes); err != nil {
				log.Printf("WARN scan event %s data_attributes: %v", e.ID, err)
			}
		}
		if propsJSON.Valid {
			if err := json.Unmarshal([]byte(propsJSON.String), &e.Properties); err != nil {
				log.Printf("WARN scan event %s properties: %v", e.ID, err)
			}
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

func (d *DuckDB) QueryTrends(ctx context.Context, projectID string, interval string, start, end time.Time) ([]TrendPoint, error) {
	bucket := "hour"
	switch interval {
	case "minute", "hour", "day", "week", "month":
		bucket = interval
	}

	query := fmt.Sprintf(`
		SELECT CAST(date_trunc('%s', CAST(timestamp AS TIMESTAMP)) AS VARCHAR) AS bucket, COUNT(*) AS count
		FROM events
		WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY bucket
		ORDER BY bucket
	`, bucket)

	rows, err := d.db.QueryContext(ctx, query, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying trends: %w", err)
	}
	defer rows.Close()

	var points []TrendPoint
	for rows.Next() {
		var p TrendPoint
		if err := rows.Scan(&p.Bucket, &p.Count); err != nil {
			return nil, fmt.Errorf("scanning trend row: %w", err)
		}
		points = append(points, p)
	}

	return points, rows.Err()
}

// UnnamedFingerprints returns one representative event per unnamed fingerprint (non-pageview).
func (d *DuckDB) UnnamedFingerprints(ctx context.Context, projectID string) ([]Event, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT fingerprint, element_tag, element_id, element_classes, element_text,
		       aria_label, parent_path, url, url_path, page_title
		FROM events
		WHERE project_id = ? AND event_type != 'pageview' AND (event_name IS NULL OR event_name = '')
		GROUP BY fingerprint, element_tag, element_id, element_classes, element_text,
		         aria_label, parent_path, url, url_path, page_title
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying unnamed fingerprints: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Fingerprint, &e.ElementTag, &e.ElementID, &e.ElementClasses,
			&e.ElementText, &e.AriaLabel, &e.ParentPath, &e.URL, &e.URLPath, &e.PageTitle); err != nil {
			return nil, fmt.Errorf("scanning unnamed event: %w", err)
		}
		e.ProjectID = projectID
		events = append(events, e)
	}
	return events, rows.Err()
}

type UserProfile struct {
	DistinctID string    `json:"distinct_id"`
	EventCount int       `json:"event_count"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

type FunnelStep struct {
	EventType string `json:"event_type"`
	EventName string `json:"event_name"`
}

type FunnelResult struct {
	Step  string `json:"step"`
	Count int64  `json:"count"`
}

type RetentionCohort struct {
	Cohort    string  `json:"cohort"`
	Size      int64   `json:"size"`
	Retention []int64 `json:"retention"`
}

// QueryPropertyKeys returns distinct top-level keys from the properties JSON column.
func (d *DuckDB) QueryPropertyKeys(ctx context.Context, projectID string) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT unnest(json_keys(properties)) AS key
		FROM events
		WHERE project_id = ? AND properties IS NOT NULL AND CAST(properties AS VARCHAR) != '{}'
		ORDER BY key
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying property keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scanning property key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (d *DuckDB) QueryPropertyValues(ctx context.Context, projectID, key string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT DISTINCT CAST(json_extract(properties, '$.' || ?) AS VARCHAR) AS val
		FROM events
		WHERE project_id = ? AND properties IS NOT NULL AND json_extract(properties, '$.' || ?) IS NOT NULL
		ORDER BY val
		LIMIT ?
	`, key, projectID, key, limit)
	if err != nil {
		return nil, fmt.Errorf("querying property values: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning property value: %w", err)
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (d *DuckDB) QueryUsers(ctx context.Context, projectID string, limit, offset int, start, end time.Time) ([]UserProfile, int, error) {
	if limit <= 0 {
		limit = 50
	}

	args := []any{projectID}
	where := "project_id = ? AND distinct_id IS NOT NULL AND distinct_id != ''"
	if !start.IsZero() {
		where += " AND timestamp >= ?"
		args = append(args, start)
	}
	if !end.IsZero() {
		where += " AND timestamp <= ?"
		args = append(args, end)
	}

	// Get total count.
	var total int
	err := d.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(DISTINCT distinct_id) FROM events WHERE %s", where,
	), args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	// Get paginated users.
	query := fmt.Sprintf(`
		SELECT distinct_id, COUNT(*) as event_count,
			MIN(timestamp) as first_seen, MAX(timestamp) as last_seen
		FROM events WHERE %s
		GROUP BY distinct_id ORDER BY last_seen DESC
		LIMIT ? OFFSET ?
	`, where)
	args = append(args, limit, offset)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying users: %w", err)
	}
	defer rows.Close()

	var users []UserProfile
	for rows.Next() {
		var u UserProfile
		if err := rows.Scan(&u.DistinctID, &u.EventCount, &u.FirstSeen, &u.LastSeen); err != nil {
			return nil, 0, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// QueryFunnel runs a session-based funnel analysis with ordered steps.
func (d *DuckDB) QueryFunnel(ctx context.Context, projectID string, steps []FunnelStep, start, end time.Time) ([]FunnelResult, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	args := []any{}

	// Build CTEs for each step.
	for i, step := range steps {
		if i == 0 {
			sb.WriteString("WITH ")
		} else {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("step%d AS (\n", i+1))
		if i == 0 {
			sb.WriteString("  SELECT DISTINCT session_id, MIN(timestamp) as ts FROM events WHERE project_id = ?")
			args = append(args, projectID)
		} else {
			sb.WriteString(fmt.Sprintf("  SELECT DISTINCT e.session_id, MIN(e.timestamp) as ts FROM events e JOIN step%d s ON e.session_id = s.session_id WHERE e.project_id = ?", i))
			args = append(args, projectID)
		}
		sb.WriteString(" AND event_type = ?")
		args = append(args, step.EventType)
		if step.EventName != "" {
			sb.WriteString(" AND event_name = ?")
			args = append(args, step.EventName)
		}
		if !start.IsZero() {
			sb.WriteString(" AND timestamp >= ?")
			args = append(args, start)
		}
		if !end.IsZero() {
			sb.WriteString(" AND timestamp <= ?")
			args = append(args, end)
		}
		if i > 0 {
			sb.WriteString(fmt.Sprintf(" AND e.timestamp > s.ts"))
		}
		sb.WriteString("\n  GROUP BY ")
		if i == 0 {
			sb.WriteString("session_id")
		} else {
			sb.WriteString("e.session_id")
		}
		sb.WriteString("\n)\n")
	}

	// Build SELECT union.
	for i, step := range steps {
		if i > 0 {
			sb.WriteString("UNION ALL\n")
		}
		label := step.EventName
		if label == "" {
			label = step.EventType
		}
		sb.WriteString(fmt.Sprintf("SELECT ? as step, COUNT(*) as count FROM step%d\n", i+1))
		args = append(args, fmt.Sprintf("Step %d: %s", i+1, label))
	}

	rows, err := d.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("querying funnel: %w", err)
	}
	defer rows.Close()

	var results []FunnelResult
	for rows.Next() {
		var r FunnelResult
		if err := rows.Scan(&r.Step, &r.Count); err != nil {
			return nil, fmt.Errorf("scanning funnel result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (d *DuckDB) QueryRetention(ctx context.Context, projectID, interval string, periods int, start, end time.Time) ([]RetentionCohort, error) {
	switch interval {
	case "day", "week", "month":
	default:
		interval = "week"
	}
	if periods <= 0 {
		periods = 8
	}

	// Build period columns dynamically.
	var periodCols strings.Builder
	for i := 0; i <= periods; i++ {
		periodCols.WriteString(fmt.Sprintf(
			",\n  COUNT(DISTINCT CASE WHEN ua.activity_period = uc.cohort + INTERVAL '%d %s' THEN uc.distinct_id END) as period_%d",
			i, interval, i,
		))
	}

	query := fmt.Sprintf(`
		WITH user_cohorts AS (
			SELECT distinct_id, date_trunc('%s', CAST(MIN(timestamp) AS TIMESTAMP)) as cohort
			FROM events WHERE project_id = ? AND distinct_id IS NOT NULL AND distinct_id != ''
				AND timestamp >= ? AND timestamp <= ?
			GROUP BY distinct_id
		),
		user_activity AS (
			SELECT DISTINCT e.distinct_id, date_trunc('%s', CAST(e.timestamp AS TIMESTAMP)) as activity_period
			FROM events e WHERE e.project_id = ? AND e.distinct_id IS NOT NULL AND e.distinct_id != ''
				AND e.timestamp >= ? AND e.timestamp <= ?
		)
		SELECT CAST(uc.cohort AS VARCHAR) as cohort, COUNT(DISTINCT uc.distinct_id) as cohort_size%s
		FROM user_cohorts uc
		LEFT JOIN user_activity ua ON uc.distinct_id = ua.distinct_id
		GROUP BY uc.cohort ORDER BY uc.cohort
	`, interval, interval, periodCols.String())

	rows, err := d.db.QueryContext(ctx, query, projectID, start, end, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying retention: %w", err)
	}
	defer rows.Close()

	var cohorts []RetentionCohort
	for rows.Next() {
		var c RetentionCohort
		c.Retention = make([]int64, periods+1)
		scanArgs := []any{&c.Cohort, &c.Size}
		for i := 0; i <= periods; i++ {
			scanArgs = append(scanArgs, &c.Retention[i])
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("scanning retention row: %w", err)
		}
		cohorts = append(cohorts, c)
	}
	return cohorts, rows.Err()
}

type FunnelCohortStep struct {
	Step  string `json:"step"`
	Count int64  `json:"count"`
}

type FunnelCohortResult struct {
	Cohort string             `json:"cohort"`
	Steps  []FunnelCohortStep `json:"steps"`
}

type EventSequence struct {
	Steps        []FunnelStep `json:"steps"`
	SessionCount int64        `json:"session_count"`
}

func (d *DuckDB) QueryFunnelCohorts(ctx context.Context, projectID string, steps []FunnelStep, interval string, start, end time.Time) ([]FunnelCohortResult, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	switch interval {
	case "day", "week", "month":
	default:
		interval = "week"
	}

	var sb strings.Builder
	args := []any{}

	// Cohorts CTE — first-seen date per session.
	sb.WriteString(fmt.Sprintf("WITH cohorts AS (\n  SELECT session_id, CAST(date_trunc('%s', CAST(MIN(timestamp) AS TIMESTAMP)) AS VARCHAR) as cohort\n  FROM events WHERE project_id = ?", interval))
	args = append(args, projectID)
	if !start.IsZero() {
		sb.WriteString(" AND timestamp >= ?")
		args = append(args, start)
	}
	if !end.IsZero() {
		sb.WriteString(" AND timestamp <= ?")
		args = append(args, end)
	}
	sb.WriteString("\n  GROUP BY session_id\n)\n")

	// Build step CTEs (same logic as QueryFunnel).
	for i, step := range steps {
		sb.WriteString(", ")
		sb.WriteString(fmt.Sprintf("step%d AS (\n", i+1))
		if i == 0 {
			sb.WriteString("  SELECT DISTINCT session_id, MIN(timestamp) as ts FROM events WHERE project_id = ?")
			args = append(args, projectID)
		} else {
			sb.WriteString(fmt.Sprintf("  SELECT DISTINCT e.session_id, MIN(e.timestamp) as ts FROM events e JOIN step%d s ON e.session_id = s.session_id WHERE e.project_id = ?", i))
			args = append(args, projectID)
		}
		sb.WriteString(" AND event_type = ?")
		args = append(args, step.EventType)
		if step.EventName != "" {
			sb.WriteString(" AND event_name = ?")
			args = append(args, step.EventName)
		}
		if !start.IsZero() {
			sb.WriteString(" AND timestamp >= ?")
			args = append(args, start)
		}
		if !end.IsZero() {
			sb.WriteString(" AND timestamp <= ?")
			args = append(args, end)
		}
		if i > 0 {
			sb.WriteString(" AND e.timestamp > s.ts")
		}
		sb.WriteString("\n  GROUP BY ")
		if i == 0 {
			sb.WriteString("session_id")
		} else {
			sb.WriteString("e.session_id")
		}
		sb.WriteString("\n)\n")
	}

	// Final SELECT: for each step, join with cohorts and group by cohort.
	for i, step := range steps {
		if i > 0 {
			sb.WriteString("UNION ALL\n")
		}
		label := step.EventName
		if label == "" {
			label = step.EventType
		}
		sb.WriteString(fmt.Sprintf("SELECT c.cohort, ? as step, COUNT(*) as count FROM step%d s JOIN cohorts c ON s.session_id = c.session_id GROUP BY c.cohort\n", i+1))
		args = append(args, fmt.Sprintf("Step %d: %s", i+1, label))
	}
	sb.WriteString("ORDER BY cohort, step")

	rows, err := d.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("querying funnel cohorts: %w", err)
	}
	defer rows.Close()

	// Group rows into cohort results.
	cohortMap := map[string]*FunnelCohortResult{}
	var order []string
	for rows.Next() {
		var cohort, step string
		var count int64
		if err := rows.Scan(&cohort, &step, &count); err != nil {
			return nil, fmt.Errorf("scanning funnel cohort row: %w", err)
		}
		cr, ok := cohortMap[cohort]
		if !ok {
			cr = &FunnelCohortResult{Cohort: cohort}
			cohortMap[cohort] = cr
			order = append(order, cohort)
		}
		cr.Steps = append(cr.Steps, FunnelCohortStep{Step: step, Count: count})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]FunnelCohortResult, 0, len(order))
	for _, k := range order {
		results = append(results, *cohortMap[k])
	}
	return results, nil
}

// QueryTopSequences finds the most common 2- and 3-step event sequences across sessions.
func (d *DuckDB) QueryTopSequences(ctx context.Context, projectID string, start, end time.Time, limit int) ([]EventSequence, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		WITH ordered AS (
			SELECT session_id, event_type, COALESCE(event_name, '') as event_name,
				ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY timestamp) as rn
			FROM events
			WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?
		),
		pairs AS (
			SELECT a.event_type as t1, a.event_name as n1,
				b.event_type as t2, b.event_name as n2,
				COUNT(DISTINCT a.session_id) as cnt
			FROM ordered a JOIN ordered b ON a.session_id = b.session_id AND b.rn = a.rn + 1
			GROUP BY a.event_type, a.event_name, b.event_type, b.event_name
			HAVING cnt >= 2
		),
		triples AS (
			SELECT a.event_type as t1, a.event_name as n1,
				b.event_type as t2, b.event_name as n2,
				c.event_type as t3, c.event_name as n3,
				COUNT(DISTINCT a.session_id) as cnt
			FROM ordered a
				JOIN ordered b ON a.session_id = b.session_id AND b.rn = a.rn + 1
				JOIN ordered c ON a.session_id = c.session_id AND c.rn = a.rn + 2
			GROUP BY a.event_type, a.event_name, b.event_type, b.event_name, c.event_type, c.event_name
			HAVING cnt >= 3
		)
		SELECT t1, n1, t2, n2, '' as t3, '' as n3, cnt FROM pairs
		UNION ALL
		SELECT t1, n1, t2, n2, t3, n3, cnt FROM triples
		ORDER BY cnt DESC
		LIMIT ?
	`

	rows, err := d.db.QueryContext(ctx, query, projectID, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top sequences: %w", err)
	}
	defer rows.Close()

	var sequences []EventSequence
	for rows.Next() {
		var t1, n1, t2, n2, t3, n3 string
		var cnt int64
		if err := rows.Scan(&t1, &n1, &t2, &n2, &t3, &n3, &cnt); err != nil {
			return nil, fmt.Errorf("scanning sequence row: %w", err)
		}
		seq := EventSequence{
			Steps:        []FunnelStep{{EventType: t1, EventName: n1}, {EventType: t2, EventName: n2}},
			SessionCount: cnt,
		}
		if t3 != "" {
			seq.Steps = append(seq.Steps, FunnelStep{EventType: t3, EventName: n3})
		}
		sequences = append(sequences, seq)
	}
	return sequences, rows.Err()
}

type PageStat struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Views    int64  `json:"views"`
	Sessions int64  `json:"sessions"`
}

type TrendSeries struct {
	Name string       `json:"name"`
	Data []TrendPoint `json:"data"`
}

func (d *DuckDB) QueryTopPages(ctx context.Context, projectID string, start, end time.Time, limit int) ([]PageStat, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT
			url_path,
			MAX(COALESCE(page_title, '')) as page_title,
			COUNT(*) as views,
			COUNT(DISTINCT session_id) as sessions
		FROM events
		WHERE project_id = ? AND event_type = 'pageview'
			AND timestamp >= ? AND timestamp <= ?
			AND url_path IS NOT NULL AND url_path != ''
		GROUP BY url_path
		ORDER BY views DESC
		LIMIT ?
	`, projectID, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("querying top pages: %w", err)
	}
	defer rows.Close()

	var stats []PageStat
	for rows.Next() {
		var s PageStat
		if err := rows.Scan(&s.Path, &s.Title, &s.Views, &s.Sessions); err != nil {
			return nil, fmt.Errorf("scanning page stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QueryTrendsBreakdown returns time-bucketed event counts split by a dimension.
// groupBy accepts "event_name", "event_type", or "url_path".
func (d *DuckDB) QueryTrendsBreakdown(ctx context.Context, projectID, interval, groupBy string, start, end time.Time) ([]TrendSeries, error) {
	switch interval {
	case "minute", "hour", "day", "week", "month":
	default:
		interval = "hour"
	}

	var seriesExpr string
	switch groupBy {
	case "event_type":
		seriesExpr = "event_type"
	case "url_path":
		seriesExpr = "url_path"
	default: // "event_name"
		seriesExpr = "COALESCE(event_name, event_type)"
	}

	query := fmt.Sprintf(`
		SELECT
			CAST(date_trunc('%s', CAST(timestamp AS TIMESTAMP)) AS VARCHAR) as bucket,
			COALESCE(CAST(%s AS VARCHAR), '') as series,
			COUNT(*) as count
		FROM events
		WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?
			AND %s IS NOT NULL AND CAST(%s AS VARCHAR) != ''
		GROUP BY bucket, series
		ORDER BY bucket, series
	`, interval, seriesExpr, seriesExpr, seriesExpr)

	rows, err := d.db.QueryContext(ctx, query, projectID, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying trends breakdown: %w", err)
	}
	defer rows.Close()

	type entry struct {
		bucket string
		count  int64
	}
	seriesData := map[string][]entry{}
	var seriesOrder []string

	for rows.Next() {
		var bucket, series string
		var count int64
		if err := rows.Scan(&bucket, &series, &count); err != nil {
			return nil, fmt.Errorf("scanning breakdown row: %w", err)
		}
		if _, ok := seriesData[series]; !ok {
			seriesOrder = append(seriesOrder, series)
		}
		seriesData[series] = append(seriesData[series], entry{bucket, count})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Pick top 8 series by total count.
	type scored struct {
		name  string
		total int64
	}
	var scores []scored
	for name, entries := range seriesData {
		var total int64
		for _, e := range entries {
			total += e.count
		}
		scores = append(scores, scored{name, total})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].total > scores[j].total })

	topN := 8
	if len(scores) < topN {
		topN = len(scores)
	}
	topNames := make(map[string]struct{}, topN)
	for i := 0; i < topN; i++ {
		topNames[scores[i].name] = struct{}{}
	}

	var result []TrendSeries
	for _, name := range seriesOrder {
		if _, ok := topNames[name]; !ok {
			continue
		}
		entries := seriesData[name]
		pts := make([]TrendPoint, len(entries))
		for i, e := range entries {
			pts[i] = TrendPoint{Bucket: e.bucket, Count: e.count}
		}
		result = append(result, TrendSeries{Name: name, Data: pts})
	}
	return result, nil
}

type EventNameStat struct {
	Name     string    `json:"name"`
	Count    int64     `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

func (d *DuckDB) QueryTopEventNames(ctx context.Context, projectID string, start, end time.Time, limit int) ([]EventNameStat, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []any{projectID}
	where := "project_id = ? AND event_name IS NOT NULL AND event_name != ''"
	if !start.IsZero() {
		where += " AND timestamp >= ?"
		args = append(args, start)
	}
	if !end.IsZero() {
		where += " AND timestamp <= ?"
		args = append(args, end)
	}
	args = append(args, limit)

	rows, err := d.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT event_name, COUNT(*) as count, MAX(timestamp) as last_seen
		FROM events WHERE %s
		GROUP BY event_name
		ORDER BY count DESC
		LIMIT ?
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("querying top event names: %w", err)
	}
	defer rows.Close()

	var stats []EventNameStat
	for rows.Next() {
		var s EventNameStat
		if err := rows.Scan(&s.Name, &s.Count, &s.LastSeen); err != nil {
			return nil, fmt.Errorf("scanning event name stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

type PathTransition struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int64  `json:"count"`
}

type HeatmapPoint struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Count int64   `json:"count"`
}

func (d *DuckDB) QueryPaths(ctx context.Context, projectID string, start, end time.Time, limit int) ([]PathTransition, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := d.db.QueryContext(ctx, `
		WITH ordered AS (
			SELECT session_id, url_path,
			       ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY timestamp) AS rn
			FROM events WHERE project_id = ? AND event_type = 'pageview'
				AND timestamp BETWEEN ? AND ?
		),
		transitions AS (
			SELECT a.url_path AS from_path, b.url_path AS to_path
			FROM ordered a JOIN ordered b ON a.session_id = b.session_id AND b.rn = a.rn + 1
		)
		SELECT from_path, to_path, COUNT(*) AS cnt
		FROM transitions
		GROUP BY from_path, to_path
		ORDER BY cnt DESC
		LIMIT ?
	`, projectID, start, end, limit)
	if err != nil {
		return nil, fmt.Errorf("querying paths: %w", err)
	}
	defer rows.Close()

	var transitions []PathTransition
	for rows.Next() {
		var t PathTransition
		if err := rows.Scan(&t.From, &t.To, &t.Count); err != nil {
			return nil, fmt.Errorf("scanning path transition: %w", err)
		}
		transitions = append(transitions, t)
	}
	return transitions, rows.Err()
}

func (d *DuckDB) QueryHeatmap(ctx context.Context, projectID, urlPath string, start, end time.Time) ([]HeatmapPoint, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT
			ROUND(CAST(json_extract(properties, '$.client_x') AS DOUBLE), 2) AS x,
			ROUND(CAST(json_extract(properties, '$.client_y') AS DOUBLE), 2) AS y,
			COUNT(*) AS cnt
		FROM events
		WHERE project_id = ? AND event_type = 'click'
			AND url_path = ?
			AND json_extract(properties, '$.client_x') IS NOT NULL
			AND timestamp BETWEEN ? AND ?
		GROUP BY x, y
		ORDER BY cnt DESC
	`, projectID, urlPath, start, end)
	if err != nil {
		return nil, fmt.Errorf("querying heatmap: %w", err)
	}
	defer rows.Close()

	var points []HeatmapPoint
	for rows.Next() {
		var p HeatmapPoint
		if err := rows.Scan(&p.X, &p.Y, &p.Count); err != nil {
			return nil, fmt.Errorf("scanning heatmap point: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

func (d *DuckDB) CountEvents(ctx context.Context, projectID, eventType, eventName string, since time.Time) (int64, error) {
	query := "SELECT COUNT(*) FROM events WHERE project_id = ?"
	args := []any{projectID}
	if eventType != "" {
		query += " AND event_type = ?"
		args = append(args, eventType)
	}
	if eventName != "" {
		query += " AND event_name = ?"
		args = append(args, eventName)
	}
	if !since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, since)
	}
	var count int64
	err := d.db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

func (d *DuckDB) BackfillEventName(ctx context.Context, projectID, fingerprint, name string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE events SET event_name = ? WHERE project_id = ? AND fingerprint = ? AND event_name IS NULL`,
		name, projectID, fingerprint,
	)
	return err
}

// Checkpoint flushes the DuckDB WAL to the main database file, making it safe to copy.
func (d *DuckDB) Checkpoint(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, "CHECKPOINT")
	return err
}

func (d *DuckDB) Close() error {
	return d.db.Close()
}
