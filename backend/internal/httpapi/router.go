package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi/handlers"
	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(cfg config.Config, mods *modules.Modules, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.TrustedRealIP(cfg))
	r.Use(middleware.RequestID())
	r.Use(middleware.SecurityHeaders(cfg))
	r.Use(middleware.CORS(cfg))
	r.Use(middleware.AccessLog(logger))
	r.Use(middleware.Metrics())
	r.Use(middleware.Recover(cfg, logger))
	r.Use(chimw.Timeout(60 * time.Second))

	r.With(middleware.ProtectMetrics(cfg)).Handle("/metrics", promhttp.Handler())

	r.Get("/health", liveHealth)
	r.Get("/health/live", liveHealth)
	r.Get("/health/ready", readinessHealth(mods))

	h := handlers.New(cfg, mods, logger)
	failClosedRateLimit := cfg.IsProdLike()
	authLoginLimit := middleware.RateLimit(mods.Redis, "auth_login", cfg.RateLimitLogin, time.Minute, failClosedRateLimit, middleware.RateLimitByIP)
	authRefreshLimit := middleware.RateLimit(mods.Redis, "auth_refresh", cfg.RateLimitRefresh, time.Minute, failClosedRateLimit, middleware.RateLimitByIP)
	authLogoutLimit := middleware.RateLimit(mods.Redis, "auth_logout", cfg.RateLimitLogout, time.Minute, failClosedRateLimit, middleware.RateLimitByIP)
	salesLimit := middleware.RateLimit(mods.Redis, "sales_create", cfg.RateLimitSales, time.Minute, failClosedRateLimit, middleware.RateLimitByTenantUserOrIP)
	fiscalLimit := middleware.RateLimit(mods.Redis, "fiscal", cfg.RateLimitFiscal, time.Minute, failClosedRateLimit, middleware.RateLimitByTenantUserOrIP)
	trustedOrigin := middleware.RequireTrustedOrigin(cfg)

	r.Route("/api/v1", func(api chi.Router) {
		api.With(authLoginLimit).Post("/auth/login", h.Auth.Login)
		api.With(authRefreshLimit, trustedOrigin).Post("/auth/refresh", h.Auth.Refresh)
		api.With(authLogoutLimit, trustedOrigin).Post("/auth/logout", h.Auth.Logout)
		api.With(middleware.AuthJWT(cfg, mods.Auth, logger)).Get("/auth/me", h.Auth.Me)

		api.Group(func(pr chi.Router) {
			pr.Use(middleware.AuthJWT(cfg, mods.Auth, logger))
			pr.Use(middleware.LoadPermissions(mods.Auth, logger))

			pr.Route("/products", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("product:read")).Get("/", h.Products.List)
				rr.With(middleware.RequirePermission("product:read")).Get("/barcode/{barcode}", h.Products.GetByBarcode)
				rr.With(middleware.RequirePermission("product:read")).Get("/{id}", h.Products.Get)
				rr.With(middleware.RequirePermission("product:write")).Post("/", h.Products.Create)
				rr.With(middleware.RequirePermission("product:write")).Put("/{id}", h.Products.Update)
			})

			pr.Route("/inventory", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("inventory:read")).Get("/low-stock", h.Inventory.LowStock)
				rr.With(middleware.RequirePermission("inventory:read")).Get("/movements", h.Inventory.ListMovements)
				rr.With(middleware.RequirePermission("inventory:adjust")).Post("/adjust", h.Inventory.Adjust)
			})

			pr.Route("/suppliers", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("procurement:read")).Get("/", h.Procurement.ListSuppliers)
				rr.With(middleware.RequirePermission("procurement:write")).Post("/", h.Procurement.CreateSupplier)
				rr.With(middleware.RequirePermission("procurement:write")).Put("/{id}", h.Procurement.UpdateSupplier)
			})

			pr.Route("/purchases", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("procurement:read")).Get("/", h.Procurement.ListPurchases)
				rr.With(middleware.RequirePermission("procurement:read")).Get("/{id}", h.Procurement.GetPurchase)
				rr.With(middleware.RequirePermission("procurement:write")).Post("/", h.Procurement.CreatePurchase)
				rr.With(middleware.RequirePermission("procurement:receive")).Post("/{id}/receive", h.Procurement.ReceivePurchase)
				rr.With(middleware.RequirePermission("procurement:write")).Post("/{id}/cancel", h.Procurement.CancelPurchase)
			})

			pr.Route("/cash", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("cash:open")).Get("/sessions/current", h.Cash.CurrentSession)
				rr.With(middleware.RequirePermission("cash:open")).Post("/sessions/open", h.Cash.OpenSession)
				rr.With(middleware.RequirePermission("cash:move")).Post("/sessions/{id}/movements", h.Cash.RecordMovement)
				rr.With(middleware.RequirePermission("cash:close")).Post("/sessions/{id}/close", h.Cash.CloseSession)
			})

			pr.Route("/sales", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("sale:read")).Get("/", h.Sales.List)
				rr.With(middleware.RequirePermission("sale:read")).Get("/{id}", h.Sales.Get)
				rr.With(middleware.RequirePermission("sale:write"), salesLimit).Post("/", h.Sales.CreateAndFinalize)
				rr.With(middleware.RequirePermission("sale:return")).Post("/{id}/returns", h.Returns.CreateForSale)
				rr.With(middleware.RequirePermission("sale:cancel")).Post("/{id}/cancel", h.Sales.Cancel)
			})

			pr.Route("/returns", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("sale:return")).Get("/", h.Returns.List)
				rr.With(middleware.RequirePermission("sale:return")).Get("/{id}", h.Returns.Get)
			})

			pr.Route("/finance", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("finance:read")).Get("/dashboard", h.Finance.Dashboard)
				rr.With(middleware.RequirePermission("finance:read")).Get("/ledger", h.Finance.ListLedger)
				rr.With(middleware.RequirePermission("finance:read")).Get("/payments", h.Finance.ListPayments)
				rr.With(middleware.RequirePermission("finance:reconcile")).Post("/payments/{id}/reconcile", h.Finance.ReconcilePayment)
				rr.With(middleware.RequirePermission("finance:read")).Get("/payments/{id}/reconciliation-history", h.Finance.GetPaymentReconciliationHistory)
				rr.With(middleware.RequirePermission("finance:reconcile")).Post("/payments/{id}/reconciliation-adjustments", h.Finance.AdjustPaymentReconciliation)
				rr.With(middleware.RequirePermission("finance:read")).Get("/refunds", h.Finance.ListRefunds)
				rr.With(middleware.RequirePermission("finance:reconcile")).Post("/returns/{id}/refunds", h.Finance.SettleRefund)
			})

			if mods.Fiscal != nil {
				pr.Route("/fiscal", func(rr chi.Router) {
					rr.With(middleware.RequirePermission("invoice:read")).Get("/nfce/readiness", h.Fiscal.NFCeReadiness)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Post("/nfce/reservations", h.Fiscal.ReserveNFCeDraft)
					rr.With(middleware.RequirePermission("invoice:read")).Get("/nfce/invoices/{invoiceID}/tax-calculations", h.Fiscal.ListInvoiceTaxCalculations)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Get("/nfce/invoices/{invoiceID}/xml-candidate", h.Fiscal.PreviewNFCeXMLCandidate)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Post("/nfce/invoices/{invoiceID}/items/{saleItemID}/tax/legacy", h.Fiscal.PrepareLegacyOnlyTaxCalculation)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Post("/nfce/invoices/{invoiceID}/items/{saleItemID}/tax/ibs-cbs", h.Fiscal.PrepareRegularIBSCBSCalculation)
					rr.With(middleware.RequirePermission("invoice:read")).Get("/nfce/products/{productID}/profile", h.Fiscal.GetProductFiscalProfile)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Put("/nfce/products/{productID}/profile", h.Fiscal.PrepareProductFiscalProfile)
					rr.With(middleware.RequirePermission("invoice:read")).Get("/nfce/issuer", h.Fiscal.GetNFCeIssuerProfile)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Put("/nfce/issuer", h.Fiscal.PrepareNFCeIssuerProfile)
					rr.With(middleware.RequirePermission("invoice:read")).Get("/nfce/config", h.Fiscal.GetNFCeConfig)
					rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Put("/nfce/config", h.Fiscal.PrepareNFCeConfig)
					rr.With(middleware.RequirePermission("invoice:read")).Get("/nfe/xml", h.Fiscal.ListXML)
					rr.With(middleware.RequirePermission("invoice:read"), fiscalLimit).Get("/nfe/xml/{id}/download", h.Fiscal.DownloadXML)
					if cfg.FiscalProvider == "" || cfg.FiscalProvider == "mvp" {
						rr.With(middleware.RequirePermission("invoice:generate"), fiscalLimit).Post("/nfe/xml", h.Fiscal.GenerateNFeXML)
					}
				})
			}

			pr.Route("/privacy", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("privacy:write")).Post("/requests", h.Privacy.CreateRequest)
				rr.With(middleware.RequirePermission("privacy:read")).Get("/requests", h.Privacy.ListRequests)
				rr.With(middleware.RequirePermission("privacy:read")).Get("/requests/{id}", h.Privacy.GetRequest)
				rr.With(middleware.RequirePermission("privacy:write")).Put("/requests/{id}/status", h.Privacy.UpdateRequestStatus)
				rr.With(middleware.RequirePermission("privacy:read")).Post("/requests/{id}/export", h.Privacy.ExportSubject)
				rr.With(middleware.RequirePermission("privacy:write")).Post("/requests/{id}/anonymize", h.Privacy.AnonymizeSubject)
				rr.With(middleware.RequirePermission("privacy:write")).Post("/requests/{id}/block", h.Privacy.BlockSubject)
				rr.With(middleware.RequirePermission("privacy:write")).Post("/consents", h.Privacy.RecordConsent)
				rr.With(middleware.RequirePermission("privacy:read")).Get("/consents", h.Privacy.ListConsents)
				rr.With(middleware.RequirePermission("privacy:write")).Post("/consents/{id}/revoke", h.Privacy.RevokeConsent)
			})

			pr.Route("/audit", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("audit:read")).Get("/logs", h.Audit.List)
			})
		})
	})

	return r
}

func liveHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readinessHealth(mods *modules.Modules) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2 * time.Second)
		defer cancel()

		if mods.DB == nil || mods.DB.Ping(ctx) != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unready","dependency":"postgres"}`))
			return
		}
		if mods.Redis != nil {
			if err := mods.Redis.Ping(ctx).Err(); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"unready","dependency":"redis"}`))
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}
