package main

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/example/sistemaemgo/internal/httpapi"
)

// Network-level smoke: the built React SPA and API share the same *secure*
// origin, API routes never fall back to HTML, and TLS 1.3 is negotiated.
func TestLocalHTTPSNetworkServesSPAAndPreservesAPI(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>esterium-lan-smoke</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/health/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	handler, err := httpapi.WithLocalFrontend(api, dir)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	client := server.Client()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/cash-register", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/html")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	html, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(html), "esterium-lan-smoke") {
		t.Fatalf("SPA not served through HTTPS, status=%d body=%q", response.StatusCode, html)
	}
	if response.TLS == nil || response.TLS.Version != tls.VersionTLS13 {
		t.Fatal("SPA request did not negotiate TLS 1.3")
	}
	if !strings.Contains(response.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("HTTPS SPA omitted its CSP")
	}

	for _, path := range []string{"/api/v1/products", "/health/ready"} {
		req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", "text/html")
		r, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusNoContent {
			t.Fatalf("%s was intercepted by SPA, status=%d", path, r.StatusCode)
		}
	}
	missing, err := client.Get(server.URL + "/assets/missing.js")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing JavaScript was replaced by HTML: %d", missing.StatusCode)
	}
}
