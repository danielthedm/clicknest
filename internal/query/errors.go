package query

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/danielthedm/clicknest/internal/auth"
	"github.com/danielthedm/clicknest/internal/storage"
)

// ErrorGroupsHandler handles GET /api/v1/errors — list error groups with sparklines.
func (h *Handler) ErrorGroupsHandler(w http.ResponseWriter, r *http.Request) {
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
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	groups, totalCount, err := h.events.QueryErrorGroups(r.Context(), project.ID, start, end, limit)
	if err != nil {
		log.Printf("ERROR querying error groups: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Fetch sparkline trends for these groups.
	if len(groups) > 0 {
		messages := make([]string, len(groups))
		for i, g := range groups {
			messages[i] = g.Message
		}
		trends, err := h.events.QueryErrorTrends(r.Context(), project.ID, start, end, messages)
		if err != nil {
			log.Printf("WARN querying error trends: %v", err)
		} else {
			for i := range groups {
				if pts, ok := trends[groups[i].Message]; ok {
					groups[i].Sparkline = pts
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"groups":      groups,
		"total_count": totalCount,
	})
}

// ErrorDetailHandler handles GET /api/v1/errors/detail?message=... — drill-down into a specific error.
func (h *Handler) ErrorDetailHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	q := r.URL.Query()
	message := q.Get("message")
	if message == "" {
		http.Error(w, `{"error":"message parameter required"}`, http.StatusBadRequest)
		return
	}

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
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	filter := storage.EventFilter{
		ProjectID:     project.ID,
		EventType:     "error",
		PropertyKey:   "message",
		PropertyValue: message,
		StartTime:     start,
		EndTime:       end,
		Limit:         limit,
	}

	events, err := h.events.QueryEvents(r.Context(), filter)
	if err != nil {
		log.Printf("ERROR querying error detail: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Attempt to resolve source link from the first event with a source property.
	var sourceLink any
	if h.matcher != nil {
		for _, e := range events {
			src, _ := e.Properties["source"].(string)
			linenoF, _ := e.Properties["lineno"].(float64)
			if src != "" {
				link, err := h.matcher.MatchSourceFile(r.Context(), project.ID, src, int(linenoF))
				if err == nil && link != nil {
					sourceLink = link
					break
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"events":      events,
		"source_link": sourceLink,
	})
}
