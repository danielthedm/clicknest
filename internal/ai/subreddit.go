package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

// SubredditSuggestion represents a single AI-suggested subreddit.
type SubredditSuggestion struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// SuggestSubreddits asks the configured LLM to suggest relevant subreddits
// where the target audience for the given product is likely active.
func SuggestSubreddits(ctx context.Context, cfg *storage.LLMConfig, projectDesc string, icpTraits string) ([]SubredditSuggestion, error) {
	systemMsg := `You are an expert Reddit community analyst. Given a product description and target customer traits, suggest 5-10 subreddits where the target audience is most likely active and where the product would be relevant.

Return ONLY valid JSON in this format:
{"subreddits": [{"name": "selfhosted", "reason": "Community of users who self-host software and services"}]}

Rules:
- Do NOT include the r/ prefix in subreddit names
- Include a mix of niche, focused subreddits and broader ones
- Only suggest real, active subreddits
- The reason should explain why the target audience frequents that subreddit
- Order from most relevant to least relevant`

	var b strings.Builder
	b.WriteString("Suggest subreddits for this product:\n\n")

	if projectDesc != "" {
		b.WriteString("PRODUCT DESCRIPTION:\n")
		b.WriteString(projectDesc)
		b.WriteString("\n\n")
	}

	if icpTraits != "" {
		b.WriteString("TARGET CUSTOMER TRAITS:\n")
		b.WriteString(icpTraits)
		b.WriteString("\n\n")
	}

	raw, err := chatComplete(ctx, cfg, systemMsg, b.String())
	if err != nil {
		return nil, fmt.Errorf("LLM subreddit suggestion: %w", err)
	}

	cleaned := extractJSON(raw)
	var result struct {
		Subreddits []SubredditSuggestion `json:"subreddits"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parsing subreddit suggestions: %w (raw: %s)", err, cleaned)
	}

	return result.Subreddits, nil
}
