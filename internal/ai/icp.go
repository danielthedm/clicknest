package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

type ICPProfile struct {
	DistinctID   string            `json:"distinct_id"`
	SessionCount int               `json:"session_count"`
	EventCount   int               `json:"event_count"`
	TopPages     []string          `json:"top_pages"`
	EntrySource  string            `json:"entry_source"`
	Properties   map[string]string `json:"properties,omitempty"`
}

type ICPAnalysis struct {
	Summary         string   `json:"summary"`
	CommonTraits    []string `json:"common_traits"`
	BestChannels    []string `json:"best_channels"`
	Recommendations []string `json:"recommendations"`
}

// AnalyzeICP sends user profiles to an LLM to identify ideal customer patterns.
func AnalyzeICP(ctx context.Context, cfg *storage.LLMConfig, profiles []ICPProfile, projectDescription string) (*ICPAnalysis, error) {
	systemMsg := `You are a customer analytics expert. Analyze the given user profiles to identify Ideal Customer Profile (ICP) patterns.

Return ONLY valid JSON in this format:
{"summary": "2-3 sentence summary of who converts best", "common_traits": ["trait 1", "trait 2", ...], "best_channels": ["channel 1", ...], "recommendations": ["recommendation 1", ...]}`

	userMsg := buildICPPrompt(profiles, projectDescription)

	raw, err := chatComplete(ctx, cfg, systemMsg, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM ICP analysis: %w", err)
	}

	cleaned := extractJSON(raw)
	var result ICPAnalysis
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parsing ICP response: %w (raw: %s)", err, cleaned)
	}

	return &result, nil
}

func buildICPPrompt(profiles []ICPProfile, projectDescription string) string {
	var b strings.Builder

	if projectDescription != "" {
		b.WriteString("PRODUCT: ")
		b.WriteString(projectDescription)
		b.WriteString("\n\n")
	}

	b.WriteString("Here are the profiles of users who completed conversion actions:\n\n")
	for i, p := range profiles {
		if i >= 20 {
			b.WriteString("... and more\n")
			break
		}
		fmt.Fprintf(&b, "User %d: %d sessions, %d events", i+1, p.SessionCount, p.EventCount)
		if p.EntrySource != "" {
			fmt.Fprintf(&b, ", came from: %s", p.EntrySource)
		}
		if len(p.TopPages) > 0 {
			fmt.Fprintf(&b, ", visited: %s", strings.Join(p.TopPages, ", "))
		}
		if len(p.Properties) > 0 {
			props := make([]string, 0, len(p.Properties))
			for k, v := range p.Properties {
				props = append(props, k+"="+v)
			}
			fmt.Fprintf(&b, ", properties: %s", strings.Join(props, ", "))
		}
		b.WriteString("\n")
	}

	b.WriteString("\nIdentify the common patterns among these converting users. What defines the ideal customer?")
	return b.String()
}
