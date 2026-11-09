package main

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// registerWebRoutes serves the built web app from dir for every path the API
// doesn't own, where nothing in front of the API does it (VibeCode; on Railway
// Cloudflare Pages serves the app and proxies /api). Paths without a file get
// index.html, so the app's own routes load on refresh.
func registerWebRoutes(r chi.Router, dir string) {
	r.NotFound(webHandler(dir, os.Getenv("WEB_FRAME_ANCESTORS")).ServeHTTP)
}

// webHandler mirrors apps/web/public/_headers. frameAncestors lists who may
// frame the app (CSP frame-ancestors); empty keeps X-Frame-Options: DENY.
func webHandler(dir string, frameAncestors string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	frameAncestors = strings.TrimSpace(frameAncestors)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" ||
			(r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}

		header := w.Header()
		header.Set("X-Robots-Tag", "noindex, nofollow")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		if frameAncestors == "" {
			header.Set("X-Frame-Options", "DENY")
		} else {
			header.Set("Content-Security-Policy", "frame-ancestors "+frameAncestors)
		}

		name := path.Clean("/" + r.URL.Path)
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || info.IsDir() {
			// index.html must not be cached: it names the current build's assets.
			header.Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(name, "/assets/") {
			// Vite puts a content hash in every asset name.
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
