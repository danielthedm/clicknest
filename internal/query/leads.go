package query

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/danielthedm/clicknest/internal/auth"
)

func (h *Handler) LeadScoresHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	start, end := parseLeadTimeRange(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	rules, err := h.meta.ListScoringRules(r.Context(), project.ID)
	if err != nil {
		http.Error(w, `{"error":"failed to load scoring rules"}`, http.StatusInternalServerError)
		return
	}

	leads, total, err := h.events.QueryLeadScores(r.Context(), project.ID, rules, start, end, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Enrich with score deltas from yesterday's snapshot.
	if snapshots, err := h.meta.GetYesterdayScores(r.Context(), project.ID); err == nil && len(snapshots) > 0 {
		for i := range leads {
			if prev, ok := snapshots[leads[i].DistinctID]; ok {
				delta := leads[i].Score - prev
				leads[i].ScoreDelta = &delta
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"leads": leads, "total": total})
}

func parseLeadTimeRange(r *http.Request) (time.Time, time.Time) {
	var start, end time.Time
	if s := r.URL.Query().Get("start"); s != "" {
		start, _ = time.Parse(time.RFC3339, s)
	}
	if e := r.URL.Query().Get("end"); e != "" {
		end, _ = time.Parse(time.RFC3339, e)
	}
	if start.IsZero() {
		start = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}
	return start, end
}
