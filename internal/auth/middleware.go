package auth

import (
	"net/http"

	"github.com/danielleslie/clicknest/internal/storage"
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
func SessionMiddleware(meta *storage.SQLite) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_, err = meta.GetUserSession(r.Context(), cookie.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			// Single-tenant: attach the first project to context.
			projects, err := meta.ListProjects(r.Context())
			if err != nil || len(projects) == 0 {
				http.Error(w, `{"error":"no project configured"}`, http.StatusUnauthorized)
				return
			}
			ctx := WithProject(r.Context(), &projects[0])
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
