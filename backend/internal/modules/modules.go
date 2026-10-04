package modules

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/audit"
	authapp "github.com/example/sistemaemgo/internal/modules/auth/application"
	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	finapp "github.com/example/sistemaemgo/internal/modules/finance/application"
	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	fiscmvp "github.com/example/sistemaemgo/internal/modules/fiscal/providers/mvp"
	fiscsefaz "github.com/example/sistemaemgo/internal/modules/fiscal/providers/sefaz"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	privacyapp "github.com/example/sistemaemgo/internal/modules/privacy/application"
	privacyinfra "github.com/example/sistemaemgo/internal/modules/privacy/infrastructure"
	procapp "github.com/example/sistemaemgo/internal/modules/procurement/application"
	procinfra "github.com/example/sistemaemgo/internal/modules/procurement/infrastructure"
	retapp "github.com/example/sistemaemgo/internal/modules/returns/application"
	retinfra "github.com/example/sistemaemgo/internal/modules/returns/infrastructure"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/example/sistemaemgo/internal/platform/events"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Modules struct {
	Auth        *authapp.AuthService
	Products    *invapp.ProductsService
	Inventory   *invapp.InventoryService
	Cash        *salesapp.CashService
	Sales       *salesapp.SalesService
	Finance     *finapp.FinanceService
	Fiscal      *fiscapp.FiscalService
	Privacy     *privacyapp.Service
	Procurement *procapp.Service
	Returns     *retapp.Service
	Events      *events.Bus
	DB          *pgxpool.Pool
	Redis       *redis.Client
	Audit       *audit.Service
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
	usersRepo := authinfra.NewUsersRepo(pool, !cfg.IsProdLike())

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
	privacyRepo := privacyinfra.NewRepo(pool)
	procurementRepo := procinfra.NewRepo(pool)
	returnsRepo := retinfra.NewRepo(pool)
	auditSvc := audit.New(pool, logger)

	// application services
	var refreshStore authapp.RefreshTokenStore
	if rdb != nil {
		refreshStore = authinfra.NewRefreshTokenStore(rdb)
	}
	authSvc := authapp.NewAuthService(cfg, usersRepo, refreshStore, logger)
	productsSvc := invapp.NewProductsService(uow, productsRepo, auditSvc, v, logger)
	inventorySvc := invapp.NewInventoryService(cfg, uow, inventoryRepo, productsRepo, auditSvc, v, logger)
	cashSvc := salesapp.NewCashService(uow, cashRepo, financeRepo, auditSvc, v, logger)
	salesSvc := salesapp.NewSalesService(cfg, uow, salesRepo, inventoryRepo, financeRepo, cashRepo, productsRepo, auditSvc, bus, v, logger)
	financeSvc := finapp.NewFinanceService(uow, financeRepo, auditSvc, v, logger)
	var nfeProvider fiscapp.NFeProvider
	if cfg.FiscalProvider == "" || cfg.FiscalProvider == "mvp" {
		nfeProvider = fiscmvp.New()
	}
	fiscalSvc := fiscapp.NewFiscalServiceWithProvider(uow, fiscalRepo, salesRepo, productsRepo, nfeProvider, auditSvc, v, logger)
	documentBuilder := fiscsefaz.NewDocumentBuilder(cfg.AppVersion)
	fiscalSvc.SetNFCeDocumentBuilder(documentBuilder)
	fiscalSvc.SetNFCeCancellationBuilder(documentBuilder)

	var certificateResolver fiscsefaz.CertificateResolver
	if cfg.NFCeCertificateSecretDir != "" {
		resolver, err := fiscsefaz.NewPEMDirectoryCertificateResolver(cfg.NFCeCertificateSecretDir)
		if err != nil {
			logger.Error("nfce_certificate_secret_resolver_disabled", slog.Any("error", err))
		} else {
			certificateResolver = resolver
			signingService := fiscsefaz.NewXMLSigningService(resolver)
			fiscalSvc.SetNFCeXMLSigner(signingService)
			fiscalSvc.SetNFCeCancellationSigner(signingService)
		}
	}
	if cfg.NFCeSchemaDir != "" && cfg.NFCeSchemaEntrypoint != "" {
		schemaValidator, err := fiscsefaz.NewXMLLintSchemaValidator(
			cfg.NFCeSchemaDir,
			cfg.NFCeSchemaEntrypoint,
		)
		if err != nil {
			logger.Error("nfce_schema_validator_disabled", slog.Any("error", err))
		} else {
			fiscalSvc.SetNFCeSchemaValidator(schemaValidator)
		}
	}
	if cfg.NFCeSchemaDir != "" && cfg.NFCeEventSchemaEntrypoint != "" {
		eventSchemaValidator, err := fiscsefaz.NewXMLLintSchemaValidator(
			cfg.NFCeSchemaDir,
			cfg.NFCeEventSchemaEntrypoint,
		)
		if err != nil {
			logger.Error("nfce_event_schema_validator_disabled", slog.Any("error", err))
		} else {
			fiscalSvc.SetNFCeEventSchemaValidator(eventSchemaValidator)
		}
	}
	if certificateResolver != nil {
		var authorizer *fiscsefaz.SEFAZAuthorizer
		switch {
		case cfg.NFCeSEFAZHomologationEnabled:
			authorizer = fiscsefaz.NewHomologationAuthorizer(certificateResolver, 30*time.Second)
		case cfg.NFCeSEFAZProductionEnabled:
			authorizer = fiscsefaz.NewProductionAuthorizer(certificateResolver, 30*time.Second)
		}
		if authorizer != nil {
			fiscalSvc.SetNFCeRemoteAuthorizer(authorizer)
			fiscalSvc.SetNFCeRemoteCancellationClient(authorizer)
		}
	}
	privacySvc := privacyapp.NewService(privacyRepo)
	procurementSvc := procapp.NewService(uow, procurementRepo, productsRepo, inventoryRepo, auditSvc, v, logger)
	returnsSvc := retapp.NewService(uow, returnsRepo, inventoryRepo, productsRepo, auditSvc, v, logger)

	return &Modules{
		Auth:        authSvc,
		Products:    productsSvc,
		Inventory:   inventorySvc,
		Cash:        cashSvc,
		Sales:       salesSvc,
		Finance:     financeSvc,
		Fiscal:      fiscalSvc,
		Privacy:     privacySvc,
		Procurement: procurementSvc,
		Returns:     returnsSvc,
		Events:      bus,
		DB:          pool,
		Redis:       rdb,
		Audit:       auditSvc,
	}
}
