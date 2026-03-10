package auth

import (
	"net/http"

	"github.com/danielthedm/clicknest/internal/storage"
)

const SessionCookieName = "clicknest_session"

// APIKeyMiddleware validates the X-API-Key header for SDK ingestion endpoints.
func APIKeyMiddleware(meta *storage.SQLite) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			project, err := ValidateAPIKey(r.Context(), meta, apiKey)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := WithProject(r.Context(), project)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionMiddleware validates cookie-based sessions for the dashboard.
// It resolves the user's active project from the session, falling back
// to their first project membership or the global project list.
func SessionMiddleware(meta *storage.SQLite) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			userID, projectID, err := meta.GetUserSession(r.Context(), cookie.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := WithUserID(r.Context(), userID)

			// Try session's project_id first.
			if projectID != "" {
				p, err := meta.GetProject(ctx, projectID)
				if err == nil {
					ctx = WithProject(ctx, p)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Fallback: user's first project membership.
			userProjects, err := meta.ListUserProjects(ctx, userID)
			if err == nil && len(userProjects) > 0 {
				ctx = WithProject(ctx, &userProjects[0])
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Final fallback: global project list (backward compat for pre-migration).
			projects, err := meta.ListProjects(ctx)
			if err != nil || len(projects) == 0 {
				http.Error(w, `{"error":"no project configured"}`, http.StatusUnauthorized)
				return
			}
			ctx = WithProject(ctx, &projects[0])
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
