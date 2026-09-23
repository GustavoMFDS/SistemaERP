package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	ret "github.com/example/sistemaemgo/internal/modules/returns/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type Service struct {
	uow       db.UnitOfWork
	repo      Repository
	sales     SalesRepository
	inventory InventoryRepository
	finance   FinanceRepository
	products  any
	validate  *validator.Validate
	logger    *slog.Logger
}

type ReturnItemRequest struct {
	SaleItemID string            `json:"sale_item_id" validate:"required"`
	Qty        platform.Quantity `json:"qty" validate:"required,gt=0"`
	Restock    bool              `json:"restock"`
}

type CreateReturnRequest struct {
	Reason string              `json:"reason" validate:"required,min=3,max=250"`
	Items  []ReturnItemRequest `json:"items" validate:"required,min=1,dive"`
}

type CreateRefundRequest struct {
	Method            string         `json:"method" validate:"required,oneof=cash pix debit credit transfer voucher store_credit"`
	Amount            platform.Money `json:"amount" validate:"required,gt=0"`
	ExternalReference *string        `json:"external_reference" validate:"omitempty,max=200"`
	Notes             *string        `json:"notes" validate:"omitempty,max=1000"`
}

func NewService(uow db.UnitOfWork, repo Repository, salesRepo SalesRepository, inventory InventoryRepository, finance FinanceRepository, products any, v *validator.Validate, logger *slog.Logger) *Service {
	return &Service{
		uow: uow, repo: repo, sales: salesRepo, inventory: inventory, finance: finance,
		products: products, validate: v, logger: logger,
	}
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func requestHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) ListReturns(ctx context.Context, tenantID, saleID string, limit, offset int) ([]ret.Return, int, error) {
	return s.repo.ListReturns(ctx, tenantID, saleID, limit, offset)
}

func (s *Service) GetReturn(ctx context.Context, tenantID, returnID string) (ret.Return, []ret.ReturnItem, []ret.Refund, error) {
	return s.repo.GetReturn(ctx, tenantID, returnID)
}

func (s *Service) CreateReturn(ctx context.Context, tenantID, actorUserID, saleID, idempotencyKey string, req CreateReturnRequest) (ret.Return, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	req.Reason = strings.TrimSpace(req.Reason)
	if idempotencyKey == "" || s.validate.Struct(req) != nil {
		return ret.Return{}, false, common.ErrValidation
	}

	seen := make(map[string]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, ok := seen[item.SaleItemID]; ok {
			return ret.Return{}, false, common.ErrValidation
		}
		seen[item.SaleItemID] = struct{}{}
	}

	hash, err := requestHash(struct {
		SaleID  string              `json:"sale_id"`
		Request CreateReturnRequest `json:"request"`
	}{SaleID: saleID, Request: req})
	if err != nil {
		return ret.Return{}, false, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return ret.Return{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	op := "sale.return.create"
	if err := s.repo.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return ret.Return{}, false, err
	}
	if resourceID, _, storedHash, ok, err := s.repo.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return ret.Return{}, false, err
	} else if ok {
		if storedHash != hash {
			return ret.Return{}, false, common.ErrConflict
		}
		value, _, _, err := s.repo.GetReturn(ctx, tenantID, resourceID)
		_ = tx.Rollback(ctx)
		return value, false, err
	}

	sale, saleItems, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, saleID)
	if err != nil {
		return ret.Return{}, false, common.ErrNotFound
	}
	if sale.Status != "finalized" {
		return ret.Return{}, false, common.ErrConflict
	}

	returned, err := s.repo.GetReturnedAggregates(ctx, tx, tenantID, saleID)
	if err != nil {
		return ret.Return{}, false, err
	}
	allocated := allocateSaleTotal(sale, saleItems)
	itemByID := make(map[string]sales.SaleItem, len(saleItems))
	for _, item := range saleItems {
		itemByID[item.ID] = item
	}

	computed := make([]ret.ReturnItem, 0, len(req.Items))
	var totalAmount platform.Money
	var recoveredCost platform.Money
	restockProductIDs := make([]string, 0, len(req.Items))

	for _, requested := range req.Items {
		item, ok := itemByID[requested.SaleItemID]
		if !ok {
			return ret.Return{}, false, common.ErrValidation
		}
		prior := returned[item.ID]
		remainingQty := item.Qty - prior.Qty
		if requested.Qty <= 0 || requested.Qty > remainingQty {
			return ret.Return{}, false, common.ErrConflict
		}

		itemAllocated := allocated[item.ID]
		remainingAmount := itemAllocated - prior.Amount
		amount := proportionalMoney(itemAllocated, requested.Qty, item.Qty)
		if requested.Qty == remainingQty || amount > remainingAmount {
			amount = remainingAmount
		}
		if amount < 0 {
			return ret.Return{}, false, common.ErrConflict
		}

		costRecovered := platform.Money(0)
		if requested.Restock {
			costRecovered = item.CostUnit.MulQty(requested.Qty)
			restockProductIDs = append(restockProductIDs, item.ProductID)
		}

		computed = append(computed, ret.ReturnItem{
			SaleItemID: item.ID, ProductID: item.ProductID, Qty: requested.Qty,
			Restock: requested.Restock, Amount: amount, RecoveredCost: costRecovered,
		})
		totalAmount += amount
		recoveredCost += costRecovered
	}

	sort.Strings(restockProductIDs)
	restockProductIDs = uniqueStrings(restockProductIDs)
	balances := map[string]inv.InventoryBalance{}
	if len(restockProductIDs) > 0 {
		if batch, ok := s.inventory.(interface {
			EnsureBalanceRows(context.Context, db.DBTX, string, []string) error
			GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
		}); ok {
			if err := batch.EnsureBalanceRows(ctx, tx, tenantID, restockProductIDs); err != nil {
				return ret.Return{}, false, err
			}
			balances, err = batch.GetBalancesForUpdate(ctx, tx, tenantID, restockProductIDs)
			if err != nil {
				return ret.Return{}, false, err
			}
		} else {
			for _, productID := range restockProductIDs {
				if err := s.inventory.EnsureBalanceRow(ctx, tx, tenantID, productID); err != nil {
					return ret.Return{}, false, err
				}
				balance, err := s.inventory.GetBalanceForUpdate(ctx, tx, tenantID, productID)
				if err != nil {
					return ret.Return{}, false, err
				}
				balances[productID] = balance
			}
		}
	}

	value := ret.Return{
		SaleID: saleID, Reason: req.Reason, TotalAmount: totalAmount,
		RecoveredCost: recoveredCost, CreatedBy: actorUserID,
	}
	returnID, err := s.repo.CreateReturn(ctx, tx, tenantID, value)
	if err != nil {
		return ret.Return{}, false, err
	}
	value.ID = returnID

	for _, item := range computed {
		item.ReturnID = returnID
		if err := s.repo.InsertReturnItem(ctx, tx, tenantID, item); err != nil {
			return ret.Return{}, false, err
		}
		if !item.Restock {
			continue
		}
		balance, ok := balances[item.ProductID]
		if !ok {
			return ret.Return{}, false, common.ErrConflict
		}
		after, domainErr := balance.Creditar(item.Qty)
		if domainErr != nil {
			return ret.Return{}, false, common.ErrValidation
		}
		if err := s.inventory.UpdateBalance(ctx, tx, tenantID, item.ProductID, after.QtyOnHand); err != nil {
			return ret.Return{}, false, err
		}
		balances[item.ProductID] = after
		reason := "Devolucao de venda: " + req.Reason
		refType := "sale_return"
		refID := returnID
		actor := actorUserID
		movement := inv.NewMovement(item.ProductID, inv.MovementReturn, item.Qty, balance, after, &reason, &refType, &refID, &actor, time.Now().Format(time.RFC3339))
		if err := s.inventory.InsertMovement(ctx, tx, tenantID, movement); err != nil {
			return ret.Return{}, false, err
		}
	}

	saleIDPtr := saleID
	cashIDPtr := sale.CashSessionID
	note := "Devolucao: " + req.Reason
	profitReversal := -(totalAmount - recoveredCost)
	if _, err := s.finance.InsertLedgerEntry(ctx, tx, tenantID, fin.LedgerEntry{
		EntryType:       "sale_return",
		SaleID:          &saleIDPtr,
		CashSessionID:   &cashIDPtr,
		AmountGross:     -totalAmount,
		AmountDiscount:  0,
		AmountNet:       -totalAmount,
		ProfitEstimated: profitReversal,
		Notes:           &note,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}, &actorUserID); err != nil {
		return ret.Return{}, false, err
	}

	if err := s.repo.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, hash, returnID, "created"); err != nil {
		return ret.Return{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ret.Return{}, false, err
	}
	s.invalidateProducts(context.WithoutCancel(ctx), tenantID, restockProductIDs)
	return value, true, nil
}

func (s *Service) CreateRefund(ctx context.Context, tenantID, actorUserID, saleID, returnID, idempotencyKey string, req CreateRefundRequest) (ret.Refund, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	req.ExternalReference = normalizeOptional(req.ExternalReference)
	req.Notes = normalizeOptional(req.Notes)
	if idempotencyKey == "" || s.validate.Struct(req) != nil {
		return ret.Refund{}, false, common.ErrValidation
	}

	hash, err := requestHash(struct {
		SaleID   string              `json:"sale_id"`
		ReturnID string              `json:"return_id"`
		Request  CreateRefundRequest `json:"request"`
	}{SaleID: saleID, ReturnID: returnID, Request: req})
	if err != nil {
		return ret.Refund{}, false, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return ret.Refund{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	op := "sale.return.refund"
	if err := s.repo.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return ret.Refund{}, false, err
	}
	if resourceID, _, storedHash, ok, err := s.repo.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return ret.Refund{}, false, err
	} else if ok {
		if storedHash != hash {
			return ret.Refund{}, false, common.ErrConflict
		}
		value, err := s.repo.GetRefund(ctx, tenantID, resourceID)
		_ = tx.Rollback(ctx)
		return value, false, err
	}

	returnValue, err := s.repo.GetReturnForUpdate(ctx, tx, tenantID, returnID)
	if err != nil {
		return ret.Refund{}, false, common.ErrNotFound
	}
	if returnValue.SaleID != saleID {
		return ret.Refund{}, false, common.ErrNotFound
	}
	if returnValue.RefundedAmount+req.Amount > returnValue.TotalAmount {
		return ret.Refund{}, false, common.ErrConflict
	}

	value := ret.Refund{
		ReturnID: returnID, SaleID: saleID, Method: req.Method, Amount: req.Amount,
		ExternalReference: req.ExternalReference, Notes: req.Notes, CreatedBy: actorUserID,
	}
	refundID, err := s.repo.CreateRefund(ctx, tx, tenantID, value)
	if err != nil {
		return ret.Refund{}, false, err
	}
	value.ID = refundID

	if err := s.repo.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, hash, refundID, "recorded"); err != nil {
		return ret.Refund{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ret.Refund{}, false, err
	}
	return value, true, nil
}

func allocateSaleTotal(sale sales.Sale, items []sales.SaleItem) map[string]platform.Money {
	out := make(map[string]platform.Money, len(items))
	var basis platform.Money
	for _, item := range items {
		basis += item.Subtotal
	}
	if len(items) == 0 || basis <= 0 {
		return out
	}

	remaining := sale.Total
	for i, item := range items {
		if i == len(items)-1 {
			out[item.ID] = remaining
			break
		}
		amount := proportionalMoneyByInt(sale.Total, item.Subtotal.Cents(), basis.Cents())
		if amount < 0 {
			amount = 0
		}
		if amount > remaining {
			amount = remaining
		}
		out[item.ID] = amount
		remaining -= amount
	}
	return out
}

func proportionalMoney(total platform.Money, part, whole platform.Quantity) platform.Money {
	return proportionalMoneyByInt(total, part.Milli(), whole.Milli())
}

func proportionalMoneyByInt(total platform.Money, part, whole int64) platform.Money {
	if total <= 0 || part <= 0 || whole <= 0 {
		return 0
	}
	numerator := total.Cents() * part
	return platform.NewMoneyCents((numerator + whole/2) / whole)
}

func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:0]
	var previous string
	for i, value := range values {
		if i == 0 || value != previous {
			out = append(out, value)
			previous = value
		}
	}
	return out
}

func (s *Service) invalidateProducts(ctx context.Context, tenantID string, productIDs []string) {
	cache, ok := s.products.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	})
	if !ok {
		return
	}
	for _, productID := range productIDs {
		_ = cache.InvalidateProduct(ctx, tenantID, productID)
	}
	if len(productIDs) > 0 {
		_ = cache.BumpProductsListVersion(ctx, tenantID)
	}
}
