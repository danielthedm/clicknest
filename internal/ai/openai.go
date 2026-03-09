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

// OpenAI implements the Provider interface using OpenAI's chat completions API.
type OpenAI struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAI creates an OpenAI provider.
func NewOpenAI(apiKey, model, baseURL string) *OpenAI {
	if model == "" {
		model = "gpt-4o-mini"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAI{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (o *OpenAI) GenerateEventName(ctx context.Context, req NamingRequest) (*NamingResult, error) {
	prompt := buildPrompt(req)

	body := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
		"max_tokens":  100,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("calling openai: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	name := strings.TrimSpace(result.Choices[0].Message.Content)
	// Remove quotes if the model wrapped the name.
	name = strings.Trim(name, "\"'`")

	return &NamingResult{
		Name:       name,
		Confidence: 0.8,
		SourceFile: req.SourceFile,
	}, nil
}

const systemPrompt = `You name analytics events. Given DOM context about a user interaction, output a short action name.

Rules:
- Use Title Case, 2-5 words max (e.g. "Toggle Pending Suggestions", "Click Company: Meta", "Open Website Link")
- DO NOT start with "User clicked", "User viewed", or any subject prefix
- For clicks: start with the action verb (Click, Toggle, Open, Select, Expand, Close, Submit)
- For navigation/links: "Open [target]" or "Click [label] Link"
- For toggles/switches: "Toggle [label]"
- Include the element's visible text or aria-label as the object
- Do NOT include the page name or URL — keep it element-focused
- If the element text is a proper noun or entity name, include it (e.g. "Click Company: Meta")
- Only output the name, nothing else`

func buildPrompt(req NamingRequest) string {
	var b strings.Builder
	b.WriteString("Generate a human-readable event name for this interaction:\n\n")

	if req.ElementTag != "" {
		fmt.Fprintf(&b, "Element: <%s>\n", req.ElementTag)
	}
	if req.ElementID != "" {
		fmt.Fprintf(&b, "ID: %s\n", req.ElementID)
	}
	if req.ElementClasses != "" {
		fmt.Fprintf(&b, "Classes: %s\n", req.ElementClasses)
	}
	if req.ElementText != "" {
		fmt.Fprintf(&b, "Text: %s\n", req.ElementText)
	}
	if req.AriaLabel != "" {
		fmt.Fprintf(&b, "Aria Label: %s\n", req.AriaLabel)
	}
	if req.ParentPath != "" {
		fmt.Fprintf(&b, "DOM Path: %s\n", req.ParentPath)
	}
	if req.URLPath != "" {
		fmt.Fprintf(&b, "Page: %s\n", req.URLPath)
	}
	if req.PageTitle != "" {
		fmt.Fprintf(&b, "Page Title: %s\n", req.PageTitle)
	}
	if req.SourceCode != "" {
		fmt.Fprintf(&b, "\nSource code (from %s):\n```\n%s\n```\n", req.SourceFile, req.SourceCode)
	}

	return b.String()
}
