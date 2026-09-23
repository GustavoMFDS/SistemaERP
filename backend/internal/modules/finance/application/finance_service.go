package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type FinanceService struct {
	uow      db.UnitOfWork
	repo     FinanceRepository
	audit    *audit.Service
	validate *validator.Validate
	logger   *slog.Logger
}

type PaymentReconcileRequest struct {
	ReceivedAmount platform.Money `json:"received_amount" validate:"min=0"`
	FeeAmount      platform.Money `json:"fee_amount" validate:"min=0"`
	Provider       *string        `json:"provider" validate:"omitempty,max=100"`
	ExternalRef    *string        `json:"external_ref" validate:"omitempty,max=200"`
	Notes          *string        `json:"notes" validate:"omitempty,max=1000"`
}

type ReturnRefundRequest struct {
	Method        string         `json:"method" validate:"required,oneof=cash pix debit credit transfer voucher"`
	Amount        platform.Money `json:"amount" validate:"required,gt=0"`
	Provider      *string        `json:"provider" validate:"omitempty,max=100"`
	ExternalRef   *string        `json:"external_ref" validate:"omitempty,max=200"`
	CashSessionID *string        `json:"cash_session_id"`
	Notes         *string        `json:"notes" validate:"omitempty,max=1000"`
}

func NewFinanceService(uow db.UnitOfWork, repo FinanceRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *FinanceService {
	return &FinanceService{uow: uow, repo: repo, audit: auditSvc, validate: v, logger: logger}
}

func (s *FinanceService) Dashboard(ctx context.Context, tenantID string, from, to string) (map[string]platform.Money, error) {
	return s.repo.Dashboard(ctx, tenantID, from, to)
}

func (s *FinanceService) ListLedger(ctx context.Context, tenantID string, limit, offset int) ([]fin.LedgerEntry, int, error) {
	return s.repo.ListLedger(ctx, tenantID, limit, offset)
}

func (s *FinanceService) ListPayments(ctx context.Context, tenantID, from, to, method, status string, limit, offset int) ([]fin.PaymentRecord, int, error) {
	switch method {
	case "", "cash", "pix", "debit", "credit", "transfer", "voucher":
	default:
		return nil, 0, common.ErrValidation
	}
	switch status {
	case "", "pending", "reconciled", "divergent", "not_applicable":
	default:
		return nil, 0, common.ErrValidation
	}
	return s.repo.ListPayments(ctx, tenantID, from, to, method, status, limit, offset)
}

func (s *FinanceService) ListReturnRefunds(ctx context.Context, tenantID, status string, limit, offset int) ([]fin.ReturnRefundSummary, int, error) {
	switch status {
	case "", "pending", "partial", "settled":
	default:
		return nil, 0, common.ErrValidation
	}
	return s.repo.ListReturnRefunds(ctx, tenantID, status, limit, offset)
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	if v == "" {
		return nil
	}
	return &v
}

func financeHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *FinanceService) ReconcilePayment(ctx context.Context, tenantID, actorUserID, paymentID, idempotencyKey string, req PaymentReconcileRequest) (string, string, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	paymentID = strings.TrimSpace(paymentID)
	req.Provider = normalizeOptional(req.Provider)
	req.ExternalRef = normalizeOptional(req.ExternalRef)
	req.Notes = normalizeOptional(req.Notes)
	if idempotencyKey == "" || paymentID == "" {
		return "", "", false, common.ErrValidation
	}
	if err := s.validate.Struct(req); err != nil || req.FeeAmount > req.ReceivedAmount {
		return "", "", false, common.ErrValidation
	}
	hash, err := financeHash(struct {
		PaymentID string                  `json:"payment_id"`
		Request   PaymentReconcileRequest `json:"request"`
	}{paymentID, req})
	if err != nil {
		return "", "", false, common.ErrValidation
	}
	op := "payment.reconcile:" + paymentID

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", "", false, err
	}
	if resourceID, status, storedHash, _, ok, err := s.repo.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", "", false, err
	} else if ok {
		if storedHash != hash {
			return "", "", false, common.ErrConflict
		}
		_ = tx.Rollback(ctx)
		return resourceID, status, false, nil
	}

	payment, err := s.repo.GetPaymentForUpdate(ctx, tx, tenantID, paymentID)
	if err != nil {
		return "", "", false, err
	}
	if payment.Method == "cash" || payment.ReconciliationStatus != "pending" {
		return "", "", false, common.ErrConflict
	}

	difference := req.ReceivedAmount.Sub(payment.Amount)
	status := "reconciled"
	if difference != 0 {
		status = "divergent"
	}
	net := req.ReceivedAmount.Sub(req.FeeAmount)
	if net < 0 {
		return "", "", false, common.ErrValidation
	}

	reconciliationID, err := s.repo.CreatePaymentReconciliation(ctx, tx, tenantID, fin.PaymentReconciliation{
		PaymentID: paymentID, ExpectedAmount: payment.Amount, ReceivedAmount: req.ReceivedAmount,
		FeeAmount: req.FeeAmount, NetAmount: net, Difference: difference, Status: status,
		Provider: req.Provider, ExternalRef: req.ExternalRef, Notes: req.Notes, CreatedBy: actorUserID,
	})
	if err != nil {
		return "", "", false, err
	}
	if err := s.repo.UpdatePaymentReconciliation(ctx, tx, tenantID, paymentID, status, req.ReceivedAmount, req.FeeAmount, req.Provider, req.ExternalRef, req.Notes, actorUserID); err != nil {
		return "", "", false, err
	}
	if err := s.repo.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, hash, reconciliationID, status, nil); err != nil {
		return "", "", false, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "payment.reconcile",
		ResourceType: "payment", ResourceID: paymentID, Outcome: "success",
		Metadata: map[string]any{
			"reconciliation_id": reconciliationID,
			"expected_amount": payment.Amount.String(),
			"received_amount": req.ReceivedAmount.String(),
			"fee_amount": req.FeeAmount.String(),
			"status": status,
		},
	}); err != nil {
		return "", "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", false, err
	}
	return reconciliationID, status, true, nil
}

func (s *FinanceService) SettleReturnRefund(ctx context.Context, tenantID, actorUserID, returnID, idempotencyKey string, req ReturnRefundRequest) (string, string, platform.Money, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	returnID = strings.TrimSpace(returnID)
	req.Method = strings.TrimSpace(req.Method)
	req.Provider = normalizeOptional(req.Provider)
	req.ExternalRef = normalizeOptional(req.ExternalRef)
	req.CashSessionID = normalizeOptional(req.CashSessionID)
	req.Notes = normalizeOptional(req.Notes)
	if idempotencyKey == "" || returnID == "" {
		return "", "", 0, false, common.ErrValidation
	}
	if err := s.validate.Struct(req); err != nil {
		return "", "", 0, false, common.ErrValidation
	}
	if req.Method == "cash" && req.CashSessionID == nil {
		return "", "", 0, false, common.ErrValidation
	}
	hash, err := financeHash(struct {
		ReturnID string              `json:"return_id"`
		Request  ReturnRefundRequest `json:"request"`
	}{returnID, req})
	if err != nil {
		return "", "", 0, false, common.ErrValidation
	}
	op := "return.refund:" + returnID

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", "", 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", "", 0, false, err
	}
	if resourceID, status, storedHash, storedRemaining, ok, err := s.repo.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", "", 0, false, err
	} else if ok {
		if storedHash != hash {
			return "", "", 0, false, common.ErrConflict
		}
		if storedRemaining != nil {
			_ = tx.Rollback(ctx)
			return resourceID, status, *storedRemaining, false, nil
		}
		// Backward-compatible fallback for rows created before result_amount existed.
		item, err := s.repo.GetReturnForUpdate(ctx, tx, tenantID, returnID)
		if err != nil {
			return "", "", 0, false, err
		}
		settled, err := s.repo.SumReturnRefunds(ctx, tx, tenantID, returnID)
		if err != nil {
			return "", "", 0, false, err
		}
		_ = tx.Rollback(ctx)
		return resourceID, status, item.RefundDue.Sub(settled), false, nil
	}

	item, err := s.repo.GetReturnForUpdate(ctx, tx, tenantID, returnID)
	if err != nil {
		return "", "", 0, false, err
	}
	settled, err := s.repo.SumReturnRefunds(ctx, tx, tenantID, returnID)
	if err != nil {
		return "", "", 0, false, err
	}
	remaining := item.RefundDue.Sub(settled)
	if remaining <= 0 || req.Amount > remaining {
		return "", "", remaining, false, common.ErrConflict
	}

	if req.CashSessionID != nil {
		available, err := s.repo.GetOpenCashAvailable(ctx, tx, tenantID, *req.CashSessionID)
		if err != nil {
			return "", "", remaining, false, err
		}
		if req.Method == "cash" {
			if req.Amount > available {
				return "", "", remaining, false, common.ErrInsufficientCash
			}
			noteText := "Reembolso de devolucao " + returnID
			if req.Notes != nil {
				noteText += ": " + *req.Notes
			}
			if _, err := s.repo.InsertCashWithdrawal(ctx, tx, tenantID, *req.CashSessionID, actorUserID, req.Amount, &noteText); err != nil {
				return "", "", remaining, false, err
			}
		}
	}

	refundID, err := s.repo.CreateReturnRefund(ctx, tx, tenantID, fin.ReturnRefund{
		ReturnID: returnID, SaleID: item.SaleID, Method: req.Method, Amount: req.Amount,
		Provider: req.Provider, ExternalRef: req.ExternalRef, CashSessionID: req.CashSessionID,
		Notes: req.Notes, CreatedBy: actorUserID,
	})
	if err != nil {
		return "", "", remaining, false, err
	}

	saleID := item.SaleID
	note := "Liquidacao de reembolso da devolucao " + returnID
	cashSessionID := req.CashSessionID
	if _, err := s.repo.InsertLedgerEntry(ctx, tx, tenantID, fin.LedgerEntry{
		EntryType: "return_refund", SaleID: &saleID, CashSessionID: cashSessionID,
		AmountGross: req.Amount.Neg(), AmountNet: req.Amount.Neg(), Notes: &note,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, &actorUserID); err != nil {
		return "", "", remaining, false, err
	}

	remainingAfter := remaining.Sub(req.Amount)
	status := "partial"
	if remainingAfter == 0 {
		status = "settled"
	}
	if err := s.repo.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, hash, refundID, status, &remainingAfter); err != nil {
		return "", "", remaining, false, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "return.refund",
		ResourceType: "sale_return", ResourceID: returnID, Outcome: "success",
		Metadata: map[string]any{
			"refund_id": refundID, "method": req.Method, "amount": req.Amount.String(),
			"remaining_amount": remainingAfter.String(), "status": status,
		},
	}); err != nil {
		return "", "", remaining, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", remaining, false, err
	}
	return refundID, status, remainingAfter, true, nil
}

func IsFinanceNotFound(err error) bool {
	return errors.Is(err, common.ErrNotFound)
}
