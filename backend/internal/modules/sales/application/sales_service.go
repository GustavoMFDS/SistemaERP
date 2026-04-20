package application

import (
	"context"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

type SalesService struct {
	cfg      config.Config
	sales    SalesRepository
	inv      InventoryRepository
	fin      FinanceRepository
	cash     CashRepository
	products ProductsRepository
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

func NewSalesService(cfg config.Config, salesRepo SalesRepository, invRepo InventoryRepository, finRepo FinanceRepository, cashRepo CashRepository, productsRepo ProductsRepository, v *validator.Validate, logger *slog.Logger) *SalesService {
	return &SalesService{cfg: cfg, sales: salesRepo, inv: invRepo, fin: finRepo, cash: cashRepo, products: productsRepo, validate: v, logger: logger}
}

func (s *SalesService) List(ctx context.Context, limit, offset int) ([]sales.Sale, int, error) {
	return s.sales.ListSales(ctx, limit, offset)
}

func (s *SalesService) Get(ctx context.Context, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	return s.sales.GetSale(ctx, id)
}

func (s *SalesService) CreateAndFinalize(ctx context.Context, pool *pgxpool.Pool, actorUserID string, req SaleCreateRequest) (string, float64, error) {
	if err := s.validate.Struct(req); err != nil {
		return "", 0, common.ErrValidation
	}
	// Validate cash session
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cs, err := s.cash.GetSession(ctx, tx, req.CashSessionID)
	if err != nil {
		return "", 0, common.ErrNotFound
	}
	if cs.Status != "open" {
		return "", 0, common.ErrCashSessionClosed
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
		return "", 0, common.ErrValidation
	}

	computedItems := make([]sales.SaleItem, 0, len(req.Items))
	for _, it := range req.Items {
		p := prodMap[it.ProductID]
		if err := p.PodeVender(it.Qty); err != nil {
			switch err {
			case inv.ErrProductInactive:
				return "", 0, common.ErrConflict
			case inv.ErrInvalidQuantity, inv.ErrInvalidPrice:
				return "", 0, common.ErrValidation
			default:
				return "", 0, err
			}
		}
		computedItems = append(computedItems, sales.SaleItem{
			ProductID:     it.ProductID,
			Qty:           it.Qty,
			UnitPrice:     it.UnitPrice,
			DiscountValue: it.DiscountValue,
			CostUnit:      p.CostPrice,
		})
	}

	sale := sales.NewFinalizedSale(req.CashSessionID, req.CustomerID, actorUserID, req.DiscountValue)
	computedItems, derr := sale.CalcularTotal(computedItems)
	if derr != nil {
		switch derr {
		case sales.ErrInvalidItem, sales.ErrInvalidMoney:
			return "", 0, common.ErrValidation
		default:
			return "", 0, derr
		}
	}

	pays := make([]sales.Payment, 0, len(req.Payments))
	for _, p := range req.Payments {
		pays = append(pays, sales.Payment{Method: p.Method, Amount: p.Amount})
	}
	if derr := sale.ValidarPagamentos(pays); derr != nil {
		switch derr {
		case sales.ErrPaymentsMismatch:
			return "", 0, common.ErrPaymentsMismatch
		case sales.ErrInvalidMoney:
			return "", 0, common.ErrValidation
		default:
			return "", 0, derr
		}
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
		after, derr := bal.Baixar(it.Qty, s.cfg.AllowNegativeStock)
		if derr != nil {
			if derr == inv.ErrInsufficientStock {
				return "", 0, common.ErrInsufficientStock
			}
			return "", 0, common.ErrValidation
		}
		if err := s.inv.UpdateBalance(ctx, tx, it.ProductID, after.QtyOnHand); err != nil {
			return "", 0, err
		}

		reason := "Venda"
		refType := "sale"
		refID := saleID
		actor := actorUserID
		mv := inv.NewMovement(it.ProductID, inv.MovementSale, -it.Qty, bal, after, &reason, &refType, &refID, &actor, time.Now().Format(time.RFC3339))
		if err := s.inv.InsertMovement(ctx, tx, mv); err != nil {
			return "", 0, err
		}

		it.SaleID = saleID
		if err := s.sales.InsertItem(ctx, tx, it); err != nil {
			return "", 0, err
		}
	}

	for _, p := range req.Payments {
		pay := sales.Payment{SaleID: saleID, Method: p.Method, Amount: p.Amount}
		if err := s.sales.InsertPayment(ctx, tx, pay); err != nil {
			return "", 0, err
		}
	}

	// Ledger entry
	saleIDPtr := saleID
	cashIDPtr := req.CashSessionID
	_, err = s.fin.InsertLedgerEntry(ctx, tx, fin.LedgerEntry{
		EntryType:       "sale",
		SaleID:          &saleIDPtr,
		CashSessionID:   &cashIDPtr,
		AmountGross:     sale.Subtotal,
		AmountDiscount:  sale.DiscountValue,
		AmountNet:       sale.Total,
		ProfitEstimated: sale.ProfitEstimated,
		Notes:           nil,
		CreatedAt:       time.Now().Format(time.RFC3339),
	}, &actorUserID)
	if err != nil {
		return "", 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, err
	}
	return saleID, sale.Total, nil
}

func (s *SalesService) Cancel(ctx context.Context, pool *pgxpool.Pool, actorUserID string, saleID string, req SaleCancelRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sale, items, _, err := s.sales.GetSaleForUpdate(ctx, tx, saleID)
	if err != nil {
		return common.ErrNotFound
	}
	if derr := sale.PodeCancelar(); derr != nil {
		switch derr {
		case sales.ErrSaleAlreadyCancelled:
			return common.ErrSaleAlreadyCancelled
		case sales.ErrSaleNotFinalized:
			return common.ErrSaleNotFinalized
		default:
			return derr
		}
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
		after, derr := bal.Creditar(it.Qty)
		if derr != nil {
			return common.ErrValidation
		}
		if err := s.inv.UpdateBalance(ctx, tx, it.ProductID, after.QtyOnHand); err != nil {
			return err
		}

		reason := "Cancelamento de venda"
		refType := "sale_cancel"
		refID := saleID
		actor := actorUserID
		mv := inv.NewMovement(it.ProductID, inv.MovementReturn, it.Qty, bal, after, &reason, &refType, &refID, &actor, time.Now().Format(time.RFC3339))
		if err := s.inv.InsertMovement(ctx, tx, mv); err != nil {
			return err
		}
	}

	// Ledger reversal
	saleIDPtr := saleID
	cashIDPtr := sale.CashSessionID
	note := "Cancelamento: " + req.Reason
	_, err = s.fin.InsertLedgerEntry(ctx, tx, fin.LedgerEntry{
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
