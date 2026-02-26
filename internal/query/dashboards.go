package query

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/danielleslie/clicknest/internal/auth"
	"github.com/danielleslie/clicknest/internal/storage"
)

// ListDashboardsHandler handles GET /api/v1/dashboards.
func (h *Handler) ListDashboardsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	dashboards, err := h.meta.ListDashboards(r.Context(), project.ID)
	if err != nil {
		log.Printf("ERROR listing dashboards: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"dashboards": dashboards})
}

// CreateDashboardHandler handles POST /api/v1/dashboards.
func (h *Handler) CreateDashboardHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var body struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if body.Name == "" || len(body.Config) == 0 {
		http.Error(w, `{"error":"name and config required"}`, http.StatusBadRequest)
		return
	}

	id, err := generateID()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	d := storage.Dashboard{
		ID:        id,
		ProjectID: project.ID,
		Name:      body.Name,
		Config:    string(body.Config),
	}

	if err := h.meta.CreateDashboard(r.Context(), d); err != nil {
		log.Printf("ERROR creating dashboard: %v", err)
		http.Error(w, `{"error":"create failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(d)
}

// GetDashboardHandler handles GET /api/v1/dashboards/{id}.
func (h *Handler) GetDashboardHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	d, err := h.meta.GetDashboard(r.Context(), project.ID, id)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// UpdateDashboardHandler handles PUT /api/v1/dashboards/{id}.
func (h *Handler) UpdateDashboardHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	var body struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	d := storage.Dashboard{
		ID:        id,
		ProjectID: project.ID,
		Name:      body.Name,
		Config:    string(body.Config),
	}

	if err := h.meta.UpdateDashboard(r.Context(), d); err != nil {
		log.Printf("ERROR updating dashboard: %v", err)
		http.Error(w, `{"error":"update failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// DeleteDashboardHandler handles DELETE /api/v1/dashboards/{id}.
func (h *Handler) DeleteDashboardHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	id := r.PathValue("id")
	if err := h.meta.DeleteDashboard(r.Context(), project.ID, id); err != nil {
		log.Printf("ERROR deleting dashboard: %v", err)
		http.Error(w, `{"error":"delete failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
