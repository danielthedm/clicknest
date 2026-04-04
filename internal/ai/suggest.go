package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

// SuggestedFunnel is a funnel definition proposed by the LLM.
type SuggestedFunnel struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Steps       []storage.FunnelStep `json:"steps"`
}

// SuggestFunnels asks an LLM to propose funnel definitions based on observed event sequences,
// product context, named events, and source code structure.
// If repoDir is non-empty and points to a synced repo on disk with an Anthropic provider,
// a CodeAgent is used to gather deeper codebase context first.
func SuggestFunnels(ctx context.Context, cfg *storage.LLMConfig, sequences []storage.EventSequence, productDesc string, namedEvents []storage.EventName, sourceFiles []string, repoDir string) ([]SuggestedFunnel, error) {
	// If we have a local repo and an Anthropic provider, use the CodeAgent
	// to gather richer codebase context before suggesting funnels.
	var codeContext string
	if repoDir != "" && cfg.Provider == "anthropic" {
		if _, err := os.Stat(repoDir); err == nil {
			agent := NewCodeAgent(repoDir, cfg)
			agentTask := "Analyze this codebase and identify the key user journeys, pages, features, and conversion points. List the most important user flows."
			agentSystem := "You are a code analyst. Explore the codebase using the search and read tools to understand the application's structure, pages, features, and user flows. Be thorough but concise in your final summary."
			result, err := agent.Run(ctx, agentSystem, agentTask)
			if err != nil {
				log.Printf("WARN code agent failed: %v", err)
			} else {
				codeContext = result
			}
		}
	}

	systemMsg := `You are a senior product analytics consultant. Given event data, product context, and source code structure, design conversion funnels that measure meaningful business outcomes.

Return ONLY valid JSON in this format:
{"suggestions": [{"name": "Funnel Name", "description": "What this funnel measures and why it matters", "steps": [{"event_type": "pageview", "event_name": "optional name"}, ...]}]}

Rules:
- Suggest 3-5 funnels ranging from basic to advanced
- Each funnel must have 3-6 steps
- Focus on BUSINESS-CRITICAL journeys: onboarding completion, feature adoption, upgrade paths, aha moments
- Use the named events and source code routes to understand what the app actually does
- event_type must be one of: pageview, click, submit, input, custom
- event_name should use the AI-named event names when available
- Include at least one funnel that measures the product's core value delivery
- Include at least one funnel that measures activation (new user → first value moment)
- Descriptions should explain WHY this funnel matters for the business, not just what it tracks`

	userMsg := buildSuggestPrompt(sequences, productDesc, namedEvents, sourceFiles)

	// Append code agent context if available.
	if codeContext != "" {
		userMsg += "\n\nDEEP CODEBASE ANALYSIS (from automated code review):\n" + codeContext
	}

	raw, err := chatComplete(ctx, cfg, systemMsg, userMsg)
	if err != nil {
		return nil, fmt.Errorf("LLM chat completion: %w", err)
	}

	cleaned := extractJSON(raw)

	var result struct {
		Suggestions []SuggestedFunnel `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parsing LLM response: %w (raw: %s)", err, cleaned)
	}

	return result.Suggestions, nil
}

func buildSuggestPrompt(sequences []storage.EventSequence, productDesc string, namedEvents []storage.EventName, sourceFiles []string) string {
	var b strings.Builder

	if productDesc != "" {
		b.WriteString("PRODUCT DESCRIPTION:\n")
		b.WriteString(productDesc)
		b.WriteString("\n\n")
	}

	// Show route structure so the AI understands the app's pages/features.
	if len(sourceFiles) > 0 {
		b.WriteString("APP ROUTES & SOURCE FILES (shows the product's feature structure):\n")
		for _, f := range sourceFiles {
			// Only show route files, not every source file.
			if strings.Contains(f, "routes/") || strings.Contains(f, "pages/") || strings.Contains(f, "app/") {
				b.WriteString("  " + f + "\n")
			}
		}
		b.WriteString("\n")
	}

	// Show named events (capped to avoid blowing up context).
	if len(namedEvents) > 0 {
		b.WriteString("NAMED EVENTS (AI-identified user interactions):\n")
		cap := 30
		if len(namedEvents) < cap {
			cap = len(namedEvents)
		}
		for _, en := range namedEvents[:cap] {
			name := en.AIName
			if en.UserName != nil && *en.UserName != "" {
				name = *en.UserName
			}
			fmt.Fprintf(&b, "  - %s\n", name)
		}
		b.WriteString("\n")
	}

	b.WriteString("OBSERVED EVENT SEQUENCES (most common user journeys):\n\n")
	for i, seq := range sequences {
		var parts []string
		for _, step := range seq.Steps {
			label := step.EventType
			if step.EventName != "" {
				label += ": " + step.EventName
			}
			parts = append(parts, label)
		}
		fmt.Fprintf(&b, "%d. %s (seen in %d sessions)\n", i+1, strings.Join(parts, " → "), seq.SessionCount)
	}

	b.WriteString("\nDesign 3-5 conversion funnels that measure meaningful business outcomes for this product.")
	return b.String()
}

// chatComplete dispatches a chat completion request to the configured LLM provider.
// ChatComplete sends a system+user message to the configured LLM provider.
func ChatComplete(ctx context.Context, cfg *storage.LLMConfig, systemMsg, userMsg string) (string, error) {
	return chatComplete(ctx, cfg, systemMsg, userMsg)
}

func chatComplete(ctx context.Context, cfg *storage.LLMConfig, systemMsg, userMsg string) (string, error) {
	switch cfg.Provider {
	case "openai":
		return openaiChat(ctx, cfg, systemMsg, userMsg)
	case "anthropic":
		return anthropicChat(ctx, cfg, systemMsg, userMsg)
	case "ollama":
		return ollamaChat(ctx, cfg, systemMsg, userMsg)
	default:
		return "", fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func openaiChat(ctx context.Context, cfg *storage.LLMConfig, systemMsg, userMsg string) (string, error) {
	apiKey := ""
	if cfg.APIKey != nil {
		apiKey = *cfg.APIKey
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	baseURL := "https://api.openai.com/v1"
	if cfg.BaseURL != nil && *cfg.BaseURL != "" {
		baseURL = strings.TrimRight(*cfg.BaseURL, "/")
	}

	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemMsg},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0.3,
		"max_tokens":  2000,
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling openai: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading openai response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func anthropicChat(ctx context.Context, cfg *storage.LLMConfig, systemMsg, userMsg string) (string, error) {
	apiKey := ""
	if cfg.APIKey != nil {
		apiKey = *cfg.APIKey
	}
	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	baseURL := "https://api.anthropic.com"
	if cfg.BaseURL != nil && *cfg.BaseURL != "" {
		baseURL = strings.TrimRight(*cfg.BaseURL, "/")
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 2000,
		"system":     systemMsg,
		"messages": []map[string]string{
			{"role": "user", "content": userMsg},
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling anthropic: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading anthropic response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}
	return strings.TrimSpace(result.Content[0].Text), nil
}

func ollamaChat(ctx context.Context, cfg *storage.LLMConfig, systemMsg, userMsg string) (string, error) {
	model := cfg.Model
	if model == "" {
		model = "llama3"
	}
	baseURL := "http://localhost:11434"
	if cfg.BaseURL != nil && *cfg.BaseURL != "" {
		baseURL = strings.TrimRight(*cfg.BaseURL, "/")
	}

	body := map[string]any{
		"model":  model,
		"prompt": systemMsg + "\n\n" + userMsg,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.3,
			"num_predict": 800,
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/generate", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading ollama response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	return strings.TrimSpace(result.Response), nil
}

// extractJSON strips markdown code fences from LLM output.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
