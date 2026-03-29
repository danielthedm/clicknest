package growth

import (
	"context"
	"time"
)

// Publisher defines the interface for outbound platform integrations (posting content).
type Publisher interface {
	// Name returns the unique identifier (e.g. "reddit", "linkedin").
	Name() string
	// DisplayName returns a human-readable label (e.g. "Reddit", "LinkedIn").
	DisplayName() string
	// Post publishes content to the platform.
	Post(ctx context.Context, content PostContent) (*PostResult, error)
	// FetchEngagement retrieves metrics for a previously published post.
	FetchEngagement(ctx context.Context, externalID string) (*EngagementMetrics, error)
	// Validate checks that the publisher credentials are valid.
	Validate(ctx context.Context) error
}

// Source defines the interface for inbound platform integrations (discovering mentions).
type Source interface {
	// Name returns the unique identifier (e.g. "reddit", "twitter").
	Name() string
	// DisplayName returns a human-readable label (e.g. "Reddit", "Twitter / X").
	DisplayName() string
	// Search queries the platform for mentions matching the given query.
	Search(ctx context.Context, query SearchQuery) ([]Mention, error)
	// Validate checks that the source credentials are valid.
	Validate(ctx context.Context) error
}

// SearchQuery contains parameters for a Source.Search call.
type SearchQuery struct {
	Keywords    []string          `json:"keywords"`
	Subreddit   string            `json:"subreddit,omitempty"`
	Since       time.Time         `json:"since,omitempty"`
	MaxResults  int               `json:"max_results,omitempty"`
	ExtraFields map[string]string `json:"extra_fields,omitempty"`
}

// Mention represents a discovered conversation or post from an external platform.
type Mention struct {
	ExternalID  string            `json:"external_id"`
	ExternalURL string            `json:"external_url"`
	Platform    string            `json:"platform"`
	Author      string            `json:"author"`
	Title       string            `json:"title,omitempty"`
	Content     string            `json:"content"`
	PostedAt    time.Time         `json:"posted_at"`
	ParentID    string            `json:"parent_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type PostContent struct {
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	URL         string            `json:"url,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Media       []MediaAttachment `json:"media,omitempty"`
	Channel     string            `json:"channel"`
	Subreddit   string            `json:"subreddit,omitempty"`
	ExtraFields map[string]string `json:"extra_fields,omitempty"`
}

type MediaAttachment struct {
	URL      string `json:"url"`
	AltText  string `json:"alt_text,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type PostResult struct {
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url"`
}

type EngagementMetrics struct {
	Views    int `json:"views"`
	Likes    int `json:"likes"`
	Comments int `json:"comments"`
	Shares   int `json:"shares"`
	Clicks   int `json:"clicks"`
}
