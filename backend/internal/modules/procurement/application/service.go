package application

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	proc "github.com/example/sistemaemgo/internal/modules/procurement/domain"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
)

type Service struct {
	uow       db.UnitOfWork
	repo      Repository
	products  invapp.ProductsRepository
	inventory invapp.InventoryRepository
	validate  *validator.Validate
	logger    *slog.Logger
}

type SupplierRequest struct {
	Name        string  `json:"name" validate:"required,min=2,max=200"`
	Document    *string `json:"document" validate:"omitempty,max=32"`
	Email       *string `json:"email" validate:"omitempty,email,max=254"`
	Phone       *string `json:"phone" validate:"omitempty,max=32"`
	ContactName *string `json:"contact_name" validate:"omitempty,max=200"`
	Notes       *string `json:"notes" validate:"omitempty,max=1000"`
	Active      bool    `json:"active"`
}

type PurchaseItemRequest struct {
	ProductID string            `json:"product_id" validate:"required"`
	Qty       platform.Quantity `json:"qty" validate:"required,gt=0"`
	UnitCost  platform.Money    `json:"unit_cost" validate:"required,gt=0"`
}

type PurchaseCreateRequest struct {
	SupplierID     string                `json:"supplier_id" validate:"required"`
	InvoiceNumber  *string               `json:"invoice_number" validate:"omitempty,max=64"`
	PaymentDueDate *string               `json:"payment_due_date"`
	Notes          *string               `json:"notes" validate:"omitempty,max=1000"`
	Items          []PurchaseItemRequest `json:"items" validate:"required,min=1,dive"`
}

type PurchaseReceiveItemRequest struct {
	PurchaseItemID string            `json:"purchase_item_id" validate:"required"`
	Qty            platform.Quantity `json:"qty" validate:"required,gt=0"`
}

type PurchaseReceiveRequest struct {
	Items []PurchaseReceiveItemRequest `json:"items" validate:"required,min=1,dive"`
	Notes *string                      `json:"notes" validate:"omitempty,max=1000"`
}

func NewService(uow db.UnitOfWork, repo Repository, products invapp.ProductsRepository, inventory invapp.InventoryRepository, v *validator.Validate, logger *slog.Logger) *Service {
	return &Service{uow: uow, repo: repo, products: products, inventory: inventory, validate: v, logger: logger}
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	v := strings.TrimSpace(*value)
	if v == "" {
		return nil
	}
	return &v
}

func normalizeSupplierRequest(req SupplierRequest) SupplierRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Document = normalizeOptional(req.Document)
	req.Email = normalizeOptional(req.Email)
	req.Phone = normalizeOptional(req.Phone)
	req.ContactName = normalizeOptional(req.ContactName)
	req.Notes = normalizeOptional(req.Notes)
	return req
}

func (s *Service) ListSuppliers(ctx context.Context, tenantID, query string, limit, offset int) ([]proc.Supplier, int, error) {
	return s.repo.ListSuppliers(ctx, tenantID, query, limit, offset)
}

func (s *Service) CreateSupplier(ctx context.Context, tenantID string, req SupplierRequest) (string, error) {
	req = normalizeSupplierRequest(req)
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	id, err := s.repo.CreateSupplier(ctx, tx, tenantID, proc.Supplier{
		Name: req.Name, Document: req.Document, Email: req.Email, Phone: req.Phone,
		ContactName: req.ContactName, Notes: req.Notes, Active: req.Active,
	})
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) UpdateSupplier(ctx context.Context, tenantID, id string, req SupplierRequest) error {
	req = normalizeSupplierRequest(req)
	if err := s.validate.Struct(req); err != nil {
		return common.ErrValidation
	}
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.repo.UpdateSupplier(ctx, tx, tenantID, id, proc.Supplier{
		Name: req.Name, Document: req.Document, Email: req.Email, Phone: req.Phone,
		ContactName: req.ContactName, Notes: req.Notes, Active: req.Active,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ListPurchases(ctx context.Context, tenantID, status string, limit, offset int) ([]proc.Purchase, int, error) {
	switch status {
	case "", string(proc.PurchaseOrdered), string(proc.PurchasePartiallyReceived), string(proc.PurchaseReceived), string(proc.PurchaseCancelled):
	default:
		return nil, 0, common.ErrValidation
	}
	return s.repo.ListPurchases(ctx, tenantID, status, limit, offset)
}

func (s *Service) GetPurchase(ctx context.Context, tenantID, id string) (proc.Purchase, []proc.PurchaseItem, []proc.Receipt, error) {
	return s.repo.GetPurchase(ctx, tenantID, id)
}

func (s *Service) CreatePurchase(ctx context.Context, tenantID, actorUserID string, req PurchaseCreateRequest) (string, error) {
	req.InvoiceNumber = normalizeOptional(req.InvoiceNumber)
	req.Notes = normalizeOptional(req.Notes)
	req.PaymentDueDate = normalizeOptional(req.PaymentDueDate)
	if err := s.validate.Struct(req); err != nil {
		return "", common.ErrValidation
	}
	if req.PaymentDueDate != nil {
		if _, err := time.Parse("2006-01-02", *req.PaymentDueDate); err != nil {
			return "", common.ErrValidation
		}
	}

	productIDs := make([]string, 0, len(req.Items))
	seen := make(map[string]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, exists := seen[item.ProductID]; exists {
			return "", common.ErrValidation
		}
		seen[item.ProductID] = struct{}{}
		productIDs = append(productIDs, item.ProductID)
	}
	sort.Strings(productIDs)

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	supplier, err := s.repo.GetSupplier(ctx, tx, tenantID, req.SupplierID)
	if err != nil || !supplier.Active {
		return "", common.ErrValidation
	}
	products, err := s.products.GetManyByIDs(ctx, tx, tenantID, productIDs)
	if err != nil {
		return "", err
	}
	if len(products) != len(productIDs) {
		return "", common.ErrValidation
	}

	items := make([]proc.PurchaseItem, 0, len(req.Items))
	var total platform.Money
	for _, item := range req.Items {
		if _, ok := products[item.ProductID]; !ok {
			return "", common.ErrValidation
		}
		lineTotal := item.UnitCost.MulQty(item.Qty)
		total = total.Add(lineTotal)
		items = append(items, proc.PurchaseItem{
			ProductID: item.ProductID, QtyOrdered: item.Qty, UnitCost: item.UnitCost, LineTotal: lineTotal,
		})
	}
	purchaseID, err := s.repo.CreatePurchase(ctx, tx, tenantID, proc.Purchase{
		SupplierID: req.SupplierID, Status: proc.PurchaseOrdered, InvoiceNumber: req.InvoiceNumber,
		PaymentDueDate: req.PaymentDueDate, Total: total, Notes: req.Notes, CreatedBy: actorUserID,
	}, items)
	if err != nil {
		return "", err
	}
	if req.PaymentDueDate != nil {
		description := "Compra " + purchaseID + " - " + supplier.Name
		if err := s.repo.CreateAccountPayable(ctx, tx, tenantID, purchaseID, req.SupplierID, description, total, *req.PaymentDueDate); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return purchaseID, nil
}

func (s *Service) ReceivePurchase(ctx context.Context, tenantID, actorUserID, purchaseID string, req PurchaseReceiveRequest) (string, proc.PurchaseStatus, error) {
	req.Notes = normalizeOptional(req.Notes)
	if err := s.validate.Struct(req); err != nil {
		return "", "", common.ErrValidation
	}
	seen := make(map[string]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, ok := seen[item.PurchaseItemID]; ok {
			return "", "", common.ErrValidation
		}
		seen[item.PurchaseItemID] = struct{}{}
	}

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	purchase, items, err := s.repo.GetPurchaseForUpdate(ctx, tx, tenantID, purchaseID)
	if err != nil {
		return "", "", common.ErrNotFound
	}
	if purchase.Status == proc.PurchaseCancelled || purchase.Status == proc.PurchaseReceived {
		return "", "", common.ErrConflict
	}

	itemByID := make(map[string]proc.PurchaseItem, len(items))
	productIDs := make([]string, 0, len(req.Items))
	for _, item := range items {
		itemByID[item.ID] = item
	}
	for _, requested := range req.Items {
		item, ok := itemByID[requested.PurchaseItemID]
		if !ok {
			return "", "", common.ErrValidation
		}
		remaining := item.QtyOrdered - item.QtyReceived
		if requested.Qty <= 0 || requested.Qty > remaining {
			return "", "", common.ErrValidation
		}
		productIDs = append(productIDs, item.ProductID)
	}
	sort.Strings(productIDs)

	if batch, ok := s.inventory.(interface {
		EnsureBalanceRows(context.Context, db.DBTX, string, []string) error
		GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
	}); ok {
		if err := batch.EnsureBalanceRows(ctx, tx, tenantID, productIDs); err != nil {
			return "", "", err
		}
	} else {
		for _, productID := range productIDs {
			if err := s.inventory.EnsureBalanceRow(ctx, tx, tenantID, productID); err != nil {
				return "", "", err
			}
		}
	}

	balances := make(map[string]inv.InventoryBalance, len(productIDs))
	if batch, ok := s.inventory.(interface {
		GetBalancesForUpdate(context.Context, db.DBTX, string, []string) (map[string]inv.InventoryBalance, error)
	}); ok {
		balances, err = batch.GetBalancesForUpdate(ctx, tx, tenantID, productIDs)
		if err != nil {
			return "", "", err
		}
	} else {
		for _, productID := range productIDs {
			bal, err := s.inventory.GetBalanceForUpdate(ctx, tx, tenantID, productID)
			if err != nil {
				return "", "", err
			}
			balances[productID] = bal
		}
	}

	receiptID, err := s.repo.CreateReceipt(ctx, tx, tenantID, purchaseID, actorUserID, req.Notes)
	if err != nil {
		return "", "", err
	}

	updatedReceived := make(map[string]platform.Quantity, len(items))
	for _, item := range items {
		updatedReceived[item.ID] = item.QtyReceived
	}
	for _, requested := range req.Items {
		item := itemByID[requested.PurchaseItemID]
		bal, ok := balances[item.ProductID]
		if !ok {
			return "", "", common.ErrValidation
		}
		after, derr := bal.Creditar(requested.Qty)
		if derr != nil {
			return "", "", common.ErrValidation
		}
		if err := s.inventory.UpdateBalance(ctx, tx, tenantID, item.ProductID, after.QtyOnHand); err != nil {
			return "", "", err
		}
		balances[item.ProductID] = after

		newReceived := item.QtyReceived + requested.Qty
		updatedReceived[item.ID] = newReceived
		if err := s.repo.UpdateItemReceived(ctx, tx, tenantID, item.ID, newReceived); err != nil {
			return "", "", err
		}
		if err := s.repo.UpdateProductCost(ctx, tx, tenantID, item.ProductID, item.UnitCost); err != nil {
			return "", "", err
		}
		if err := s.repo.InsertReceiptItem(ctx, tx, tenantID, proc.ReceiptItem{
			ReceiptID: receiptID, PurchaseItemID: item.ID, ProductID: item.ProductID,
			Qty: requested.Qty, UnitCost: item.UnitCost,
		}); err != nil {
			return "", "", err
		}
		reason := "Recebimento de compra"
		refType := "purchase_receipt"
		refID := receiptID
		actor := actorUserID
		mv := inv.NewMovement(item.ProductID, inv.MovementPurchase, requested.Qty, bal, after, &reason, &refType, &refID, &actor, time.Now().Format(time.RFC3339))
		if err := s.inventory.InsertMovement(ctx, tx, tenantID, mv); err != nil {
			return "", "", err
		}
	}

	status := proc.PurchaseReceived
	for _, item := range items {
		if updatedReceived[item.ID] < item.QtyOrdered {
			status = proc.PurchasePartiallyReceived
			break
		}
	}
	var receivedAt *string
	if status == proc.PurchaseReceived {
		now := time.Now().UTC().Format(time.RFC3339)
		receivedAt = &now
	}
	if err := s.repo.UpdatePurchaseStatus(ctx, tx, tenantID, purchaseID, status, receivedAt); err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	s.invalidateProductCaches(context.WithoutCancel(ctx), tenantID, productIDs)
	return receiptID, status, nil
}

func (s *Service) CancelPurchase(ctx context.Context, tenantID, purchaseID string) error {
	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	purchase, items, err := s.repo.GetPurchaseForUpdate(ctx, tx, tenantID, purchaseID)
	if err != nil {
		return common.ErrNotFound
	}
	if purchase.Status == proc.PurchaseCancelled {
		return common.ErrConflict
	}
	for _, item := range items {
		if item.QtyReceived > 0 {
			return common.ErrConflict
		}
	}
	if err := s.repo.CancelPurchase(ctx, tx, tenantID, purchaseID); err != nil {
		return err
	}
	if err := s.repo.CancelAccountPayable(ctx, tx, tenantID, purchaseID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) invalidateProductCaches(ctx context.Context, tenantID string, productIDs []string) {
	cache, ok := s.products.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	})
	if !ok {
		return
	}
	seen := map[string]struct{}{}
	for _, productID := range productIDs {
		if _, exists := seen[productID]; exists {
			continue
		}
		seen[productID] = struct{}{}
		_ = cache.InvalidateProduct(ctx, tenantID, productID)
	}
	_ = cache.BumpProductsListVersion(ctx, tenantID)
}
