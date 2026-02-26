package query

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
	"github.com/danielleslie/clicknest/internal/storage"
)

// ErrorsHandler handles GET /api/v1/errors — list captured JS errors.
func (h *Handler) ErrorsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	end := time.Now().UTC()
	start := end.Add(-7 * 24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}
	limit := 500
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	filter := storage.EventFilter{
		ProjectID: project.ID,
		EventType: "error",
		StartTime: start,
		EndTime:   end,
		Limit:     limit,
	}

	errors, err := h.events.QueryEvents(r.Context(), filter)
	if err != nil {
		log.Printf("ERROR querying errors: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"errors": errors,
		"count":  len(errors),
	})
}
