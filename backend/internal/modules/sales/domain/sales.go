package domain

import "github.com/example/sistemaemgo/internal/platform"

type Sale struct {
	ID              string         `json:"id"`
	CashSessionID   string         `json:"cash_session_id"`
	CustomerID      *string        `json:"customer_id"`
	Status          string         `json:"status"`
	Subtotal        platform.Money `json:"subtotal"`
	DiscountValue   platform.Money `json:"discount_value"`
	Total           platform.Money `json:"total"`
	ProfitEstimated platform.Money `json:"profit_estimated"`
	CreatedByUserID string         `json:"created_by_user_id"`
	CancelReason    *string        `json:"cancel_reason"`
}

func NewFinalizedSale(cashSessionID string, customerID *string, createdByUserID string, saleLevelDiscount platform.Money) Sale {
	return Sale{
		CashSessionID:   cashSessionID,
		CustomerID:      customerID,
		Status:          "finalized",
		DiscountValue:   saleLevelDiscount,
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

	var subtotal platform.Money
	var itemsDiscount platform.Money
	var profitEstimated platform.Money

	computed := make([]SaleItem, 0, len(items))
	for _, it := range items {
		lineGross, lineNet, err := it.CalcularSubtotal()
		if err != nil {
			return nil, err
		}
		subtotal, err = subtotal.AddChecked(lineGross)
		if err != nil {
			return nil, ErrInvalidMoney
		}
		itemsDiscount, err = itemsDiscount.AddChecked(it.DiscountValue)
		if err != nil {
			return nil, ErrInvalidMoney
		}
		itemProfit, err := it.ProfitEstimado()
		if err != nil {
			return nil, err
		}
		profitEstimated, err = profitEstimated.AddChecked(itemProfit)
		if err != nil {
			return nil, ErrInvalidMoney
		}

		it.Subtotal = lineNet
		computed = append(computed, it)
	}

	totalDiscount, err := itemsDiscount.AddChecked(s.DiscountValue)
	if err != nil {
		return nil, ErrInvalidMoney
	}
	total, err := subtotal.SubChecked(totalDiscount)
	if err != nil {
		return nil, ErrInvalidMoney
	}
	profitEstimated, err = profitEstimated.SubChecked(s.DiscountValue)
	if err != nil {
		return nil, ErrInvalidMoney
	}
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
	var paid platform.Money
	for _, p := range pays {
		if p.Amount <= 0 {
			return ErrInvalidMoney
		}
		var err error
		paid, err = paid.AddChecked(p.Amount)
		if err != nil {
			return ErrInvalidMoney
		}
	}
	if paid != s.Total {
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
	ID            string            `json:"id"`
	SaleID        string            `json:"sale_id"`
	ProductID     string            `json:"product_id"`
	Qty           platform.Quantity `json:"qty"`
	UnitPrice     platform.Money    `json:"unit_price"`
	DiscountValue platform.Money    `json:"discount_value"`
	Subtotal      platform.Money    `json:"subtotal"`
	CostUnit      platform.Money    `json:"cost_unit"`
}

func (it SaleItem) CalcularSubtotal() (lineGross platform.Money, lineNet platform.Money, err error) {
	if it.Qty <= 0 || it.UnitPrice <= 0 || it.DiscountValue < 0 {
		return 0, 0, ErrInvalidItem
	}
	var calcErr error
	lineGross, calcErr = it.UnitPrice.MulQtyChecked(it.Qty)
	if calcErr != nil {
		return 0, 0, ErrInvalidMoney
	}
	lineNet, calcErr = lineGross.SubChecked(it.DiscountValue)
	if calcErr != nil || lineNet < 0 {
		return 0, 0, ErrInvalidMoney
	}
	return lineGross, lineNet, nil
}

func (it SaleItem) ProfitEstimado() (platform.Money, error) {
	grossProfit, err := (it.UnitPrice - it.CostUnit).MulQtyChecked(it.Qty)
	if err != nil {
		return 0, ErrInvalidMoney
	}
	profit, err := grossProfit.SubChecked(it.DiscountValue)
	if err != nil {
		return 0, ErrInvalidMoney
	}
	return profit, nil
}

type Payment struct {
	ID                   string         `json:"id"`
	SaleID               string         `json:"sale_id"`
	Method               string         `json:"method"`
	Amount               platform.Money `json:"amount"`
	Provider             *string        `json:"provider,omitempty"`
	TransactionRef       *string        `json:"transaction_ref,omitempty"`
	AuthorizationCode    *string        `json:"authorization_code,omitempty"`
	Installments         int            `json:"installments"`
	ReconciliationStatus string         `json:"reconciliation_status"`
}

type CashSession struct {
	ID             string
	RegisterID     string
	OpenedByUserID string
	Status         string
	OpeningAmount  platform.Money
}

type CashMovement struct {
	ID        string         `json:"id"`
	SessionID string         `json:"cash_session_id"`
	Type      string         `json:"movement_type"`
	Amount    platform.Money `json:"amount"`
	Notes     *string        `json:"notes,omitempty"`
}

type CashCloseResult struct {
	ExpectedCash       platform.Money            `json:"expected_cash"`
	ClosingAmount      platform.Money            `json:"closing_amount"`
	ClosingDifference  platform.Money            `json:"closing_difference"`
	ExpectedByMethod   map[string]platform.Money `json:"expected_by_method"`
	DeclaredByMethod   map[string]platform.Money `json:"declared_by_method"`
	DifferenceByMethod map[string]platform.Money `json:"difference_by_method"`
}
