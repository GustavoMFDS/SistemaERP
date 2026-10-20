package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
)

type backupReport struct {
	LastRunUTC      string `json:"last_run_utc"`
	State           string `json:"state"`
	ArchiveVerified bool   `json:"archive_verified"`
}

// BackupStatus exposes only a sanitized global status to audit administrators.
// It does not publish backup locations or read arbitrary caller-supplied paths.
func (h *AuditHandler) BackupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if _, ok := middleware.GetAuthUser(r.Context()); !ok {
		writeError(w, r, http.StatusUnauthorized, "authentication_error", "não autenticado", nil)
		return
	}
	path := os.Getenv("ESTERIUM_BACKUP_STATUS_PATH")
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false, "state": "not_configured"})
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "state": "unavailable"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "state": "unavailable"})
		return
	}
	var record backupReport
	attemptAt := time.Time{}
	if json.Unmarshal(data, &record) == nil {
		attemptAt, err = time.Parse(time.RFC3339, record.LastRunUTC)
	}
	if err != nil || attemptAt.IsZero() || attemptAt.After(time.Now().Add(5*time.Minute)) ||
		(record.State != "ok" && record.State != "failed") {
		writeJSON(w, http.StatusOK, map[string]any{"configured": true, "state": "invalid"})
		return
	}
	state := record.State
	if state == "ok" && (!record.ArchiveVerified || time.Since(attemptAt) > 36*time.Hour) {
		state = "stale"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":       true,
		"state":            state,
		"last_run_utc":     attemptAt.UTC().Format(time.RFC3339),
		"archive_verified": record.ArchiveVerified,
	})
}
