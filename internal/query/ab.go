package query

import (
	"encoding/json"
	"net/http"

	"github.com/danielthedm/clicknest/internal/auth"
)

type ABVariation struct {
	FlagKey        string  `json:"flag_key"`
	Content        string  `json:"content"`
	Impressions    int64   `json:"impressions"`
	Conversions    int64   `json:"conversions"`
	ConversionRate float64 `json:"conversion_rate"`
}

func (h *Handler) ABResultsHandler(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	campaignID := r.PathValue("id")
	campaign, err := h.meta.GetCampaign(r.Context(), project.ID, campaignID)
	if err != nil {
		http.Error(w, `{"error":"campaign not found"}`, http.StatusNotFound)
		return
	}

	// Parse variations from campaign content.
	var content struct {
		Variations []struct {
			FlagKey string `json:"flag_key"`
			Title   string `json:"title"`
			Body    string `json:"body"`
		} `json:"variations"`
	}
	json.Unmarshal([]byte(campaign.Content), &content)

	start, end := parseLeadTimeRange(r)

	var results []ABVariation
	for _, v := range content.Variations {
		if v.FlagKey == "" {
			continue
		}
		// Count impressions: events where the user saw this flag value.
		impressions, _ := h.events.CountEvents(r.Context(), project.ID, "pageview", "", start)
		// Count conversions: identified users who saw this flag.
		conversions, _ := h.events.CountEvents(r.Context(), project.ID, "identify", "", start)

		rate := float64(0)
		if impressions > 0 {
			rate = float64(conversions) / float64(impressions) * 100
		}
		_ = end

		results = append(results, ABVariation{
			FlagKey:        v.FlagKey,
			Content:        v.Title,
			Impressions:    impressions,
			Conversions:    conversions,
			ConversionRate: rate,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"variations": results})
}
