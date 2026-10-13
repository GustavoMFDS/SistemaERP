//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi"
	"github.com/example/sistemaemgo/internal/modules"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Real JWT middleware, RBAC, CSRF, SQL and audit integration, never real SEFAZ.
func TestCashPrinterHTTPScopedAuthorizedAuditAndClosedSession(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" ||
		os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var tenant, actor string
	if err := pool.QueryRow(ctx, `
		SELECT ur.tenant_id::text,ur.user_id::text
		FROM user_tenant_roles ur JOIN roles r
		ON r.id=ur.role_id AND r.name='admin'
		LIMIT 1
	`).Scan(&tenant, &actor); err != nil {
		t.Fatal(err)
	}
	makeRegister := func(tenantID string) (string, string) {
		t.Helper()
		var register, session string
		if err := pool.QueryRow(ctx, `
			INSERT INTO cash_registers(tenant_id,name,active)
			VALUES($1,$2,true) RETURNING id::text
		`, tenantID, "Printer HTTP "+uuid.NewString()).Scan(&register); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,
				opening_amount,status)
			VALUES($1,$2,$3,0,'open') RETURNING id::text
		`, tenantID, register, actor).Scan(&session); err != nil {
			t.Fatal(err)
		}
		return register, session
	}
	registerA, sessionA := makeRegister(tenant)
	tenantB := ""
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name,cnpj)
		VALUES($1,$2) RETURNING id::text
	`, "Printer HTTP Second Store "+uuid.NewString(),
		fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)).
		Scan(&tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id,tenant_id) VALUES($1,$2)
	`, actor, tenantB); err != nil {
		t.Fatal(err)
	}
	registerB, sessionB := makeRegister(tenantB)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		Env: "dev", FiscalProvider: "mvp", JWTIssuer: "printer-integration",
		JWTSecret:          "disposable-test-only-printer-jwt",
		AccessTokenTTL:     time.Hour,
		CORSAllowedOrigins: []string{"http://127.0.0.1:5189"},
	}
	api := httpapi.NewRouter(cfg, modules.New(cfg, pool, nil, logger), logger)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": cfg.JWTIssuer, "sub": actor, "tenant_id": tenant,
		"typ": "access", "iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}

	callEndpoint := func(method, session, suffix, body, origin, auth string) *httptest.ResponseRecorder {
		t.Helper()
		var data io.Reader
		if body != "" {
			data = bytes.NewBufferString(body)
		}
		req := httptest.NewRequest(method, "/api/v1/cash/sessions/"+session+"/printer"+suffix, data)
		req.Header.Set("Origin", origin)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		return w
	}
	call := func(method, session, body, origin, auth string) *httptest.ResponseRecorder {
		return callEndpoint(method, session, "", body, origin, auth)
	}
	testReceipt := func(session, auth string) *httptest.ResponseRecorder {
		return callEndpoint(http.MethodGet, session, "/test", "", "http://127.0.0.1:5189", auth)
	}
	goodOrigin := "http://127.0.0.1:5189"
	if got := call(http.MethodGet, sessionA, "", goodOrigin, "").Code; got != 401 {
		t.Fatalf("unauthenticated printer read=%d, want 401", got)
	}
	if got := testReceipt(sessionA, "").Code; got != 401 {
		t.Fatalf("unauthenticated test printing=%d, want 401", got)
	}
	rr := call(http.MethodGet, sessionA, "", goodOrigin, token)
	if rr.Code != 200 {
		t.Fatalf("authorized read=%d body=%s", rr.Code, rr.Body.String())
	}
	var initial struct {
		PaperWidthMM int `json:"paper_width_mm"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &initial); err != nil || initial.PaperWidthMM != 80 {
		t.Fatalf("wrong default roll width: %+v %v", initial, err)
	}
	if rr := call(http.MethodPut, sessionA, `{"paper_width_mm":58}`,
		"https://attacker.example", token); rr.Code != 403 {
		t.Fatalf("untrusted-origin printer change=%d want 403", rr.Code)
	}
	if rr := call(http.MethodPut, sessionA, `{"paper_width_mm":60}`,
		goodOrigin, token); rr.Code != 422 {
		t.Fatalf("invalid roll size accepted: %d", rr.Code)
	}
	if rr := call(http.MethodPut, sessionB, `{"paper_width_mm":58}`,
		goodOrigin, token); rr.Code != 404 {
		t.Fatalf("cross-company printer modified: %d", rr.Code)
	}
	if rr := call(http.MethodGet, sessionB, "", goodOrigin, token); rr.Code != 404 {
		t.Fatalf("cross-company printer read leaked: %d", rr.Code)
	}
	if rr := testReceipt(sessionB, token); rr.Code != 404 {
		t.Fatalf("cross-company test coupon leaked: %d", rr.Code)
	}
	if rr := call(http.MethodPut, sessionA, `{"paper_width_mm":58}`,
		goodOrigin, token); rr.Code != 200 {
		t.Fatalf("legitimate paper change=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = call(http.MethodGet, sessionA, "", goodOrigin, token)
	var after struct {
		PaperWidthMM int `json:"paper_width_mm"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &after); err != nil || rr.Code != 200 || after.PaperWidthMM != 58 {
		t.Fatalf("persisted paper preference unavailable: %d %+v %v", rr.Code, after, err)
	}
	// The calibration receipt is generated without any sale, invoice or
	// tax engine invocation. It must stay clearly non-fiscal at both widths.
	var invoicesBefore, invoicesAfter int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invoices WHERE tenant_id=$1", tenant).
		Scan(&invoicesBefore); err != nil {
		t.Fatal(err)
	}
	receipt := testReceipt(sessionA, token)
	if receipt.Code != 200 ||
		!bytes.Contains(receipt.Body.Bytes(), []byte("TESTE DE IMPRESSÃO")) ||
		!bytes.Contains(receipt.Body.Bytes(), []byte("NÃO É DOCUMENTO FISCAL")) ||
		!bytes.Contains(receipt.Body.Bytes(), []byte("max-width: 58mm")) ||
		!bytes.Contains(receipt.Body.Bytes(), []byte("data:image/png;base64,")) {
		t.Fatalf("invalid 58mm nonfiscal test receipt: status=%d body=%s",
			receipt.Code, receipt.Body.String())
	}
	for k, want := range map[string]string{
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
		"Content-Type":           "text/html; charset=utf-8",
	} {
		if got := receipt.Header().Get(k); got != want {
			t.Fatalf("calibration header %s=%q want %q", k, got, want)
		}
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invoices WHERE tenant_id=$1", tenant).
		Scan(&invoicesAfter); err != nil {
		t.Fatal(err)
	}
	if invoicesAfter != invoicesBefore {
		t.Fatal("printer test generated a real/fake NFC-e invoice")
	}
	var widthB, audits int
	if err := pool.QueryRow(ctx, `
		SELECT danfe_paper_width_mm FROM cash_registers
		WHERE tenant_id=$1 AND id=$2
	`, tenantB, registerB).Scan(&widthB); err != nil || widthB != 80 {
		t.Fatalf("cross-company roll width changed: %d %v", widthB, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE tenant_id=$1 AND resource_id=$2
		  AND action='cash.printer.paper_width.updated'
	`, tenant, registerA).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("not exactly one roll width audit: %d %v", audits, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cash_sessions SET status='closed',closed_at=now()
		WHERE tenant_id=$1 AND id=$2
	`, tenant, sessionA); err != nil {
		t.Fatal(err)
	}
	if rr := call(http.MethodPut, sessionA, `{"paper_width_mm":80}`,
		goodOrigin, token); rr.Code != 404 {
		t.Fatalf("closed register settings altered: %d", rr.Code)
	}
	if rr := testReceipt(sessionA, token); rr.Code != 404 {
		t.Fatalf("closed cash session printed calibration: %d", rr.Code)
	}
}
