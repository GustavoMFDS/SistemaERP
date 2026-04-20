package domain

import "math"

type Sale struct {
	ID              string   `json:"id"`
	CashSessionID   string   `json:"cash_session_id"`
	CustomerID      *string  `json:"customer_id"`
	Status          string   `json:"status"`
	Subtotal        float64  `json:"subtotal"`
	DiscountValue   float64  `json:"discount_value"`
	Total           float64  `json:"total"`
	ProfitEstimated float64  `json:"profit_estimated"`
	CreatedByUserID string   `json:"created_by_user_id"`
	CancelReason    *string  `json:"cancel_reason"`
}

func NewFinalizedSale(cashSessionID string, customerID *string, createdByUserID string, saleLevelDiscount float64) Sale {
	return Sale{
		CashSessionID:   cashSessionID,
		CustomerID:      customerID,
		Status:          "finalized",
		DiscountValue:   saleLevelDiscount, // input: sale-level discount; CalcularTotal will transform to total discount
		CreatedByUserID: createdByUserID,
	}
}

func (s *Sale) CalcularTotal(items []SaleItem) ([]SaleItem, error) {
	if len(items) == 0 {
		return nil, ErrInvalidItem
	}
	if s.DiscountValue < 0 {
		return nil, ErrInvalidMoney
	}

	var subtotal float64
	var itemsDiscount float64
	var profitEstimated float64

	computed := make([]SaleItem, 0, len(items))
	for _, it := range items {
		lineGross, lineNet, err := it.CalcularSubtotal()
		if err != nil {
			return nil, err
		}
		subtotal += lineGross
		itemsDiscount += it.DiscountValue
		profitEstimated += it.ProfitEstimado()

		it.Subtotal = lineNet
		computed = append(computed, it)
	}

	subtotal = round2(subtotal)
	totalDiscount := round2(itemsDiscount + s.DiscountValue)
	total := round2(subtotal - totalDiscount)
	profitEstimated = round2(profitEstimated - s.DiscountValue)
	if total < 0 {
		return nil, ErrInvalidMoney
	}

	s.Subtotal = subtotal
	s.DiscountValue = totalDiscount
	s.Total = total
	s.ProfitEstimated = profitEstimated
	return computed, nil
}

func (s Sale) ValidarPagamentos(pays []Payment) error {
	if len(pays) == 0 {
		return ErrPaymentsMismatch
	}
	var paid float64
	for _, p := range pays {
		if p.Amount <= 0 {
			return ErrInvalidMoney
		}
		paid += p.Amount
	}
	paid = round2(paid)
	if math.Abs(paid-s.Total) > 0.01 {
		return ErrPaymentsMismatch
	}
	return nil
}

func (s Sale) PodeCancelar() error {
	if s.Status == "cancelled" {
		return ErrSaleAlreadyCancelled
	}
	if s.Status != "finalized" {
		return ErrSaleNotFinalized
	}
	return nil
}

type SaleItem struct {
	ID            string  `json:"id"`
	SaleID        string  `json:"sale_id"`
	ProductID     string  `json:"product_id"`
	Qty           float64 `json:"qty"`
	UnitPrice     float64 `json:"unit_price"`
	DiscountValue float64 `json:"discount_value"`
	Subtotal      float64 `json:"subtotal"`
	CostUnit      float64 `json:"cost_unit"`
}

func (it SaleItem) CalcularSubtotal() (lineGross float64, lineNet float64, err error) {
	if it.Qty <= 0 || it.UnitPrice <= 0 || it.DiscountValue < 0 {
		return 0, 0, ErrInvalidItem
	}
	lineGross = round2(it.UnitPrice * it.Qty)
	lineNet = round2(lineGross - it.DiscountValue)
	if lineNet < 0 {
		return 0, 0, ErrInvalidMoney
	}
	return lineGross, lineNet, nil
}

func (it SaleItem) ProfitEstimado() float64 {
	// Lucro estimado: (preço - custo) * qty - desconto item
	return round2((it.UnitPrice-it.CostUnit)*it.Qty - it.DiscountValue)
}

type Payment struct {
	ID     string  `json:"id"`
	SaleID string  `json:"sale_id"`
	Method string  `json:"method"`
	Amount float64 `json:"amount"`
}

type CashSession struct {
	ID             string
	RegisterID     string
	OpenedByUserID string
	Status         string
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
