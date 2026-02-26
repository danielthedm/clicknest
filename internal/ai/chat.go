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

// ChatMessage is a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"`
}

// ChatWithHistory sends a multi-turn chat to the configured LLM provider.
func ChatWithHistory(ctx context.Context, cfg *storage.LLMConfig, systemMsg string, history []ChatMessage) (string, error) {
	switch cfg.Provider {
	case "openai":
		return openaiChatHistory(ctx, cfg, systemMsg, history)
	case "anthropic":
		return anthropicChatHistory(ctx, cfg, systemMsg, history)
	case "ollama":
		return ollamaChatHistory(ctx, cfg, systemMsg, history)
	default:
		return "", fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func openaiChatHistory(ctx context.Context, cfg *storage.LLMConfig, systemMsg string, history []ChatMessage) (string, error) {
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

	messages := []map[string]string{{"role": "system", "content": systemMsg}}
	for _, m := range history {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": 0.5,
		"max_tokens":  1200,
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

func anthropicChatHistory(ctx context.Context, cfg *storage.LLMConfig, systemMsg string, history []ChatMessage) (string, error) {
	apiKey := ""
	if cfg.APIKey != nil {
		apiKey = *cfg.APIKey
	}
	model := cfg.Model
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	baseURL := "https://api.anthropic.com"
	if cfg.BaseURL != nil && *cfg.BaseURL != "" {
		baseURL = strings.TrimRight(*cfg.BaseURL, "/")
	}

	messages := []map[string]string{}
	for _, m := range history {
		messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": 1200,
		"system":     systemMsg,
		"messages":   messages,
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

func ollamaChatHistory(ctx context.Context, cfg *storage.LLMConfig, systemMsg string, history []ChatMessage) (string, error) {
	model := cfg.Model
	if model == "" {
		model = "llama3"
	}
	baseURL := "http://localhost:11434"
	if cfg.BaseURL != nil && *cfg.BaseURL != "" {
		baseURL = strings.TrimRight(*cfg.BaseURL, "/")
	}

	var sb strings.Builder
	sb.WriteString(systemMsg)
	sb.WriteString("\n\n")
	for _, m := range history {
		if m.Role == "user" {
			sb.WriteString("User: ")
		} else {
			sb.WriteString("Assistant: ")
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}

	body := map[string]any{
		"model":  model,
		"prompt": sb.String(),
		"stream": false,
		"options": map[string]any{
			"temperature": 0.5,
			"num_predict": 1200,
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
