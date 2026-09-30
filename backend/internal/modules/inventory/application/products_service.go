package application

import (
	"context"
	"log/slog"
	"strings"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type ProductsService struct {
	uow      db.UnitOfWork
	repo     ProductsRepository
	audit    *audit.Service
	validate *validator.Validate
	logger   *slog.Logger
}

type ProductCreateRequest struct {
	CategoryID  *string           `json:"category_id"`
	SKU         string            `json:"sku" validate:"required,min=1,max=64"`
	Barcode     *string           `json:"barcode" validate:"omitempty,min=8,max=32"`
	NCM         *string           `json:"ncm" validate:"omitempty,numeric,len=8"`
	CEST        *string           `json:"cest" validate:"omitempty,numeric,len=7"`
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

func NewProductsService(uow db.UnitOfWork, r ProductsRepository, auditSvc *audit.Service, v *validator.Validate, logger *slog.Logger) *ProductsService {
	return &ProductsService{uow: uow, repo: r, audit: auditSvc, validate: v, logger: logger}
}

func (s *ProductsService) List(ctx context.Context, tenantID string, query string, limit, offset int) ([]inv.Product, int, error) {
	return s.repo.List(ctx, tenantID, query, limit, offset)
}

func (s *ProductsService) Get(ctx context.Context, tenantID string, id string) (inv.Product, error) {
	return s.repo.Get(ctx, tenantID, id)
}

func (s *ProductsService) GetByBarcode(ctx context.Context, tenantID string, barcode string) (inv.Product, error) {
	barcode = strings.TrimSpace(barcode)
	if err := s.validate.Var(barcode, "required,min=8,max=32"); err != nil {
		return inv.Product{}, common.ErrValidation
	}
	return s.repo.GetByBarcode(ctx, tenantID, barcode)
}

func normalizeOptionalProductString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func normalizeBarcode(value *string) *string {
	return normalizeOptionalProductString(value)
}

func normalizeProductRequest(req ProductCreateRequest) ProductCreateRequest {
	req.SKU = strings.TrimSpace(req.SKU)
	req.Name = strings.TrimSpace(req.Name)
	req.Unit = strings.TrimSpace(req.Unit)
	req.Barcode = normalizeBarcode(req.Barcode)
	req.NCM = normalizeOptionalProductString(req.NCM)
	req.CEST = normalizeOptionalProductString(req.CEST)
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		if description == "" {
			req.Description = nil
		} else {
			req.Description = &description
		}
	}
	return req
}

func validateProductPricing(req ProductCreateRequest) error {
	if req.PromoPrice != nil && *req.PromoPrice > req.PriceCash {
		return common.ErrValidation
	}
	return nil
}

func (s *ProductsService) Create(ctx context.Context, tenantID, actorUserID string, req ProductCreateRequest) (string, error) {
	req = normalizeProductRequest(req)
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	if err := validateProductPricing(req); err != nil {
		return "", err
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
		NCM:         req.NCM,
		CEST:        req.CEST,
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
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "product.create",
		ResourceType: "product", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{
			"sku":        p.SKU,
			"price_cash": p.PriceCash.String(),
		},
	}); err != nil {
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
		cacheCtx := context.WithoutCancel(ctx)
		_ = inv.InvalidateProduct(cacheCtx, tenantID, id)
		_ = inv.BumpProductsListVersion(cacheCtx, tenantID)
	}
	return id, nil
}

func (s *ProductsService) Update(ctx context.Context, tenantID, actorUserID, id string, preserveCost bool, req ProductUpdateRequest) error {
	req = normalizeProductRequest(req)
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	if err := validateProductPricing(req); err != nil {
		return err
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
		NCM:         req.NCM,
		CEST:        req.CEST,
		Name:        req.Name,
		Description: req.Description,
		Unit:        req.Unit,
		CostPrice:   req.CostPrice,
		PriceCash:   req.PriceCash,
		PromoPrice:  req.PromoPrice,
		MinStock:    req.MinStock,
		Active:      req.Active,
	}
	if err := s.repo.Update(ctx, tx, tenantID, id, p, preserveCost); err != nil {
		return err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "product.update",
		ResourceType: "product", ResourceID: id, Outcome: "success",
		Metadata: map[string]any{
			"sku":        p.SKU,
			"price_cash": p.PriceCash.String(),
		},
	}); err != nil {
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
		cacheCtx := context.WithoutCancel(ctx)
		_ = inv.InvalidateProduct(cacheCtx, tenantID, id)
		_ = inv.BumpProductsListVersion(cacheCtx, tenantID)
	}
	return nil
}
