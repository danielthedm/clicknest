// Package store re-exports storage types from internal/storage so that
// external modules (e.g. clicknest-cloud/ee) can access the SQLite store
// without violating Go's internal package rules.
package store

import "github.com/danielthedm/clicknest/internal/storage"

// Re-export types so external packages can reference them.
type (
	SQLite  = storage.SQLite
	User    = storage.User
	Project = storage.Project
)
