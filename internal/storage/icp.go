package storage

import (
	"context"
	"fmt"
	"time"
)

type ICPUserProfile struct {
	DistinctID   string   `json:"distinct_id"`
	SessionCount int      `json:"session_count"`
	EventCount   int      `json:"event_count"`
	TopPages     []string `json:"top_pages"`
	EntrySource  string   `json:"entry_source"`
}

// QueryICPProfiles finds users who visited specified conversion pages and aggregates their behavior.
func (d *DuckDB) QueryICPProfiles(ctx context.Context, projectID string, conversionPaths []string, start, end time.Time, limit int) ([]ICPUserProfile, error) {
	if len(conversionPaths) == 0 {
		return nil, nil
	}
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

	// Build conversion path filter.
	pathPlaceholders := ""
	for i, p := range conversionPaths {
		if i > 0 {
			pathPlaceholders += ", "
		}
		pathPlaceholders += "?"
		args = append(args, p)
	}

	// Find users who visited conversion pages.
	query := fmt.Sprintf(`
		WITH converters AS (
			SELECT DISTINCT distinct_id
			FROM events
			WHERE %s AND url_path IN (%s)
		),
		user_stats AS (
			SELECT
				e.distinct_id,
				COUNT(DISTINCT e.session_id) AS session_count,
				COUNT(*) AS event_count,
				FIRST(e.referrer) AS entry_source
			FROM events e
			JOIN converters c ON e.distinct_id = c.distinct_id
			WHERE e.project_id = ? AND e.distinct_id IS NOT NULL AND e.distinct_id != ''
			GROUP BY e.distinct_id
			ORDER BY event_count DESC
			LIMIT ?
		)
		SELECT distinct_id, session_count, event_count, entry_source
		FROM user_stats
	`, where, pathPlaceholders)

	args = append(args, projectID, limit)

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying ICP profiles: %w", err)
	}
	defer rows.Close()

	var profiles []ICPUserProfile
	for rows.Next() {
		var p ICPUserProfile
		var entrySource string
		if err := rows.Scan(&p.DistinctID, &p.SessionCount, &p.EventCount, &entrySource); err != nil {
			return nil, fmt.Errorf("scanning ICP profile: %w", err)
		}
		p.EntrySource = entrySource
		profiles = append(profiles, p)
	}

	// Get top pages for each user (batch query).
	if len(profiles) > 0 {
		for i := range profiles {
			pages, _ := d.queryUserTopPages(ctx, projectID, profiles[i].DistinctID, 5)
			profiles[i].TopPages = pages
		}
	}

	return profiles, rows.Err()
}

func (d *DuckDB) queryUserTopPages(ctx context.Context, projectID, distinctID string, limit int) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT url_path, COUNT(*) AS cnt
		FROM events
		WHERE project_id = ? AND distinct_id = ? AND event_type = 'pageview'
			AND url_path IS NOT NULL AND url_path != ''
		GROUP BY url_path
		ORDER BY cnt DESC
		LIMIT ?
	`, projectID, distinctID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []string
	for rows.Next() {
		var path string
		var cnt int
		if err := rows.Scan(&path, &cnt); err != nil {
			return nil, err
		}
		pages = append(pages, path)
	}
	return pages, rows.Err()
}
