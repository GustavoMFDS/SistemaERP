package handlers

import (
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules"
)

type Handlers struct {
	Auth            *AuthHandler
	Products        *ProductsHandler
	ProductImages   *ProductImagesHandler
	ProductVariations *ProductVariationsHandler
	Inventory       *InventoryHandler
	Cash            *CashHandler
	Sales           *SalesHandler
	SaleFiscal      *SaleFiscalHandler
	Finance         *FinanceHandler
	FinanceAccounts *FinanceAccountsHandler
	Fiscal          *FiscalHandler
	Privacy         *PrivacyHandler
	Procurement     *ProcurementHandler
	Returns         *ReturnsHandler
	Setup           *SetupHandler
	Team            *TeamHandler
	Customers       *CustomersHandler
	Audit           *AuditHandler
}

func New(cfg config.Config, mods *modules.Modules, logger *slog.Logger) *Handlers {
	return &Handlers{
		Auth:            NewAuthHandler(cfg, mods.Auth, mods.Audit, mods.Redis, logger),
		Products:        NewProductsHandler(mods.Products, logger),
		ProductImages:   NewProductImagesHandler(mods.DB, mods.Audit),
		ProductVariations: NewProductVariationsHandler(mods.DB, mods.Products),
		Inventory:       NewInventoryHandler(mods.Inventory, logger),
		Cash:            NewCashHandler(mods.Cash, logger),
		Sales:           NewSalesHandler(mods.Sales, logger),
		SaleFiscal:      NewSaleFiscalHandler(mods.DB, mods.Fiscal, mods.Audit),
		Finance:         NewFinanceHandler(mods.Finance, logger),
		FinanceAccounts: NewFinanceAccountsHandler(mods.DB, mods.Audit),
		Fiscal:          NewFiscalHandler(mods.Fiscal, mods.Audit, logger),
		Privacy:         NewPrivacyHandler(mods.Privacy, mods.Audit, logger),
		Procurement:     NewProcurementHandler(mods.Procurement, logger),
		Returns:         NewReturnsHandler(mods.Returns, logger),
		Setup:           NewSetupHandler(mods.Setup),
		Team:            NewTeamHandler(mods.Team),
		Customers:       NewCustomersHandler(mods.Customers),
		Audit:           NewAuditHandler(mods.Audit, logger),
	}
}
