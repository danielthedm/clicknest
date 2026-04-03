package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

// ScoredMention holds a relevance score for a mention identified by its external ID.
type ScoredMention struct {
	ExternalID string  `json:"external_id"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
}

// MentionForScoring is a lightweight representation of a mention for the LLM.
type MentionForScoring struct {
	ExternalID string `json:"external_id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Subreddit  string `json:"subreddit"`
}

// ScoreMentionRelevance takes a batch of mentions and scores each one's
// relevance (0.0–1.0) to the product and ICP using the configured LLM.
// Keywords boost relevance. Returns scores for all mentions; the caller
// decides the threshold.
func ScoreMentionRelevance(ctx context.Context, cfg *storage.LLMConfig, mentions []MentionForScoring, productDesc, icpTraits string, keywords []string) ([]ScoredMention, error) {
	if len(mentions) == 0 {
		return nil, nil
	}

	// Truncate to avoid blowing up context windows.
	if len(mentions) > 25 {
		mentions = mentions[:25]
	}

	systemMsg := `You are a lead qualification expert. You will receive a list of Reddit posts and information about a product and its ideal customer profile. Score each post's relevance on a scale of 0.0 to 1.0:

- 1.0 = The person is actively looking for exactly this type of product
- 0.7-0.9 = Highly relevant discussion where the product could genuinely help
- 0.4-0.6 = Somewhat related topic, possible lead
- 0.1-0.3 = Tangentially related, unlikely lead
- 0.0 = Completely irrelevant

Return ONLY valid JSON in this format:
{"scores": [{"external_id": "abc123", "score": 0.85, "reason": "brief reason"}]}

Rules:
- Score based on how likely the author would benefit from the product
- Posts asking for recommendations or expressing pain points score highest
- Generic discussions score lower than specific questions
- Keyword matches in context boost the score
- Return a score for EVERY post provided`

	userMsg := buildRelevancePrompt(mentions, productDesc, icpTraits, keywords)

	raw, err := chatComplete(ctx, cfg, systemMsg, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM relevance scoring: %w", err)
	}

	cleaned := extractJSON(raw)
	var result struct {
		Scores []ScoredMention `json:"scores"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parsing relevance response: %w (raw: %s)", err, cleaned)
	}

	return result.Scores, nil
}

func buildRelevancePrompt(mentions []MentionForScoring, productDesc, icpTraits string, keywords []string) string {
	var b strings.Builder

	if productDesc != "" {
		b.WriteString("PRODUCT DESCRIPTION:\n")
		b.WriteString(productDesc)
		b.WriteString("\n\n")
	}

	if icpTraits != "" {
		b.WriteString("IDEAL CUSTOMER PROFILE:\n")
		b.WriteString(icpTraits)
		b.WriteString("\n\n")
	}

	if len(keywords) > 0 {
		fmt.Fprintf(&b, "KEYWORDS (boost relevance when present): %s\n\n", strings.Join(keywords, ", "))
	}

	b.WriteString("POSTS TO SCORE:\n\n")
	for i, m := range mentions {
		fmt.Fprintf(&b, "--- Post %d (ID: %s) ---\n", i+1, m.ExternalID)
		if m.Subreddit != "" {
			fmt.Fprintf(&b, "Subreddit: r/%s\n", m.Subreddit)
		}
		if m.Title != "" {
			fmt.Fprintf(&b, "Title: %s\n", m.Title)
		}
		content := m.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		if content != "" {
			fmt.Fprintf(&b, "Content: %s\n", content)
		}
		b.WriteString("\n")
	}

	return b.String()
}
