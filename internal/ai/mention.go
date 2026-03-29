package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

// MentionDraftContext holds the information needed to draft a reply to a mention.
type MentionDraftContext struct {
	ProjectDescription string
	MentionTitle       string
	MentionContent     string
	MentionAuthor      string
	Platform           string
	ICPTraits          string
	ProductURL         string
}

// DraftMentionReply generates a contextual reply to an external mention using AI.
func DraftMentionReply(ctx context.Context, cfg *storage.LLMConfig, mc MentionDraftContext) (string, error) {
	systemMsg := mentionSystemPrompt(mc.Platform)
	userMsg := buildMentionPrompt(mc)

	raw, err := chatComplete(ctx, cfg, systemMsg, userMsg)
	if err != nil {
		return "", fmt.Errorf("LLM mention reply: %w", err)
	}

	cleaned := extractJSON(raw)
	var result struct {
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		// Fall back to raw text if not JSON.
		return strings.TrimSpace(raw), nil
	}
	return result.Reply, nil
}

func mentionSystemPrompt(platform string) string {
	base := `You are a helpful growth marketer. A user on an external platform posted something relevant to the product you represent. Draft a genuine, helpful reply that addresses their question or pain point, and naturally mentions how the product could help.

Return ONLY valid JSON in this format:
{"reply": "your reply text here"}

Rules:
- Be genuinely helpful first, promotional second
- Never sound like a bot or a sales pitch
- Match the tone and norms of the platform
- Keep it concise and on-topic
- If the post is not a good fit for the product, say so honestly`

	switch platform {
	case "reddit":
		return base + "\n\nPlatform: Reddit. Be casual and conversational. Redditors despise obvious marketing. Lead with a helpful answer. If you mention the product, be transparent (e.g. 'disclaimer: I work on this'). No emojis. No corporate speak."
	case "twitter":
		return base + "\n\nPlatform: Twitter/X. Keep the reply under 280 characters. Be punchy and direct. One clear point. Hashtags only if relevant."
	case "linkedin":
		return base + "\n\nPlatform: LinkedIn. Professional but personable tone. Use data or specific experience to support your point. End with a natural invitation to learn more."
	default:
		return base
	}
}

func buildMentionPrompt(mc MentionDraftContext) string {
	var b strings.Builder

	b.WriteString("Draft a reply to this post:\n\n")

	if mc.MentionTitle != "" {
		fmt.Fprintf(&b, "TITLE: %s\n", mc.MentionTitle)
	}
	fmt.Fprintf(&b, "CONTENT: %s\n", mc.MentionContent)
	if mc.MentionAuthor != "" {
		fmt.Fprintf(&b, "AUTHOR: %s\n", mc.MentionAuthor)
	}
	b.WriteString("\n")

	if mc.ProjectDescription != "" {
		b.WriteString("YOUR PRODUCT:\n")
		b.WriteString(mc.ProjectDescription)
		b.WriteString("\n\n")
	}

	if mc.ProductURL != "" {
		fmt.Fprintf(&b, "PRODUCT URL: %s\n\n", mc.ProductURL)
	}

	if mc.ICPTraits != "" {
		b.WriteString("TARGET CUSTOMER TRAITS:\n")
		b.WriteString(mc.ICPTraits)
		b.WriteString("\n\n")
	}

	return b.String()
}
