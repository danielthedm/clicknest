package query

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/danielleslie/clicknest/internal/auth"
	"github.com/danielleslie/clicknest/internal/storage"
)

// ListFunnelsHandler handles GET /api/v1/funnels.
func (h *Handler) ListFunnelsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	funnels, err := h.meta.ListFunnels(r.Context(), project.ID)
	if err != nil {
		log.Printf("ERROR listing funnels: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"funnels": funnels})
}

// CreateFunnelHandler handles POST /api/v1/funnels.
func (h *Handler) CreateFunnelHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		Name  string             `json:"name"`
		Steps []storage.FunnelStep `json:"steps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if body.Name == "" || len(body.Steps) < 2 {
		http.Error(w, `{"error":"name and at least 2 steps required"}`, http.StatusBadRequest)
		return
	}

	stepsJSON, err := json.Marshal(body.Steps)
	if err != nil {
		http.Error(w, `{"error":"invalid steps"}`, http.StatusBadRequest)
		return
	}

	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	funnel := storage.Funnel{
		ID:        id,
		ProjectID: project.ID,
		Name:      body.Name,
		Steps:     string(stepsJSON),
	}

	if err := h.meta.CreateFunnel(r.Context(), funnel); err != nil {
		log.Printf("ERROR creating funnel: %v", err)
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}

	funnel.CreatedAt = time.Now()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(funnel)
}

// GetFunnelHandler handles GET /api/v1/funnels/{id}.
func (h *Handler) GetFunnelHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	funnel, err := h.meta.GetFunnel(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(funnel)
}

// DeleteFunnelHandler handles DELETE /api/v1/funnels/{id}.
func (h *Handler) DeleteFunnelHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if err := h.meta.DeleteFunnel(r.Context(), project.ID, id); err != nil {
		log.Printf("ERROR deleting funnel: %v", err)
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// FunnelResultsHandler handles GET /api/v1/funnels/{id}/results.
func (h *Handler) FunnelResultsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	funnel, err := h.meta.GetFunnel(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"funnel not found"}`, http.StatusNotFound)
		return
	}

	var steps []storage.FunnelStep
	if err := json.Unmarshal([]byte(funnel.Steps), &steps); err != nil {
		http.Error(w, `{"error":"invalid funnel steps"}`, http.StatusInternalServerError)
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

	results, err := h.events.QueryFunnel(r.Context(), project.ID, steps, start, end)
	if err != nil {
		log.Printf("ERROR querying funnel results: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

// FunnelCohortsHandler handles GET /api/v1/funnels/{id}/cohorts.
func (h *Handler) FunnelCohortsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	funnel, err := h.meta.GetFunnel(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"funnel not found"}`, http.StatusNotFound)
		return
	}

	var steps []storage.FunnelStep
	if err := json.Unmarshal([]byte(funnel.Steps), &steps); err != nil {
		http.Error(w, `{"error":"invalid funnel steps"}`, http.StatusInternalServerError)
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

	interval := q.Get("interval")
	if interval == "" {
		interval = "week"
	}

	cohorts, err := h.events.QueryFunnelCohorts(r.Context(), project.ID, steps, interval, start, end)
	if err != nil {
		log.Printf("ERROR querying funnel cohorts: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"cohorts": cohorts})
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
