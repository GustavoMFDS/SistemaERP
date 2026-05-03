package audit

import (
	"strings"
	"testing"
)

func TestSanitizeMetadataRedactsSensitiveKeysRecursively(t *testing.T) {
	got := SanitizeMetadata(map[string]any{
		"status":        "ok",
		"password":      "secret",
		"refresh_token": "rt",
		"nested": map[string]any{
			"authorization": "Bearer token",
			"count":         2,
		},
		"items": []any{
			map[string]any{"cookie": "raw-cookie", "id": "safe-id"},
		},
	})

	if got["status"] != "ok" {
		t.Fatalf("safe metadata was not preserved: %#v", got)
	}
	if got["password"] != "[REDACTED]" || got["refresh_token"] != "[REDACTED]" {
		t.Fatalf("top-level sensitive metadata was not redacted: %#v", got)
	}
	nested := got["nested"].(map[string]any)
	if nested["authorization"] != "[REDACTED]" || nested["count"] != 2 {
		t.Fatalf("nested metadata sanitization failed: %#v", nested)
	}
	items := got["items"].([]any)
	item := items[0].(map[string]any)
	if item["cookie"] != "[REDACTED]" || item["id"] != "safe-id" {
		t.Fatalf("array metadata sanitization failed: %#v", item)
	}
}

func TestSanitizeMetadataLimitsSize(t *testing.T) {
	got := SanitizeMetadata(map[string]any{"safe": strings.Repeat("x", maxMetadataBytes+1)})
	if got["truncated"] != true {
		t.Fatalf("expected oversized metadata to be truncated, got %#v", got)
	}
}

func TestSanitizeMetadataHandlesTypedMapsAndStructs(t *testing.T) {
	type nested struct {
		Authorization string `json:"authorization"`
		Count         int    `json:"count"`
	}
	got := SanitizeMetadata(map[string]any{
		"headers": map[string]string{
			"api_key": "raw-key",
			"id":      "safe-id",
		},
		"struct": nested{Authorization: "Bearer raw", Count: 3},
	})

	headers := got["headers"].(map[string]any)
	if headers["api_key"] != "[REDACTED]" || headers["id"] != "safe-id" {
		t.Fatalf("typed map was not sanitized: %#v", headers)
	}
	strct := got["struct"].(map[string]any)
	if strct["authorization"] != "[REDACTED]" || strct["count"] != 3 {
		t.Fatalf("struct was not sanitized: %#v", strct)
	}
}
