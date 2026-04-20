package handlers

import (
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules"
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

func New(cfg config.Config, mods *modules.Modules, logger *slog.Logger) *Handlers {
	return &Handlers{
		Auth:      NewAuthHandler(cfg, mods.Auth, logger),
		Products:  NewProductsHandler(mods.Products, logger),
		Inventory: NewInventoryHandler(mods.Inventory, logger),
		Cash:      NewCashHandler(mods.Cash, logger),
		Sales:     NewSalesHandler(mods.Sales, logger),
		Finance:   NewFinanceHandler(mods.Finance, logger),
		Fiscal:    NewFiscalHandler(mods.Fiscal, logger),
	}
}
