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

// EventsHandler handles GET /api/v1/events — list events with filters.
func (h *Handler) EventsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	filter := storage.EventFilter{
		ProjectID:     project.ID,
		EventType:     q.Get("event_type"),
		EventName:     q.Get("event_name"),
		Fingerprint:   q.Get("fingerprint"),
		SessionID:     q.Get("session_id"),
		DistinctID:    q.Get("distinct_id"),
		PropertyKey:   q.Get("property_key"),
		PropertyValue: q.Get("property_value"),
	}

	if v := q.Get("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
	}
	if v := q.Get("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if v := q.Get("start"); v != "" {
		filter.StartTime, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("end"); v != "" {
		filter.EndTime, _ = time.Parse(time.RFC3339, v)
	}

	events, err := h.events.QueryEvents(r.Context(), filter)
	if err != nil {
		log.Printf("ERROR querying events: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Batch-resolve AI names from cache.
	fps := make([]string, 0, len(events))
	seen := make(map[string]bool, len(events))
	for _, e := range events {
		if !seen[e.Fingerprint] {
			fps = append(fps, e.Fingerprint)
			seen[e.Fingerprint] = true
		}
	}
	nameCache, _ := h.meta.BatchGetEventNames(r.Context(), project.ID, fps)
	for i := range events {
		if events[i].EventName != nil {
			continue
		}
		if en, ok := nameCache[events[i].Fingerprint]; ok {
			name := en.AIName
			if en.UserName != nil && *en.UserName != "" {
				name = *en.UserName
			}
			events[i].EventName = &name
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"events": events,
		"count":  len(events),
	})
}

// EventStatsHandler handles GET /api/v1/events/stats — top named events by frequency.
func (h *Handler) EventStatsHandler(w http.ResponseWriter, r *http.Request) {
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
	limit := 50
	if v := q.Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}

	stats, err := h.events.QueryTopEventNames(r.Context(), project.ID, start, end, limit)
	if err != nil {
		log.Printf("ERROR querying event stats: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"stats": stats,
	})
}
