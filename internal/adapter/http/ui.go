package http

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/asakaida/dandori/web"
)

// NewUIHandler creates an http.Handler that serves the embedded web UI.
// It serves files from web.Content under /ui/ and falls back to index.html for SPA routing.
func NewUIHandler() http.Handler {
	fsys := web.Content
	fileServer := http.FileServer(http.FS(fsys))

	return http.StripPrefix("/ui/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Serve existing files directly; fall back to index.html for SPA client-side routing
		if path != "" {
			if _, err := fs.Stat(fsys, path); err != nil {
				r.URL.Path = "/index.html"
			}
		}

		fileServer.ServeHTTP(w, r)
	}))
}
