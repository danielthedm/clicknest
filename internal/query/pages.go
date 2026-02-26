package query

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
)

// PagesHandler handles GET /api/v1/pages — top pages by traffic.
func (h *Handler) PagesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	end := time.Now().UTC()
	start := end.Add(-30 * 24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	pages, err := h.events.QueryTopPages(r.Context(), project.ID, start, end, limit)
	if err != nil {
		log.Printf("ERROR querying top pages: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"pages": pages})
}
