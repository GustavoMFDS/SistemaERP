package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WithLocalFrontend serves a prebuilt Vite frontend from the same origin as
// the API. It intentionally never intercepts API, health, or metrics routes.
// The directory must be populated at build/package time, not on the shop PC.
func WithLocalFrontend(api http.Handler, directory string) (http.Handler, error) {
	root, err := filepath.EvalSymlinks(strings.TrimSpace(directory))
	if err != nil {
		return nil, fmt.Errorf("frontend directory: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("frontend directory: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return nil, fmt.Errorf("frontend directory is not a directory: %q", root)
	}
	index := filepath.Join(root, "index.html")
	info, err := os.Lstat(index)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("frontend index.html is missing or not a regular file in %q", root)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := path.Clean("/" + r.URL.Path)
		if reservedBackendPath(requested) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			api.ServeHTTP(w, r)
			return
		}

		// Static responses do not pass through the API's security middleware.
		// The API uses default-src 'none'; the bundled SPA needs self-hosted
		// scripts, CSS, icons and a same-origin connection for API calls.
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; worker-src 'self' blob:; object-src 'none'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")

		relative := strings.TrimPrefix(requested, "/")
		if relative != "" {
			candidate := filepath.Join(root, filepath.FromSlash(relative))
			resolved, resolveErr := filepath.EvalSymlinks(candidate)
			if resolveErr == nil && insideFrontendRoot(root, resolved) {
				stat, statErr := os.Stat(resolved)
				if statErr == nil && stat.Mode().IsRegular() {
					if relative == "index.html" {
						w.Header().Set("Cache-Control", "no-store")
					} else if strings.HasPrefix(relative, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					} else {
						w.Header().Set("Cache-Control", "no-cache")
					}
					http.ServeFile(w, r, resolved)
					return
				}
			}
		}

		// Vite uses client-side routing: only navigation requests may fall
		// back to index.html. Missing scripts, CSS and image files remain 404.
		if relative == "" || (!strings.Contains(path.Base(relative), ".") &&
			strings.Contains(r.Header.Get("Accept"), "text/html")) {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, index)
			return
		}
		http.NotFound(w, r)
	}), nil
}

func insideFrontendRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func reservedBackendPath(p string) bool {
	for _, prefix := range []string{"/api", "/health", "/metrics", "/debug"} {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}
