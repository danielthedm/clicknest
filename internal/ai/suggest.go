package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/danielleslie/clicknest/internal/storage"
)

// SuggestedFunnel is a funnel definition proposed by the LLM.
type SuggestedFunnel struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Steps       []storage.FunnelStep `json:"steps"`
}

// SuggestFunnels asks an LLM to propose funnel definitions based on observed event sequences.
func SuggestFunnels(ctx context.Context, cfg *storage.LLMConfig, sequences []storage.EventSequence) ([]SuggestedFunnel, error) {
	systemMsg := `You are an analytics funnel design assistant. Given a list of common event sequences observed in user sessions, suggest 2-4 meaningful conversion funnels.

Return ONLY valid JSON in this format:
{"suggestions": [{"name": "Funnel Name", "description": "What this funnel measures", "steps": [{"event_type": "pageview", "event_name": "optional name"}, ...]}]}

Rules:
- Each funnel must have 2-5 steps
- Focus on sequences that represent meaningful user journeys (signup, purchase, onboarding, etc.)
- Use descriptive funnel names
- The description should explain what conversion this funnel tracks
- event_type must be one of: pageview, click, submit, input, custom
- event_name can be empty string if the sequence only uses event_type`

	userMsg := buildSuggestPrompt(sequences)

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

func buildSuggestPrompt(sequences []storage.EventSequence) string {
	var b strings.Builder
	b.WriteString("Here are the most common event sequences observed in user sessions:\n\n")
	for i, seq := range sequences {
		var parts []string
		for _, step := range seq.Steps {
			label := step.EventType
			if step.EventName != "" {
				label += ":" + step.EventName
			}
			parts = append(parts, label)
		}
		fmt.Fprintf(&b, "%d. %s (seen in %d sessions)\n", i+1, strings.Join(parts, " → "), seq.SessionCount)
	}
	b.WriteString("\nSuggest 2-4 meaningful conversion funnels based on these patterns.")
	return b.String()
}

// chatComplete dispatches a chat completion request to the configured LLM provider.
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
		"max_tokens":  800,
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
		"max_tokens": 800,
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
