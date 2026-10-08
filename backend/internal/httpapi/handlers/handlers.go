package handlers

import (
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules"
)

type Handlers struct {
	Auth        *AuthHandler
	Products    *ProductsHandler
	Inventory   *InventoryHandler
	Cash        *CashHandler
	Sales       *SalesHandler
	Finance     *FinanceHandler
	Fiscal      *FiscalHandler
	Privacy     *PrivacyHandler
	Procurement *ProcurementHandler
	Returns     *ReturnsHandler
	Setup       *SetupHandler
	Audit       *AuditHandler
}

func New(cfg config.Config, mods *modules.Modules, logger *slog.Logger) *Handlers {
	return &Handlers{
		Auth:        NewAuthHandler(cfg, mods.Auth, mods.Audit, mods.Redis, logger),
		Products:    NewProductsHandler(mods.Products, logger),
		Inventory:   NewInventoryHandler(mods.Inventory, logger),
		Cash:        NewCashHandler(mods.Cash, logger),
		Sales:       NewSalesHandler(mods.Sales, logger),
		Finance:     NewFinanceHandler(mods.Finance, logger),
		Fiscal:      NewFiscalHandler(mods.Fiscal, mods.Audit, logger),
		Privacy:     NewPrivacyHandler(mods.Privacy, mods.Audit, logger),
		Procurement: NewProcurementHandler(mods.Procurement, logger),
		Returns:     NewReturnsHandler(mods.Returns, logger),
		Setup:       NewSetupHandler(mods.Setup),
		Audit:       NewAuditHandler(mods.Audit, logger),
	}
}
