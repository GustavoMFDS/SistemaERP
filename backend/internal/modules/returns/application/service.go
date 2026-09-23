package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	ret "github.com/example/sistemaemgo/internal/modules/returns/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type Service struct {
	uow       db.UnitOfWork
	repo      Repository
	inventory invapp.InventoryRepository
	products  invapp.ProductsRepository
	audit     *audit.Service
	validate  *validator.Validate
	logger    *slog.Logger
}

type ItemRequest struct {
	SaleItemID string            `json:"sale_item_id" validate:"required"`
	Qty        platform.Quantity `json:"qty" validate:"required,gt=0"`
	Restock    bool              `json:"restock"`
}

type CreateRequest struct {
	Kind   ret.Kind      `json:"kind" validate:"required,oneof=return exchange"`
	Reason string        `json:"reason" validate:"required,min=3,max=250"`
	Items  []ItemRequest `json:"items" validate:"required,min=1,dive"`
}

func NewService(uow db.UnitOfWork, repo Repository, inventory invapp.InventoryRepository, products invapp.ProductsRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *Service {
	return &Service{uow: uow, repo: repo, inventory: inventory, products: products, audit: auditSvc, validate: v, logger: logger}
}

func (s *Service) List(ctx context.Context, tenantID, saleID string, limit, offset int) ([]ret.SaleReturn, int, error) {
	return s.repo.List(ctx, tenantID, strings.TrimSpace(saleID), limit, offset)
}

func (s *Service) Get(ctx context.Context, tenantID, id string) (ret.SaleReturn, []ret.Item, error) {
	return s.repo.Get(ctx, tenantID, id)
}

func normalizeCreateRequest(req CreateRequest) CreateRequest {
	req.Reason = strings.TrimSpace(req.Reason)
	sort.Slice(req.Items, func(i, j int) bool { return req.Items[i].SaleItemID < req.Items[j].SaleItemID })
	return req
}

func requestHash(req CreateRequest, saleID string) (string, error) {
	raw, err := json.Marshal(struct {
		SaleID  string        `json:"sale_id"`
		Request CreateRequest `json:"request"`
	}{SaleID: saleID, Request: req})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func roundDiv(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	if n < 0 {
		return -roundDiv(-n, d)
	}
	return (n + d / 2) / d
}

func (s *Service) Create(ctx context.Context, tenantID, actorUserID, saleID, idempotencyKey string, req CreateRequest) (string, platform.Money, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	saleID = strings.TrimSpace(saleID)
	req = normalizeCreateRequest(req)
	if idempotencyKey == "" || saleID == "" {
		return "", 0, false, common.ErrValidation
	}
	if err := s.validate.Struct(req); err != nil {
		return "", 0, false, common.ErrValidation
	}

	seen := make(map[string]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, exists := seen[item.SaleItemID]; exists {
			return "", 0, false, common.ErrValidation
		}
		seen[item.SaleItemID] = struct{}{}
	}
	hash, err := requestHash(req, saleID)
	if err != nil {
		return "", 0, false, common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const op = "sale.return"
	if err := s.repo.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", 0, false, err
	}
	if returnID, storedHash, refundDue, ok, err := s.repo.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", 0, false, err
	} else if ok {
		if storedHash != hash {
			return "", 0, false, common.ErrConflict
		}
		_ = tx.Rollback(ctx)
		return returnID, refundDue, false, nil
	}

	sale, saleItems, err := s.repo.GetSaleForUpdate(ctx, tx, tenantID, saleID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return "", 0, false, common.ErrNotFound
		}
		return "", 0, false, err
	}
	if sale.Status != "finalized" {
		return "", 0, false, common.ErrConflict
	}

	itemByID := make(map[string]salesItemView, len(saleItems))
	itemIDs := make([]string, 0, len(saleItems))
	var totalBasis int64
	for _, item := range saleItems {
		itemByID[item.ID] = salesItemView{
			ID: item.ID, ProductID: item.ProductID, Qty: item.Qty, Subtotal: item.Subtotal,
		}
		itemIDs = append(itemIDs, item.ID)
		totalBasis += item.Subtotal.Cents()
	}
	if totalBasis <= 0 {
		return "", 0, false, common.ErrConflict
	}

	alreadyReturned, err := s.repo.SumReturnedBySaleItem(ctx, tx, tenantID, itemIDs)
	if err != nil {
		return "", 0, false, err
	}
	alreadyRefunded, err := s.repo.SumRefundDue(ctx, tx, tenantID, saleID)
	if err != nil {
		return "", 0, false, err
	}
	remainingRefund := sale.Total.Sub(alreadyRefunded)
	if remainingRefund < 0 {
		return "", 0, false, common.ErrConflict
	}

	productIDs := make([]string, 0, len(req.Items))
	restockProducts := make(map[string]struct{}, len(req.Items))
	resultItems := make([]ret.Item, 0, len(req.Items))
	var refundDue platform.Money
	allFullyReturned := true
	for _, requested := range req.Items {
		item, ok := itemByID[requested.SaleItemID]
		if !ok {
			return "", 0, false, common.ErrValidation
		}
		remainingQty := item.Qty - alreadyReturned[item.ID]
		if requested.Qty <= 0 || requested.Qty > remainingQty {
			return "", 0, false, common.ErrValidation
		}
		newReturnedQty := alreadyReturned[item.ID] + requested.Qty
		alreadyReturned[item.ID] = newReturnedQty

		returnedLineBasis := roundDiv(item.Subtotal.Cents() * requested.Qty.Milli(), item.Qty.Milli())
		itemRefund := platform.NewMoneyCents(roundDiv(sale.Total.Cents() * returnedLineBasis, totalBasis))
		refundDue = refundDue.Add(itemRefund)
		resultItems = append(resultItems, ret.Item{
			SaleItemID: item.ID, ProductID: item.ProductID, Qty: requested.Qty,
			Restock: requested.Restock, RefundValue: itemRefund,
		})
		if requested.Restock {
			if _, exists := restockProducts[item.ProductID]; !exists {
				restockProducts[item.ProductID] = struct{}{}
				productIDs = append(productIDs, item.ProductID)
			}
		}
	}
	for _, item := range saleItems {
		if alreadyReturned[item.ID] < item.Qty {
			allFullyReturned = false
			break
		}
	}
	if refundDue > remainingRefund {
		return "", 0, false, common.ErrConflict
	}
	if allFullyReturned && refundDue != remainingRefund && len(resultItems) > 0 {
		delta := remainingRefund.Sub(refundDue)
		resultItems[len(resultItems) - 1].RefundValue = resultItems[len(resultItems) - 1].RefundValue.Add(delta)
		refundDue = remainingRefund
	}

	sort.Strings(productIDs)
	if len(productIDs) > 0 {
		if batch, ok := s.inventory.(interface {
			EnsureBalanceRows(context.Context, db.DBTX, string, []string) error
			GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
		}); ok {
			if err := batch.EnsureBalanceRows(ctx, tx, tenantID, productIDs); err != nil {
				return "", 0, false, err
			}
		} else {
			for _, productID := range productIDs {
				if err := s.inventory.EnsureBalanceRow(ctx, tx, tenantID, productID); err != nil {
					return "", 0, false, err
				}
			}
		}
	}

	balances := make(map[string]inv.InventoryBalance, len(productIDs))
	if len(productIDs) > 0 {
		if batch, ok := s.inventory.(interface {
			GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
		}); ok {
			balances, err = batch.GetBalancesForUpdate(ctx, tx, tenantID, productIDs)
			if err != nil {
				return "", 0, false, err
			}
		} else {
			for _, productID := range productIDs {
				if _, exists := balances[productID]; exists {
					continue
				}
				bal, err := s.inventory.GetBalanceForUpdate(ctx, tx, tenantID, productID)
				if err != nil {
					return "", 0, false, err
				}
				balances[productID] = bal
			}
		}
	}

	returnID, err := s.repo.CreateReturn(ctx, tx, tenantID, ret.SaleReturn{
		SaleID: saleID, Kind: req.Kind, Reason: req.Reason, RefundDue: refundDue,
		CreatedByUserID: actorUserID,
	})
	if err != nil {
		return "", 0, false, err
	}

	for _, item := range resultItems {
		item.ReturnID = returnID
		if item.Restock {
			bal, ok := balances[item.ProductID]
			if !ok {
				return "", 0, false, common.ErrValidation
			}
			after, err := bal.Creditar(item.Qty)
			if err != nil {
				return "", 0, false, common.ErrValidation
			}
			if err := s.inventory.UpdateBalance(ctx, tx, tenantID, item.ProductID, after.QtyOnHand); err != nil {
				return "", 0, false, err
			}
			balances[item.ProductID] = after
			reason := "Devolucao de venda"
			refType := "sale_return"
			refID := returnID
			actor := actorUserID
			mv := inv.NewMovement(item.ProductID, inv.MovementReturn, item.Qty, bal, after, &reason, &refType, &refID, &actor, time.Now().UTC().Format(time.RFC3339))
			if err := s.inventory.InsertMovement(ctx, tx, tenantID, mv); err != nil {
				return "", 0, false, err
			}
		}
		if err := s.repo.InsertItem(ctx, tx, tenantID, item); err != nil {
			return "", 0, false, err
		}
	}

	if err := s.repo.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, hash, returnID, refundDue); err != nil {
		return "", 0, false, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "sale.return",
		ResourceType: "sale_return", ResourceID: returnID, Outcome: "success",
		Metadata: map[string]any{
			"sale_id": saleID, "kind": string(req.Kind), "refund_due": refundDue.String(),
			"item_count": len(resultItems),
		},
	}); err != nil {
		return "", 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", 0, false, err
	}
	s.invalidateProductCaches(context.WithoutCancel(ctx), tenantID, productIDs)
	return returnID, refundDue, true, nil
}

type salesItemView struct {
	ID        string
	ProductID string
	Qty       platform.Quantity
	Subtotal  platform.Money
}

func (s *Service) invalidateProductCaches(ctx context.Context, tenantID string, productIDs []string) {
	cache, ok := s.products.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	})
	if !ok {
		return
	}
	seen := map[string]struct{}{}
	for _, productID := range productIDs {
		if _, exists := seen[productID]; exists {
			continue
		}
		seen[productID] = struct{}{}
		_ = cache.InvalidateProduct(ctx, tenantID, productID)
	}
	_ = cache.BumpProductsListVersion(ctx, tenantID)
}
