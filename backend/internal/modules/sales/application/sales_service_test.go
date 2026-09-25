package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/common"
	fin "github.com/example/sistemaemgo/internal/modules/finance/domain"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateAndFinalizeRequiresIdempotencyKey(t *testing.T) {
	svc, _, _, _ := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(1_000)}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(1000)}},
	}

	_, _, _, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "   ", req)
	if !errors.Is(err, common.ErrValidation) {
		t.Fatalf("want ErrValidation for blank idempotency key, got %v", err)
	}
}

func TestCreateAndFinalizeIgnoresClientTamperedUnitPrice(t *testing.T) {
	svc, salesRepo, _, _ := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	tampered := platform.NewMoneyCents(1)
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(2_000), UnitPrice: &tampered}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(2000)}},
	}

	_, total, created, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "idem-1", req)
	if err != nil {
		t.Fatalf("CreateAndFinalize: %v", err)
	}
	if !created {
		t.Fatalf("expected created sale")
	}
	if total.Cents() != 2000 {
		t.Fatalf("want total 2000 cents, got %d", total.Cents())
	}
	if got := salesRepo.insertedItems[0].UnitPrice.Cents(); got != 1000 {
		t.Fatalf("backend must use product price 1000 cents, got %d", got)
	}
}

func TestCreateAndFinalizeNormalSaleCreationAndTotal(t *testing.T) {
	svc, salesRepo, invRepo, _ := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		DiscountValue: platform.NewMoneyCents(100),
		Items: []SaleItemRequest{
			{ProductID: "prod-1", Qty: platform.NewQuantityMilli(2_000), DiscountValue: platform.NewMoneyCents(50)},
		},
		Payments: []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(1850)}},
	}

	_, total, created, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "normal-key", req)
	if err != nil {
		t.Fatalf("CreateAndFinalize: %v", err)
	}
	if !created {
		t.Fatalf("expected created sale")
	}
	if total.Cents() != 1850 {
		t.Fatalf("want total 1850 cents, got %d", total.Cents())
	}
	if got := salesRepo.insertedSales[0].Subtotal.Cents(); got != 2000 {
		t.Fatalf("want subtotal 2000 cents, got %d", got)
	}
	if got := invRepo.stock.Milli(); got != 8_000 {
		t.Fatalf("want stock 8000 milli after sale, got %d", got)
	}
}

func TestCreateAndFinalizeInsufficientStock(t *testing.T) {
	svc, _, _, _ := newSalesServiceFixture(platform.NewQuantityMilli(1_000))
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(2_000)}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(2000)}},
	}

	_, _, _, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "idem-1", req)
	if !errors.Is(err, common.ErrInsufficientStock) {
		t.Fatalf("want ErrInsufficientStock, got %v", err)
	}
}

func TestCreateAndFinalizeIdempotencyReplayAndConflict(t *testing.T) {
	svc, _, _, _ := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(1_000)}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(1000)}},
	}

	_, _, created, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "same-key", req)
	if err != nil || !created {
		t.Fatalf("first call err=%v created=%v", err, created)
	}
	_, _, created, err = svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "same-key", req)
	if err != nil {
		t.Fatalf("replay err=%v", err)
	}
	if created {
		t.Fatalf("replay should not create a second sale")
	}

	req.Items[0].Qty = platform.NewQuantityMilli(2_000)
	req.Payments[0].Amount = platform.NewMoneyCents(2000)
	_, _, _, err = svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "same-key", req)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestCreateAndFinalizeRejectsCustomerOutsideTenant(t *testing.T) {
	svc, salesRepo, _, _ := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	allowed := false
	salesRepo.customerAllowed = &allowed
	customerID := "11111111-1111-1111-1111-111111111111"
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		CustomerID:    &customerID,
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(1_000)}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(1000)}},
	}

	_, _, _, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "customer-tenant-key", req)
	if !errors.Is(err, common.ErrValidation) {
		t.Fatalf("want ErrValidation for customer outside tenant, got %v", err)
	}
	if salesRepo.insertSaleCount != 0 {
		t.Fatalf("cross-tenant customer must be rejected before inserting sale")
	}
}

func TestCreateAndFinalizeUsesPromotionalPrice(t *testing.T) {
	svc, salesRepo, _, productsRepo := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	promo := platform.NewMoneyCents(750)
	p := productsRepo.products["prod-1"]
	p.PromoPrice = &promo
	productsRepo.products["prod-1"] = p
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(2_000)}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(1500)}},
	}

	_, total, _, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "promo-key", req)
	if err != nil {
		t.Fatalf("CreateAndFinalize: %v", err)
	}
	if total.Cents() != 1500 {
		t.Fatalf("want promo total 1500 cents, got %d", total.Cents())
	}
	if got := salesRepo.insertedItems[0].UnitPrice.Cents(); got != 750 {
		t.Fatalf("want promo unit price 750 cents, got %d", got)
	}
}

func TestCreateAndFinalizeConcurrentDuplicateRequests(t *testing.T) {
	svc, salesRepo, _, _ := newSalesServiceFixture(platform.NewQuantityMilli(10_000))
	req := SaleCreateRequest{
		CashSessionID: "cash-1",
		Items:         []SaleItemRequest{{ProductID: "prod-1", Qty: platform.NewQuantityMilli(1_000)}},
		Payments:      []SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(1000)}},
	}

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	created := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, c, err := svc.CreateAndFinalize(context.Background(), "tenant-1", "user-1", "concurrent-key", req)
			errs <- err
			created <- c
		}()
	}
	wg.Wait()
	close(errs)
	close(created)

	var createdCount int
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent duplicate returned error: %v", err)
		}
	}
	for c := range created {
		if c {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("want exactly one created request, got %d", createdCount)
	}
	if salesRepo.insertSaleCount != 1 {
		t.Fatalf("want one inserted sale, got %d", salesRepo.insertSaleCount)
	}
}

func newSalesServiceFixture(stock platform.Quantity) (*SalesService, *fakeSalesRepo, *fakeInventoryRepo, *fakeProductsRepo) {
	salesRepo := &fakeSalesRepo{results: map[string]idemResult{}}
	fakeTxUnlock = salesRepo.mu.Unlock
	invRepo := &fakeInventoryRepo{stock: stock}
	productsRepo := &fakeProductsRepo{products: map[string]inv.Product{
		"prod-1": {ID: "prod-1", Active: true, PriceCash: platform.NewMoneyCents(1000), CostPrice: platform.NewMoneyCents(600)},
	}}
	svc := NewSalesService(config.Config{}, fakeUOW{}, salesRepo, invRepo, fakeFinanceRepo{}, fakeCashRepo{}, productsRepo, nil, nil, validator.New(), nil)
	return svc, salesRepo, invRepo, productsRepo
}

type fakeUOW struct{}

var fakeTxUnlock func()

func (fakeUOW) Begin(context.Context) (db.Tx, error) {
	return &fakeTx{unlock: fakeTxUnlock}, nil
}

type fakeTx struct {
	once   sync.Once
	unlock func()
}

func (*fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (*fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (*fakeTx) QueryRow(context.Context, string, ...any) pgx.Row        { return fakeRow{} }
func (t *fakeTx) Commit(context.Context) error {
	t.once.Do(func() {
		if t.unlock != nil {
			t.unlock()
		}
	})
	return nil
}
func (t *fakeTx) Rollback(context.Context) error {
	t.once.Do(func() {
		if t.unlock != nil {
			t.unlock()
		}
	})
	return nil
}

type fakeRow struct{}

func (fakeRow) Scan(...any) error { return pgx.ErrNoRows }

type idemResult struct {
	saleID      string
	total       platform.Money
	requestHash string
}

type fakeSalesRepo struct {
	mu              sync.Mutex
	insertedItems   []sales.SaleItem
	insertedSales   []sales.Sale
	insertSaleCount int
	results         map[string]idemResult
	customerAllowed *bool
}

func (r *fakeSalesRepo) CustomerBelongsToTenant(context.Context, db.DBTX, string, string) (bool, error) {
	if r.customerAllowed != nil {
		return *r.customerAllowed, nil
	}
	return true, nil
}
func (r *fakeSalesRepo) InsertSale(_ context.Context, _ db.DBTX, _ string, sale sales.Sale) (string, error) {
	r.insertedSales = append(r.insertedSales, sale)
	r.insertSaleCount++
	return "sale-1", nil
}
func (r *fakeSalesRepo) InsertItem(_ context.Context, _ db.DBTX, _ string, it sales.SaleItem) error {
	r.insertedItems = append(r.insertedItems, it)
	return nil
}
func (r *fakeSalesRepo) InsertPayment(context.Context, db.DBTX, string, sales.Payment) error {
	return nil
}
func (r *fakeSalesRepo) GetSale(context.Context, string, string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	return sales.Sale{}, nil, nil, nil
}
func (r *fakeSalesRepo) ListSales(context.Context, string, int, int) ([]sales.Sale, int, error) {
	return nil, 0, nil
}
func (r *fakeSalesRepo) CancelSale(context.Context, db.DBTX, string, string, string) error {
	return nil
}
func (r *fakeSalesRepo) GetSaleForUpdate(context.Context, db.DBTX, string, string) (sales.Sale, []sales.SaleItem, []sales.Payment, error) {
	return sales.Sale{}, nil, nil, nil
}
func (r *fakeSalesRepo) HasInvoiceForSale(context.Context, db.DBTX, string, string) (bool, error) {
	return false, nil
}
func (r *fakeSalesRepo) LockIdempotencyKey(context.Context, db.DBTX, string, string, string) error {
	r.mu.Lock()
	return nil
}
func (r *fakeSalesRepo) GetIdempotencyResult(_ context.Context, _ db.DBTX, _, operation, key string) (string, platform.Money, string, bool, error) {
	res, ok := r.results[operation+":"+key]
	return res.saleID, res.total, res.requestHash, ok, nil
}
func (r *fakeSalesRepo) SaveIdempotencyResult(_ context.Context, _ db.DBTX, _, operation, key, requestHash, saleID string, total platform.Money) error {
	if _, exists := r.results[operation+":"+key]; exists {
		return common.ErrConflict
	}
	r.results[operation+":"+key] = idemResult{saleID: saleID, total: total, requestHash: requestHash}
	return nil
}

type fakeInventoryRepo struct {
	stock platform.Quantity
}

func (r *fakeInventoryRepo) EnsureBalanceRow(context.Context, db.DBTX, string, string) error {
	return nil
}
func (r *fakeInventoryRepo) EnsureBalanceRows(context.Context, db.DBTX, string, []string) error {
	return nil
}
func (r *fakeInventoryRepo) GetBalanceForUpdate(context.Context, db.DBTX, string, string) (inv.InventoryBalance, error) {
	return inv.InventoryBalance{ProductID: "prod-1", QtyOnHand: r.stock}, nil
}
func (r *fakeInventoryRepo) GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error) {
	return map[string]inv.InventoryBalance{"prod-1": {ProductID: "prod-1", QtyOnHand: r.stock}}, nil
}
func (r *fakeInventoryRepo) UpdateBalance(_ context.Context, _ db.DBTX, _, _ string, qty platform.Quantity) error {
	r.stock = qty
	return nil
}
func (r *fakeInventoryRepo) InsertMovement(context.Context, db.DBTX, string, inv.InventoryMovement) error {
	return nil
}

type fakeProductsRepo struct {
	products map[string]inv.Product
}

func (r *fakeProductsRepo) GetManyByIDs(context.Context, db.DBTX, string, []string) (map[string]inv.Product, error) {
	return r.products, nil
}

type fakeCashRepo struct{}

func (fakeCashRepo) EnsureDefaultRegister(context.Context, string) (string, error) {
	return "reg-1", nil
}
func (fakeCashRepo) OpenSession(context.Context, db.DBTX, string, string, string, platform.Money, *string) (string, error) {
	return "cash-1", nil
}
func (fakeCashRepo) GetOpenSession(context.Context, string) (sales.CashSession, bool, error) {
	return sales.CashSession{ID: "cash-1", Status: "open"}, true, nil
}
func (fakeCashRepo) ListOpenSessionsByUser(context.Context, string) ([]sales.CashSession, error) {
	return nil, nil
}
func (fakeCashRepo) CloseSession(context.Context, db.DBTX, string, string, string, platform.Money, platform.Money, *string) error {
	return nil
}
func (fakeCashRepo) GetSession(context.Context, db.DBTX, string, string) (sales.CashSession, error) {
	return sales.CashSession{ID: "cash-1", Status: "open", OpeningAmount: 0}, nil
}
func (fakeCashRepo) InsertMovement(context.Context, db.DBTX, string, string, string, string, platform.Money, *string) (string, error) {
	return "movement-1", nil
}
func (fakeCashRepo) SumPaymentsByMethod(context.Context, db.DBTX, string, string) (map[string]platform.Money, error) {
	return map[string]platform.Money{}, nil
}
func (fakeCashRepo) SumMovements(context.Context, db.DBTX, string, string) (platform.Money, platform.Money, error) {
	return 0, 0, nil
}
func (fakeCashRepo) SaveReconciliation(context.Context, db.DBTX, string, string, map[string]platform.Money, map[string]platform.Money) error {
	return nil
}

type fakeFinanceRepo struct{}

func (fakeFinanceRepo) InsertLedgerEntry(context.Context, db.DBTX, string, fin.LedgerEntry, *string) (string, error) {
	return "ledger-1", nil
}
