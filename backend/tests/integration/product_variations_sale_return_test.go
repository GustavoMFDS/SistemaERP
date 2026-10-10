//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi/handlers"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	procapp "github.com/example/sistemaemgo/internal/modules/procurement/application"
	procinfra "github.com/example/sistemaemgo/internal/modules/procurement/infrastructure"
	retapp "github.com/example/sistemaemgo/internal/modules/returns/application"
	ret "github.com/example/sistemaemgo/internal/modules/returns/domain"
	retinfra "github.com/example/sistemaemgo/internal/modules/returns/infrastructure"
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Exercises actual sales and returns services against PostgreSQL. No SEFAZ
// calls or live payment provider are involved. The tenant is uniquely owned
// by this test; cleanup never touches real/seeded tenant records.
func TestProductVariationSaleReturnIsolatesEveryBalance(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	// Later t.Cleanup handlers must run before this pool is closed.
	t.Cleanup(pool.Close)

	var actor, role string
	if err := pool.QueryRow(ctx, `
        SELECT utr.user_id::text, utr.role_id::text
        FROM user_tenant_roles utr JOIN roles r ON r.id=utr.role_id
        WHERE r.name='admin' ORDER BY utr.created_at LIMIT 1
    `).Scan(&actor, &role); err != nil {
		t.Fatalf("seeded admin: %v", err)
	}

	cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)
	var tenant string
	if err := pool.QueryRow(ctx, `
        INSERT INTO companies(legal_name,trade_name,cnpj)
        VALUES('Variant Sale Return Test','Variant Sale Return Test',$1)
        RETURNING id::text
    `, cnpj).Scan(&tenant); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 12*time.Second)
		defer stop()
		// The company is test-only and uniquely created above.
		for _, query := range []string{
			"DELETE FROM audit_logs WHERE tenant_id=$1",
			"DELETE FROM sale_return_items WHERE tenant_id=$1",
			"DELETE FROM return_idempotency_keys WHERE tenant_id=$1",
			"DELETE FROM sale_returns WHERE tenant_id=$1",
			"DELETE FROM idempotency_keys WHERE tenant_id=$1",
			"DELETE FROM payments WHERE tenant_id=$1",
			"DELETE FROM ledger_entries WHERE tenant_id=$1",
			"DELETE FROM sale_items WHERE tenant_id=$1",
			"DELETE FROM invoice_xml_files WHERE tenant_id=$1",
			"DELETE FROM invoices WHERE tenant_id=$1",
			"DELETE FROM sales WHERE tenant_id=$1",
			"DELETE FROM inventory_movements WHERE tenant_id=$1",
			"DELETE FROM purchase_receipt_items WHERE tenant_id=$1",
			"DELETE FROM purchase_receipts WHERE tenant_id=$1",
			"DELETE FROM accounts_payable WHERE tenant_id=$1",
			"DELETE FROM procurement_idempotency_keys WHERE tenant_id=$1",
			"DELETE FROM purchase_items WHERE tenant_id=$1",
			"DELETE FROM purchases WHERE tenant_id=$1",
			"DELETE FROM suppliers WHERE tenant_id=$1",
			"DELETE FROM product_variations WHERE tenant_id=$1",
			"DELETE FROM products WHERE tenant_id=$1",
			"DELETE FROM cash_sessions WHERE tenant_id=$1",
			"DELETE FROM cash_registers WHERE tenant_id=$1",
			"DELETE FROM user_tenant_roles WHERE tenant_id=$1",
			"DELETE FROM user_tenants WHERE tenant_id=$1",
			"DELETE FROM companies WHERE id=$1",
		} {
			if _, err := pool.Exec(cleanupCtx, query, tenant); err != nil {
				t.Errorf("isolated fixture cleanup failed (%s): %v", query, err)
			}
		}
	})

	if _, err := pool.Exec(ctx, `
        INSERT INTO user_tenants(tenant_id,user_id) VALUES($1,$2)
    `, tenant, actor); err != nil {
		t.Fatalf("associate actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO user_tenant_roles(tenant_id,user_id,role_id) VALUES($1,$2,$3)
    `, tenant, actor, role); err != nil {
		t.Fatalf("authorize actor: %v", err)
	}

	var register, session string
	if err := pool.QueryRow(ctx, `
        INSERT INTO cash_registers(tenant_id,name,active)
        VALUES($1,'Variant E2E Cash Register',true) RETURNING id::text
    `, tenant).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
        INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,opening_amount,status)
        VALUES($1,$2,$3,0,'open') RETURNING id::text
    `, tenant, register, actor).Scan(&session); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	v := validator.New()
	uow := db.NewPgxUnitOfWork(pool)
	aud := audit.New(pool, logger)
	// Exercise the same Redis-backed repository decorator as the real API.
	baseProducts := invinfra.NewProductsRepo(pool)
	var products invapp.ProductsRepository = baseProducts
	if addr := os.Getenv("TEST_REDIS_ADDR"); addr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: addr})
		t.Cleanup(func() { _ = rdb.Close() })
		if err := rdb.Ping(ctx).Err(); err != nil {
			t.Fatalf("test Redis unavailable: %v", err)
		}
		products = invinfra.NewCachedProductsRepo(baseProducts, rdb)
	}
	inventory := invinfra.NewInventoryRepo(pool)
	prodSvc := invapp.NewProductsService(uow, products, aud, v, logger)
	saleSvc := salesapp.NewSalesService(config.Config{AllowNegativeStock: false}, uow,
		salesinfra.NewSalesRepo(pool), inventory, fininfra.NewFinanceRepo(pool),
		salesinfra.NewCashRepo(pool), products, aud, nil, v, logger)
	returnSvc := retapp.NewService(uow, retinfra.NewRepo(pool),
		inventory, products, aud, v, logger)
	stockSvc := invapp.NewInventoryService(config.Config{AllowNegativeStock: false},
		uow, inventory, products, aud, v, logger)
	procSvc := procapp.NewService(uow, procinfra.NewRepo(pool),
		products, inventory, aud, v, logger)

	sku := uuid.NewString()
	baseProduct := invapp.ProductCreateRequest{
		SKU: "FAM-" + sku, Name: "Caderno de teste", Unit: "un",
		PriceCash: platform.NewMoneyCents(1290), Active: true,
	}
	base, err := prodSvc.Create(ctx, tenant, actor, baseProduct)
	if err != nil {
		t.Fatal(err)
	}
	blueProduct := baseProduct
	blueProduct.SKU = "FAM-BLUE-" + sku
	blueProduct.Name = "Caderno Azul"
	pinkProduct := baseProduct
	pinkProduct.SKU = "FAM-PINK-" + sku
	pinkProduct.Name = "Caderno Rosa"
	blue, err := prodSvc.CreateVariation(ctx, tenant, actor, base, "Azul", blueProduct)
	if err != nil {
		t.Fatal(err)
	}
	pink, err := prodSvc.CreateVariation(ctx, tenant, actor, base, "Rosa", pinkProduct)
	if err != nil {
		t.Fatal(err)
	}

	for _, pair := range []struct {
		id  string
		qty int
	}{{base, 10}, {blue, 5}, {pink, 3}} {
		if _, err := pool.Exec(ctx, `
            UPDATE inventory_balances SET qty_on_hand=$3 WHERE tenant_id=$1 AND product_id=$2
        `, tenant, pair.id, pair.qty); err != nil {
			t.Fatal(err)
		}
	}
	// Populate product Get cache with the pre-sale stock. A sale must
	// invalidate the exact variant rather than leave an outdated balance.
	if cached, err := products.Get(ctx, tenant, blue); err != nil || cached.QtyOnHand != platform.NewQuantityMilli(5000) {
		t.Fatalf("before sale cached stock invalid: %+v %v", cached, err)
	}
	check := func(wantBase, wantBlue, wantPink int) {
		t.Helper()
		for _, pair := range []struct {
			id   string
			want int
		}{{base, wantBase}, {blue, wantBlue}, {pink, wantPink}} {
			var qty string
			if err := pool.QueryRow(ctx, `
                SELECT qty_on_hand::text FROM inventory_balances WHERE tenant_id=$1 AND product_id=$2
            `, tenant, pair.id).Scan(&qty); err != nil {
				t.Fatal(err)
			}
			expected, err := platform.ParseQuantity(fmt.Sprint(pair.want))
			if err != nil {
				t.Fatal(err)
			}
			got, err := platform.ParseQuantity(qty)
			if err != nil || got != expected {
				t.Fatalf("product %s stock %q, want %d; err=%v", pair.id, qty, pair.want, err)
			}
		}
	}

	key := uuid.NewString()
	saleReq := salesapp.SaleCreateRequest{
		CashSessionID: session,
		Items: []salesapp.SaleItemRequest{{
			ProductID: blue, Qty: platform.NewQuantityMilli(2000),
		}},
		Payments: []salesapp.SalePaymentRequest{{
			Method: "cash", Amount: platform.NewMoneyCents(2580),
		}},
	}
	saleID, amount, created, err := saleSvc.CreateAndFinalize(ctx, tenant, actor, key, saleReq)
	if err != nil || !created || amount != platform.NewMoneyCents(2580) {
		t.Fatalf("sell blue only: id=%s amount=%v created=%v err=%v", saleID, amount, created, err)
	}
	// Fiscal document issuance is independent of paper printing: PostgreSQL
	// inserts the durable obligation in the SAME transaction as the sale.
	// A pending obligation is not a SEFAZ-authorized note.
	var fiscalKind string
	var fiscalLegacy bool
	if err := pool.QueryRow(ctx, `
		SELECT document_kind, legacy_review
		FROM sale_fiscal_intents WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, saleID).Scan(&fiscalKind, &fiscalLegacy); err != nil {
		t.Fatalf("sale committed without fiscal intent: %v", err)
	}
	if fiscalKind != "nfce" || fiscalLegacy {
		t.Fatalf("wrong fiscal obligation: kind=%s legacy=%v", fiscalKind, fiscalLegacy)
	}
	// The manager's operational queue must include unissued sales, with no
	// cross-CNPJ leakage, even when there is no invoice row to join.
	obligations := handlers.NewSaleFiscalHandler(pool, nil, nil)
	pending, err := obligations.QueryObligations(ctx, tenant, 20, 0, true)
	if err != nil || pending.Total != 1 || len(pending.Items) != 1 {
		t.Fatalf("unissued fiscal queue: total=%d items=%+v err=%v", pending.Total, pending.Items, err)
	}
	if pending.Items[0].SaleID != saleID || pending.Items[0].Status != "pending" ||
		pending.Items[0].Authorized || pending.Items[0].LegacyReview {
		t.Fatalf("wrong queue state: %+v", pending.Items[0])
	}
	allObligations, err := obligations.QueryObligations(ctx, tenant, 20, 0, false)
	if err != nil || allObligations.Total != 1 {
		t.Fatalf("all fiscal obligations: %+v err=%v", allObligations, err)
	}
	otherTenant, err := obligations.QueryObligations(ctx, uuid.NewString(), 20, 0, true)
	if err != nil || otherTenant.Total != 0 || len(otherTenant.Items) != 0 {
		t.Fatalf("cross-tenant fiscal queue: %+v err=%v", otherTenant, err)
	}
	check(10, 3, 3)
	if cached, err := products.Get(ctx, tenant, blue); err != nil || cached.QtyOnHand != platform.NewQuantityMilli(3000) {
		t.Fatalf("sale cache invalidation failed: %+v %v", cached, err)
	}
	replayID, replayTotal, replayCreated, err := saleSvc.CreateAndFinalize(ctx, tenant, actor, key, saleReq)
	if err != nil || replayID != saleID || replayCreated || replayTotal != amount {
		t.Fatalf("sale idempotency: id=%s created=%v err=%v", replayID, replayCreated, err)
	}
	check(10, 3, 3)

	failReq := saleReq
	failReq.Items = []salesapp.SaleItemRequest{{ProductID: blue, Qty: platform.NewQuantityMilli(100000)}}
	failReq.Payments = []salesapp.SalePaymentRequest{{Method: "cash", Amount: platform.NewMoneyCents(129000)}}
	_, _, _, err = saleSvc.CreateAndFinalize(ctx, tenant, actor, uuid.NewString(), failReq)
	if !errors.Is(err, common.ErrInsufficientStock) {
		t.Fatalf("oversell did not fail closed: %v", err)
	}
	check(10, 3, 3)

	_, saleItems, _, err := saleSvc.Get(ctx, tenant, saleID)
	if err != nil || len(saleItems) != 1 || saleItems[0].ProductID != blue {
		t.Fatalf("wrong sale item association: %+v %v", saleItems, err)
	}
	returnKey := uuid.NewString()
	returnReq := retapp.CreateRequest{
		Kind: ret.Kind("return"), Reason: "Devolução de teste", Items: []retapp.ItemRequest{{
			SaleItemID: saleItems[0].ID, Qty: platform.NewQuantityMilli(1000), Restock: true,
		}},
	}
	returnID, refund, returnCreated, err := returnSvc.Create(ctx, tenant, actor, saleID, returnKey, returnReq)
	if err != nil || !returnCreated || refund != platform.NewMoneyCents(1290) {
		t.Fatalf("restock one blue: id=%s refund=%v created=%v err=%v", returnID, refund, returnCreated, err)
	}
	check(10, 4, 3)
	if cached, err := products.Get(ctx, tenant, blue); err != nil || cached.QtyOnHand != platform.NewQuantityMilli(4000) {
		t.Fatalf("return cache invalidation failed: %+v %v", cached, err)
	}
	repeatedID, repeatedRefund, repeatedCreated, err := returnSvc.Create(ctx, tenant, actor, saleID, returnKey, returnReq)
	if err != nil || repeatedCreated || repeatedID != returnID || repeatedRefund != refund {
		t.Fatalf("return idempotency failed: %v %v %v", repeatedID, repeatedCreated, err)
	}
	check(10, 4, 3)
	returnReq.Items[0].Qty = platform.NewQuantityMilli(2000)
	_, _, _, err = returnSvc.Create(ctx, tenant, actor, saleID, uuid.NewString(), returnReq)
	if !errors.Is(err, common.ErrValidation) {
		t.Fatalf("overreturn did not fail closed: %v", err)
	}
	check(10, 4, 3)

	// A counted adjustment must touch only the selected variant.
	if err := stockSvc.Adjust(ctx, tenant, actor, invapp.InventoryAdjustRequest{
		ProductID: pink, Delta: platform.NewQuantityMilli(1000),
		Reason: "Contagem positiva de teste", Type: "adjustment",
	}); err != nil {
		t.Fatalf("increase pink only: %v", err)
	}
	check(10, 4, 4)
	if err := stockSvc.Adjust(ctx, tenant, actor, invapp.InventoryAdjustRequest{
		ProductID: pink, Delta: platform.NewQuantityMilli(-1000),
		Reason: "Contagem negativa de teste", Type: "adjustment",
	}); err != nil {
		t.Fatalf("decrease pink only: %v", err)
	}
	check(10, 4, 3)
	if err := stockSvc.Adjust(ctx, tenant, actor, invapp.InventoryAdjustRequest{
		ProductID: pink, Delta: platform.NewQuantityMilli(-100000),
		Reason: "Saldo insuficiente teste", Type: "adjustment",
	}); err == nil {
		t.Fatal("invalid negative adjustment unexpectedly accepted")
	}
	check(10, 4, 3)

	// An actual supplier purchase/receipt must replenish the purchased SKU,
	// never the parent nor another color. Same receipt key must be idempotent.
	supplierID, supplierCreated, err := procSvc.CreateSupplier(ctx, tenant, actor,
		uuid.NewString(), procapp.SupplierRequest{
			Name: "Fornecedor somente teste " + sku, Active: true,
		})
	if err != nil || !supplierCreated {
		t.Fatalf("create supplier: %v", err)
	}
	purchaseID, purchaseCreated, err := procSvc.CreatePurchase(ctx, tenant, actor,
		uuid.NewString(), procapp.PurchaseCreateRequest{
			SupplierID: supplierID,
			Items: []procapp.PurchaseItemRequest{{
				ProductID: pink, Qty: platform.NewQuantityMilli(2000),
				UnitCost: platform.NewMoneyCents(600),
			}},
		})
	if err != nil || !purchaseCreated {
		t.Fatalf("purchase pink: %v", err)
	}
	_, purchaseItems, _, err := procSvc.GetPurchase(ctx, tenant, purchaseID)
	if err != nil || len(purchaseItems) != 1 || purchaseItems[0].ProductID != pink {
		t.Fatalf("purchase item association incorrect: %+v err=%v", purchaseItems, err)
	}
	receiptKey := uuid.NewString()
	receiptReq := procapp.PurchaseReceiveRequest{Items: []procapp.PurchaseReceiveItemRequest{{
		PurchaseItemID: purchaseItems[0].ID, Qty: platform.NewQuantityMilli(2000),
	}}}
	receiptID, receiptStatus, receiptCreated, err := procSvc.ReceivePurchase(
		ctx, tenant, actor, purchaseID, receiptKey, receiptReq)
	if err != nil || !receiptCreated {
		t.Fatalf("receive pink: %s %v", receiptStatus, err)
	}
	check(10, 4, 5)
	replayedReceiptID, _, replayedReceiptCreated, err := procSvc.ReceivePurchase(
		ctx, tenant, actor, purchaseID, receiptKey, receiptReq)
	if err != nil || replayedReceiptCreated || replayedReceiptID != receiptID {
		t.Fatalf("receipt retry duplicated or failed: %s %v", replayedReceiptID, err)
	}
	check(10, 4, 5)

	// Preparing an XML row does not authorize the NF-e/NFC-e. The manager's
	// queue must preserve a visible unresolved obligation even after a fiscal
	// record was created, rather than silently marking it as issued.
	if _, err := pool.Exec(ctx, `
		INSERT INTO invoices(tenant_id, sale_id, company_id, status)
		VALUES($1, $2, $1, 'xml_generated')
	`, tenant, saleID); err != nil {
		t.Fatalf("create test-only XML invoice placeholder: %v", err)
	}
	stillPending, err := obligations.QueryObligations(ctx, tenant, 20, 0, true)
	if err != nil || stillPending.Total != 1 || len(stillPending.Items) != 1 {
		t.Fatalf("XML alone dropped fiscal obligation: %+v err=%v", stillPending, err)
	}
	if stillPending.Items[0].Status != "xml_generated" ||
		stillPending.Items[0].Authorized {
		t.Fatalf("XML incorrectly treated as authorized: %+v", stillPending.Items[0])
	}
}
