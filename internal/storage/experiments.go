package storage

import (
	"context"
	"fmt"
	"time"
)

// ExperimentVariantResult holds per-variant metrics from an experiment.
type ExperimentVariantResult struct {
	Variant        string  `json:"variant"`
	Exposures      int64   `json:"exposures"`
	Conversions    int64   `json:"conversions"`
	ConversionRate float64 `json:"conversion_rate"`
	Revenue        float64 `json:"revenue"`
	ConfidenceLow  float64 `json:"confidence_low"`
	ConfidenceHigh float64 `json:"confidence_high"`
}

// QueryExperimentResults returns per-variant exposure and conversion metrics.
// It joins $exposure events (which carry $flag_key and $variant in properties)
// with conversion events matching the goal criteria.
func (d *DuckDB) QueryExperimentResults(ctx context.Context, projectID, flagKey string, variants []string, goal *GoalCriteria, start, end time.Time) ([]ExperimentVariantResult, error) {
	valueProp := "$value"
	if goal != nil && goal.ValueProperty != "" {
		valueProp = goal.ValueProperty
	}

	// Build conversion event filter.
	convFilter := "c.project_id = ? AND c.timestamp BETWEEN ? AND ?"
	convArgs := []any{projectID, start, end}
	if goal != nil {
		if goal.EventType != "" {
			convFilter += " AND c.event_type = ?"
			convArgs = append(convArgs, goal.EventType)
		}
		if goal.EventName != "" {
			convFilter += " AND c.event_name = ?"
			convArgs = append(convArgs, goal.EventName)
		}
		if goal.URLPattern != "" {
			convFilter += " AND c.url_path LIKE ?"
			convArgs = append(convArgs, goal.URLPattern)
		}
	}

	query := `
WITH exposures AS (
    SELECT
        distinct_id,
        json_extract_string(CAST(properties AS VARCHAR), '$.$variant') AS variant
    FROM events
    WHERE project_id = ? AND event_type = '$exposure'
        AND json_extract_string(CAST(properties AS VARCHAR), '$.$flag_key') = ?
        AND timestamp BETWEEN ? AND ?
        AND distinct_id IS NOT NULL AND distinct_id != ''
),
exposure_users AS (
    SELECT
        variant,
        distinct_id,
        ROW_NUMBER() OVER (PARTITION BY distinct_id ORDER BY variant) AS rn
    FROM exposures
    WHERE variant IS NOT NULL AND variant != ''
),
deduped AS (
    SELECT variant, distinct_id FROM exposure_users WHERE rn = 1
),
variant_exposures AS (
    SELECT variant, COUNT(*) AS exposure_count
    FROM deduped GROUP BY variant
),
conversions AS (
    SELECT
        c.distinct_id,
        COUNT(*) AS conv_count,
        COALESCE(SUM(TRY_CAST(json_extract_string(CAST(c.properties AS VARCHAR), '$.' || ?) AS DOUBLE)), 0) AS revenue
    FROM events c
    WHERE ` + convFilter + `
        AND c.distinct_id IS NOT NULL AND c.distinct_id != ''
    GROUP BY c.distinct_id
),
variant_conversions AS (
    SELECT
        d.variant,
        COALESCE(SUM(cv.conv_count), 0) AS conversions,
        COALESCE(SUM(cv.revenue), 0) AS revenue
    FROM deduped d
    LEFT JOIN conversions cv ON d.distinct_id = cv.distinct_id
    GROUP BY d.variant
)
SELECT
    ve.variant,
    ve.exposure_count,
    COALESCE(vc.conversions, 0) AS conversions,
    COALESCE(vc.revenue, 0) AS revenue
FROM variant_exposures ve
LEFT JOIN variant_conversions vc ON ve.variant = vc.variant
ORDER BY ve.variant
`
	allArgs := []any{projectID, flagKey, start, end, valueProp}
	allArgs = append(allArgs, convArgs...)

	rows, err := d.db.QueryContext(ctx, query, allArgs...)
	if err != nil {
		return nil, fmt.Errorf("querying experiment results: %w", err)
	}
	defer rows.Close()

	var results []ExperimentVariantResult
	for rows.Next() {
		var r ExperimentVariantResult
		if err := rows.Scan(&r.Variant, &r.Exposures, &r.Conversions, &r.Revenue); err != nil {
			return nil, fmt.Errorf("scanning experiment variant result: %w", err)
		}
		if r.Exposures > 0 {
			r.ConversionRate = float64(r.Conversions) / float64(r.Exposures) * 100
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
