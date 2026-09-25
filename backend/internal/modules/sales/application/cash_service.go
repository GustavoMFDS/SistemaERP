package application

import (
	"context"
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

func NewCashService(uow db.UnitOfWork, cash CashRepository, finRepo FinanceRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *CashService {
	return &CashService{uow: uow, cash: cash, fin: finRepo, audit: auditSvc, validate: v, logger: logger}
}

func (s *CashService) CurrentSession(ctx context.Context, tenantID string) (sales.CashSession, bool, error) {
	registerID, err := s.cash.EnsureDefaultRegister(ctx, tenantID)
	if err != nil {
		return sales.CashSession{}, false, err
	}
	return s.cash.GetOpenSession(ctx, tenantID, registerID)
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

func (s *CashService) RecordMovement(ctx context.Context, tenantID, userID, sessionID string, req CashMovementRequest) (string, error) {
	req.Type = strings.TrimSpace(req.Type)
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	session, err := s.cash.GetSession(ctx, tx, tenantID, sessionID)
	if err != nil || session.Status != "open" {
		return "", common.ErrCashSessionClosed
	}

	if req.Type == "withdrawal" {
		payments, err := s.cash.SumPaymentsByMethod(ctx, tx, tenantID, sessionID)
		if err != nil {
			return "", err
		}
		supply, withdrawal, err := s.cash.SumMovements(ctx, tx, tenantID, sessionID)
		if err != nil {
			return "", err
		}
		available := session.OpeningAmount.Add(payments["cash"]).Add(supply).Sub(withdrawal)
		if req.Amount > available {
			return "", common.ErrInsufficientCash
		}
	}

	id, err := s.cash.InsertMovement(ctx, tx, tenantID, sessionID, userID, req.Type, req.Amount, req.Notes)
	if err != nil {
		return "", err
	}

	signedAmount := req.Amount
	if req.Type == "withdrawal" {
		signedAmount = -signedAmount
	}
	if s.fin != nil {
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
			return "", err
		}
	}

	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: userID, Action: "cash." + req.Type,
		ResourceType: "cash_movement", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{
			"cash_session_id": sessionID,
			"amount":          req.Amount.String(),
		},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *CashService) CloseSession(ctx context.Context, tenantID string, userID, sessionID string, req CashCloseRequest) (sales.CashCloseResult, error) {
	if err := s.validate.Struct(req); err != nil {
		return sales.CashCloseResult{}, common.ErrValidation
	}
	for method, amount := range req.ClosingByMethod {
		if !isCashReconciliationMethod(method) || amount < 0 {
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
	expected["cash"] = session.OpeningAmount.Add(payments["cash"]).Add(supply).Sub(withdrawal)

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
			"closing_difference": req.ClosingAmount.Sub(expected["cash"]).String(),
			"expected_by_method": moneyMapStrings(expected),
			"declared_by_method": moneyMapStrings(declared),
		},
	}); err != nil {
		return sales.CashCloseResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return sales.CashCloseResult{}, err
	}

	difference := make(map[string]platform.Money, len(cashReconciliationMethods))
	for _, method := range cashReconciliationMethods {
		difference[method] = declared[method].Sub(expected[method])
	}
	return sales.CashCloseResult{
		ExpectedCash:       expected["cash"],
		ClosingAmount:      req.ClosingAmount,
		ClosingDifference:  req.ClosingAmount.Sub(expected["cash"]),
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
