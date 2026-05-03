package infrastructure

import (
	"context"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FiscalRepo struct {
	db *pgxpool.Pool
}

func NewFiscalRepo(dbpool *pgxpool.Pool) *FiscalRepo {
	return &FiscalRepo{db: dbpool}
}

func (r *FiscalRepo) CreateInvoiceWithXML(ctx context.Context, tx db.DBTX, tenantID string, saleID, companyID string, createdByUserID *string, fileName string, content []byte, sha256 string) (invoiceID, xmlID string, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO invoices(tenant_id, sale_id, company_id, created_by_user_id)
		VALUES ($1,$2,$3,$4)
		RETURNING id::text
	`, tenantID, saleID, companyID, createdByUserID).Scan(&invoiceID)
	if err != nil {
		return "", "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO invoice_xml_files(tenant_id, invoice_id, file_name, content, sha256)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text
	`, tenantID, invoiceID, fileName, content, sha256).Scan(&xmlID)
	if err != nil {
		return "", "", err
	}
	return invoiceID, xmlID, nil
}

func (r *FiscalRepo) ListXML(ctx context.Context, tenantID string, limit, offset int) ([]fisc.XMLFile, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM invoice_xml_files WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, invoice_id::text, file_name, sha256, created_at::text
		FROM invoice_xml_files
		WHERE tenant_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []fisc.XMLFile
	for rows.Next() {
		var x fisc.XMLFile
		if err := rows.Scan(&x.ID, &x.InvoiceID, &x.FileName, &x.SHA256, &x.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, x)
	}
	return items, total, rows.Err()
}

func (r *FiscalRepo) GetXMLContent(ctx context.Context, tenantID string, id string) (fileName string, content []byte, err error) {
	err = r.db.QueryRow(ctx, `SELECT file_name, content FROM invoice_xml_files WHERE tenant_id=$1 AND id=$2`, tenantID, id).Scan(&fileName, &content)
	return fileName, content, err
}

func (r *FiscalRepo) GetCompanyID(ctx context.Context, tx db.DBTX) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM companies ORDER BY created_at LIMIT 1`).Scan(&id)
	return id, err
}

func (r *FiscalRepo) ExistsInvoiceForSale(ctx context.Context, tx db.DBTX, tenantID string, saleID string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM invoices WHERE tenant_id=$1 AND sale_id=$2)`, tenantID, saleID).Scan(&exists)
	return exists, err
}
