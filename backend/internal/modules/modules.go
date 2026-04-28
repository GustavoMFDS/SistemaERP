package modules

import (
	"context"
	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	finapp "github.com/example/sistemaemgo/internal/modules/finance/application"
	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	fiscmvp "github.com/example/sistemaemgo/internal/modules/fiscal/providers/mvp"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/example/sistemaemgo/internal/platform/events"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Modules struct {
	Auth      *authapp.AuthService
	Products  *invapp.ProductsService
	Inventory *invapp.InventoryService
	Cash      *salesapp.CashService
	Sales     *salesapp.SalesService
	Finance   *finapp.FinanceService
	Fiscal    *fiscapp.FiscalService
	Events    *events.Bus
}

func New(cfg config.Config, pool *pgxpool.Pool, rdb *redis.Client, logger *slog.Logger) *Modules {
	v := validator.New()
	uow := db.NewPgxUnitOfWork(pool)

	bus := events.NewBus(logger)
	bus.Subscribe("sale.created", func(ctx context.Context, ev events.DomainEvent) error {
		logger.Info("event_sale_created", slog.Any("event", ev))
		return nil
	})
	bus.Subscribe("inventory.debited", func(ctx context.Context, ev events.DomainEvent) error {
		logger.Info("event_inventory_debited", slog.Any("event", ev))
		return nil
	})
	bus.Subscribe("inventory.low_stock", func(ctx context.Context, ev events.DomainEvent) error {
		logger.Warn("event_inventory_low_stock", slog.Any("event", ev))
		return nil
	})

	// infrastructure
	usersRepo := authinfra.NewUsersRepo(pool)

	baseProductsRepo := invinfra.NewProductsRepo(pool)
	var productsRepo invapp.ProductsRepository = baseProductsRepo
	if rdb != nil {
		productsRepo = invinfra.NewCachedProductsRepo(baseProductsRepo, rdb)
	}
	inventoryRepo := invinfra.NewInventoryRepo(pool)

	salesRepo := salesinfra.NewSalesRepo(pool)
	cashRepo := salesinfra.NewCashRepo(pool)

	financeRepo := fininfra.NewFinanceRepo(pool)
	fiscalRepo := fiscinfra.NewFiscalRepo(pool)

	// application services
	var refreshStore authapp.RefreshTokenStore
	if rdb != nil {
		refreshStore = authinfra.NewRefreshTokenStore(rdb)
	}
	authSvc := authapp.NewAuthService(cfg, usersRepo, refreshStore, logger)
	productsSvc := invapp.NewProductsService(uow, productsRepo, v, logger)
	inventorySvc := invapp.NewInventoryService(cfg, uow, inventoryRepo, productsRepo, v, logger)
	cashSvc := salesapp.NewCashService(uow, cashRepo, v, logger)
	salesSvc := salesapp.NewSalesService(cfg, uow, salesRepo, inventoryRepo, financeRepo, cashRepo, productsRepo, bus, v, logger)
	financeSvc := finapp.NewFinanceService(financeRepo, v, logger)
	nfeProvider := fiscmvp.New()
	fiscalSvc := fiscapp.NewFiscalServiceWithProvider(uow, fiscalRepo, salesRepo, productsRepo, nfeProvider, v, logger)

	return &Modules{
		Auth:      authSvc,
		Products:  productsSvc,
		Inventory: inventorySvc,
		Cash:      cashSvc,
		Sales:     salesSvc,
		Finance:   financeSvc,
		Fiscal:    fiscalSvc,
		Events:    bus,
	}
}
