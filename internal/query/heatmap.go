package query

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
)

// HeatmapHandler handles GET /api/v1/heatmap — click density for a URL path.
func (h *Handler) HeatmapHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	urlPath := q.Get("url_path")

	end := time.Now().UTC()
	start := end.Add(-7 * 24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	points, err := h.events.QueryHeatmap(r.Context(), project.ID, urlPath, start, end)
	if err != nil {
		log.Printf("ERROR querying heatmap: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"points": points})
}
