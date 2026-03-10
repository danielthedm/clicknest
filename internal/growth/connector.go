package growth

import "context"

// Connector defines the interface for social/content platform integrations.
// Implementations live in the private clicknest-cloud repo.
// Self-hosters can implement their own connectors.
type Connector interface {
	// Name returns the unique identifier (e.g. "reddit", "linkedin").
	Name() string
	// DisplayName returns a human-readable label (e.g. "Reddit", "LinkedIn").
	DisplayName() string
	// Post publishes content to the platform.
	Post(ctx context.Context, content PostContent) (*PostResult, error)
	// FetchEngagement retrieves metrics for a previously published post.
	FetchEngagement(ctx context.Context, externalID string) (*EngagementMetrics, error)
	// Validate checks that the connector credentials are valid.
	Validate(ctx context.Context) error
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
