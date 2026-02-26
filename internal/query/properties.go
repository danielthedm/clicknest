package query

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/danielleslie/clicknest/internal/auth"
)

// PropertyKeysHandler handles GET /api/v1/properties/keys.
func (h *Handler) PropertyKeysHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	keys, err := h.events.QueryPropertyKeys(r.Context(), project.ID)
	if err != nil {
		log.Printf("ERROR querying property keys: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"keys": keys})
}

// PropertyValuesHandler handles GET /api/v1/properties/values?key=...
func (h *Handler) PropertyValuesHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, `{"error":"key parameter required"}`, http.StatusBadRequest)
		return
	}

	values, err := h.events.QueryPropertyValues(r.Context(), project.ID, key, 100)
	if err != nil {
		log.Printf("ERROR querying property values: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"values": values})
}
