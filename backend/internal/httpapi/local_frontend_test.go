package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithLocalFrontend(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>Esterium</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log('local')"), 0600); err != nil {
		t.Fatal(err)
	}

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler, err := WithLocalFrontend(api, dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, method, url, accept string
		status                    int
		body                      string
	}{
		{"root", "GET", "/", "text/html", 200, "Esterium"},
		{"spa navigation", "GET", "/products/123", "text/html", 200, "Esterium"},
		{"static asset", "GET", "/assets/app.js", "*/*", 200, "console.log"},
		{"missing asset", "GET", "/assets/no-such.js", "*/*", 404, ""},
		{"non-html navigation", "GET", "/products/123", "application/json", 404, ""},
		{"API remains API", "GET", "/api/v1/no-route", "text/html", 204, ""},
		{"health remains API", "GET", "/health/ready", "text/html", 204, ""},
		{"metrics remains API", "GET", "/metrics", "text/html", 204, ""},
		{"debug remains API", "GET", "/debug/config", "text/html", 204, ""},
		{"POST not converted to HTML", "POST", "/login", "text/html", 204, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.url, nil)
			req.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d, want %d", w.Code, tc.status)
			}
			if tc.body != "" && !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("body=%q, missing %q", w.Body.String(), tc.body)
			}
			if tc.status == http.StatusOK {
				csp := w.Header().Get("Content-Security-Policy")
				if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
					t.Fatalf("unsafe/missing SPA content security policy: %q", csp)
				}
				if w.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("SPA responses must send nosniff")
				}
			}
		})
	}
}

func TestWithLocalFrontendRejectsMissingIndex(t *testing.T) {
	_, err := WithLocalFrontend(http.NotFoundHandler(), t.TempDir())
	if err == nil {
		t.Fatal("expected missing index.html error")
	}
}

func TestLocalFrontendSymlinksDoNotEscapeBundle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("safe page"), 0600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("PRIVATE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "secret.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	handler, err := WithLocalFrontend(http.NotFoundHandler(), dir)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/secret.txt", nil)
	req.Header.Set("Accept", "*/*")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 404 || strings.Contains(w.Body.String(), "PRIVATE") {
		t.Fatalf("symlink leaked data: status=%d body=%q", w.Code, w.Body.String())
	}
}
