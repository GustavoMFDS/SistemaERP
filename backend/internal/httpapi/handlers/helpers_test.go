package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSONValidation(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		var dst struct {
			Name string `json:"name"`
		}
		err := readJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok","extra":true}`)), &dst)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("want unknown field error, got %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		var dst map[string]string
		err := readJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":`)), &dst)
		if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("want invalid JSON error, got %v", err)
		}
	})

	t.Run("multiple json values", func(t *testing.T) {
		var dst map[string]string
		err := readJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"b"} {"c":"d"}`)), &dst)
		if err == nil || !strings.Contains(err.Error(), "only one JSON") {
			t.Fatalf("want multiple JSON value error, got %v", err)
		}
	})

	t.Run("body too large", func(t *testing.T) {
		var dst map[string]string
		body := `{"x":"` + strings.Repeat("a", maxJSONBodyBytes+1) + `"}`
		err := readJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)), &dst)
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("want body too large error, got %v", err)
		}
	})
}
