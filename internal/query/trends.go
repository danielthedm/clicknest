package query

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
)

// TrendsBreakdownHandler handles GET /api/v1/trends/breakdown — multi-series trends split by a dimension.
func (h *Handler) TrendsBreakdownHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	interval := q.Get("interval")
	if interval == "" {
		interval = "hour"
	}
	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = "event_name"
	}

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	series, err := h.events.QueryTrendsBreakdown(r.Context(), project.ID, interval, groupBy, start, end)
	if err != nil {
		log.Printf("ERROR querying trends breakdown: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"series":   series,
		"interval": interval,
		"group_by": groupBy,
	})
}

// TrendsHandler handles GET /api/v1/trends — time-series event counts.
func (h *Handler) TrendsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	interval := q.Get("interval")
	if interval == "" {
		interval = "hour"
	}

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)

	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	points, err := h.events.QueryTrends(r.Context(), project.ID, interval, start, end)
	if err != nil {
		log.Printf("ERROR querying trends: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data":     points,
		"interval": interval,
		"start":    start.Format(time.RFC3339),
		"end":      end.Format(time.RFC3339),
	})
}
