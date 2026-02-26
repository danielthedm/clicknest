package ai

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danielleslie/clicknest/internal/storage"
)

// Cache wraps the SQLite event_names table for fast fingerprint → name lookups.
type Cache struct {
	meta *storage.SQLite
}

// NewCache creates a new naming cache backed by SQLite.
func NewCache(meta *storage.SQLite) *Cache {
	return &Cache{meta: meta}
}

// Get returns the display name for a fingerprint, or empty string if not cached.
// User overrides take priority over AI-generated names.
func (c *Cache) Get(ctx context.Context, projectID, fingerprint string) (string, bool) {
	en, err := c.meta.GetEventName(ctx, projectID, fingerprint)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return "", false
	}
	if en.UserName != nil && *en.UserName != "" {
		return *en.UserName, true
	}
	return en.AIName, true
}

// Set stores an AI-generated event name in the cache.
func (c *Cache) Set(ctx context.Context, projectID, fingerprint string, result *NamingResult) error {
	return c.meta.SetEventName(ctx, storage.EventName{
		Fingerprint: fingerprint,
		ProjectID:   projectID,
		AIName:      result.Name,
		SourceFile:  &result.SourceFile,
		Confidence:  &result.Confidence,
	})
}
