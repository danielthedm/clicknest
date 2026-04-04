package github

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/danielthedm/clicknest/internal/ai"
	"github.com/danielthedm/clicknest/internal/storage"
)

// SourceLink represents a link to a source file on GitHub.
type SourceLink struct {
	FilePath  string `json:"file_path"`
	GitHubURL string `json:"github_url"`
	Line      int    `json:"line"`
}

// SourceMatch represents a matched source file for a DOM element.
type SourceMatch struct {
	FilePath      string
	ComponentName string
	Score         float64
}

// Matcher finds source files that correspond to DOM elements.
type Matcher struct {
	meta *storage.SQLite
}

// NewMatcher creates a source file matcher.
func NewMatcher(meta *storage.SQLite) *Matcher {
	return &Matcher{meta: meta}
}

// MatchAndFetch finds the best source file for a DOM element, then fetches its content
// from GitHub. Uses two strategies:
// 1. URL path → route file mapping (works for SvelteKit, Next.js, etc.)
// 2. CSS selector matching (works for React apps with semantic IDs/classes)
// Satisfies ai.SourceMatcher interface.
func (m *Matcher) MatchAndFetch(ctx context.Context, projectID, elementID, elementClasses, parentPath, urlPath string) (sourceCode, sourceFile string, ok bool) {
	// Strategy 1: AI-powered file selection — send the file list to the LLM
	// and let it pick the most relevant file based on all available context.
	if match, err := m.MatchWithAI(ctx, projectID, elementID, elementClasses, parentPath, urlPath); err == nil && match != nil {
		code, file, found := m.fetchSource(ctx, projectID, match)
		if found {
			return code, file, true
		}
	}

	// Strategy 2: Try route-based matching (URL path → component file).
	if urlPath != "" {
		match, err := m.MatchByRoute(ctx, projectID, urlPath)
		if err == nil && match != nil {
			code, file, found := m.fetchSource(ctx, projectID, match)
			if found {
				return code, file, true
			}
		}
	}

	// Strategy 3: Fall back to selector-based matching.
	match, err := m.Match(ctx, projectID, elementID, elementClasses, parentPath)
	if err != nil || match == nil {
		return "", "", false
	}

	return m.fetchSource(ctx, projectID, match)
}

// MatchWithAI asks the LLM to pick the most relevant source file from the
// indexed file list, given the DOM element context. This works for any
// framework since the AI understands file naming conventions.
func (m *Matcher) MatchWithAI(ctx context.Context, projectID, elementID, elementClasses, parentPath, urlPath string) (*SourceMatch, error) {
	// Get LLM config.
	cfg, err := m.meta.GetLLMConfig(ctx, projectID)
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("no LLM config")
	}

	// Get all indexed file paths.
	rows, err := m.meta.DB().QueryContext(ctx,
		`SELECT file_path, component_name FROM source_index WHERE project_id = ?`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type indexedFile struct {
		path string
		name string
	}
	var files []indexedFile
	for rows.Next() {
		var fp string
		var cn sql.NullString
		if err := rows.Scan(&fp, &cn); err != nil {
			continue
		}
		name := ""
		if cn.Valid {
			name = cn.String
		}
		files = append(files, indexedFile{path: fp, name: name})
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no indexed files")
	}

	// Build a compact file list for the prompt.
	var fileList strings.Builder
	for _, f := range files {
		fileList.WriteString(f.path)
		fileList.WriteString("\n")
	}

	prompt := fmt.Sprintf(`Given this UI element context, which source file is most likely to contain the component that renders this element?

Element tag: %s
Element ID: %s
Element classes: %s
Element text context: %s
Page URL path: %s
DOM path: %s

SOURCE FILES IN THE REPO:
%s
Reply with ONLY the file path, nothing else. If no file is a good match, reply "none".`,
		"", elementID, elementClasses, "", urlPath, parentPath, fileList.String())

	raw, err := ai.ChatComplete(ctx, cfg, "You are a source code expert. Given a UI element's DOM context and a list of source files, identify which file contains the component that renders this element. Reply with only the file path.", prompt)
	if err != nil {
		return nil, err
	}

	result := strings.TrimSpace(raw)
	result = strings.Trim(result, "`\"'")
	if result == "" || result == "none" {
		return nil, nil
	}

	// Find the matching file in our list.
	for _, f := range files {
		if f.path == result || strings.HasSuffix(f.path, result) || strings.HasSuffix(result, f.path) {
			return &SourceMatch{
				FilePath:      f.path,
				ComponentName: f.name,
				Score:         1.0,
			}, nil
		}
	}

	return nil, nil
}

// fetchSource retrieves the source code for a matched file from GitHub.
func (m *Matcher) fetchSource(ctx context.Context, projectID string, match *SourceMatch) (sourceCode, sourceFile string, ok bool) {
	conn, err := m.meta.GetGitHubConnection(ctx, projectID)
	if err != nil {
		return "", "", false
	}

	client := NewClient(conn.AccessToken)
	content, err := client.GetFileContent(ctx, conn.RepoOwner, conn.RepoName, match.FilePath, conn.DefaultBranch)
	if err != nil {
		return "", "", false
	}

	if len(content) > 3000 {
		content = content[:3000] + "\n// ... truncated"
	}

	return content, match.FilePath, true
}

// MatchByRoute finds the source file that corresponds to a URL path using
// framework routing conventions (SvelteKit, Next.js, Nuxt, etc.).
func (m *Matcher) MatchByRoute(ctx context.Context, projectID, urlPath string) (*SourceMatch, error) {
	// Clean the URL path.
	urlPath = strings.TrimRight(urlPath, "/")
	if urlPath == "" {
		urlPath = "/"
	}

	rows, err := m.meta.DB().QueryContext(ctx,
		`SELECT file_path, component_name FROM source_index WHERE project_id = ?`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bestMatch *SourceMatch
	var bestScore int

	for rows.Next() {
		var filePath string
		var componentName sql.NullString
		if err := rows.Scan(&filePath, &componentName); err != nil {
			continue
		}

		score := routeMatchScore(filePath, urlPath)
		if score > bestScore {
			bestScore = score
			name := ""
			if componentName.Valid {
				name = componentName.String
			}
			bestMatch = &SourceMatch{
				FilePath:      filePath,
				ComponentName: name,
				Score:         float64(score),
			}
		}
	}

	if bestMatch == nil || bestScore < 1 {
		return nil, nil
	}
	return bestMatch, nil
}

// routeMatchScore computes how well a source file path matches a URL route.
// Higher score = better match.
func routeMatchScore(filePath, urlPath string) int {
	lower := strings.ToLower(filePath)

	// SvelteKit: src/routes/people/sessions/+page.svelte → /people/sessions
	if strings.Contains(lower, "+page.svelte") || strings.Contains(lower, "+page.ts") || strings.Contains(lower, "+page.server") {
		routePath := svelteKitRoute(filePath)
		if routePath == urlPath {
			return 10
		}
		// Partial match (prefix).
		if urlPath != "/" && strings.HasPrefix(urlPath, routePath) {
			return 5
		}
	}

	// Next.js: app/people/sessions/page.tsx or pages/people/sessions.tsx
	if strings.Contains(lower, "/page.tsx") || strings.Contains(lower, "/page.jsx") || strings.Contains(lower, "/page.ts") {
		routePath := nextJSRoute(filePath)
		if routePath == urlPath {
			return 10
		}
		if urlPath != "/" && strings.HasPrefix(urlPath, routePath) {
			return 5
		}
	}

	// pages/ directory (Next.js pages router, Nuxt)
	if strings.Contains(lower, "pages/") && !strings.Contains(lower, "+page") {
		routePath := pagesRoute(filePath)
		if routePath == urlPath {
			return 10
		}
	}

	return 0
}

// svelteKitRoute extracts the URL path from a SvelteKit route file path.
// e.g. "web/src/routes/people/sessions/+page.svelte" → "/people/sessions"
// e.g. "src/routes/analytics/events/+page.svelte" → "/analytics/events"
func svelteKitRoute(filePath string) string {
	// Find "routes/" in the path.
	idx := strings.Index(strings.ToLower(filePath), "routes/")
	if idx < 0 {
		return ""
	}
	routePart := filePath[idx+7:] // after "routes/"

	// Remove the filename (+page.svelte, +page.ts, etc.)
	dir := path.Dir(routePart)
	if dir == "." {
		return "/"
	}
	return "/" + dir
}

// nextJSRoute extracts URL path from Next.js app router file path.
// e.g. "app/people/sessions/page.tsx" → "/people/sessions"
func nextJSRoute(filePath string) string {
	idx := strings.Index(strings.ToLower(filePath), "app/")
	if idx < 0 {
		return ""
	}
	routePart := filePath[idx+4:]
	dir := path.Dir(routePart)
	if dir == "." {
		return "/"
	}
	return "/" + dir
}

// pagesRoute extracts URL path from pages-based routing.
// e.g. "pages/people/sessions.tsx" → "/people/sessions"
func pagesRoute(filePath string) string {
	idx := strings.Index(strings.ToLower(filePath), "pages/")
	if idx < 0 {
		return ""
	}
	routePart := filePath[idx+6:]
	ext := path.Ext(routePart)
	routePart = strings.TrimSuffix(routePart, ext)
	if strings.HasSuffix(routePart, "/index") {
		routePart = strings.TrimSuffix(routePart, "/index")
	}
	if routePart == "index" {
		return "/"
	}
	return "/" + routePart
}

// Match finds the best source file for the given DOM context using selectors.
func (m *Matcher) Match(ctx context.Context, projectID string, elementID, elementClasses, parentPath string) (*SourceMatch, error) {
	rows, err := m.meta.DB().QueryContext(ctx,
		`SELECT file_path, component_name, selectors FROM source_index WHERE project_id = ?`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bestMatch *SourceMatch
	var bestScore float64

	for rows.Next() {
		var filePath, selectors string
		var componentName sql.NullString
		if err := rows.Scan(&filePath, &componentName, &selectors); err != nil {
			continue
		}

		score := computeMatchScore(selectors, elementID, elementClasses, parentPath)
		if score > bestScore {
			bestScore = score
			name := ""
			if componentName.Valid {
				name = componentName.String
			}
			bestMatch = &SourceMatch{
				FilePath:      filePath,
				ComponentName: name,
				Score:         score,
			}
		}
	}

	if bestMatch == nil || bestScore < 0.1 {
		return nil, nil
	}
	return bestMatch, nil
}

// MatchSourceFile attempts to find a source file in the indexed repo that matches
// the given error source URL and line number, returning a GitHub link if found.
func (m *Matcher) MatchSourceFile(ctx context.Context, projectID, sourceURL string, lineno int) (*SourceLink, error) {
	if sourceURL == "" {
		return nil, nil
	}

	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return nil, nil
	}
	filename := path.Base(parsed.Path)
	if filename == "" || filename == "." || filename == "/" {
		return nil, nil
	}

	rows, err := m.meta.DB().QueryContext(ctx,
		`SELECT file_path FROM source_index WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("querying source index: %w", err)
	}
	defer rows.Close()

	var bestPath string
	var bestScore int
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			continue
		}
		score := pathSuffixScore(fp, parsed.Path)
		if score > bestScore {
			bestScore = score
			bestPath = fp
		}
	}

	if bestPath == "" {
		return nil, nil
	}

	conn, err := m.meta.GetGitHubConnection(ctx, projectID)
	if err != nil {
		return nil, nil
	}

	ghURL := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s",
		conn.RepoOwner, conn.RepoName, conn.DefaultBranch, bestPath)
	if lineno > 0 {
		ghURL += fmt.Sprintf("#L%d", lineno)
	}

	return &SourceLink{
		FilePath:  bestPath,
		GitHubURL: ghURL,
		Line:      lineno,
	}, nil
}

func pathSuffixScore(candidate, sourcePath string) int {
	cParts := strings.Split(candidate, "/")
	sParts := strings.Split(sourcePath, "/")

	score := 0
	ci := len(cParts) - 1
	si := len(sParts) - 1
	for ci >= 0 && si >= 0 {
		if strings.EqualFold(cParts[ci], sParts[si]) {
			score++
			ci--
			si--
		} else {
			break
		}
	}
	return score
}

func computeMatchScore(selectors, elementID, elementClasses, parentPath string) float64 {
	score := 0.0
	selectorLower := strings.ToLower(selectors)

	if elementID != "" && strings.Contains(selectorLower, strings.ToLower(elementID)) {
		score += 0.5
	}

	for _, class := range strings.Fields(elementClasses) {
		if strings.Contains(selectorLower, strings.ToLower(class)) {
			score += 0.2
		}
	}

	pathParts := strings.Split(parentPath, ">")
	for _, part := range pathParts {
		part = strings.TrimSpace(part)
		if part != "" && strings.Contains(selectorLower, strings.ToLower(part)) {
			score += 0.1
		}
	}

	return score
}
