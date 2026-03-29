package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type ScoredLead struct {
	DistinctID        string         `json:"distinct_id"`
	Score             int            `json:"score"`
	RawScore          int            `json:"raw_score"`
	ScoreDelta        *int           `json:"score_delta,omitempty"` // change vs yesterday's snapshot; nil if no snapshot
	EventCount        int            `json:"event_count"`
	SessionCount      int            `json:"session_count"`
	PageViews         int            `json:"page_views"`
	FirstSeen         time.Time      `json:"first_seen"`
	LastSeen          time.Time      `json:"last_seen"`
	DaysSinceLastSeen int            `json:"days_since_last_seen"`
	TopPages          []string       `json:"top_pages"`
	Properties        map[string]any `json:"properties,omitempty"`
}

type RuleConfig struct {
	URLPath       string `json:"url_path,omitempty"`
	EventName     string `json:"event_name,omitempty"`
	MinCount      int    `json:"min_count,omitempty"`
	PropertyKey   string `json:"property_key,omitempty"`
	PropertyValue string `json:"property_value,omitempty"`
	// Time decay settings.
	HalfLifeDays int `json:"half_life_days,omitempty"` // Score halves every N days of inactivity.
	// Inactivity settings.
	InactiveDays int `json:"inactive_days,omitempty"` // Days without activity to trigger negative score.
	// Behavioral pattern settings.
	TargetURLPath string `json:"target_url_path,omitempty"` // "visited X within N days of event Y"
	WithinDays    int    `json:"within_days,omitempty"`
}

// LeadScoringConfig holds project-level scoring settings.
type LeadScoringConfig struct {
	DecayHalfLifeDays int `json:"decay_half_life_days"` // 0 = no decay
}

// QueryLeadScores evaluates scoring rules against event data and returns scored leads.
func (d *DuckDB) QueryLeadScores(ctx context.Context, projectID string, rules []ScoringRule, start, end time.Time, limit, offset int) ([]ScoredLead, int, error) {
	if limit <= 0 {
		limit = 50
	}

	// First get base user stats.
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

	// Count total users.
	var total int
	err := d.db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(DISTINCT distinct_id) FROM events WHERE %s", where,
	), args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting leads: %w", err)
	}

	// Build scoring expression using CASE statements for each rule.
	// Separate positive and negative rules.
	var scoreParts []string
	scoreArgs := []any{}
	var decayHalfLife int

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		var cfg RuleConfig
		json.Unmarshal([]byte(rule.Config), &cfg)

		switch rule.RuleType {
		case "page_visit":
			if cfg.URLPath != "" {
				scoreParts = append(scoreParts,
					fmt.Sprintf("CASE WHEN SUM(CASE WHEN url_path = ? THEN 1 ELSE 0 END) > 0 THEN %d ELSE 0 END", rule.Points))
				scoreArgs = append(scoreArgs, cfg.URLPath)
			}
		case "event_count":
			minCount := cfg.MinCount
			if minCount <= 0 {
				minCount = 1
			}
			if cfg.EventName != "" {
				scoreParts = append(scoreParts,
					fmt.Sprintf("CASE WHEN SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END) >= %d THEN %d ELSE 0 END", minCount, rule.Points))
				scoreArgs = append(scoreArgs, cfg.EventName)
			} else {
				scoreParts = append(scoreParts,
					fmt.Sprintf("CASE WHEN COUNT(*) >= %d THEN %d ELSE 0 END", minCount, rule.Points))
			}
		case "session_count":
			minCount := cfg.MinCount
			if minCount <= 0 {
				minCount = 2
			}
			scoreParts = append(scoreParts,
				fmt.Sprintf("CASE WHEN COUNT(DISTINCT session_id) >= %d THEN %d ELSE 0 END", minCount, rule.Points))
		case "identified":
			// All users in this query already have distinct_id, so they all get points.
			scoreParts = append(scoreParts, fmt.Sprintf("%d", rule.Points))
		case "property_match":
			if cfg.PropertyKey != "" && cfg.PropertyValue != "" {
				scoreParts = append(scoreParts,
					fmt.Sprintf("CASE WHEN SUM(CASE WHEN json_extract_string(properties, '$.' || ?) = ? THEN 1 ELSE 0 END) > 0 THEN %d ELSE 0 END", rule.Points))
				scoreArgs = append(scoreArgs, cfg.PropertyKey, cfg.PropertyValue)
			}
		case "negative":
			// Negative scoring — deduct points for matching a property (e.g., bot detection).
			if cfg.PropertyKey != "" && cfg.PropertyValue != "" {
				scoreParts = append(scoreParts,
					fmt.Sprintf("CASE WHEN SUM(CASE WHEN json_extract_string(properties, '$.' || ?) = ? THEN 1 ELSE 0 END) > 0 THEN %d ELSE 0 END", -abs(rule.Points)))
				scoreArgs = append(scoreArgs, cfg.PropertyKey, cfg.PropertyValue)
			}
		case "inactivity":
			// Deduct points if the user hasn't been active in N days.
			days := cfg.InactiveDays
			if days <= 0 {
				days = 14
			}
			scoreParts = append(scoreParts,
				fmt.Sprintf("CASE WHEN MAX(timestamp) < CURRENT_TIMESTAMP - INTERVAL '%d days' THEN %d ELSE 0 END", days, -abs(rule.Points)))
		case "recency_decay":
			// Store the half-life for post-query application.
			if cfg.HalfLifeDays > 0 {
				decayHalfLife = cfg.HalfLifeDays
			}
		case "behavioral":
			// Visited target URL within N days of a specific event.
			if cfg.URLPath != "" && cfg.TargetURLPath != "" && cfg.WithinDays > 0 {
				// Check if user visited both URLs within the time window.
				scoreParts = append(scoreParts,
					fmt.Sprintf(`CASE WHEN
						SUM(CASE WHEN url_path = ? THEN 1 ELSE 0 END) > 0
						AND SUM(CASE WHEN url_path = ? THEN 1 ELSE 0 END) > 0
						AND (MAX(CASE WHEN url_path = ? THEN timestamp END) - MIN(CASE WHEN url_path = ? THEN timestamp END)) <= INTERVAL '%d days'
					THEN %d ELSE 0 END`, cfg.WithinDays, rule.Points))
				scoreArgs = append(scoreArgs, cfg.URLPath, cfg.TargetURLPath, cfg.TargetURLPath, cfg.URLPath)
			}
		}
	}

	scoreExpr := "0"
	if len(scoreParts) > 0 {
		scoreExpr = strings.Join(scoreParts, " + ")
	}

	query := fmt.Sprintf(`
		SELECT
			distinct_id,
			(%s) AS score,
			COUNT(*) AS event_count,
			COUNT(DISTINCT session_id) AS session_count,
			SUM(CASE WHEN event_type = 'pageview' THEN 1 ELSE 0 END) AS page_views,
			MIN(timestamp) AS first_seen,
			MAX(timestamp) AS last_seen
		FROM events
		WHERE %s
		GROUP BY distinct_id
		ORDER BY score DESC, event_count DESC
		LIMIT ? OFFSET ?
	`, scoreExpr, where)

	allArgs := make([]any, 0, len(args)+len(scoreArgs)+2)
	allArgs = append(allArgs, scoreArgs...)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, limit, offset)

	rows, err := d.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("querying lead scores: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	var leads []ScoredLead
	for rows.Next() {
		var l ScoredLead
		if err := rows.Scan(&l.DistinctID, &l.Score, &l.EventCount, &l.SessionCount, &l.PageViews, &l.FirstSeen, &l.LastSeen); err != nil {
			return nil, 0, fmt.Errorf("scanning lead: %w", err)
		}
		l.RawScore = l.Score
		l.DaysSinceLastSeen = int(now.Sub(l.LastSeen).Hours() / 24)

		// Apply time decay if configured.
		if decayHalfLife > 0 && l.DaysSinceLastSeen > 0 {
			decay := math.Pow(0.5, float64(l.DaysSinceLastSeen)/float64(decayHalfLife))
			l.Score = int(float64(l.Score) * decay)
		}

		leads = append(leads, l)
	}

	// Re-sort by decayed score if decay was applied.
	if decayHalfLife > 0 {
		sortLeadsByScore(leads)
	}

	return leads, total, rows.Err()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sortLeadsByScore(leads []ScoredLead) {
	for i := 1; i < len(leads); i++ {
		for j := i; j > 0 && leads[j].Score > leads[j-1].Score; j-- {
			leads[j], leads[j-1] = leads[j-1], leads[j]
		}
	}
}
