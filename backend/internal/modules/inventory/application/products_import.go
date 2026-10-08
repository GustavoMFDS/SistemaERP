package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
)

type ProductImportRequest struct {
	Items []ProductCreateRequest `json:"items" validate:"required,min=1,max=500,dive"`
}

type ProductImportResult struct {
	BatchID  string `json:"batch_id"`
	ItemCount int    `json:"item_count"`
	Replayed bool   `json:"replayed"`
}

// A committed receipt is returned without disclosing the CSV or product IDs.
func (s *ProductsService) LookupProductImportBatch(
	ctx context.Context, tenantID, key string,
) (ProductImportResult, bool, error) {
	if len(key) < 8 || len(key) > 128 || strings.TrimSpace(key) != key {
		return ProductImportResult{}, false, common.ErrValidation
	}
	id, count, found, err := s.repo.LookupProductImportBatch(ctx, tenantID, key)
	if err != nil || !found {
		return ProductImportResult{}, found, err
	}
	return ProductImportResult{BatchID: id, ItemCount: count, Replayed: true}, true, nil
}

// ImportProducts commits all product rows, balances, the receipt and an audit
// event in one PostgreSQL transaction. Retrying identical items with the same
// key returns the existing receipt; a changed payload is always rejected.
func (s *ProductsService) ImportProducts(
	ctx context.Context, tenantID, actorID, idempotencyKey string,
	req ProductImportRequest,
) (ProductImportResult, error) {
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 ||
		strings.TrimSpace(idempotencyKey) != idempotencyKey ||
		len(req.Items) < 1 || len(req.Items) > 500 {
		return ProductImportResult{}, common.ErrValidation
	}

	items := make([]ProductCreateRequest, len(req.Items))
	seenSKU := make(map[string]bool, len(items))
	seenBarcode := make(map[string]bool, len(items))
	for i, item := range req.Items {
		item = normalizeProductRequest(item)
		if s.validate.Struct(item) != nil || validateProductPricing(item) != nil {
			return ProductImportResult{}, common.ErrValidation
		}
		skuKey := strings.ToLower(item.SKU)
		if seenSKU[skuKey] {
			return ProductImportResult{}, common.ErrValidation
		}
		seenSKU[skuKey] = true
		if item.Barcode != nil {
			if seenBarcode[*item.Barcode] {
				return ProductImportResult{}, common.ErrValidation
			}
			seenBarcode[*item.Barcode] = true
		}
		items[i] = item
	}
	// Order-independent fingerprint after server-side normalization.
	sort.Slice(items, func(i, j int) bool { return items[i].SKU < items[j].SKU })
	payload, err := json.Marshal(items)
	if err != nil {
		return ProductImportResult{}, err
	}
	hash := sha256.Sum256(payload)
	requestHash := hex.EncodeToString(hash[:])

	tx, err := s.uow.Begin(ctx)
	if err != nil {
		return ProductImportResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.repo.LockProductImportKey(ctx, tx, tenantID, idempotencyKey); err != nil {
		return ProductImportResult{}, err
	}
	batchID, previousHash, itemCount, found, err :=
		s.repo.GetProductImportBatch(ctx, tx, tenantID, idempotencyKey)
	if err != nil {
		return ProductImportResult{}, err
	}
	if found {
		if previousHash != requestHash {
			return ProductImportResult{}, common.ErrConflict
		}
		return ProductImportResult{BatchID: batchID, ItemCount: itemCount, Replayed: true}, nil
	}

	batchID, err = s.repo.CreateProductImportBatch(
		ctx, tx, tenantID, actorID, idempotencyKey, requestHash, len(items),
	)
	if err != nil {
		return ProductImportResult{}, err
	}
	createdIDs := make([]string, 0, len(items))
	for _, item := range items {
		p := inv.Product{
			CategoryID: item.CategoryID,
			SKU: item.SKU,
			Barcode: item.Barcode,
			NCM: item.NCM,
			CEST: item.CEST,
			Name: item.Name,
			Description: item.Description,
			Unit: item.Unit,
			CostPrice: item.CostPrice,
			PriceCash: item.PriceCash,
			PromoPrice: item.PromoPrice,
			MinStock: item.MinStock,
			Active: item.Active,
		}
		id, err := s.repo.Create(ctx, tx, tenantID, p)
		if err != nil {
			// Unique SKU/barcode conflicts and every other failure abort the
			// whole receipt + product/balance transaction.
			return ProductImportResult{}, err
		}
		createdIDs = append(createdIDs, id)
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID,
		Action: "product.import.batch", ResourceType: "product_import_batch",
		ResourceID: batchID, Outcome: "success",
		Metadata: map[string]any{"item_count": len(items), "request_sha256": requestHash},
	}); err != nil {
		return ProductImportResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProductImportResult{}, err
	}
	if cache, ok := s.repo.(interface {
		InvalidateProduct(context.Context, string, string) error
		BumpProductsListVersion(context.Context, string) error
	}); ok {
		cacheCtx := context.WithoutCancel(ctx)
		for _, id := range createdIDs {
			_ = cache.InvalidateProduct(cacheCtx, tenantID, id)
		}
		_ = cache.BumpProductsListVersion(cacheCtx, tenantID)
	}
	return ProductImportResult{BatchID: batchID, ItemCount: len(items)}, nil
}
