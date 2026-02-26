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

// UsersHandler handles GET /api/v1/users — list aggregated user profiles.
func (h *Handler) UsersHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}

	end := time.Now().UTC()
	start := end.Add(-30 * 24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	users, total, err := h.events.QueryUsers(r.Context(), project.ID, limit, offset, start, end)
	if err != nil {
		log.Printf("ERROR querying users: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"users": users,
		"total": total,
	})
}

// UserEventsHandler handles GET /api/v1/users/{id}/events — events for a user.
func (h *Handler) UserEventsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	distinctID := r.PathValue("id")
	if distinctID == "" {
		http.Error(w, `{"error":"user id required"}`, http.StatusBadRequest)
		return
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}

	events, err := h.events.QueryEvents(r.Context(), storage.EventFilter{
		ProjectID:  project.ID,
		DistinctID: distinctID,
		Limit:      limit,
	})
	if err != nil {
		log.Printf("ERROR querying user events: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"events": events,
		"count":  len(events),
	})
}
