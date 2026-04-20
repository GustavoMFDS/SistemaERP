package modules

import (
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	finapp "github.com/example/sistemaemgo/internal/modules/finance/application"
	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Modules struct {
	Auth      *authapp.AuthService
	Products  *invapp.ProductsService
	Inventory *invapp.InventoryService
	Cash      *salesapp.CashService
	Sales     *salesapp.SalesService
	Finance   *finapp.FinanceService
	Fiscal    *fiscapp.FiscalService
}

func New(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) *Modules {
	v := validator.New()
	uow := db.NewPgxUnitOfWork(pool)

	// infrastructure
	usersRepo := authinfra.NewUsersRepo(pool)

	productsRepo := invinfra.NewProductsRepo(pool)
	inventoryRepo := invinfra.NewInventoryRepo(pool)

	salesRepo := salesinfra.NewSalesRepo(pool)
	cashRepo := salesinfra.NewCashRepo(pool)

	financeRepo := fininfra.NewFinanceRepo(pool)
	fiscalRepo := fiscinfra.NewFiscalRepo(pool)

	// application services
	authSvc := authapp.NewAuthService(cfg, usersRepo, logger)
	productsSvc := invapp.NewProductsService(uow, productsRepo, v, logger)
	inventorySvc := invapp.NewInventoryService(cfg, uow, inventoryRepo, productsRepo, v, logger)
	cashSvc := salesapp.NewCashService(uow, cashRepo, v, logger)
	salesSvc := salesapp.NewSalesService(cfg, uow, salesRepo, inventoryRepo, financeRepo, cashRepo, productsRepo, v, logger)
	financeSvc := finapp.NewFinanceService(financeRepo, v, logger)
	fiscalSvc := fiscapp.NewFiscalService(uow, fiscalRepo, salesRepo, productsRepo, v, logger)

	return &Modules{
		Auth:      authSvc,
		Products:  productsSvc,
		Inventory: inventorySvc,
		Cash:      cashSvc,
		Sales:     salesSvc,
		Finance:   financeSvc,
		Fiscal:    fiscalSvc,
	}
}
