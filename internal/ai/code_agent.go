package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

// CodeAgent runs an AI agent loop with code search tools.
type CodeAgent struct {
	repoDir string // path to the local repo copy on disk
	cfg     *storage.LLMConfig
}

// NewCodeAgent creates a new code agent.
func NewCodeAgent(repoDir string, cfg *storage.LLMConfig) *CodeAgent {
	return &CodeAgent{repoDir: repoDir, cfg: cfg}
}

// tool definitions sent to the Anthropic API
var agentTools = []map[string]any{
	{
		"name":        "search_code",
		"description": "Search the codebase for a pattern. Returns matching lines with file paths and line numbers.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]string{
					"type":        "string",
					"description": "Search pattern (case-insensitive substring match)",
				},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "read_file",
		"description": "Read the contents of a source file.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{
					"type":        "string",
					"description": "File path relative to the repo root",
				},
			},
			"required": []string{"path"},
		},
	},
}

// skipDirs are directories to skip during code search.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	".next":        true,
	"__pycache__":  true,
	"vendor":       true,
}

// agentMessage represents a message in the Anthropic messages API.
type agentMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

// contentBlock represents a content block in the Anthropic API.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

// anthropicToolResponse is the response from the Anthropic messages API.
type anthropicToolResponse struct {
	Content  []contentBlock `json:"content"`
	StopReason string       `json:"stop_reason"`
}

// Run executes the agent loop with the given system prompt and user task.
// The agent can call search_code and read_file tools to explore the codebase.
// Returns the final text response.
func (a *CodeAgent) Run(ctx context.Context, systemPrompt, task string) (string, error) {
	apiKey := ""
	if a.cfg.APIKey != nil {
		apiKey = *a.cfg.APIKey
	}
	model := a.cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	baseURL := "https://api.anthropic.com"
	if a.cfg.BaseURL != nil && *a.cfg.BaseURL != "" {
		baseURL = strings.TrimRight(*a.cfg.BaseURL, "/")
	}

	messages := []agentMessage{
		{Role: "user", Content: task},
	}

	const maxIterations = 10

	for i := 0; i < maxIterations; i++ {
		resp, err := a.callAPI(ctx, baseURL, apiKey, model, systemPrompt, messages)
		if err != nil {
			return "", fmt.Errorf("agent API call %d: %w", i, err)
		}

		// Check if there are any tool_use blocks.
		var toolUses []contentBlock
		var textParts []string
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				toolUses = append(toolUses, block)
			} else if block.Type == "text" && block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		}

		// No tool calls -- return the text response.
		if len(toolUses) == 0 {
			return strings.Join(textParts, "\n"), nil
		}

		// Add the assistant message with tool_use blocks.
		messages = append(messages, agentMessage{
			Role:    "assistant",
			Content: resp.Content,
		})

		// Execute each tool and build tool_result blocks.
		var toolResults []contentBlock
		for _, tu := range toolUses {
			result := a.executeTool(tu.Name, tu.Input)
			toolResults = append(toolResults, contentBlock{
				Type:      "tool_result",
				ToolUseID: tu.ID,
				Content:   result,
			})
		}

		// Add tool results as a user message.
		messages = append(messages, agentMessage{
			Role:    "user",
			Content: toolResults,
		})
	}

	return "", fmt.Errorf("agent exceeded maximum iterations (%d)", maxIterations)
}

func (a *CodeAgent) callAPI(ctx context.Context, baseURL, apiKey, model, systemPrompt string, messages []agentMessage) (*anthropicToolResponse, error) {
	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"tools":      agentTools,
		"messages":   messages,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling anthropic: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result anthropicToolResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

func (a *CodeAgent) executeTool(name string, rawInput json.RawMessage) string {
	switch name {
	case "search_code":
		var input struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return fmt.Sprintf("error parsing input: %v", err)
		}
		return a.searchCode(input.Query)
	case "read_file":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(rawInput, &input); err != nil {
			return fmt.Sprintf("error parsing input: %v", err)
		}
		return a.readFile(input.Path)
	default:
		return fmt.Sprintf("unknown tool: %s", name)
	}
}

func (a *CodeAgent) searchCode(query string) string {
	if query == "" {
		return "error: empty query"
	}

	lowerQuery := strings.ToLower(query)
	var results []string
	maxResults := 20

	_ = filepath.WalkDir(a.repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip excluded directories.
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary/non-text files by extension.
		ext := strings.ToLower(filepath.Ext(path))
		if isBinaryExt(ext) {
			return nil
		}

		// Read file and search.
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(content), "\n")
		relPath, _ := filepath.Rel(a.repoDir, path)

		for lineNum, line := range lines {
			if strings.Contains(strings.ToLower(line), lowerQuery) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, lineNum+1, strings.TrimSpace(line)))
				if len(results) >= maxResults {
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if len(results) == 0 {
		return "no matches found"
	}
	return strings.Join(results, "\n")
}

func (a *CodeAgent) readFile(path string) string {
	// Sanitize path to prevent directory traversal.
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return "error: path traversal not allowed"
	}

	fullPath := filepath.Join(a.repoDir, cleanPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("error reading file: %v", err)
	}

	result := string(content)
	if len(result) > 4000 {
		result = result[:4000] + "\n... (truncated)"
	}
	return result
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg",
		".woff", ".woff2", ".ttf", ".eot",
		".zip", ".tar", ".gz", ".br",
		".exe", ".dll", ".so", ".dylib",
		".pdf", ".mp3", ".mp4", ".wav",
		".lock", ".map":
		return true
	}
	return false
}
