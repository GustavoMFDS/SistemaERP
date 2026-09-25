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

func TestRequireUUID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	validRec := httptest.NewRecorder()
	got, ok := requireUUID(validRec, req, " 11111111-1111-1111-1111-111111111111 ")
	if !ok || got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("valid UUID rejected: ok=%v got=%q", ok, got)
	}

	invalidRec := httptest.NewRecorder()
	if _, ok := requireUUID(invalidRec, req, "not-a-uuid"); ok {
		t.Fatal("invalid UUID must be rejected")
	}
	if invalidRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid UUID status=%d, want 422", invalidRec.Code)
	}
}

