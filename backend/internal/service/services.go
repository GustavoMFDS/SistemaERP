package service

import (
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/repo"
	"github.com/go-playground/validator/v10"
)

type Services struct {
	Auth      *AuthService
	Products  *ProductsService
	Inventory *InventoryService
	Sales     *SalesService
	Finance   *FinanceService
	Fiscal    *FiscalService
	Cash      *CashService

	validate *validator.Validate
}

func New(cfg config.Config, repos *repo.Repositories, logger *slog.Logger) *Services {
	v := validator.New(validator.WithRequiredStructEnabled())
	services := &Services{validate: v}

	services.Auth = NewAuthService(cfg, repos.Users, logger)
	services.Products = NewProductsService(repos.Products, v, logger)
	services.Inventory = NewInventoryService(cfg, repos.Inventory, repos.Products, v, logger)
	services.Cash = NewCashService(repos.Cash, v, logger)
	services.Finance = NewFinanceService(repos.Finance, v, logger)
	services.Sales = NewSalesService(cfg, repos.Sales, repos.Inventory, repos.Finance, repos.Cash, repos.Products, v, logger)
	services.Fiscal = NewFiscalService(repos.Fiscal, repos.Sales, repos.Products, repos.Users, v, logger)

	return services
}
