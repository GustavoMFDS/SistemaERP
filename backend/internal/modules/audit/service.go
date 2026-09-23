package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxMetadataBytes = 4096

type Service struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

type Event struct {
	TenantID     string
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      string
	Metadata     map[string]any
	RequestID    string
	IP           string
	UserAgent    string
	CreatedAt    time.Time
}

type LogEntry struct {
	ID           string         `json:"id"`
	TenantID     *string        `json:"tenant_id,omitempty"`
	ActorUserID  *string        `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   *string        `json:"resource_id,omitempty"`
	Outcome      string         `json:"outcome"`
	Metadata     map[string]any `json:"metadata"`
	IP           *string        `json:"ip,omitempty"`
	UserAgent    *string        `json:"user_agent,omitempty"`
	RequestID    *string        `json:"request_id,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

type ListFilter struct {
	Action       string
	ResourceType string
	ActorUserID  string
	Outcome      string
	From         string
	To           string
	Limit        int
	Offset       int
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{pool: pool, logger: logger}
}

func (s *Service) Record(ctx context.Context, ev Event) {
	if s == nil || s.pool == nil {
		return
	}
	if err := recordWithExecutor(ctx, s.pool, ev); err != nil && s.logger != nil {
		s.logger.Warn("audit_log_failed", slog.String("action", ev.Action), slog.String("request_id", ev.RequestID), slog.Any("err", err))
	}
}

// RecordTx persists a critical audit event using the caller's transaction.
// Returning the database error lets the business operation roll back rather
// than commit without its required audit evidence.
func (s *Service) RecordTx(ctx context.Context, tx db.DBTX, ev Event) error {
	if s == nil || tx == nil {
		return nil
	}
	return recordWithExecutor(ctx, tx, ev)
}

func recordWithExecutor(ctx context.Context, exec db.DBTX, ev Event) error {
	if strings.TrimSpace(ev.Action) == "" {
		return nil
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	if ev.ResourceType == "" {
		ev.ResourceType = "unknown"
	}
	if ev.Outcome == "" {
		ev.Outcome = "success"
	}
	meta := ev.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	meta["outcome"] = ev.Outcome
	meta = SanitizeMetadata(meta)
	metadata, err := json.Marshal(meta)
	if err != nil {
		metadata = []byte(`{"outcome":"metadata_error"}`)
	}
	if len(metadata) > maxMetadataBytes {
		metadata = []byte(`{"truncated":true}`)
	}

	_, err = exec.Exec(ctx, `
		INSERT INTO audit_logs (
			tenant_id, actor_user_id, action, entity_type, entity_id,
			resource_type, resource_id, metadata, ip, user_agent, request_id, created_at
		)
		VALUES (
			NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4, NULLIF($5, '')::uuid,
			$4, NULLIF($5, '')::uuid, $6::jsonb, NULLIF($7, '')::inet, $8, $9, $10
		)
	`, ev.TenantID, ev.ActorUserID, ev.Action, ev.ResourceType, ev.ResourceID, string(metadata), ev.IP, ev.UserAgent, ev.RequestID, ev.CreatedAt)
	return err
}

func (s *Service) List(ctx context.Context, tenantID string, filter ListFilter) ([]LogEntry, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(tenantID) == "" {
		return nil, pgx.ErrNoRows
	}
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	where := []string{"tenant_id=$1"}
	args := []any{tenantID}
	add := func(cond string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(cond, "?", "$"+itoa(len(args)), 1))
	}
	if strings.TrimSpace(filter.Action) != "" {
		add("action=?", strings.TrimSpace(filter.Action))
	}
	if strings.TrimSpace(filter.ResourceType) != "" {
		add("resource_type=?", strings.TrimSpace(filter.ResourceType))
	}
	if strings.TrimSpace(filter.ActorUserID) != "" {
		add("actor_user_id=?::uuid", strings.TrimSpace(filter.ActorUserID))
	}
	if strings.TrimSpace(filter.Outcome) != "" {
		add("metadata->>'outcome'=?", strings.TrimSpace(filter.Outcome))
	}
	if strings.TrimSpace(filter.From) != "" {
		add("created_at >= ?::timestamptz", strings.TrimSpace(filter.From))
	}
	if strings.TrimSpace(filter.To) != "" {
		add("created_at <= ?::timestamptz", strings.TrimSpace(filter.To))
	}

	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, tenant_id::text, actor_user_id::text, action, resource_type,
		       resource_id::text, metadata, ip::text, user_agent, request_id, created_at::text
		FROM audit_logs
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+itoa(limitArg)+` OFFSET $`+itoa(offsetArg), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogEntry
	for rows.Next() {
		var item LogEntry
		var raw []byte
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ActorUserID, &item.Action, &item.ResourceType, &item.ResourceID, &raw, &item.IP, &item.UserAgent, &item.RequestID, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Metadata)
		item.Metadata = SanitizeMetadata(item.Metadata)
		item.Outcome = outcomeFromMetadata(item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}

func RequestContext(r *http.Request) (requestID, ip, userAgent string) {
	requestID = middleware.GetRequestID(r.Context())
	userAgent = r.UserAgent()
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		ip = host
	} else {
		ip = r.RemoteAddr
	}
	return requestID, ip, userAgent
}

func SanitizeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(metadata))
	for k, v := range metadata {
		if sensitiveAuditKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = sanitizeValue(v, 0)
	}
	b, err := json.Marshal(out)
	if err == nil && len(b) > maxMetadataBytes {
		return map[string]any{"truncated": true}
	}
	return out
}

func sanitizeValue(v any, depth int) any {
	if depth > 8 {
		return "[TRUNCATED]"
	}
	switch x := v.(type) {
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case map[string]any:
		return SanitizeMetadata(x)
	case map[string]string:
		m := make(map[string]any, len(x))
		for k, v := range x {
			m[k] = v
		}
		return SanitizeMetadata(m)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, sanitizeValue(item, depth+1))
		}
		return out
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return v
	}
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		return sanitizeValue(rv.Elem().Interface(), depth+1)
	}
	if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
		m := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			m[iter.Key().String()] = sanitizeValue(iter.Value().Interface(), depth+1)
		}
		return SanitizeMetadata(m)
	}
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out = append(out, sanitizeValue(rv.Index(i).Interface(), depth+1))
		}
		return out
	}
	if rv.Kind() == reflect.Struct {
		t := rv.Type()
		m := make(map[string]any, rv.NumField())
		for i := 0; i < rv.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				name = field.Name
			}
			m[name] = sanitizeValue(rv.Field(i).Interface(), depth+1)
		}
		return SanitizeMetadata(m)
	}
	return v
}

func sensitiveAuditKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	for _, needle := range []string{"password", "passwd", "pwd", "token", "access_token", "refresh_token", "cookie", "authorization", "secret", "api_key", "key", "credential", "session", "jwt"} {
		if k == needle || strings.Contains(k, needle) {
			return true
		}
	}
	return false
}

func outcomeFromMetadata(meta map[string]any) string {
	if v, ok := meta["outcome"].(string); ok && v != "" {
		return v
	}
	return "unknown"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
