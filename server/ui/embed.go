// Package ui embeds the compiled dashboard and serves it as a static SPA.
// Build the dashboard first: make dashboard:build
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
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
	setSecurityHeaders(w.Header())
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestedPath := r.URL.Path
	f, err := s.fs.Open(requestedPath)
	if err != nil {
		// Extensionless paths are dashboard routes. Missing static assets remain
		// 404s so a stale script URL can never receive HTML with the wrong MIME
		// type.
		if path.Ext(requestedPath) != "" {
			http.NotFound(w, r)
			return
		}
		requestedPath = "/"
		f, err = s.fs.Open(requestedPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	_ = f.Close()
	if requestedPath == "/" || requestedPath == "/index.html" {
		w.Header().Set("Cache-Control", "no-store")
	} else if strings.HasPrefix(requestedPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	request := r.Clone(r.Context())
	request.URL.Path = requestedPath
	http.FileServer(s.fs).ServeHTTP(w, request)
}

func setSecurityHeaders(header http.Header) {
	header.Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; connect-src 'self'; font-src 'self'; form-action 'none'; frame-ancestors 'none'; img-src 'self' data:; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=(), usb=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}
