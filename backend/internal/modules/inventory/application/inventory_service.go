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

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type InventoryService struct {
	cfg      config.Config
	uow      db.UnitOfWork
	inv      InventoryRepository
	products ProductsRepository
	audit    *audit.Service
	validate *validator.Validate
	logger   *slog.Logger
}

// OpeningStockItem represents an absolute opening count, not a repeatable
// additive adjustment. A product with any prior stock activity is rejected.
type OpeningStockItem struct {
	SKU      string            `json:"sku" validate:"required,min=1,max=64"`
	Quantity platform.Quantity `json:"quantity" validate:"gt=0"`
}

type OpeningStockRequest struct {
	Items []OpeningStockItem `json:"items" validate:"required,min=1,max=100,dive"`
}

type OpeningStockResult struct {
	BatchID   string `json:"batch_id"`
	ItemCount int    `json:"item_count"`
	Replayed  bool   `json:"replayed"`
}

type InventoryAdjustRequest struct {
	ProductID string            `json:"product_id" validate:"required"`
	Delta     platform.Quantity `json:"delta" validate:"required,ne=0"`
	Reason    string            `json:"reason" validate:"required,min=3,max=250"`
	Type      string            `json:"type" validate:"required,oneof=adjustment loss damage"`
}

func NewInventoryService(cfg config.Config, uow db.UnitOfWork, invRepo InventoryRepository, productsRepo ProductsRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *InventoryService {
	return &InventoryService{cfg: cfg, uow: uow, inv: invRepo, products: productsRepo, audit: auditSvc, validate: v, logger: logger}
}

func (s *InventoryService) LowStockCount(ctx context.Context, tenantID string) (int, error) {
	return s.inv.LowStockCount(ctx, tenantID)
}

func (s *InventoryService) LowStock(ctx context.Context, tenantID string, limit int) ([]inv.Product, error) {
	return s.inv.LowStock(ctx, tenantID, limit)
}

func (s *InventoryService) ListMovements(ctx context.Context, tenantID string, productID string, limit, offset int) ([]inv.InventoryMovement, int, error) {
	return s.inv.ListMovements(ctx, tenantID, productID, limit, offset)
}

// ImportOpeningStock is all-or-nothing. The key and item fingerprint are
// committed with inventory movements in one PostgreSQL transaction.
func (s *InventoryService) ImportOpeningStock(
	ctx context.Context, tenantID, actorUserID, idempotencyKey string,
	req OpeningStockRequest,
) (OpeningStockResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 ||
		s.validate.Struct(req) != nil {
		return OpeningStockResult{}, common.ErrValidation
	}

	items := append([]OpeningStockItem(nil), req.Items...)
	for i := range items {
		items[i].SKU = strings.TrimSpace(items[i].SKU)
		if items[i].SKU == "" || items[i].Quantity <= 0 {
			return OpeningStockResult{}, common.ErrValidation
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SKU < items[j].SKU })
	skus := make([]string, 0, len(items))
	for i, item := range items {
		if i > 0 && item.SKU == items[i-1].SKU {
			return OpeningStockResult{}, common.ErrValidation
		}
		skus = append(skus, item.SKU)
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return OpeningStockResult{}, err
	}
	sum := sha256.Sum256(payload)
	requestHash := hex.EncodeToString(sum[:])

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return OpeningStockResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.inv.LockOpeningStockKey(ctx, tx, tenantID, idempotencyKey); err != nil {
		return OpeningStockResult{}, err
	}
	batchID, oldHash, itemCount, found, err :=
		s.inv.GetOpeningStockBatch(ctx, tx, tenantID, idempotencyKey)
	if err != nil {
		return OpeningStockResult{}, err
	}
	if found {
		if oldHash != requestHash {
			return OpeningStockResult{}, common.ErrConflict
		}
		return OpeningStockResult{BatchID: batchID, ItemCount: itemCount, Replayed: true}, nil
	}

	products, err := s.products.GetManyBySKUs(ctx, tx, tenantID, skus)
	if err != nil {
		return OpeningStockResult{}, err
	}
	if len(products) != len(skus) {
		return OpeningStockResult{}, common.ErrNotFound
	}
	ids := make([]string, 0, len(skus))
	for _, sku := range skus {
		product := products[sku]
		if !product.Active {
			return OpeningStockResult{}, common.ErrConflict
		}
		ids = append(ids, product.ID)
	}
	sort.Strings(ids)
	if err := s.inv.EnsureBalanceRows(ctx, tx, tenantID, ids); err != nil {
		return OpeningStockResult{}, err
	}
	balances, err := s.inv.GetBalancesForUpdate(ctx, tx, tenantID, ids)
	if err != nil {
		return OpeningStockResult{}, err
	}
	if len(balances) != len(ids) {
		return OpeningStockResult{}, common.ErrConflict
	}
	for _, id := range ids {
		if balances[id].QtyOnHand != 0 {
			return OpeningStockResult{}, common.ErrConflict
		}
	}
	hadMovements, err := s.inv.HasAnyStockMovements(ctx, tx, tenantID, ids)
	if err != nil {
		return OpeningStockResult{}, err
	}
	if hadMovements {
		return OpeningStockResult{}, common.ErrConflict
	}

	batchID, err = s.inv.CreateOpeningStockBatch(
		ctx, tx, tenantID, actorUserID, idempotencyKey, requestHash, len(items),
	)
	if err != nil {
		return OpeningStockResult{}, err
	}
	for _, item := range items {
		productID := products[item.SKU].ID
		before := balances[productID]
		after, err := before.Creditar(item.Quantity)
		if err != nil {
			return OpeningStockResult{}, common.ErrValidation
		}
		if err := s.inv.UpdateBalance(ctx, tx, tenantID, productID, after.QtyOnHand); err != nil {
			return OpeningStockResult{}, err
		}
		reason := "Contagem inicial confirmada"
		refType := "opening_stock"
		actor := actorUserID
		reference := batchID
		movement := inv.NewMovement(
			productID, inv.MovementAdjustment, item.Quantity, before, after,
			&reason, &refType, &reference, &actor, time.Now().Format(time.RFC3339),
		)
		if err := s.inv.InsertMovement(ctx, tx, tenantID, movement); err != nil {
			return OpeningStockResult{}, err
		}
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "inventory.opening_stock.import",
		ResourceType: "opening_stock_batch", ResourceID: batchID, Outcome: "success",
		Metadata: map[string]any{"item_count": len(items), "request_sha256": requestHash},
	}); err != nil {
		return OpeningStockResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OpeningStockResult{}, err
	}

	if cache, ok := s.products.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	}); ok {
		cacheCtx := context.WithoutCancel(ctx)
		for _, id := range ids {
			_ = cache.InvalidateProduct(cacheCtx, tenantID, id)
		}
		_ = cache.BumpProductsListVersion(cacheCtx, tenantID)
	}
	return OpeningStockResult{BatchID: batchID, ItemCount: len(items)}, nil
}

func (s *InventoryService) Adjust(ctx context.Context, tenantID string, actorUserID string, req InventoryAdjustRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	mt := inv.MovementType(req.Type)
	delta, err := mt.NormalizeDelta(req.Delta)
	if err != nil {
		return common.ErrValidation
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.inv.EnsureBalanceRow(ctx, tx, tenantID, req.ProductID); err != nil {
		return err
	}
	bal, err := s.inv.GetBalanceForUpdate(ctx, tx, tenantID, req.ProductID)
	if err != nil {
		return err
	}
	after, derr := bal.AplicarDelta(delta, s.cfg.AllowNegativeStock)
	if derr != nil {
		switch derr {
		case inv.ErrInvalidDelta, inv.ErrInvalidQuantity, inv.ErrInvalidMovementType:
			return common.ErrValidation
		case inv.ErrInsufficientStock:
			return common.ErrInsufficientStock
		default:
			return derr
		}
	}
	if err := s.inv.UpdateBalance(ctx, tx, tenantID, req.ProductID, after.QtyOnHand); err != nil {
		return err
	}

	reason := req.Reason
	refType := "manual_adjust"
	actor := actorUserID
	m := inv.NewMovement(req.ProductID, mt, delta, bal, after, &reason, &refType, nil, &actor, time.Now().Format(time.RFC3339))
	if err := s.inv.InsertMovement(ctx, tx, tenantID, m); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "inventory.adjust",
		ResourceType: "product", ResourceID: req.ProductID, Outcome: "success",
		Metadata: map[string]any{
			"type":  req.Type,
			"delta": delta.String(),
		},
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if cache, ok := s.products.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	}); ok {
		cacheCtx := context.WithoutCancel(ctx)
		_ = cache.InvalidateProduct(cacheCtx, tenantID, req.ProductID)
		_ = cache.BumpProductsListVersion(cacheCtx, tenantID)
	}
	return nil
}
