package httpapi

import (
	"github.com/example/sistemaemgo/internal/httpapi/handlers"
	"github.com/example/sistemaemgo/internal/modules"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Wire attaches shared dependencies that are not passed via route constructors.
func Wire(h *handlers.Handlers, mods *modules.Modules, pool *pgxpool.Pool) {
	h.Products.BindDB(pool)
	h.Inventory.BindDB(pool)
	h.Cash.BindDB(pool)
	h.Sales.BindDB(pool)
	h.Fiscal.BindDB(pool)

	_ = mods
}
