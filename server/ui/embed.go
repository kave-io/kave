// Package ui embeds the compiled dashboard and serves it as a static SPA.
// Build the dashboard first: make dashboard:build
package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var dist embed.FS

// Handler returns an http.Handler that serves the dashboard SPA.
// All paths fall through to index.html so Vue Router handles routing.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("ui: embed dist not found — run 'make dashboard:build' first")
	}
	return &spaHandler{fs: http.FS(sub)}
}

// spaHandler serves static files and falls back to index.html for unknown paths
// so Vue Router's history mode works correctly.
type spaHandler struct {
	fs http.FileSystem
}

func (s *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, err := s.fs.Open(r.URL.Path)
	if err != nil {
		// Not found → serve index.html for SPA routing
		r.URL.Path = "/"
		f, err = s.fs.Open("/")
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	f.Close()
	http.FileServer(s.fs).ServeHTTP(w, r)
}
