package domain

type Product struct {
	ID          string   `json:"id"`
	CategoryID  *string  `json:"category_id"`
	SKU         string   `json:"sku"`
	Barcode     *string  `json:"barcode"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	Unit        string   `json:"unit"`
	CostPrice   float64  `json:"cost_price"`
	PriceCash   float64  `json:"price_cash"`
	PromoPrice  *float64 `json:"promo_price"`
	MinStock    float64  `json:"min_stock"`
	Active      bool     `json:"active"`
	QtyOnHand   float64  `json:"qty_on_hand"`
}

// PodeVender centraliza validações de negócio relacionadas ao produto.
// Regras de estoque (saldo) pertencem ao agregado de estoque, não ao produto.
func (p Product) PodeVender(qty float64) error {
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
