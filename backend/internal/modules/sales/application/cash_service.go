package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

var cashReconciliationMethods = []string{"cash", "pix", "debit", "credit", "transfer", "voucher"}

type CashService struct {
	uow      db.UnitOfWork
	cash     CashRepository
	fin      FinanceRepository
	audit    *audit.Service
	validate *validator.Validate
	logger   *slog.Logger
}

type CashOpenRequest struct {
	OpeningAmount platform.Money `json:"opening_amount" validate:"min=0"`
	Notes         *string        `json:"notes"`
}

type CashCloseRequest struct {
	ClosingAmount   platform.Money            `json:"closing_amount" validate:"min=0"`
	ClosingByMethod map[string]platform.Money `json:"closing_by_method,omitempty"`
	Notes           *string                   `json:"notes"`
}

type CashMovementRequest struct {
	Type   string         `json:"movement_type" validate:"required,oneof=supply withdrawal"`
	Amount platform.Money `json:"amount" validate:"required,gt=0"`
	Notes  *string        `json:"notes"`
}

func cashMovementRequestHash(sessionID string, req CashMovementRequest) (string, error) {
	payload, err := json.Marshal(struct {
		SessionID string  `json:"cash_session_id"`
		Type      string  `json:"movement_type"`
		Amount    string  `json:"amount"`
		Notes     *string `json:"notes,omitempty"`
	}{
		SessionID: sessionID,
		Type:      req.Type,
		Amount:    req.Amount.String(),
		Notes:     req.Notes,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func NewCashService(uow db.UnitOfWork, cash CashRepository, finRepo FinanceRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *CashService {
	return &CashService{uow: uow, cash: cash, fin: finRepo, audit: auditSvc, validate: v, logger: logger}
}

func (s *CashService) CurrentSession(ctx context.Context, tenantID string) (sales.CashSession, error) {
	return s.cash.GetOpenSession(ctx, tenantID)
}

func (s *CashService) OpenSession(ctx context.Context, tenantID string, userID string, req CashOpenRequest) (string, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	registerID, err := s.cash.EnsureDefaultRegister(ctx, tenantID)
	if err != nil {
		return "", err
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := s.cash.OpenSession(ctx, tx, tenantID, registerID, userID, req.OpeningAmount, req.Notes)
	if err != nil {
		return "", err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: userID, Action: "cash.open",
		ResourceType: "cash_session", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{"opening_amount": req.OpeningAmount.String()},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *CashService) RecordMovement(ctx context.Context, tenantID, userID, sessionID, idempotencyKey string, req CashMovementRequest) (string, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	sessionID = strings.TrimSpace(sessionID)
	req.Type = strings.TrimSpace(req.Type)
	if req.Notes != nil {
		notes := strings.TrimSpace(*req.Notes)
		if notes == "" {
			req.Notes = nil
		} else {
			req.Notes = &notes
		}
	}
	if idempotencyKey == "" || sessionID == "" {
		return "", false, common.ErrValidation
	}
	if err := s.validate.Struct(req); err != nil {
		return "", false, common.ErrValidation
	}
	requestHash, err := cashMovementRequestHash(sessionID, req)
	if err != nil {
		return "", false, common.ErrValidation
	}
	const op = "cash.record_movement"

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.fin.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", false, err
	}
	if resourceID, _, storedHash, _, ok, err := s.fin.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", false, err
	} else if ok {
		if storedHash != requestHash {
			return "", false, common.ErrConflict
		}
		_ = tx.Rollback(ctx)
		return resourceID, false, nil
	}

	session, err := s.cash.GetSession(ctx, tx, tenantID, sessionID)
	if err != nil || session.Status != "open" {
		return "", false, common.ErrCashSessionClosed
	}

	if req.Type == "withdrawal" {
		payments, err := s.cash.SumPaymentsByMethod(ctx, tx, tenantID, sessionID)
		if err != nil {
			return "", false, err
		}
		supply, withdrawal, err := s.cash.SumMovements(ctx, tx, tenantID, sessionID)
		if err != nil {
			return "", false, err
		}
		available, err := session.OpeningAmount.AddChecked(payments["cash"])
		if err != nil {
			return "", false, common.ErrValidation
		}
		available, err = available.AddChecked(supply)
		if err != nil {
			return "", false, common.ErrValidation
		}
		available, err = available.SubChecked(withdrawal)
		if err != nil {
			return "", false, common.ErrValidation
		}
		if req.Amount > available {
			return "", false, common.ErrInsufficientCash
		}
	}

	id, err := s.cash.InsertMovement(ctx, tx, tenantID, sessionID, userID, req.Type, req.Amount, req.Notes)
	if err != nil {
		return "", false, err
	}

	signedAmount := req.Amount
	if req.Type == "withdrawal" {
		signedAmount = -signedAmount
	}
	cashID := sessionID
	actor := userID
	if _, err := s.fin.InsertLedgerEntry(ctx, tx, tenantID, fin.LedgerEntry{
		EntryType:     req.Type,
		CashSessionID: &cashID,
		AmountGross:   signedAmount,
		AmountNet:     signedAmount,
		Notes:         req.Notes,
		CreatedAt:     time.Now().Format(time.RFC3339),
	}, &actor); err != nil {
		return "", false, err
	}

	resultAmount := req.Amount
	if err := s.fin.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, requestHash, id, req.Type, &resultAmount); err != nil {
		return "", false, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: userID, Action: "cash." + req.Type,
		ResourceType: "cash_movement", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{
			"cash_session_id": sessionID,
			"amount":          req.Amount.String(),
		},
	}); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (s *CashService) CloseSession(ctx context.Context, tenantID string, userID, sessionID string, req CashCloseRequest) (sales.CashCloseResult, error) {
	if err := s.validate.Struct(req); err != nil {
		return sales.CashCloseResult{}, common.ErrValidation
	}
	for method, amount := range req.ClosingByMethod {
		if !isCashReconciliationMethod(method) || (method == "cash" && amount < 0) {
			return sales.CashCloseResult{}, common.ErrValidation
		}
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// This row lock serializes sales, movements, cancellations and closure.
	// It is intentionally acquired in a separate statement so the aggregate
	// queries below get a fresh READ COMMITTED snapshot after any waiter.
	session, err := s.cash.GetSession(ctx, tx, tenantID, sessionID)
	if err != nil || session.Status != "open" {
		return sales.CashCloseResult{}, common.ErrCashSessionClosed
	}

	payments, err := s.cash.SumPaymentsByMethod(ctx, tx, tenantID, sessionID)
	if err != nil {
		return sales.CashCloseResult{}, err
	}
	supply, withdrawal, err := s.cash.SumMovements(ctx, tx, tenantID, sessionID)
	if err != nil {
		return sales.CashCloseResult{}, err
	}

	expected := make(map[string]platform.Money, len(cashReconciliationMethods))
	for _, method := range cashReconciliationMethods {
		expected[method] = payments[method]
	}
	expectedCash, err := session.OpeningAmount.AddChecked(payments["cash"])
	if err != nil {
		return sales.CashCloseResult{}, common.ErrValidation
	}
	expectedCash, err = expectedCash.AddChecked(supply)
	if err != nil {
		return sales.CashCloseResult{}, common.ErrValidation
	}
	expectedCash, err = expectedCash.SubChecked(withdrawal)
	if err != nil {
		return sales.CashCloseResult{}, common.ErrValidation
	}
	expected["cash"] = expectedCash

	declared := make(map[string]platform.Money, len(cashReconciliationMethods))
	if len(req.ClosingByMethod) == 0 {
		// Legacy clients only declare physical cash. Non-cash methods are
		// considered system-confirmed until the client adopts per-method close.
		for _, method := range cashReconciliationMethods {
			declared[method] = expected[method]
		}
	} else {
		for _, method := range cashReconciliationMethods {
			declared[method] = req.ClosingByMethod[method]
		}
	}
	declared["cash"] = req.ClosingAmount

	difference := make(map[string]platform.Money, len(cashReconciliationMethods))
	for _, method := range cashReconciliationMethods {
		methodDifference, err := declared[method].SubChecked(expected[method])
		if err != nil {
			return sales.CashCloseResult{}, common.ErrValidation
		}
		difference[method] = methodDifference
	}

	if err := s.cash.CloseSession(
		ctx,
		tx,
		tenantID,
		sessionID,
		userID,
		expected["cash"],
		req.ClosingAmount,
		req.Notes,
	); err != nil {
		return sales.CashCloseResult{}, err
	}
	if err := s.cash.SaveReconciliation(ctx, tx, tenantID, sessionID, expected, declared); err != nil {
		return sales.CashCloseResult{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: userID, Action: "cash.close",
		ResourceType: "cash_session", ResourceID: sessionID, Outcome: "success",
		Metadata: map[string]any{
			"expected_cash":      expected["cash"].String(),
			"closing_amount":     req.ClosingAmount.String(),
			"closing_difference": difference["cash"].String(),
			"expected_by_method": moneyMapStrings(expected),
			"declared_by_method": moneyMapStrings(declared),
		},
	}); err != nil {
		return sales.CashCloseResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return sales.CashCloseResult{}, err
	}

	return sales.CashCloseResult{
		ExpectedCash:       expected["cash"],
		ClosingAmount:      req.ClosingAmount,
		ClosingDifference:  difference["cash"],
		ExpectedByMethod:   expected,
		DeclaredByMethod:   declared,
		DifferenceByMethod: difference,
	}, nil
}

func isCashReconciliationMethod(method string) bool {
	for _, candidate := range cashReconciliationMethods {
		if method == candidate {
			return true
		}
	}
	return false
}

func moneyMapStrings(values map[string]platform.Money) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value.String()
	}
	return out
}
