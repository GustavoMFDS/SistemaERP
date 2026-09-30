package infrastructure

import (
	"context"
	"errors"

	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
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

func (r *FiscalRepo) GetNFCeIssuerProfile(ctx context.Context, tenantID string) (fisc.NFCeIssuerProfile, error) {
	var profile fisc.NFCeIssuerProfile
	profile.TenantID = tenantID
	err := r.db.QueryRow(ctx, `
		SELECT
			legal_name,
			trade_name,
			cnpj,
			COALESCE(ie, ''),
			COALESCE(crt, ''),
			COALESCE(address_street, ''),
			COALESCE(address_number, ''),
			address_complement,
			COALESCE(address_neighborhood, ''),
			COALESCE(address_city, ''),
			COALESCE(address_city_code, ''),
			COALESCE(address_state, ''),
			COALESCE(address_zip, '')
		FROM companies
		WHERE id=$1
	`, tenantID).Scan(
		&profile.LegalName,
		&profile.TradeName,
		&profile.CNPJ,
		&profile.IE,
		&profile.CRT,
		&profile.AddressStreet,
		&profile.AddressNumber,
		&profile.AddressComplement,
		&profile.AddressNeighborhood,
		&profile.AddressCity,
		&profile.AddressCityCode,
		&profile.AddressState,
		&profile.AddressZIP,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeIssuerProfile{}, common.ErrNotFound
		}
		return fisc.NFCeIssuerProfile{}, err
	}
	return profile, nil
}

func (r *FiscalRepo) UpdateNFCeIssuerProfile(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	profile fisc.NFCeIssuerProfile,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE companies
		SET ie=$2,
		    crt=$3,
		    address_street=$4,
		    address_number=$5,
		    address_complement=$6,
		    address_neighborhood=$7,
		    address_city=$8,
		    address_city_code=$9,
		    address_state=$10,
		    address_zip=$11,
		    updated_at=now()
		WHERE id=$1
	`,
		tenantID,
		profile.IE,
		profile.CRT,
		profile.AddressStreet,
		profile.AddressNumber,
		profile.AddressComplement,
		profile.AddressNeighborhood,
		profile.AddressCity,
		profile.AddressCityCode,
		profile.AddressState,
		profile.AddressZIP,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return common.ErrNotFound
	}
	return nil
}

func (r *FiscalRepo) GetNFCeConfig(ctx context.Context, tenantID string) (fisc.NFCeConfig, error) {
	var cfg fisc.NFCeConfig
	cfg.TenantID = tenantID
	err := r.db.QueryRow(ctx, `
		SELECT enabled, environment, series, csc_id, csc_secret_ref, certificate_secret_ref
		FROM nfce_configs
		WHERE tenant_id=$1
	`, tenantID).Scan(
		&cfg.Enabled,
		&cfg.Environment,
		&cfg.Series,
		&cfg.CSCID,
		&cfg.CSCSecretRef,
		&cfg.CertificateSecretRef,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeConfig{}, common.ErrNotFound
		}
		return fisc.NFCeConfig{}, err
	}
	cfg.CSCReferenceConfigured = cfg.CSCID != nil && cfg.CSCSecretRef != nil
	cfg.CertificateReferenceConfigured = cfg.CertificateSecretRef != nil
	return cfg, nil
}

func (r *FiscalRepo) UpsertNFCeConfig(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	actorUserID string,
	cfg fisc.NFCeConfig,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO nfce_configs(
			tenant_id, enabled, environment, series, csc_id,
			csc_secret_ref, certificate_secret_ref, updated_by_user_id
		)
		VALUES ($1, false, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id) DO UPDATE
		SET enabled=false,
		    environment=EXCLUDED.environment,
		    series=EXCLUDED.series,
		    csc_id=EXCLUDED.csc_id,
		    csc_secret_ref=EXCLUDED.csc_secret_ref,
		    certificate_secret_ref=EXCLUDED.certificate_secret_ref,
		    updated_by_user_id=EXCLUDED.updated_by_user_id,
		    updated_at=now()
	`, tenantID, cfg.Environment, cfg.Series, cfg.CSCID, cfg.CSCSecretRef, cfg.CertificateSecretRef, actorUserID)
	return err
}

func (r *FiscalRepo) GetNFCeReadiness(ctx context.Context, tenantID string) (fisc.NFCeReadiness, error) {
	out := fisc.NFCeReadiness{
		TenantID: tenantID,
		Model:    65,
	}
	err := r.db.QueryRow(ctx, `
		SELECT
			NULLIF(btrim(c.legal_name), '') IS NOT NULL
			  AND NULLIF(btrim(c.cnpj), '') IS NOT NULL
			  AND NULLIF(btrim(c.ie), '') IS NOT NULL
			  AND NULLIF(btrim(c.crt), '') IS NOT NULL AS issuer_identity_configured,
			NULLIF(btrim(c.address_street), '') IS NOT NULL
			  AND NULLIF(btrim(c.address_number), '') IS NOT NULL
			  AND NULLIF(btrim(c.address_neighborhood), '') IS NOT NULL
			  AND NULLIF(btrim(c.address_city), '') IS NOT NULL
			  AND COALESCE(char_length(btrim(c.address_state)) = 2, false)
			  AND NULLIF(btrim(c.address_zip), '') IS NOT NULL AS issuer_address_configured,
			COALESCE(c.address_city_code ~ '^[0-9]{7}$', false) AS municipality_code_configured,
			cfg.tenant_id IS NOT NULL AS config_exists,
			COALESCE(cfg.enabled, false) AS transmission_enabled,
			COALESCE(cfg.environment, '') AS environment,
			COALESCE(cfg.series, 0) AS series,
			COALESCE(
				NULLIF(btrim(cfg.csc_id), '') IS NOT NULL
				AND NULLIF(btrim(cfg.csc_secret_ref), '') IS NOT NULL,
				false
			) AS csc_reference_configured,
			COALESCE(NULLIF(btrim(cfg.certificate_secret_ref), '') IS NOT NULL, false)
				AS certificate_reference_configured,
			(SELECT count(*)::int FROM products p WHERE p.tenant_id=c.id AND p.active=true) AS active_products,
			(
				SELECT count(*)::int
				FROM products p
				WHERE p.tenant_id=c.id
				  AND p.active=true
				  AND NULLIF(btrim(p.ncm), '') IS NULL
			) AS products_missing_ncm
		FROM companies c
		LEFT JOIN nfce_configs cfg ON cfg.tenant_id=c.id
		WHERE c.id=$1
	`, tenantID).Scan(
		&out.IssuerIdentityConfigured,
		&out.IssuerAddressConfigured,
		&out.MunicipalityCodeConfigured,
		&out.ConfigExists,
		&out.TransmissionEnabled,
		&out.Environment,
		&out.Series,
		&out.CSCReferenceConfigured,
		&out.CertificateReferenceConfigured,
		&out.ActiveProducts,
		&out.ProductsMissingNCM,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeReadiness{}, common.ErrNotFound
		}
		return fisc.NFCeReadiness{}, err
	}

	reasons := make([]string, 0, 8)
	if !out.IssuerIdentityConfigured {
		reasons = append(reasons, "issuer_identity")
	}
	if !out.IssuerAddressConfigured {
		reasons = append(reasons, "issuer_address")
	}
	if !out.MunicipalityCodeConfigured {
		reasons = append(reasons, "issuer_municipality_code")
	}
	if !out.ConfigExists {
		reasons = append(reasons, "nfce_config")
	} else {
		if out.Environment != "homologation" {
			reasons = append(reasons, "homologation_environment")
		}
		if !out.CSCReferenceConfigured {
			reasons = append(reasons, "csc_secret_reference")
		}
		if !out.CertificateReferenceConfigured {
			reasons = append(reasons, "certificate_secret_reference")
		}
	}
	if out.ActiveProducts < 1 {
		reasons = append(reasons, "active_products")
	}
	if out.ProductsMissingNCM > 0 {
		reasons = append(reasons, "product_ncm")
	}
	out.BlockingReasons = reasons
	out.ReadyForHomologationData = len(reasons) == 0
	return out, nil
}
