package service

import (
	"context"
	"math"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/repo"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

type SalesService struct {
	cfg      config.Config
	sales    *repo.SalesRepo
	inv      *repo.InventoryRepo
	fin      *repo.FinanceRepo
	cash     *repo.CashRepo
	products *repo.ProductsRepo
	validate *validator.Validate
	logger   *slog.Logger
}

type SaleCreateRequest struct {
	CashSessionID string              `json:"cash_session_id" validate:"required"`
	CustomerID    *string             `json:"customer_id"`
	DiscountValue float64             `json:"discount_value" validate:"min=0"`
	Items         []SaleItemRequest    `json:"items" validate:"required,min=1,dive"`
	Payments      []SalePaymentRequest `json:"payments" validate:"required,min=1,dive"`
}

type SaleItemRequest struct {
	ProductID     string  `json:"product_id" validate:"required"`
	Qty           float64 `json:"qty" validate:"required,gt=0"`
	UnitPrice     float64 `json:"unit_price" validate:"required,gt=0"`
	DiscountValue float64 `json:"discount_value" validate:"min=0"`
}

type SalePaymentRequest struct {
	Method string  `json:"method" validate:"required,oneof=cash pix debit credit transfer voucher"`
	Amount float64 `json:"amount" validate:"required,gt=0"`
}

type SaleCancelRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=250"`
}

func NewSalesService(cfg config.Config, sales *repo.SalesRepo, inv *repo.InventoryRepo, fin *repo.FinanceRepo, cash *repo.CashRepo, products *repo.ProductsRepo, v *validator.Validate, logger *slog.Logger) *SalesService {
	return &SalesService{cfg: cfg, sales: sales, inv: inv, fin: fin, cash: cash, products: products, validate: v, logger: logger}
}

func (s *SalesService) List(ctx context.Context, limit, offset int) ([]repo.Sale, int, error) {
	return s.sales.ListSales(ctx, limit, offset)
}

func (s *SalesService) Get(ctx context.Context, id string) (repo.Sale, []repo.SaleItem, []repo.Payment, error) {
	return s.sales.GetSale(ctx, id)
}

func (s *SalesService) CreateAndFinalize(ctx context.Context, pool *pgxpool.Pool, actorUserID string, req SaleCreateRequest) (string, float64, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", 0, ErrValidation
	}
	// Validate cash session
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cs, err := s.cash.GetSession(ctx, tx, req.CashSessionID)
	if err != nil {
		return "", 0, ErrNotFound
	}
	if cs.Status != "open" {
		return "", 0, ErrCashSessionClosed
	}

	// Load products in batch (snapshot)
	ids := make([]string, 0, len(req.Items))
	seen := map[string]bool{}
	for _, it := range req.Items {
		if !seen[it.ProductID] {
			seen[it.ProductID] = true
			ids = append(ids, it.ProductID)
		}
	}
	prodMap, err := s.products.GetManyByIDs(ctx, tx, ids)
	if err != nil {
		return "", 0, err
	}
	if len(prodMap) != len(ids) {
		return "", 0, ErrValidation
	}

	// Compute totals
	var subtotal float64
	var itemsDiscount float64
	var profitEstimated float64

	computedItems := make([]repo.SaleItem, 0, len(req.Items))
	for _, it := range req.Items {
		p := prodMap[it.ProductID]
		if !p.Active {
			return "", 0, ErrConflict
		}

		lineGross := round2(it.UnitPrice * it.Qty)
		lineNet := round2(lineGross - it.DiscountValue)
		if lineNet < 0 {
			return "", 0, ErrValidation
		}
		subtotal += lineGross
		itemsDiscount += it.DiscountValue
		profitEstimated += round2((it.UnitPrice-p.CostPrice)*it.Qty - it.DiscountValue)

		computedItems = append(computedItems, repo.SaleItem{
			ProductID:     it.ProductID,
			Qty:           it.Qty,
			UnitPrice:     it.UnitPrice,
			DiscountValue: it.DiscountValue,
			Subtotal:      lineNet,
			CostUnit:      p.CostPrice,
		})
	}

	subtotal = round2(subtotal)
	totalDiscount := round2(itemsDiscount + req.DiscountValue)
	total := round2(subtotal - totalDiscount)
	profitEstimated = round2(profitEstimated - req.DiscountValue)
	if total < 0 {
		return "", 0, ErrValidation
	}

	// Payments must match
	var paid float64
	for _, p := range req.Payments {
		paid += p.Amount
	}
	paid = round2(paid)
	if math.Abs(paid-total) > 0.01 {
		return "", 0, ErrPaymentsMismatch
	}

	sale := repo.Sale{
		CashSessionID:    req.CashSessionID,
		CustomerID:       req.CustomerID,
		Status:           "finalized",
		Subtotal:         subtotal,
		DiscountValue:    totalDiscount,
		Total:            total,
		ProfitEstimated:  profitEstimated,
		CreatedByUserID:  actorUserID,
		CancelReason:     nil,
	}
	saleID, err := s.sales.InsertSale(ctx, tx, sale)
	if err != nil {
		return "", 0, err
	}

	// Stock update per item (with row locks)
	for _, it := range computedItems {
		if err := s.inv.EnsureBalanceRow(ctx, tx, it.ProductID); err != nil {
			return "", 0, err
		}
		bal, err := s.inv.GetBalanceForUpdate(ctx, tx, it.ProductID)
		if err != nil {
			return "", 0, err
		}
		newQty := bal.QtyOnHand - it.Qty
		if !s.cfg.AllowNegativeStock && newQty < 0 {
			return "", 0, ErrInsufficientStock
		}
		if err := s.inv.UpdateBalance(ctx, tx, it.ProductID, newQty); err != nil {
			return "", 0, err
		}

		reason := "Venda"
		refType := "sale"
		refID := saleID
		actor := actorUserID
		mv := repo.InventoryMovement{
			ProductID:     it.ProductID,
			MovementType:  "sale",
			Delta:         -it.Qty,
			QtyBefore:     bal.QtyOnHand,
			QtyAfter:      newQty,
			Reason:        &reason,
			ReferenceType: &refType,
			ReferenceID:   &refID,
			ActorUserID:   &actor,
			CreatedAt:     time.Now().Format(time.RFC3339),
		}
		if err := s.inv.InsertMovement(ctx, tx, mv); err != nil {
			return "", 0, err
		}

		it.SaleID = saleID
		if err := s.sales.InsertItem(ctx, tx, it); err != nil {
			return "", 0, err
		}
	}

	for _, p := range req.Payments {
		pay := repo.Payment{SaleID: saleID, Method: p.Method, Amount: p.Amount}
		if err := s.sales.InsertPayment(ctx, tx, pay); err != nil {
			return "", 0, err
		}
	}

	// Ledger entry
	saleIDPtr := saleID
	cashIDPtr := req.CashSessionID
	_, err = s.fin.InsertLedgerEntry(ctx, tx, repo.LedgerEntry{
		EntryType:       "sale",
		SaleID:          &saleIDPtr,
		CashSessionID:   &cashIDPtr,
		AmountGross:     subtotal,
		AmountDiscount:  totalDiscount,
		AmountNet:       total,
		ProfitEstimated: profitEstimated,
		Notes:           nil,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}, &actorUserID)
	if err != nil {
		return "", 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	return saleID, total, nil
}

func (s *SalesService) Cancel(ctx context.Context, pool *pgxpool.Pool, actorUserID string, saleID string, req SaleCancelRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return ErrValidation
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sale, items, _, err := s.sales.GetSaleForUpdate(ctx, tx, saleID)
	if err != nil {
		return ErrNotFound
	}
	if sale.Status == "cancelled" {
		return ErrSaleAlreadyCancelled
	}
	if sale.Status != "finalized" {
		return ErrSaleNotFinalized
	}

	if err := s.sales.CancelSale(ctx, tx, saleID, req.Reason); err != nil {
		return err
	}

	// restore stock
	for _, it := range items {
		if err := s.inv.EnsureBalanceRow(ctx, tx, it.ProductID); err != nil {
			return err
		}
		bal, err := s.inv.GetBalanceForUpdate(ctx, tx, it.ProductID)
		if err != nil {
			return err
		}
		newQty := bal.QtyOnHand + it.Qty
		if err := s.inv.UpdateBalance(ctx, tx, it.ProductID, newQty); err != nil {
			return err
		}

		reason := "Cancelamento de venda"
		refType := "sale_cancel"
		refID := saleID
		actor := actorUserID
		mv := repo.InventoryMovement{
			ProductID:     it.ProductID,
			MovementType:  "return",
			Delta:         it.Qty,
			QtyBefore:     bal.QtyOnHand,
			QtyAfter:      newQty,
			Reason:        &reason,
			ReferenceType: &refType,
			ReferenceID:   &refID,
			ActorUserID:   &actor,
			CreatedAt:     time.Now().Format(time.RFC3339),
		}
		if err := s.inv.InsertMovement(ctx, tx, mv); err != nil {
			return err
		}
	}

	// Ledger reversal
	saleIDPtr := saleID
	cashIDPtr := sale.CashSessionID
	note := "Cancelamento: " + req.Reason
	_, err = s.fin.InsertLedgerEntry(ctx, tx, repo.LedgerEntry{
		EntryType:       "sale_cancel",
		SaleID:          &saleIDPtr,
		CashSessionID:   &cashIDPtr,
		AmountGross:     -sale.Subtotal,
		AmountDiscount:  -sale.DiscountValue,
		AmountNet:       -sale.Total,
		ProfitEstimated: -sale.ProfitEstimated,
		Notes:           &note,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}, &actorUserID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
