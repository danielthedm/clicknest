package server

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// WebFS holds the embedded SvelteKit build output.
// This will be populated during build with //go:embed in a build-specific file.
// For development, we serve from disk.
var WebFS embed.FS

// SDKFS holds the embedded SDK JS file.
var SDKFS embed.FS

// SPAHandler serves the embedded SvelteKit SPA.
// It serves static files when they exist, and falls back to index.html for SPA routing.
func SPAHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	// Read index.html into memory so we can serve it with no-cache headers.
	indexHTML, _ := fs.ReadFile(fsys, "index.html")

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		// Never cache index.html — it references content-hashed assets
		// that change on every build. Without this, browsers serve a stale
		// index.html that points to old JS files that no longer exist.
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Root or SPA route — serve index.html with no-cache.
		if path == "" || path == "index.html" {
			serveIndex(w, r)
			return
		}

		// Try to open the file. If it exists, serve it.
		f, err := fsys.Open(path)
		if err == nil {
			f.Close()
			// Hashed assets (_app/immutable/*) can be cached forever.
			if strings.HasPrefix(path, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// For SPA routing, serve index.html for non-file paths.
		if !strings.Contains(path, ".") {
			serveIndex(w, r)
			return
		}

		http.NotFound(w, r)
	})
}
