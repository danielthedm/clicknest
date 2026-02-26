package github

import (
	"context"
	"database/sql"
	"strings"

	"github.com/danielleslie/clicknest/internal/storage"
)

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
// from GitHub. Satisfies ai.SourceMatcher interface.
func (m *Matcher) MatchAndFetch(ctx context.Context, projectID, elementID, elementClasses, parentPath string) (sourceCode, sourceFile string, ok bool) {
	match, err := m.Match(ctx, projectID, elementID, elementClasses, parentPath)
	if err != nil || match == nil {
		return "", "", false
	}

	conn, err := m.meta.GetGitHubConnection(ctx, projectID)
	if err != nil {
		return "", "", false
	}

	client := NewClient(conn.AccessToken)
	content, err := client.GetFileContent(ctx, conn.RepoOwner, conn.RepoName, match.FilePath, conn.DefaultBranch)
	if err != nil {
		return "", "", false
	}

	// Truncate to avoid blowing up the LLM context.
	if len(content) > 3000 {
		content = content[:3000] + "\n// ... truncated"
	}

	return content, match.FilePath, true
}

// Match finds the best source file for the given DOM context.
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
