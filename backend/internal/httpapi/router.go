package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi/handlers"
	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(cfg config.Config, mods *modules.Modules, pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RealIP)
	r.Use(middleware.RequestID())
	r.Use(middleware.AccessLog(logger))
	r.Use(middleware.Recover(logger))
	r.Use(chimw.Timeout(60 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	h := handlers.New(cfg, mods, logger)
	h.Products.BindDB(pool)
	h.Inventory.BindDB(pool)
	h.Cash.BindDB(pool)
	h.Sales.BindDB(pool)
	h.Fiscal.BindDB(pool)

	r.Route("/api/v1", func(api chi.Router) {
		api.Post("/auth/login", h.Auth.Login)
		api.Post("/auth/refresh", h.Auth.Refresh)
		api.With(middleware.AuthJWT(cfg, mods.Auth, logger)).Get("/auth/me", h.Auth.Me)

		api.Group(func(pr chi.Router) {
			pr.Use(middleware.AuthJWT(cfg, mods.Auth, logger))
			pr.Use(middleware.LoadPermissions(mods.Auth, logger))

			pr.Route("/products", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("product:read")).Get("/", h.Products.List)
				rr.With(middleware.RequirePermission("product:read")).Get("/{id}", h.Products.Get)
				rr.With(middleware.RequirePermission("product:write")).Post("/", h.Products.Create)
				rr.With(middleware.RequirePermission("product:write")).Put("/{id}", h.Products.Update)
			})

			pr.Route("/inventory", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("inventory:read")).Get("/low-stock", h.Inventory.LowStock)
				rr.With(middleware.RequirePermission("inventory:read")).Get("/movements", h.Inventory.ListMovements)
				rr.With(middleware.RequirePermission("inventory:adjust")).Post("/adjust", h.Inventory.Adjust)
			})

			pr.Route("/cash", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("cash:open")).Post("/sessions/open", h.Cash.OpenSession)
				rr.With(middleware.RequirePermission("cash:close")).Post("/sessions/{id}/close", h.Cash.CloseSession)
			})

			pr.Route("/sales", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("sale:read")).Get("/", h.Sales.List)
				rr.With(middleware.RequirePermission("sale:read")).Get("/{id}", h.Sales.Get)
				rr.With(middleware.RequirePermission("sale:write")).Post("/", h.Sales.CreateAndFinalize)
				rr.With(middleware.RequirePermission("sale:cancel")).Post("/{id}/cancel", h.Sales.Cancel)
			})

			pr.Route("/finance", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("finance:read")).Get("/dashboard", h.Finance.Dashboard)
				rr.With(middleware.RequirePermission("finance:read")).Get("/ledger", h.Finance.ListLedger)
			})

			pr.Route("/fiscal", func(rr chi.Router) {
				rr.With(middleware.RequirePermission("invoice:generate")).Post("/nfe/xml", h.Fiscal.GenerateNFeXML)
				rr.With(middleware.RequirePermission("invoice:read")).Get("/nfe/xml", h.Fiscal.ListXML)
				rr.With(middleware.RequirePermission("invoice:read")).Get("/nfe/xml/{id}/download", h.Fiscal.DownloadXML)
			})
		})
	})

	return r
}
