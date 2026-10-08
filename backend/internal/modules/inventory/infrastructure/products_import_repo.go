package infrastructure

import (
	"context"

	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

// Serialize retries from independent devices before comparing the committed
// receipt. A hash collision can only delay another tenant's request.
func (r *ProductsRepo) LockProductImportKey(
	ctx context.Context, tx db.DBTX, tenantID, key string,
) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`,
		tenantID+":product-import:"+key)
	return err
}

func (r *ProductsRepo) GetProductImportBatch(
	ctx context.Context, tx db.DBTX, tenantID, key string,
) (batchID, requestHash string, itemCount int, found bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT id::text, request_hash, item_count
		FROM product_import_batches
		WHERE tenant_id=$1 AND idem_key=$2
	`, tenantID, key).Scan(&batchID, &requestHash, &itemCount)
	if err == pgx.ErrNoRows {
		return "", "", 0, false, nil
	}
	if err != nil {
		return "", "", 0, false, err
	}
	return batchID, requestHash, itemCount, true, nil
}

// Reconciliation reports committed receipts only; it never exposes products,
// CSV content or hashes across companies.
func (r *ProductsRepo) LookupProductImportBatch(
	ctx context.Context, tenantID, key string,
) (batchID string, itemCount int, found bool, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT id::text, item_count
		FROM product_import_batches
		WHERE tenant_id=$1 AND idem_key=$2
	`, tenantID, key).Scan(&batchID, &itemCount)
	if err == pgx.ErrNoRows {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return batchID, itemCount, true, nil
}

func (r *ProductsRepo) CreateProductImportBatch(
	ctx context.Context, tx db.DBTX, tenantID, actorID, key, requestHash string, itemCount int,
) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO product_import_batches (
			tenant_id, idem_key, request_hash, item_count, created_by_user_id
		) VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text
	`, tenantID, key, requestHash, itemCount, actorID).Scan(&id)
	return id, err
}

func (r *CachedProductsRepo) LockProductImportKey(
	ctx context.Context, tx db.DBTX, tenantID, key string,
) error {
	return r.base.LockProductImportKey(ctx, tx, tenantID, key)
}

func (r *CachedProductsRepo) GetProductImportBatch(
	ctx context.Context, tx db.DBTX, tenantID, key string,
) (string, string, int, bool, error) {
	return r.base.GetProductImportBatch(ctx, tx, tenantID, key)
}

func (r *CachedProductsRepo) LookupProductImportBatch(
	ctx context.Context, tenantID, key string,
) (string, int, bool, error) {
	return r.base.LookupProductImportBatch(ctx, tenantID, key)
}

func (r *CachedProductsRepo) CreateProductImportBatch(
	ctx context.Context, tx db.DBTX, tenantID, actorID, key, requestHash string, itemCount int,
) (string, error) {
	return r.base.CreateProductImportBatch(ctx, tx, tenantID, actorID, key, requestHash, itemCount)
}
