package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielthedm/clicknest/internal/storage"
)

type contextKey string

const (
	projectKey contextKey = "project"
	userIDKey  contextKey = "user_id"
)

var ErrUnauthorized = errors.New("unauthorized")

// UserIDFromContext retrieves the authenticated user ID from the request context.
func UserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey).(string)
	return id
}

// WithUserID stores the authenticated user ID in the context.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

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
