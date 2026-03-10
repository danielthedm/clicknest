// Package growth re-exports the connector interface and types from
// internal/growth so that external modules (e.g. clicknest-cloud)
// can implement connectors without violating Go's internal package rules.
package growth

import "github.com/danielthedm/clicknest/internal/growth"

// Re-export types so external packages can reference them.
type (
	Connector         = growth.Connector
	PostContent       = growth.PostContent
	PostResult        = growth.PostResult
	EngagementMetrics = growth.EngagementMetrics
	MediaAttachment   = growth.MediaAttachment
	Registry          = growth.Registry
)

// NewRegistry delegates to the internal constructor.
var NewRegistry = growth.NewRegistry
