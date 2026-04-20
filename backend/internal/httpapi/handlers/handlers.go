package handlers

import (
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/repo"
	"github.com/example/sistemaemgo/internal/service"
)

type Handlers struct {
	Auth      *AuthHandler
	Products  *ProductsHandler
	Inventory *InventoryHandler
	Cash      *CashHandler
	Sales     *SalesHandler
	Finance   *FinanceHandler
	Fiscal    *FiscalHandler
}

func New(cfg config.Config, svcs *service.Services, repos *repo.Repositories, logger *slog.Logger) *Handlers {
	return &Handlers{
		Auth:      NewAuthHandler(cfg, svcs.Auth, logger),
		Products:  NewProductsHandler(svcs.Products, logger),
		Inventory: NewInventoryHandler(svcs.Inventory, logger),
		Cash:      NewCashHandler(svcs.Cash, logger),
		Sales:     NewSalesHandler(svcs.Sales, logger),
		Finance:   NewFinanceHandler(svcs.Finance, logger),
		Fiscal:    NewFiscalHandler(svcs.Fiscal, logger),
	}
}
