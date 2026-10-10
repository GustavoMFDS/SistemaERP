package handlers

import (
	"errors"
	"net/http"

	"github.com/example/sistemaemgo/internal/httpapi/middleware"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductVariationsHandler struct {
	pool     *pgxpool.Pool
	products *invapp.ProductsService
}

func NewProductVariationsHandler(pool *pgxpool.Pool, products *invapp.ProductsService) *ProductVariationsHandler {
	return &ProductVariationsHandler{pool: pool, products: products}
}

type productVariationOption struct {
	ID         string   `json:"id"`
	SKU        string   `json:"sku"`
	Name       string   `json:"name"`
	Unit       string   `json:"unit"`
	Barcode    *string  `json:"barcode"`
	PromoPrice *float64 `json:"promo_price"`
	PriceCash  float64  `json:"price_cash"`
	QtyOnHand  float64  `json:"qty_on_hand"`
	Active     bool     `json:"active"`
	Label      string   `json:"option_label"`
	IsBase     bool     `json:"is_base"`
}

// List resolves either a parent or a child to its family. It returns only
// same-company products; inactive choices remain visible but cannot be sold.
func (h *ProductVariationsHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		photoError(w, r, http.StatusUnauthorized, "Sessão inválida")
		return
	}
	productID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || productID == uuid.Nil {
		photoError(w, r, http.StatusUnprocessableEntity, "Produto inválido")
		return
	}
	var parentID string
	err = h.pool.QueryRow(r.Context(), `
        SELECT COALESCE(v.parent_product_id::text, p.id::text)
        FROM products p
        LEFT JOIN product_variations v ON v.tenant_id=p.tenant_id AND v.variant_product_id=p.id
        WHERE p.tenant_id=$1 AND p.id=$2
    `, user.TenantID, productID).Scan(&parentID)
	if errors.Is(err, pgx.ErrNoRows) {
		photoError(w, r, http.StatusNotFound, "Produto não encontrado nesta loja")
		return
	}
	if err != nil {
		photoError(w, r, http.StatusInternalServerError, "Não foi possível consultar as opções")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
        SELECT p.id::text, p.sku, p.name, p.unit, p.barcode, p.price_cash::float8, p.promo_price::float8,
               COALESCE(b.qty_on_hand,0)::float8, p.active,
               COALESCE(v.option_label,''), (p.id=$2::uuid)
        FROM products p
        LEFT JOIN product_variations v
          ON v.tenant_id=p.tenant_id AND v.variant_product_id=p.id
        LEFT JOIN inventory_balances b
          ON b.tenant_id=p.tenant_id AND b.product_id=p.id
        WHERE p.tenant_id=$1 AND (p.id=$2::uuid OR v.parent_product_id=$2::uuid)
        ORDER BY (p.id=$2::uuid) DESC, v.option_label, p.name
    `, user.TenantID, parentID)
	if err != nil {
		photoError(w, r, http.StatusInternalServerError, "Não foi possível listar as opções")
		return
	}
	defer rows.Close()
	options := make([]productVariationOption, 0, 8)
	for rows.Next() {
		var item productVariationOption
		if err := rows.Scan(&item.ID, &item.SKU, &item.Name, &item.Unit, &item.Barcode, &item.PriceCash, &item.PromoPrice,
			&item.QtyOnHand, &item.Active, &item.Label, &item.IsBase); err != nil {
			photoError(w, r, http.StatusInternalServerError, "Falha ao ler opções")
			return
		}
		options = append(options, item)
	}
	if err := rows.Err(); err != nil {
		photoError(w, r, http.StatusInternalServerError, "Falha ao consultar opções")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"parent_id": parentID, "items": options})
}

// Create atomically inserts a second SKU and its family link. The existing
// product and all inventory, sale, return and fiscal records are untouched.
func (h *ProductVariationsHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetAuthUser(r.Context())
	if !ok {
		photoError(w, r, http.StatusUnauthorized, "Sessão inválida")
		return
	}
	parent, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || parent == uuid.Nil {
		photoError(w, r, http.StatusUnprocessableEntity, "Produto inválido")
		return
	}
	var req struct {
		OptionLabel string                      `json:"option_label"`
		Product     invapp.ProductCreateRequest `json:"product"`
	}
	if err := readJSON(w, r, &req); err != nil {
		photoError(w, r, http.StatusBadRequest, "Informações de opção inválidas")
		return
	}
	// Fiscal codes and barcode must be explicitly entered for a new SKU.
	// Do not automatically inherit the original product's tax profile.
	if !middleware.HasPermission(r.Context(), "finance:read") {
		req.Product.CostPrice = 0
	}
	id, err := h.products.CreateVariation(r.Context(), user.TenantID, user.UserID,
		parent.String(), req.OptionLabel, req.Product)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, common.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(err, common.ErrConflict):
			status = http.StatusConflict
		case errors.Is(err, common.ErrValidation):
			status = http.StatusUnprocessableEntity
		}
		photoError(w, r, status, friendlyErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "parent_id": parent.String()})
}
