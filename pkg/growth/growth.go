// Package growth re-exports the Publisher, Source, and related types from
// internal/growth so that external modules (e.g. clicknest-cloud)
// can implement publishers and sources without violating Go's internal package rules.
package growth

import "github.com/danielthedm/clicknest/internal/growth"

// Re-export types so external packages can reference them.
type (
	Publisher         = growth.Publisher
	Source            = growth.Source
	PostContent       = growth.PostContent
	PostResult        = growth.PostResult
	EngagementMetrics = growth.EngagementMetrics
	MediaAttachment   = growth.MediaAttachment
	SearchQuery       = growth.SearchQuery
	Mention           = growth.Mention
	Registry          = growth.Registry
)

// NewRegistry delegates to the internal constructor.
var NewRegistry = growth.NewRegistry
