//go:build integration

package integration_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	fiscsefaz "github.com/example/sistemaemgo/internal/modules/fiscal/providers/sefaz"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

type inutilizationFakeSigner struct{}

func (inutilizationFakeSigner) SignInutilization(
	_ context.Context,
	_ string,
	_ string,
	unsignedXML []byte,
) ([]byte, error) {
	return append([]byte(nil), unsignedXML...), nil
}

type inutilizationFakeValidator struct{}

func (inutilizationFakeValidator) Validate(context.Context, []byte) error { return nil }

type inutilizationFakeClient struct {
	calls  int
	result fisc.NFCeInutilizationRemoteResult
	err    error
}

func (f *inutilizationFakeClient) Inutilize(
	_ context.Context,
	_ string,
	_ fisc.NFCeInutilizationDraft,
	requestID string,
	_ []byte,
) (fisc.NFCeInutilizationRemoteResult, error) {
	f.calls++
	if f.err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, f.err
	}
	out := f.result
	out.RequestID = requestID
	return out, nil
}

func TestNFCeInutilizationPersistsRangeAndNeverBlindlyRetransmits(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var actorUserID string
	if err := pool.QueryRow(ctx, `
		SELECT id::text FROM users WHERE email='admin@sistema.local' LIMIT 1
	`).Scan(&actorUserID); err != nil {
		t.Fatalf("seeded admin required: %v", err)
	}

	var tenantID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(
			legal_name, trade_name, cnpj, ie, crt, address_state, created_at
		)
		VALUES (
			'Empresa Inutilizacao CI', 'Inutilizacao CI',
			'12ABC34501DE35', '110042490114', '1', 'MG', now()
		)
		RETURNING id::text
	`).Scan(&tenantID); err != nil {
		t.Fatalf("prepare isolated tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id, tenant_id)
		VALUES ($1,$2)
		ON CONFLICT DO NOTHING
	`, actorUserID, tenantID); err != nil {
		t.Fatalf("map admin to isolated tenant: %v", err)
	}

	uow := db.NewPgxUnitOfWork(pool)
	repo := fiscinfra.NewFiscalRepo(pool)

	// Inutilization and regular allocation must contend on the identical
	// transaction-scoped lock, including across different fiscal years.
	// A second connection must be unable to allocate while the range
	// validator holds the lock.
	lockTx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin range lock transaction: %v", err)
	}
	if err := repo.LockNFCeInutilizationRange(ctx, lockTx, tenantID, 2026, 778); err != nil {
		_ = lockTx.Rollback(ctx)
		t.Fatalf("lock inutilization range: %v", err)
	}
	blockedCtx, blockedCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	blockedTx, err := uow.Begin(blockedCtx)
	if err != nil {
		_ = lockTx.Rollback(ctx)
		blockedCancel()
		t.Fatalf("begin competing reservation: %v", err)
	}
	_, allocationErr := repo.ReserveNextNFCeNumber(blockedCtx, blockedTx, tenantID, 778)
	_ = blockedTx.Rollback(context.Background())
	blockedCancel()
	if allocationErr == nil {
		_ = lockTx.Rollback(ctx)
		t.Fatal("fiscal allocation bypassed pending inutilization range lock")
	}
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release inutilization range lock: %v", err)
	}
	releasedTx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstNumber, err := repo.ReserveNextNFCeNumber(ctx, releasedTx, tenantID, 778)
	if err != nil {
		_ = releasedTx.Rollback(ctx)
		t.Fatalf("reservation after range unlock: %v", err)
	}
	if firstNumber != 1 {
		_ = releasedTx.Rollback(ctx)
		t.Fatalf("range lock must not consume number; got %d", firstNumber)
	}
	if err := releasedTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback probe allocation: %v", err)
	}
	certRef := "integration-inutilization-cert"
	tx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNFCeConfig(ctx, tx, tenantID, actorUserID, fisc.NFCeConfig{
		TenantID:             tenantID,
		Environment:          "homologation",
		Series:               777,
		CertificateSecretRef: &certRef,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("prepare NFC-e config: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := fiscapp.NewFiscalService(
		uow,
		repo,
		salesinfra.NewSalesRepo(pool),
		invinfra.NewProductsRepo(pool),
		audit.New(pool, logger),
		validator.New(),
		logger,
	)
	service.SetNFCeInutilizationBuilder(fiscsefaz.NewDocumentBuilder("integration"))
	service.SetNFCeInutilizationSigner(inutilizationFakeSigner{})
	service.SetNFCeInutilizationSchemaValidator(inutilizationFakeValidator{})

	start := int64(time.Now().UnixNano()%800_000_000 + 1_000_000)
	// Simulate a committed sequence frontier past the skipped range.
	// Future numbers must never be inutilized before allocation.
	if _, err := pool.Exec(ctx, `
		INSERT INTO fiscal_document_sequences(tenant_id, model, series, next_number)
		VALUES ($1,65,777,$2)
	`, tenantID, start+150); err != nil {
		t.Fatalf("set sequence frontier for unused historical range: %v", err)
	}
	registeredAt := time.Now().Truncate(time.Second)
	client := &inutilizationFakeClient{result: fisc.NFCeInutilizationRemoteResult{
		StatusCode:   102,
		Reason:       "Inutilizacao de numero homologado",
		FinalStatus:  fisc.NFCeInutilizationStatusRegistered,
		Protocol:     "131260000000001",
		RegisteredAt: registeredAt,
		ResponseXML:  []byte("<retInutNFe><infInut><cStat>102</cStat></infInut></retInutNFe>"),
	}}
	service.SetNFCeRemoteInutilizationClient(client)

	req := fiscapp.InutilizeNFCeNumbersRequest{
		Year:          2026,
		Series:        777,
		StartNumber:   start,
		EndNumber:     start + 9,
		Justification: "Falha operacional pulou a faixa durante o fechamento.",
	}
	result, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, req,
	)
	if err != nil {
		t.Fatalf("InutilizeNFCeNumbers: %v", err)
	}
	if !result.Registered() || client.calls != 1 {
		t.Fatalf("unexpected registered result/calls: %+v calls=%d", result, client.calls)
	}

	var status string
	var storedStart, storedEnd int64
	if err := pool.QueryRow(ctx, `
		SELECT status, start_number, end_number
		FROM nfce_number_inutilizations
		WHERE tenant_id=$1 AND request_id=$2
	`, tenantID, result.RequestID).Scan(&status, &storedStart, &storedEnd); err != nil {
		t.Fatalf("read persisted inutilization: %v", err)
	}
	if status != fisc.NFCeInutilizationStatusRegistered ||
		storedStart != req.StartNumber || storedEnd != req.EndNumber {
		t.Fatalf("unexpected persisted inutilization: status=%s range=%d-%d", status, storedStart, storedEnd)
	}

	replay, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, req,
	)
	if err != nil {
		t.Fatalf("registered replay: %v", err)
	}
	if !replay.Registered() || client.calls != 1 {
		t.Fatalf("registered replay retransmitted: %+v calls=%d", replay, client.calls)
	}

	overlap := req
	overlap.StartNumber = start + 5
	overlap.EndNumber = start + 15
	overlap.Justification = "Tentativa sobre faixa parcialmente ja inutilizada."
	if _, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, overlap,
	); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("overlap error=%v want ErrConflict", err)
	}
	if client.calls != 1 {
		t.Fatalf("overlap reached remote client: calls=%d", client.calls)
	}

	future := req
	future.StartNumber = start + 150
	future.EndNumber = start + 151
	future.Justification = "Tentativa de inutilizar numeros ainda nao alocados."
	if _, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, future,
	); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("future-range inutilization must fail closed: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("future-range request reached remote client: calls=%d", client.calls)
	}

	ambiguousClient := &inutilizationFakeClient{err: errors.New("simulated timeout")}
	service.SetNFCeRemoteInutilizationClient(ambiguousClient)
	ambiguous := req
	ambiguous.StartNumber = start + 100
	ambiguous.EndNumber = start + 101
	ambiguous.Justification = "Falha operacional exige inutilizar outra faixa fiscal."
	if _, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, ambiguous,
	); err == nil {
		t.Fatal("expected simulated ambiguous transmission error")
	}
	if ambiguousClient.calls != 1 {
		t.Fatalf("ambiguous first attempt calls=%d want=1", ambiguousClient.calls)
	}

	pending, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, ambiguous,
	)
	if err != nil {
		t.Fatalf("ambiguous replay: %v", err)
	}
	if !pending.Pending() || ambiguousClient.calls != 1 {
		t.Fatalf("ambiguous replay retransmitted: %+v calls=%d", pending, ambiguousClient.calls)
	}

	if err := pool.QueryRow(ctx, `
		SELECT status
		FROM nfce_number_inutilizations
		WHERE tenant_id=$1 AND request_id=$2
	`, tenantID, pending.RequestID).Scan(&status); err != nil {
		t.Fatalf("read pending inutilization: %v", err)
	}
	if status != fisc.NFCeInutilizationStatusSubmitted {
		t.Fatalf("pending status=%s want=submitted", status)
	}

	duplicateClient := &inutilizationFakeClient{result: fisc.NFCeInutilizationRemoteResult{
		StatusCode: 563,
		Reason: "Ja existe pedido de inutilizacao com a mesma faixa",
		Protocol: "131260000000009",
		ResponseXML: []byte("<retInutNFe><infInut><cStat>563</cStat><nProt>131260000000009</nProt></infInut></retInutNFe>"),
	}}
	service.SetNFCeRemoteInutilizationClient(duplicateClient)
	duplicate := req
	duplicate.StartNumber = start + 130
	duplicate.EndNumber = start + 131
	duplicate.Justification = "Faixa ja inutilizada durante transmissao anterior."
	observed, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, duplicate,
	)
	if err != nil || !observed.Pending() || observed.StatusCode != 563 ||
		duplicateClient.calls != 1 {
		t.Fatalf("duplicate observation must remain pending: %+v calls=%d err=%v",
			observed, duplicateClient.calls, err)
	}

	var (
		observedStatus string
		observedCode int
		observedProtocol string
	)
	if err := pool.QueryRow(ctx, `
		SELECT status, status_code, protocol
		FROM nfce_number_inutilizations
		WHERE tenant_id=$1 AND request_id=$2
	`, tenantID, observed.RequestID).Scan(
		&observedStatus, &observedCode, &observedProtocol,
	); err != nil {
		t.Fatalf("read duplicate evidence: %v", err)
	}
	if observedStatus != fisc.NFCeInutilizationStatusSubmitted ||
		observedCode != 563 || observedProtocol != "131260000000009" {
		t.Fatalf("duplicate evidence lost: status=%s code=%d protocol=%s",
			observedStatus, observedCode, observedProtocol)
	}

	replayedDuplicate, err := service.InutilizeNFCeNumbers(
		ctx, tenantID, actorUserID, duplicate,
	)
	if err != nil || !replayedDuplicate.Pending() ||
		replayedDuplicate.StatusCode != 563 ||
		replayedDuplicate.Protocol != observedProtocol ||
		duplicateClient.calls != 1 {
		t.Fatalf("duplicate replay must preserve evidence without resend: %+v calls=%d err=%v",
			replayedDuplicate, duplicateClient.calls, err)
	}
}
