// Package auth re-exports auth types and middleware from internal/auth so
// that external modules (e.g. clicknest-cloud/ee) can use session auth
// without violating Go's internal package rules.
package auth

import (
	"context"
	"net/http"

	iauth "github.com/danielthedm/clicknest/internal/auth"
	"github.com/danielthedm/clicknest/internal/storage"
)

// SessionMiddleware validates cookie-based sessions for the dashboard.
var SessionMiddleware = iauth.SessionMiddleware

// UserIDFromContext retrieves the authenticated user ID from the request context.
var UserIDFromContext = iauth.UserIDFromContext

// ProjectFromContext retrieves the authenticated project from the request context.
func ProjectFromContext(ctx context.Context) *storage.Project {
	return iauth.ProjectFromContext(ctx)
}

// SessionCookieName is the name of the session cookie.
const SessionCookieName = iauth.SessionCookieName

// APIKeyMiddleware validates the X-API-Key header for SDK ingestion endpoints.
func APIKeyMiddleware(meta *storage.SQLite) func(http.Handler) http.Handler {
	return iauth.APIKeyMiddleware(meta)
}
