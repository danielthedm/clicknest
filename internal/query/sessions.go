package query

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
	"github.com/danielleslie/clicknest/internal/storage"
)

// SessionsHandler handles GET /api/v1/sessions — list sessions.
func (h *Handler) SessionsHandler(w http.ResponseWriter, r *http.Request) {
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
	start := end.Add(-7 * 24 * time.Hour)
	if v := q.Get("start"); v != "" {
		start, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		end, _ = time.Parse(time.RFC3339, v)
	}

	// Get events grouped by session — query all events and group in memory.
	events, err := h.events.QueryEvents(r.Context(), storage.EventFilter{
		ProjectID: project.ID,
		StartTime: start,
		EndTime:   end,
		Limit:     10000,
	})
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	type Session struct {
		SessionID  string    `json:"session_id"`
		DistinctID string    `json:"distinct_id,omitempty"`
		EventCount int       `json:"event_count"`
		FirstSeen  time.Time `json:"first_seen"`
		LastSeen   time.Time `json:"last_seen"`
		EntryURL   string    `json:"entry_url"`
	}

	sessMap := make(map[string]*Session)
	sessOrder := []string{}
	for _, e := range events {
		s, ok := sessMap[e.SessionID]
		if !ok {
			s = &Session{
				SessionID:  e.SessionID,
				DistinctID: e.DistinctID,
				FirstSeen:  e.Timestamp,
				LastSeen:   e.Timestamp,
				EntryURL:   e.URL,
			}
			sessMap[e.SessionID] = s
			sessOrder = append(sessOrder, e.SessionID)
		}
		s.EventCount++
		if e.Timestamp.Before(s.FirstSeen) {
			s.FirstSeen = e.Timestamp
			s.EntryURL = e.URL
		}
		if e.Timestamp.After(s.LastSeen) {
			s.LastSeen = e.Timestamp
		}
	}

	sessions := make([]Session, 0, len(sessOrder))
	for _, id := range sessOrder {
		sessions = append(sessions, *sessMap[id])
	}

	// Apply pagination.
	total := len(sessions)
	if offset >= total {
		sessions = nil
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		sessions = sessions[offset:end]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sessions": sessions,
		"total":    total,
	})
}

// SessionDetailHandler handles GET /api/v1/sessions/{id} — session event timeline.
func (h *Handler) SessionDetailHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	sessionID := r.PathValue("id")
	if sessionID == "" {
		http.Error(w, `{"error":"session_id required"}`, http.StatusBadRequest)
		return
	}

	events, err := h.events.QueryEvents(r.Context(), storage.EventFilter{
		ProjectID: project.ID,
		SessionID: sessionID,
		Limit:     1000,
	})
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"session_id": sessionID,
		"events":     events,
		"count":      len(events),
	})
}
