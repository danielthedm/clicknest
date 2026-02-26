package ingest

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/danielleslie/clicknest/internal/ai"
	"github.com/danielleslie/clicknest/internal/auth"
	"github.com/danielleslie/clicknest/internal/storage"
)

type IngestEvent struct {
	EventType      string            `json:"event_type"`
	ElementTag     string            `json:"element_tag,omitempty"`
	ElementID      string            `json:"element_id,omitempty"`
	ElementClasses string            `json:"element_classes,omitempty"`
	ElementText    string            `json:"element_text,omitempty"`
	AriaLabel      string            `json:"aria_label,omitempty"`
	DataAttributes map[string]string `json:"data_attributes,omitempty"`
	ParentPath     string            `json:"parent_path,omitempty"`
	URL            string            `json:"url"`
	URLPath        string            `json:"url_path,omitempty"`
	PageTitle      string            `json:"page_title,omitempty"`
	Referrer       string            `json:"referrer,omitempty"`
	ScreenWidth    int               `json:"screen_width,omitempty"`
	ScreenHeight   int               `json:"screen_height,omitempty"`
	Timestamp      int64             `json:"timestamp"`
	Properties     map[string]any    `json:"properties,omitempty"`
}

type IngestPayload struct {
	Events     []IngestEvent `json:"events"`
	SessionID  string        `json:"session_id"`
	DistinctID string        `json:"distinct_id,omitempty"`
}

type Handler struct {
	events *storage.DuckDB
	namer  *ai.Namer
}

func NewHandler(events *storage.DuckDB, namer *ai.Namer) *Handler {
	return &Handler{events: events, namer: namer}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var payload IngestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if err := ValidatePayload(&payload); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	userAgent := r.Header.Get("User-Agent")

	events := make([]storage.Event, len(payload.Events))
	for i, e := range payload.Events {
		fingerprint := ComputeFingerprint(
			e.ElementTag, e.ElementID, e.ElementClasses, e.ParentPath, e.URLPath,
		)

		ts := time.UnixMilli(e.Timestamp)
		if e.Timestamp == 0 {
			ts = time.Now().UTC()
		}

		events[i] = storage.Event{
			ProjectID:      project.ID,
			SessionID:      payload.SessionID,
			DistinctID:     payload.DistinctID,
			EventType:      e.EventType,
			Fingerprint:    fingerprint,
			ElementTag:     e.ElementTag,
			ElementID:      e.ElementID,
			ElementClasses: e.ElementClasses,
			ElementText:    e.ElementText,
			AriaLabel:      e.AriaLabel,
			DataAttributes: e.DataAttributes,
			ParentPath:     e.ParentPath,
			URL:            e.URL,
			URLPath:        e.URLPath,
			PageTitle:      e.PageTitle,
			Referrer:       e.Referrer,
			ScreenWidth:    e.ScreenWidth,
			ScreenHeight:   e.ScreenHeight,
			UserAgent:      userAgent,
			Timestamp:      ts,
			Properties:     e.Properties,
		}
	}

	if err := h.events.InsertEvents(r.Context(), events); err != nil {
		log.Printf("ERROR inserting events: %v", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Submit naming jobs for interaction events (not pageviews).
	if h.namer != nil {
		for i, e := range payload.Events {
			if e.EventType == "pageview" {
				continue
			}
			h.namer.Submit(r.Context(), ai.NamingJob{
				ProjectID:   project.ID,
				Fingerprint: events[i].Fingerprint,
				Request: ai.NamingRequest{
					ElementTag:     e.ElementTag,
					ElementID:      e.ElementID,
					ElementClasses: e.ElementClasses,
					ElementText:    e.ElementText,
					AriaLabel:      e.AriaLabel,
					ParentPath:     e.ParentPath,
					URL:            e.URL,
					URLPath:        e.URLPath,
					PageTitle:      e.PageTitle,
				},
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status":   "ok",
		"accepted": len(events),
	})
}
