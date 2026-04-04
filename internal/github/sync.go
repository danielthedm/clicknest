package github

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/danielthedm/clicknest/internal/storage"
)

var componentExtensions = map[string]bool{
	".tsx":    true,
	".jsx":    true,
	".vue":    true,
	".svelte": true,
	".html":   true,
	".astro":  true,
	".js":     true,
	".ts":     true,
	".mjs":    true,
}

// re-patterns for extracting selectors from source code
var (
	idPattern    = regexp.MustCompile(`(?:id=["']([^"']+)["'])`)
	classPattern = regexp.MustCompile(`(?:class(?:Name)?=["']([^"']+)["'])`)
)

// Syncer handles background repo syncing and indexing.
type Syncer struct {
	meta    *storage.SQLite
	dataDir string
}

// NewSyncer creates a new repo syncer.
func NewSyncer(meta *storage.SQLite, dataDir string) *Syncer {
	return &Syncer{meta: meta, dataDir: dataDir}
}

// SyncRepo syncs a GitHub repo and indexes component files.
func (s *Syncer) SyncRepo(ctx context.Context, projectID string) error {
	conn, err := s.meta.GetGitHubConnection(ctx, projectID)
	if err != nil {
		return err
	}

	client := NewClient(conn.AccessToken)

	return s.syncDirectory(ctx, client, conn, projectID, "")
}

func (s *Syncer) syncDirectory(ctx context.Context, client *Client, conn *storage.GitHubConnection, projectID, path string) error {
	entries, err := client.ListDirectory(ctx, conn.RepoOwner, conn.RepoName, path, conn.DefaultBranch)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Type == "dir" {
			// Skip common non-component directories.
			base := filepath.Base(entry.Path)
			if base == "node_modules" || base == ".git" || base == "dist" || base == "build" {
				continue
			}
			if err := s.syncDirectory(ctx, client, conn, projectID, entry.Path); err != nil {
				log.Printf("WARN syncing dir %s: %v", entry.Path, err)
			}
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Path))
		if !componentExtensions[ext] {
			continue
		}

		content, err := client.GetFileContent(ctx, conn.RepoOwner, conn.RepoName, entry.Path, conn.DefaultBranch)
		if err != nil {
			log.Printf("WARN fetching %s: %v", entry.Path, err)
			continue
		}

		// Save file content to disk for code agent search.
		if s.dataDir != "" {
			diskPath := filepath.Join(s.dataDir, "repos", conn.RepoOwner, conn.RepoName, entry.Path)
			if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
				log.Printf("WARN creating dir for %s: %v", entry.Path, err)
			} else if err := os.WriteFile(diskPath, []byte(content), 0644); err != nil {
				log.Printf("WARN writing %s: %v", diskPath, err)
			}
		}

		selectors := extractSelectors(content)
		componentName := inferComponentName(entry.Path)

		if err := s.meta.UpsertSourceIndex(ctx, projectID, entry.Path, componentName, selectors, entry.SHA); err != nil {
			log.Printf("WARN indexing %s: %v", entry.Path, err)
		}
	}

	return nil
}

func extractSelectors(content string) string {
	var selectors []string

	for _, match := range idPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			selectors = append(selectors, "#"+match[1])
		}
	}

	for _, match := range classPattern.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			for _, cls := range strings.Fields(match[1]) {
				selectors = append(selectors, "."+cls)
			}
		}
	}

	return strings.Join(selectors, " ")
}

func inferComponentName(filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Handle index files — use parent directory name.
	if strings.ToLower(name) == "index" || name == "+page" {
		dir := filepath.Dir(filePath)
		name = filepath.Base(dir)
	}

	return name
}
