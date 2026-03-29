package query

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/danielthedm/clicknest/internal/auth"
	"github.com/danielthedm/clicknest/internal/storage"
)

// ConversionGoalResultsHandler handles GET /api/v1/conversion-goals/{id}/results.
// Query params: start, end, model (first_touch|last_touch|linear).
func (h *Handler) ConversionGoalResultsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	goalID := r.PathValue("id")
	goal, err := h.meta.GetConversionGoal(r.Context(), project.ID, goalID)
	if err != nil {
		http.Error(w, `{"error":"goal not found"}`, http.StatusNotFound)
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

	model := q.Get("model")
	if model == "" {
		model = "first_touch"
	}

	criteria := storage.GoalCriteria{
		EventType:     goal.EventType,
		EventName:     goal.EventName,
		URLPattern:    goal.URLPattern,
		ValueProperty: goal.ValueProperty,
	}

	attributions, err := h.events.QueryConversionsByGoal(r.Context(), project.ID, criteria, model, start, end)
	if err != nil {
		log.Printf("ERROR querying conversion goal results: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	// Compute totals.
	var totalConversions int64
	var totalRevenue float64
	for _, a := range attributions {
		totalConversions += a.Conversions
		totalRevenue += a.Revenue
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"goal":              goal,
		"model":             model,
		"attributions":      attributions,
		"total_conversions": totalConversions,
		"total_revenue":     totalRevenue,
	})
}

// RevenueAttributionHandler handles GET /api/v1/attribution/revenue.
// Query params: goal_id (optional, uses all conversions if omitted), start, end.
func (h *Handler) RevenueAttributionHandler(w http.ResponseWriter, r *http.Request) {
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

	var criteria storage.GoalCriteria
	if goalID := q.Get("goal_id"); goalID != "" {
		goal, err := h.meta.GetConversionGoal(r.Context(), project.ID, goalID)
		if err != nil {
			http.Error(w, `{"error":"goal not found"}`, http.StatusNotFound)
			return
		}
		criteria = storage.GoalCriteria{
			EventType:     goal.EventType,
			EventName:     goal.EventName,
			URLPattern:    goal.URLPattern,
			ValueProperty: goal.ValueProperty,
		}
	} else {
		// Default: any custom event with $value.
		criteria = storage.GoalCriteria{
			EventType:     "custom",
			ValueProperty: "$value",
		}
	}

	overview, err := h.events.QueryRevenueOverview(r.Context(), project.ID, criteria, start, end)
	if err != nil {
		log.Printf("ERROR querying revenue attribution: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(overview)
}
