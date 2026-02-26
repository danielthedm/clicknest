package query

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
)

// RetentionHandler handles GET /api/v1/retention.
func (h *Handler) RetentionHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	interval := q.Get("interval")
	if interval == "" {
		interval = "week"
	}
	periods := 8
	if v := q.Get("periods"); v != "" {
		periods, _ = strconv.Atoi(v)
	}

	end := time.Now().UTC()
	start := end.Add(-90 * 24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	cohorts, err := h.events.QueryRetention(r.Context(), project.ID, interval, periods, start, end)
	if err != nil {
		log.Printf("ERROR querying retention: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cohorts": cohorts})
}
