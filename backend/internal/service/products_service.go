package service

import (
	"context"
	"log/slog"

	"github.com/example/sistemaemgo/internal/repo"
	"github.com/go-playground/validator/v10"
)

type ProductsService struct {
	repo     *repo.ProductsRepo
	validate *validator.Validate
	logger   *slog.Logger
}

type ProductCreateRequest struct {
	CategoryID  *string  `json:"category_id"`
	SKU         string   `json:"sku" validate:"required,min=1,max=64"`
	Barcode     *string  `json:"barcode" validate:"omitempty,min=8,max=32"`
	Name        string   `json:"name" validate:"required,min=2,max=200"`
	Description *string  `json:"description"`
	Unit        string   `json:"unit" validate:"required,min=1,max=8"`
	CostPrice   float64  `json:"cost_price" validate:"min=0"`
	PriceCash   float64  `json:"price_cash" validate:"required,gt=0"`
	PromoPrice  *float64 `json:"promo_price" validate:"omitempty,gt=0"`
	MinStock    float64  `json:"min_stock" validate:"min=0"`
	Active      bool     `json:"active"`
}

type ProductUpdateRequest = ProductCreateRequest

func NewProductsService(r *repo.ProductsRepo, v *validator.Validate, logger *slog.Logger) *ProductsService {
	return &ProductsService{repo: r, validate: v, logger: logger}
}

func (s *ProductsService) List(ctx context.Context, query string, limit, offset int) ([]repo.Product, int, error) {
	return s.repo.List(ctx, query, limit, offset)
}

func (s *ProductsService) Get(ctx context.Context, id string) (repo.Product, error) {
	return s.repo.Get(ctx, id)
}

func (s *ProductsService) Create(ctx context.Context, tx repo.DBTX, req ProductCreateRequest) (string, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", ErrValidation
	}
	p := repo.Product{
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
	return s.repo.Create(ctx, tx, p)
}

func (s *ProductsService) Update(ctx context.Context, tx repo.DBTX, id string, req ProductUpdateRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return ErrValidation
	}
	p := repo.Product{
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
	return s.repo.Update(ctx, tx, id, p)
}
