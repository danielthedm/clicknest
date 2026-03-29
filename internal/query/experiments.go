package query

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/danielthedm/clicknest/internal/auth"
	"github.com/danielthedm/clicknest/internal/storage"
)

// ExperimentResultsResponse is the JSON response for experiment results.
type ExperimentResultsResponse struct {
	Experiment       *storage.Experiment              `json:"experiment"`
	Variants         []storage.ExperimentVariantResult `json:"variants"`
	Significance     *ZTestResult                     `json:"significance,omitempty"`
	ChiSquared       *ChiSquaredResult                `json:"chi_squared,omitempty"`
	SampleSizeNeeded int64                            `json:"sample_size_needed"`
	Winner           string                           `json:"winner,omitempty"`
}

// ExperimentResultsHandler handles GET /api/v1/experiments/{id}/results.
func (h *Handler) ExperimentResultsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	expID := r.PathValue("id")
	exp, err := h.meta.GetExperiment(r.Context(), project.ID, expID)
	if err != nil {
		http.Error(w, `{"error":"experiment not found"}`, http.StatusNotFound)
		return
	}

	var variants []string
	json.Unmarshal([]byte(exp.Variants), &variants)

	start := exp.StartedAt
	end := exp.UpdatedAt
	if exp.EndedAt != nil {
		end = *exp.EndedAt
	}

	// Resolve conversion goal criteria if set.
	var goal *storage.GoalCriteria
	if exp.ConversionGoalID != "" {
		g, err := h.meta.GetConversionGoal(r.Context(), project.ID, exp.ConversionGoalID)
		if err == nil {
			goal = &storage.GoalCriteria{
				EventType:     g.EventType,
				EventName:     g.EventName,
				URLPattern:    g.URLPattern,
				ValueProperty: g.ValueProperty,
			}
		}
	}

	results, err := h.events.QueryExperimentResults(r.Context(), project.ID, exp.FlagKey, variants, goal, start, end)
	if err != nil {
		log.Printf("ERROR querying experiment results: %v", err)
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	resp := ExperimentResultsResponse{
		Experiment: exp,
		Variants:   results,
	}

	// Add confidence intervals.
	for i := range resp.Variants {
		low, high := WilsonConfidenceInterval(resp.Variants[i].Conversions, resp.Variants[i].Exposures, 0.95)
		resp.Variants[i].ConversionRate = math_round2(resp.Variants[i].ConversionRate)
		// Store CI as percentage.
		resp.Variants[i] = storage.ExperimentVariantResult{
			Variant:        resp.Variants[i].Variant,
			Exposures:      resp.Variants[i].Exposures,
			Conversions:    resp.Variants[i].Conversions,
			ConversionRate: resp.Variants[i].ConversionRate,
			Revenue:        resp.Variants[i].Revenue,
			ConfidenceLow:  math_round2(low * 100),
			ConfidenceHigh: math_round2(high * 100),
		}
	}

	// Statistical significance.
	if len(resp.Variants) == 2 {
		z := ZTestProportions(
			resp.Variants[0].Conversions, resp.Variants[0].Exposures,
			resp.Variants[1].Conversions, resp.Variants[1].Exposures,
		)
		resp.Significance = &z
	}
	if len(resp.Variants) > 2 {
		counts := make([]VariantCounts, len(resp.Variants))
		for i, v := range resp.Variants {
			counts[i] = VariantCounts{Exposures: v.Exposures, Conversions: v.Conversions}
		}
		chi := ChiSquaredTest(counts)
		resp.ChiSquared = &chi
	}

	// Sample size needed.
	if len(resp.Variants) >= 2 && resp.Variants[0].Exposures > 0 {
		baseRate := float64(resp.Variants[0].Conversions) / float64(resp.Variants[0].Exposures)
		if baseRate > 0 && baseRate < 1 {
			needed := RequiredSampleSize(baseRate, 0.1, 0.05, 0.80) // 10% MDE
			maxExposures := resp.Variants[0].Exposures
			for _, v := range resp.Variants[1:] {
				if v.Exposures > maxExposures {
					maxExposures = v.Exposures
				}
			}
			if needed > maxExposures {
				resp.SampleSizeNeeded = needed - maxExposures
			}
		}
	}

	// Determine winner if significant.
	isSignificant := (resp.Significance != nil && resp.Significance.Significant) ||
		(resp.ChiSquared != nil && resp.ChiSquared.Significant)
	if isSignificant {
		bestRate := -1.0
		for _, v := range resp.Variants {
			if v.ConversionRate > bestRate {
				bestRate = v.ConversionRate
				resp.Winner = v.Variant
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ExperimentSampleSizeHandler handles GET /api/v1/experiments/{id}/sample-size.
func (h *Handler) ExperimentSampleSizeHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	expID := r.PathValue("id")
	exp, err := h.meta.GetExperiment(r.Context(), project.ID, expID)
	if err != nil {
		http.Error(w, `{"error":"experiment not found"}`, http.StatusNotFound)
		return
	}

	var variants []string
	json.Unmarshal([]byte(exp.Variants), &variants)

	start := exp.StartedAt
	end := exp.UpdatedAt
	if exp.EndedAt != nil {
		end = *exp.EndedAt
	}

	results, err := h.events.QueryExperimentResults(r.Context(), project.ID, exp.FlagKey, variants, nil, start, end)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}

	var needed int64
	if len(results) >= 2 && results[0].Exposures > 0 {
		baseRate := float64(results[0].Conversions) / float64(results[0].Exposures)
		if baseRate > 0 && baseRate < 1 {
			needed = RequiredSampleSize(baseRate, 0.1, 0.05, 0.80)
		}
	}

	var currentMax int64
	for _, v := range results {
		if v.Exposures > currentMax {
			currentMax = v.Exposures
		}
	}

	remaining := needed - currentMax
	if remaining < 0 {
		remaining = 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sample_size_needed":     needed,
		"current_max_exposures":  currentMax,
		"remaining":              remaining,
	})
}

func math_round2(v float64) float64 {
	return float64(int64(v*100)) / 100
}
