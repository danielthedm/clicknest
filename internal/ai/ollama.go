package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Ollama implements the Provider interface using a local Ollama instance.
type Ollama struct {
	model   string
	baseURL string
	client  *http.Client
}

// NewOllama creates an Ollama provider for self-hosted LLM inference.
func NewOllama(model, baseURL string) *Ollama {
	if model == "" {
		model = "llama3"
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Ollama{
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (o *Ollama) GenerateEventName(ctx context.Context, req NamingRequest) (*NamingResult, error) {
	prompt := systemPrompt + "\n\n" + buildPrompt(req)

	body := map[string]any{
		"model":  o.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.2,
			"num_predict": 100,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	name := strings.TrimSpace(result.Response)
	name = strings.Trim(name, "\"'`")
	// Take only the first line if multi-line.
	if idx := strings.IndexByte(name, '\n'); idx >= 0 {
		name = name[:idx]
	}

	return &NamingResult{
		Name:       name,
		Confidence: 0.6, // lower confidence for local models
		SourceFile: req.SourceFile,
	}, nil
}
