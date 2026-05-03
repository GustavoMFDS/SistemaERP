package domain

import "github.com/example/sistemaemgo/internal/platform"

type Product struct {
	ID          string            `json:"id"`
	CategoryID  *string           `json:"category_id"`
	SKU         string            `json:"sku"`
	Barcode     *string           `json:"barcode"`
	Name        string            `json:"name"`
	Description *string           `json:"description"`
	Unit        string            `json:"unit"`
	CostPrice   platform.Money    `json:"cost_price"`
	PriceCash   platform.Money    `json:"price_cash"`
	PromoPrice  *platform.Money   `json:"promo_price"`
	MinStock    platform.Quantity `json:"min_stock"`
	Active      bool              `json:"active"`
	QtyOnHand   platform.Quantity `json:"qty_on_hand"`
}

// PodeVender centraliza validações de negócio relacionadas ao produto.
// Regras de estoque (saldo) pertencem ao agregado de estoque, não ao produto.
func (p Product) PodeVender(qty platform.Quantity) error {
	if !p.Active {
		return ErrProductInactive
	}
	if qty <= 0 {
		return ErrInvalidQuantity
	}
	if p.PriceCash <= 0 {
		return ErrInvalidPrice
	}
	return nil
}

func (p Product) EffectiveSalePrice() platform.Money {
	if p.PromoPrice != nil && *p.PromoPrice > 0 {
		return *p.PromoPrice
	}
	return p.PriceCash
}
