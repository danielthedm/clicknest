package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielleslie/clicknest/internal/storage"
)

type contextKey string

const projectKey contextKey = "project"

var ErrUnauthorized = errors.New("unauthorized")

// ProjectFromContext retrieves the authenticated project from the request context.
func ProjectFromContext(ctx context.Context) *storage.Project {
	p, _ := ctx.Value(projectKey).(*storage.Project)
	return p
}

// WithProject stores the authenticated project in the context.
func WithProject(ctx context.Context, p *storage.Project) context.Context {
	return context.WithValue(ctx, projectKey, p)
}

// ValidateAPIKey looks up a project by API key. Returns ErrUnauthorized if not found.
func ValidateAPIKey(ctx context.Context, meta *storage.SQLite, apiKey string) (*storage.Project, error) {
	if apiKey == "" {
		return nil, ErrUnauthorized
	}
	p, err := meta.GetProjectByAPIKey(ctx, apiKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
