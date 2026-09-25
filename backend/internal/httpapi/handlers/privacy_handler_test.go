package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/jackc/pgx/v5"
)

func TestWritePrivacyErrorNotFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "domain not found", err: common.ErrNotFound},
		{name: "repository no rows", err: pgx.ErrNoRows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/consents", nil)
			rec := httptest.NewRecorder()

			writePrivacyError(rec, req, tc.err)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status=%d, want 404", rec.Code)
			}
		})
	}
}
