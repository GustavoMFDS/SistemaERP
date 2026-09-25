package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"log/slog"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/example/sistemaemgo/internal/platform/events"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type SalesService struct {
	cfg      config.Config
	uow      db.UnitOfWork
	sales    SalesRepository
	inv      InventoryRepository
	fin      FinanceRepository
	cash     CashRepository
	products ProductsRepository
	audit    *audit.Service
	events   *events.Bus
	validate *validator.Validate
	logger   *slog.Logger
}

type SaleCreateRequest struct {
	CashSessionID string               `json:"cash_session_id" validate:"required"`
	CustomerID    *string              `json:"customer_id"`
	DiscountValue platform.Money       `json:"discount_value" validate:"min=0"`
	Items         []SaleItemRequest    `json:"items" validate:"required,min=1,dive"`
	Payments      []SalePaymentRequest `json:"payments" validate:"required,min=1,dive"`
}

type SaleItemRequest struct {
	ProductID     string            `json:"product_id" validate:"required"`
	Qty           platform.Quantity `json:"qty" validate:"required,gt=0"`
	UnitPrice     *platform.Money   `json:"unit_price,omitempty" validate:"omitempty,gt=0"` // Deprecated: ignored for calculation.
	DiscountValue platform.Money    `json:"discount_value" validate:"min=0"`
}

type SalePaymentRequest struct {
	Method string         `json:"method" validate:"required,oneof=cash pix debit credit transfer voucher"`
	Amount platform.Money `json:"amount" validate:"required,gt=0"`
}

type SaleCancelRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=250"`
}

func NewSalesService(cfg config.Config, uow db.UnitOfWork, salesRepo SalesRepository, invRepo InventoryRepository, finRepo FinanceRepository, cashRepo CashRepository, productsRepo ProductsRepository, auditSvc *audit.Service, bus *events.Bus, v *validator.Validate, logger *slog.Logger) *SalesService {
	return &SalesService{cfg: cfg, uow: uow, sales: salesRepo, inv: invRepo, fin: finRepo, cash: cashRepo, products: productsRepo, audit: auditSvc, events: bus, validate: v, logger: logger}
}

func (s *SalesService) List(ctx context.Context, tenantID string, limit, offset int) ([]sales.Sale, int, error) {
	return s.sales.ListSales(ctx, tenantID, limit, offset)
}

func (s *SalesService) Get(ctx context.Context, tenantID string, id string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	return s.sales.GetSale(ctx, tenantID, id)
}

func (s *SalesService) CreateAndFinalize(ctx context.Context, tenantID string, actorUserID string, idempotencyKey string, req SaleCreateRequest) (string, platform.Money, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return "", 0, false, common.ErrValidation
	}
	if err := s.validate.Struct(req); err != nil {
		return "", 0, false, common.ErrValidation
	}
	if err := normalizeOptionalCustomerID(req.CustomerID); err != nil {
		return "", 0, false, common.ErrValidation
	}
	op := "sales.create_and_finalize"
	requestHash, err := saleRequestHash(req)
	if err != nil {
		return "", 0, false, common.ErrValidation
	}
	// Validate cash session
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.sales.LockIdempotencyKey(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", 0, false, err
	}
	if saleID, total, storedHash, ok, err := s.sales.GetIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey); err != nil {
		return "", 0, false, err
	} else if ok {
		if storedHash != requestHash {
			return "", 0, false, common.ErrConflict
		}
		_ = tx.Rollback(ctx)
		return saleID, total, false, nil
	}

	if req.CustomerID != nil {
		ok, err := s.sales.CustomerBelongsToTenant(ctx, tx, tenantID, *req.CustomerID)
		if err != nil {
			return "", 0, false, err
		}
		if !ok {
			return "", 0, false, common.ErrValidation
		}
	}

	cs, err := s.cash.GetSession(ctx, tx, tenantID, req.CashSessionID)
	if err != nil {
		return "", 0, false, common.ErrNotFound
	}
	if cs.Status != "open" {
		return "", 0, false, common.ErrCashSessionClosed
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
	prodMap, err := s.products.GetManyByIDs(ctx, tx, tenantID, ids)
	if err != nil {
		return "", 0, false, err
	}
	if len(prodMap) != len(ids) {
		return "", 0, false, common.ErrValidation
	}

	computedItems := make([]sales.SaleItem, 0, len(req.Items))
	for _, it := range req.Items {
		p := prodMap[it.ProductID]
		if err := p.PodeVender(it.Qty); err != nil {
			switch err {
			case inv.ErrProductInactive:
				return "", 0, false, common.ErrConflict
			case inv.ErrInvalidQuantity, inv.ErrInvalidPrice:
				return "", 0, false, common.ErrValidation
			default:
				return "", 0, false, err
			}
		}
		computedItems = append(computedItems, sales.SaleItem{
			ProductID:     it.ProductID,
			Qty:           it.Qty,
			UnitPrice:     p.EffectiveSalePrice(),
			DiscountValue: it.DiscountValue,
			CostUnit:      p.CostPrice,
		})
	}

	sale := sales.NewFinalizedSale(req.CashSessionID, req.CustomerID, actorUserID, req.DiscountValue)
	computedItems, derr := sale.CalcularTotal(computedItems)
	if derr != nil {
		switch derr {
		case sales.ErrInvalidItem, sales.ErrInvalidMoney:
			return "", 0, false, common.ErrValidation
		default:
			return "", 0, false, derr
		}
	}

	pays := make([]sales.Payment, 0, len(req.Payments))
	for _, p := range req.Payments {
		pays = append(pays, sales.Payment{Method: p.Method, Amount: p.Amount})
	}
	if derr := sale.ValidarPagamentos(pays); derr != nil {
		switch derr {
		case sales.ErrPaymentsMismatch:
			return "", 0, false, common.ErrPaymentsMismatch
		case sales.ErrInvalidMoney:
			return "", 0, false, common.ErrValidation
		default:
			return "", 0, false, derr
		}
	}
	saleID, err := s.sales.InsertSale(ctx, tx, tenantID, sale)
	if err != nil {
		return "", 0, false, err
	}

	var pendingEvents []events.DomainEvent

	// Stock update per item (pessimistic locking)
	// Important: acquire row locks in a stable order to avoid deadlocks when multiple sales touch the same products.
	productIDs := uniqueSortedProductIDsFromSaleItems(computedItems)
	balances := make(map[string]inv.InventoryBalance, len(productIDs))
	if batch, ok := s.inv.(interface {
		EnsureBalanceRows(context.Context, db.DBTX, string, []string) error
		GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
	}); ok {
		if err := batch.EnsureBalanceRows(ctx, tx, tenantID, productIDs); err != nil {
			return "", 0, false, err
		}
		bals, err := batch.GetBalancesForUpdate(ctx, tx, tenantID, productIDs)
		if err != nil {
			return "", 0, false, err
		}
		balances = bals
	} else {
		for _, pid := range productIDs {
			if err := s.inv.EnsureBalanceRow(ctx, tx, tenantID, pid); err != nil {
				return "", 0, false, err
			}
		}
		for _, pid := range productIDs {
			bal, err := s.inv.GetBalanceForUpdate(ctx, tx, tenantID, pid)
			if err != nil {
				return "", 0, false, err
			}
			balances[pid] = bal
		}
	}

	for _, it := range computedItems {
		bal := balances[it.ProductID]
		after, derr := bal.Baixar(it.Qty, s.cfg.AllowNegativeStock)
		if derr != nil {
			if derr == inv.ErrInsufficientStock {
				return "", 0, false, common.ErrInsufficientStock
			}
			return "", 0, false, common.ErrValidation
		}
		if err := s.inv.UpdateBalance(ctx, tx, tenantID, it.ProductID, after.QtyOnHand); err != nil {
			return "", 0, false, err
		}
		balances[it.ProductID] = after

		if s.events != nil {
			ev := events.InventoryDebitedEvent{
				ProductID:  it.ProductID,
				TenantID:   tenantID,
				SaleID:     saleID,
				QtyDebited: it.Qty,
				QtyAfter:   after.QtyOnHand,
				At:         time.Now(),
			}
			pendingEvents = append(pendingEvents, ev)
			p := prodMap[it.ProductID]
			if p.MinStock > 0 && after.QtyOnHand <= p.MinStock {
				pendingEvents = append(pendingEvents, events.InventoryLowStockEvent{
					ProductID:  it.ProductID,
					TenantID:   tenantID,
					ProductSKU: p.SKU,
					QtyOnHand:  after.QtyOnHand,
					MinStock:   p.MinStock,
					At:         time.Now(),
				})
			}
		}

		reason := "Venda"
		refType := "sale"
		refID := saleID
		actor := actorUserID
		mv := inv.NewMovement(it.ProductID, inv.MovementSale, -it.Qty, bal, after, &reason, &refType, &refID, &actor, time.Now().Format(time.RFC3339))
		if err := s.inv.InsertMovement(ctx, tx, tenantID, mv); err != nil {
			return "", 0, false, err
		}

		it.SaleID = saleID
		if err := s.sales.InsertItem(ctx, tx, tenantID, it); err != nil {
			return "", 0, false, err
		}
	}

	for _, p := range req.Payments {
		pay := sales.Payment{SaleID: saleID, Method: p.Method, Amount: p.Amount}
		if err := s.sales.InsertPayment(ctx, tx, tenantID, pay); err != nil {
			return "", 0, false, err
		}
	}

	// Ledger entry
	saleIDPtr := saleID
	cashIDPtr := req.CashSessionID
	_, err = s.fin.InsertLedgerEntry(ctx, tx, tenantID, fin.LedgerEntry{
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
		return "", 0, false, err
	}

	if err := s.sales.SaveIdempotencyResult(ctx, tx, tenantID, op, idempotencyKey, requestHash, saleID, sale.Total); err != nil {
		return "", 0, false, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "sale.create",
		ResourceType: "sale", ResourceID: saleID, Outcome: "success",
		Metadata: map[string]any{"total": sale.Total.String()},
	}); err != nil {
		return "", 0, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, false, err
	}
	s.invalidateProductCaches(context.WithoutCancel(ctx), tenantID, productIDs)
	if s.events != nil {
		snaps := make([]events.SaleItemSnapshot, 0, len(computedItems))
		for _, it := range computedItems {
			snaps = append(snaps, events.SaleItemSnapshot{ProductID: it.ProductID, Qty: it.Qty, UnitPrice: it.UnitPrice, CostUnit: it.CostUnit})
		}
		base := []events.DomainEvent{
			events.SaleCreatedEvent{
				SaleID:    saleID,
				TenantID:  tenantID,
				SessionID: req.CashSessionID,
				Total:     sale.Total,
				Items:     snaps,
				At:        time.Now(),
			},
		}
		s.events.PublishAll(ctx, append(base, pendingEvents...))
	}
	return saleID, sale.Total, true, nil
}

func saleRequestHash(req SaleCreateRequest) (string, error) {
	type item struct {
		ProductID     string            `json:"product_id"`
		Qty           platform.Quantity `json:"qty"`
		DiscountValue platform.Money    `json:"discount_value"`
	}
	type payment struct {
		Method string         `json:"method"`
		Amount platform.Money `json:"amount"`
	}
	payload := struct {
		CashSessionID string         `json:"cash_session_id"`
		CustomerID    *string        `json:"customer_id"`
		DiscountValue platform.Money `json:"discount_value"`
		Items         []item         `json:"items"`
		Payments      []payment      `json:"payments"`
	}{
		CashSessionID: req.CashSessionID,
		CustomerID:    req.CustomerID,
		DiscountValue: req.DiscountValue,
		Items:         make([]item, 0, len(req.Items)),
		Payments:      make([]payment, 0, len(req.Payments)),
	}
	for _, it := range req.Items {
		payload.Items = append(payload.Items, item{ProductID: it.ProductID, Qty: it.Qty, DiscountValue: it.DiscountValue})
	}
	for _, p := range req.Payments {
		payload.Payments = append(payload.Payments, payment{Method: p.Method, Amount: p.Amount})
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (s *SalesService) Cancel(ctx context.Context, tenantID string, actorUserID string, saleID string, req SaleCancelRequest) error {
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sale, items, _, err := s.sales.GetSaleForUpdate(ctx, tx, tenantID, saleID)
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

	hasInvoice, err := s.sales.HasInvoiceForSale(ctx, tx, tenantID, saleID)
	if err != nil {
		return err
	}
	if hasInvoice {
		return common.ErrConflict
	}

	session, err := s.cash.GetSession(ctx, tx, tenantID, sale.CashSessionID)
	if err != nil {
		return common.ErrCashSessionClosed
	}
	if session.Status != "open" {
		return common.ErrCashSessionClosed
	}

	if err := s.sales.CancelSale(ctx, tx, tenantID, saleID, req.Reason); err != nil {
		return err
	}

	// restore stock
	productIDs := uniqueSortedProductIDsFromSaleItems(items)
	balances := make(map[string]inv.InventoryBalance, len(productIDs))
	if batch, ok := s.inv.(interface {
		EnsureBalanceRows(context.Context, db.DBTX, string, []string) error
		GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
	}); ok {
		if err := batch.EnsureBalanceRows(ctx, tx, tenantID, productIDs); err != nil {
			return err
		}
		balances, err = batch.GetBalancesForUpdate(ctx, tx, tenantID, productIDs)
		if err != nil {
			return err
		}
	} else {
		for _, pid := range productIDs {
			if err := s.inv.EnsureBalanceRow(ctx, tx, tenantID, pid); err != nil {
				return err
			}
		}
		for _, pid := range productIDs {
			bal, err := s.inv.GetBalanceForUpdate(ctx, tx, tenantID, pid)
			if err != nil {
				return err
			}
			balances[pid] = bal
		}
	}
	for _, it := range items {
		bal, ok := balances[it.ProductID]
		if !ok {
			return common.ErrValidation
		}
		after, derr := bal.Creditar(it.Qty)
		if derr != nil {
			return common.ErrValidation
		}
		if err := s.inv.UpdateBalance(ctx, tx, tenantID, it.ProductID, after.QtyOnHand); err != nil {
			return err
		}
		balances[it.ProductID] = after

		reason := "Cancelamento de venda"
		refType := "sale_cancel"
		refID := saleID
		actor := actorUserID
		mv := inv.NewMovement(it.ProductID, inv.MovementReturn, it.Qty, bal, after, &reason, &refType, &refID, &actor, time.Now().Format(time.RFC3339))
		if err := s.inv.InsertMovement(ctx, tx, tenantID, mv); err != nil {
			return err
		}
	}

	// Ledger reversal
	saleIDPtr := saleID
	cashIDPtr := sale.CashSessionID
	note := "Cancelamento: " + req.Reason
	_, err = s.fin.InsertLedgerEntry(ctx, tx, tenantID, fin.LedgerEntry{
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
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "sale.cancel",
		ResourceType: "sale", ResourceID: saleID, Outcome: "success",
		Metadata: map[string]any{"reason": req.Reason},
	}); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.invalidateProductCaches(context.WithoutCancel(ctx), tenantID, productIDs)
	if s.events != nil {
		out := []events.DomainEvent{
			events.SaleCancelledEvent{
				SaleID:         saleID,
				TenantID:       tenantID,
				CancelledByID:  actorUserID,
				AmountReversed: sale.Total,
				At:             time.Now(),
			},
		}
		for _, it := range items {
			// Use current balance map as "after" values.
			bal := balances[it.ProductID]
			out = append(out, events.InventoryCreditedEvent{
				ProductID:   it.ProductID,
				TenantID:    tenantID,
				SaleID:      saleID,
				QtyCredited: it.Qty,
				QtyAfter:    bal.QtyOnHand,
				At:          time.Now(),
			})
		}
		s.events.PublishAll(ctx, out)
	}
	return nil
}

func (s *SalesService) invalidateProductCaches(ctx context.Context, tenantID string, productIDs []string) {
	cache, ok := s.products.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	})
	if !ok {
		return
	}
	for _, productID := range productIDs {
		_ = cache.InvalidateProduct(ctx, tenantID, productID)
	}
	_ = cache.BumpProductsListVersion(ctx, tenantID)
}

func uniqueSortedProductIDsFromSaleItems(items []sales.SaleItem) []string {
	seen := make(map[string]struct{}, len(items))
	ids := make([]string, 0, len(items))
	for _, it := range items {
		pid := it.ProductID
		if pid == "" {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		ids = append(ids, pid)
	}
	sort.Strings(ids)
	return ids
}

func normalizeOptionalCustomerID(value *string) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return common.ErrValidation
	}
	if _, err := uuid.Parse(trimmed); err != nil {
		return common.ErrValidation
	}
	*value = trimmed
	return nil
}
