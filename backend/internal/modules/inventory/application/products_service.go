package application

import (
	"context"
	"log/slog"

	"github.com/example/sistemaemgo/internal/modules/common"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type ProductsService struct {
	uow      db.UnitOfWork
	repo     ProductsRepository
	validate *validator.Validate
	logger   *slog.Logger
}

type ProductCreateRequest struct {
	CategoryID  *string           `json:"category_id"`
	SKU         string            `json:"sku" validate:"required,min=1,max=64"`
	Barcode     *string           `json:"barcode" validate:"omitempty,min=8,max=32"`
	Name        string            `json:"name" validate:"required,min=2,max=200"`
	Description *string           `json:"description"`
	Unit        string            `json:"unit" validate:"required,min=1,max=8"`
	CostPrice   platform.Money    `json:"cost_price" validate:"min=0"`
	PriceCash   platform.Money    `json:"price_cash" validate:"required,gt=0"`
	PromoPrice  *platform.Money   `json:"promo_price" validate:"omitempty,gt=0"`
	MinStock    platform.Quantity `json:"min_stock" validate:"min=0"`
	Active      bool              `json:"active"`
}

type ProductUpdateRequest = ProductCreateRequest

func NewProductsService(uow db.UnitOfWork, r ProductsRepository, v *validator.Validate, logger *slog.Logger) *ProductsService {
	return &ProductsService{uow: uow, repo: r, validate: v, logger: logger}
}

func (s *ProductsService) List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error) {
	return s.repo.List(ctx, tenantID, query, limit, offset)
}

func (s *ProductsService) Get(ctx context.Context, tenantID string, id string) (inv.Product, error) {
	return s.repo.Get(ctx, tenantID, id)
}

func (s *ProductsService) Create(ctx context.Context, tenantID string, req ProductCreateRequest) (string, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	p := inv.Product{
		CategoryID:  req.CategoryID,
		SKU:         req.SKU,
		Barcode:     req.Barcode,
		Name:        req.Name,
		Description: req.Description,
		Unit:        req.Unit,
		CostPrice:   req.CostPrice,
		PriceCash:   req.PriceCash,
		PromoPrice:  req.PromoPrice,
		MinStock:    req.MinStock,
		Active:      req.Active,
	}
	id, err := s.repo.Create(ctx, tx, tenantID, p)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	// Best-effort cache invalidation (after commit).
	if inv, ok := s.repo.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	}); ok {
		_ = inv.InvalidateProduct(ctx, tenantID, id)
		_ = inv.BumpProductsListVersion(ctx, tenantID)
	}
	return id, nil
}

func (s *ProductsService) Update(ctx context.Context, tenantID string, id string, req ProductUpdateRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	p := inv.Product{
		CategoryID:  req.CategoryID,
		SKU:         req.SKU,
		Barcode:     req.Barcode,
		Name:        req.Name,
		Description: req.Description,
		Unit:        req.Unit,
		CostPrice:   req.CostPrice,
		PriceCash:   req.PriceCash,
		PromoPrice:  req.PromoPrice,
		MinStock:    req.MinStock,
		Active:      req.Active,
	}
	if err := s.repo.Update(ctx, tx, tenantID, id, p); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// Best-effort cache invalidation (after commit).
	if inv, ok := s.repo.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	}); ok {
		_ = inv.InvalidateProduct(ctx, tenantID, id)
		_ = inv.BumpProductsListVersion(ctx, tenantID)
	}
	return nil
}
