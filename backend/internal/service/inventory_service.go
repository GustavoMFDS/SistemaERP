package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/repo"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
)

type InventoryService struct {
	cfg      config.Config
	inv      *repo.InventoryRepo
	products *repo.ProductsRepo
	validate *validator.Validate
	logger   *slog.Logger
}

type InventoryAdjustRequest struct {
	ProductID string  `json:"product_id" validate:"required"`
	Delta     float64 `json:"delta" validate:"required,ne=0"`
	Reason    string  `json:"reason" validate:"required,min=3,max=250"`
	Type      string  `json:"type" validate:"required,oneof=purchase adjustment loss damage return"`
}

func NewInventoryService(cfg config.Config, inv *repo.InventoryRepo, products *repo.ProductsRepo, v *validator.Validate, logger *slog.Logger) *InventoryService {
	return &InventoryService{cfg: cfg, inv: inv, products: products, validate: v, logger: logger}
}

func (s *InventoryService) LowStock(ctx context.Context, limit int) ([]repo.Product, error) {
	return s.inv.LowStock(ctx, limit)
}

func (s *InventoryService) ListMovements(ctx context.Context, productID string, limit, offset int) ([]repo.InventoryMovement, int, error) {
	return s.inv.ListMovements(ctx, productID, limit, offset)
}

func (s *InventoryService) Adjust(ctx context.Context, pool PgxBeginner, actorUserID string, req InventoryAdjustRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return ErrValidation
	}
	if req.Delta == 0 {
		return ErrValidation
	}

	// enforce sign conventions for some types (optional)
	if req.Type == "loss" || req.Type == "damage" {
		if req.Delta > 0 {
			req.Delta = -req.Delta
		}
	}
	if req.Type == "purchase" || req.Type == "return" {
		if req.Delta < 0 {
			req.Delta = -req.Delta
		}
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.inv.EnsureBalanceRow(ctx, tx, req.ProductID); err != nil {
		return err
	}
	bal, err := s.inv.GetBalanceForUpdate(ctx, tx, req.ProductID)
	if err != nil {
		return err
	}
	newQty := bal.QtyOnHand + req.Delta
	if !s.cfg.AllowNegativeStock && newQty < 0 {
		return ErrInsufficientStock
	}
	if err := s.inv.UpdateBalance(ctx, tx, req.ProductID, newQty); err != nil {
		return err
	}

	reason := req.Reason
	refType := "manual_adjust"
	actor := actorUserID
	m := repo.InventoryMovement{
		ProductID:     req.ProductID,
		MovementType:  req.Type,
		Delta:         req.Delta,
		QtyBefore:     bal.QtyOnHand,
		QtyAfter:      newQty,
		Reason:        &reason,
		ReferenceType: &refType,
		ReferenceID:   nil,
		ActorUserID:   &actor,
		CreatedAt:     time.Now().Format(time.RFC3339),
	}
	if err := s.inv.InsertMovement(ctx, tx, m); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

// PgxBeginner is satisfied by *pgxpool.Pool.
// It is defined here to ease testing and to avoid importing pgxpool everywhere.
//
//go:generate false

type PgxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

var _ = errors.New
