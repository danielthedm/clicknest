package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

type CampaignContent struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	URL   string   `json:"url,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

type CampaignContext struct {
	ProjectDescription string
	TopPages           []storage.PageStat
	TopEvents          []storage.EventNameStat
	ICPTraits          string
	RefURL             string
	Channel            string
	Topic              string
}

// GenerateCampaign creates channel-specific marketing content using AI.
func GenerateCampaign(ctx context.Context, cfg *storage.LLMConfig, cc CampaignContext) (*CampaignContent, error) {
	systemMsg := channelSystemPrompt(cc.Channel)
	userMsg := buildCampaignPrompt(cc)

	raw, err := chatComplete(ctx, cfg, systemMsg, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM campaign generation: %w", err)
	}

	cleaned := extractJSON(raw)
	var result CampaignContent
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parsing campaign response: %w (raw: %s)", err, cleaned)
	}

	if cc.RefURL != "" && result.URL == "" {
		result.URL = cc.RefURL
	}

	return &result, nil
}

// GenerateVariations creates N alternative versions of existing campaign content.
func GenerateVariations(ctx context.Context, cfg *storage.LLMConfig, original CampaignContent, channel string, count int) ([]CampaignContent, error) {
	if count <= 0 {
		count = 2
	}

	systemMsg := fmt.Sprintf(`You are a marketing copywriter. Generate %d alternative variations of the given content for %s.
Each variation should have a different angle, hook, or CTA while conveying the same core message.

Return ONLY valid JSON in this format:
{"variations": [{"title": "...", "body": "...", "url": "...", "tags": ["..."]}]}`, count, channel)

	originalJSON, _ := json.Marshal(original)
	userMsg := fmt.Sprintf("Create %d variations of this content:\n\n%s", count, string(originalJSON))

	raw, err := chatComplete(ctx, cfg, systemMsg, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM variation generation: %w", err)
	}

	cleaned := extractJSON(raw)
	var result struct {
		Variations []CampaignContent `json:"variations"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parsing variations response: %w (raw: %s)", err, cleaned)
	}

	return result.Variations, nil
}

func channelSystemPrompt(channel string) string {
	base := `You are a growth marketing content creator. Generate compelling content based on the product context provided.
Return ONLY valid JSON in this format:
{"title": "...", "body": "...", "url": "...", "tags": ["..."]}`

	switch channel {
	case "reddit":
		return base + "\n\nStyle: Casual, authentic, conversational. Reddit users hate obvious marketing. Lead with value, mention the product naturally. No emojis. Title should be a genuine question or insight."
	case "linkedin":
		return base + "\n\nStyle: Professional, data-driven, thought-leadership. Use industry terminology. Include a clear CTA. Structure with line breaks for readability."
	case "twitter":
		return base + "\n\nStyle: Concise (under 280 chars for body). Punchy, attention-grabbing. Use 1-2 relevant hashtags in tags. Title can be empty."
	case "youtube":
		return base + "\n\nStyle: SEO-optimized title. Body is the video description with timestamps placeholder and key points. Tags should be search-friendly keywords."
	case "blog":
		return base + "\n\nStyle: Long-form outline. Title should be SEO-friendly. Body should be a structured blog post outline with headers (##) and key points under each. Tags are topic keywords."
	default:
		return base
	}
}

func buildCampaignPrompt(cc CampaignContext) string {
	var b strings.Builder

	b.WriteString("Generate marketing content for the following product:\n\n")

	if cc.ProjectDescription != "" {
		b.WriteString("PRODUCT DESCRIPTION:\n")
		b.WriteString(cc.ProjectDescription)
		b.WriteString("\n\n")
	}

	if len(cc.TopPages) > 0 {
		b.WriteString("TOP PAGES (by traffic):\n")
		for i, p := range cc.TopPages {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "- %s (%d views)\n", p.Path, p.Views)
		}
		b.WriteString("\n")
	}

	if len(cc.TopEvents) > 0 {
		b.WriteString("KEY USER ACTIONS:\n")
		for i, e := range cc.TopEvents {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "- %s (%d occurrences)\n", e.Name, e.Count)
		}
		b.WriteString("\n")
	}

	if cc.ICPTraits != "" {
		b.WriteString("TARGET AUDIENCE TRAITS:\n")
		b.WriteString(cc.ICPTraits)
		b.WriteString("\n\n")
	}

	if cc.RefURL != "" {
		fmt.Fprintf(&b, "LINK TO INCLUDE: %s\n\n", cc.RefURL)
	}

	if cc.Topic != "" {
		fmt.Fprintf(&b, "TOPIC/ANGLE: %s\n\n", cc.Topic)
	}

	fmt.Fprintf(&b, "CHANNEL: %s\n", cc.Channel)

	return b.String()
}
