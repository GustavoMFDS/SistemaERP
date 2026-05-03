package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/sistemaemgo/internal/config"
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
	validate *validator.Validate
	logger   *slog.Logger
}

type InventoryAdjustRequest struct {
	ProductID string            `json:"product_id" validate:"required"`
	Delta     platform.Quantity `json:"delta" validate:"required,ne=0"`
	Reason    string            `json:"reason" validate:"required,min=3,max=250"`
	Type      string            `json:"type" validate:"required,oneof=purchase adjustment loss damage return"`
}

func NewInventoryService(cfg config.Config, uow db.UnitOfWork, invRepo InventoryRepository, productsRepo ProductsRepository, v *validator.Validate, logger *slog.Logger) *InventoryService {
	return &InventoryService{cfg: cfg, uow: uow, inv: invRepo, products: productsRepo, validate: v, logger: logger}
}

func (s *InventoryService) LowStock(ctx context.Context, tenantID string, limit int) ([]inv.Product, error) {
	return s.inv.LowStock(ctx, tenantID, limit)
}

func (s *InventoryService) ListMovements(ctx context.Context, tenantID string, productID string, limit, offset int) ([]inv.InventoryMovement, int, error) {
	return s.inv.ListMovements(ctx, tenantID, productID, limit, offset)
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

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
