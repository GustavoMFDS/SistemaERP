package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *FiscalRepo) SetNFCeTransmissionEnabled(
	ctx context.Context,
	tx db.DBTX,
	tenantID, actorUserID string,
	enabled bool,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE nfce_configs
		SET enabled=$3,
		    updated_by_user_id=$2,
		    updated_at=now()
		WHERE tenant_id=$1
	`, tenantID, actorUserID, enabled)
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
	cfg.CSCReferenceConfigured =
		cfg.CSCID != nil && strings.TrimSpace(*cfg.CSCID) != "" &&
			cfg.CSCSecretRef != nil && strings.TrimSpace(*cfg.CSCSecretRef) != ""
	cfg.CertificateReferenceConfigured =
		cfg.CertificateSecretRef != nil && strings.TrimSpace(*cfg.CertificateSecretRef) != ""
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

func (r *FiscalRepo) GetNFCeReservationContextForUpdate(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
) (fisc.NFCeReservationContext, error) {
	var out fisc.NFCeReservationContext
	out.Issuer.TenantID = tenantID
	out.Config.TenantID = tenantID

	err := tx.QueryRow(ctx, `
		SELECT
			c.legal_name,
			c.trade_name,
			c.cnpj,
			COALESCE(c.ie, ''),
			COALESCE(c.crt, ''),
			COALESCE(c.address_street, ''),
			COALESCE(c.address_number, ''),
			c.address_complement,
			COALESCE(c.address_neighborhood, ''),
			COALESCE(c.address_city, ''),
			COALESCE(c.address_city_code, ''),
			COALESCE(c.address_state, ''),
			COALESCE(c.address_zip, ''),
			cfg.enabled,
			cfg.environment,
			cfg.series,
			cfg.csc_id,
			cfg.csc_secret_ref,
			cfg.certificate_secret_ref
		FROM companies c
		JOIN nfce_configs cfg ON cfg.tenant_id=c.id
		WHERE c.id=$1
		FOR UPDATE OF c, cfg
	`, tenantID).Scan(
		&out.Issuer.LegalName,
		&out.Issuer.TradeName,
		&out.Issuer.CNPJ,
		&out.Issuer.IE,
		&out.Issuer.CRT,
		&out.Issuer.AddressStreet,
		&out.Issuer.AddressNumber,
		&out.Issuer.AddressComplement,
		&out.Issuer.AddressNeighborhood,
		&out.Issuer.AddressCity,
		&out.Issuer.AddressCityCode,
		&out.Issuer.AddressState,
		&out.Issuer.AddressZIP,
		&out.Config.Enabled,
		&out.Config.Environment,
		&out.Config.Series,
		&out.Config.CSCID,
		&out.Config.CSCSecretRef,
		&out.Config.CertificateSecretRef,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeReservationContext{}, common.ErrNotFound
		}
		return fisc.NFCeReservationContext{}, err
	}

	out.Config.CSCReferenceConfigured =
		out.Config.CSCID != nil && strings.TrimSpace(*out.Config.CSCID) != "" &&
			out.Config.CSCSecretRef != nil && strings.TrimSpace(*out.Config.CSCSecretRef) != ""
	out.Config.CertificateReferenceConfigured =
		out.Config.CertificateSecretRef != nil && strings.TrimSpace(*out.Config.CertificateSecretRef) != ""
	return out, nil
}

func (r *FiscalRepo) GetNFCeReservationByInvoiceForUpdate(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID string,
) (fisc.NFCeReservation, error) {
	var out fisc.NFCeReservation
	err := tx.QueryRow(ctx, `
		SELECT
			id::text,
			sale_id::text,
			status,
			model,
			series,
			document_number,
			environment,
			access_key,
			emission_type,
			numeric_code,
			access_key_check_digit,
			issued_at,
			authorization_protocol,
			authorized_at
		FROM invoices
		WHERE tenant_id=$1
		  AND id=$2
		  AND model=65
		  AND document_number IS NOT NULL
		FOR UPDATE
	`, tenantID, invoiceID).Scan(
		&out.InvoiceID,
		&out.SaleID,
		&out.Status,
		&out.Model,
		&out.Series,
		&out.DocumentNumber,
		&out.Environment,
		&out.AccessKey,
		&out.EmissionType,
		&out.NumericCode,
		&out.CheckDigit,
		&out.IssuedAt,
		&out.AuthorizationProtocol,
		&out.AuthorizedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeReservation{}, common.ErrNotFound
		}
		return fisc.NFCeReservation{}, err
	}
	return out, nil
}

func (r *FiscalRepo) GetNFCeReservationBySale(
	ctx context.Context,
	tx db.DBTX,
	tenantID, saleID string,
) (fisc.NFCeReservation, error) {
	var out fisc.NFCeReservation
	err := tx.QueryRow(ctx, `
		SELECT
			id::text,
			sale_id::text,
			status,
			model,
			series,
			document_number,
			environment,
			access_key,
			emission_type,
			numeric_code,
			access_key_check_digit,
			issued_at,
			authorization_protocol,
			authorized_at
		FROM invoices
		WHERE tenant_id=$1
		  AND sale_id=$2
		  AND model=65
		  AND document_number IS NOT NULL
	`, tenantID, saleID).Scan(
		&out.InvoiceID,
		&out.SaleID,
		&out.Status,
		&out.Model,
		&out.Series,
		&out.DocumentNumber,
		&out.Environment,
		&out.AccessKey,
		&out.EmissionType,
		&out.NumericCode,
		&out.CheckDigit,
		&out.IssuedAt,
		&out.AuthorizationProtocol,
		&out.AuthorizedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeReservation{}, common.ErrNotFound
		}
		return fisc.NFCeReservation{}, err
	}
	return out, nil
}

func (r *FiscalRepo) CreateNFCeReservation(
	ctx context.Context,
	tx db.DBTX,
	tenantID, actorUserID string,
	reservation fisc.NFCeReservation,
) (string, error) {
	var invoiceID string
	err := tx.QueryRow(ctx, `
		INSERT INTO invoices(
			tenant_id,
			sale_id,
			company_id,
			status,
			created_by_user_id,
			model,
			series,
			document_number,
			environment,
			access_key,
			emission_type,
			numeric_code,
			access_key_check_digit,
			issued_at,
			updated_at
		)
		VALUES (
			$1,$2,$1,'reserved',$3,65,$4,$5,$6,$7,$8,$9,$10,$11,now()
		)
		RETURNING id::text
	`,
		tenantID,
		reservation.SaleID,
		actorUserID,
		reservation.Series,
		reservation.DocumentNumber,
		reservation.Environment,
		reservation.AccessKey,
		reservation.EmissionType,
		reservation.NumericCode,
		reservation.CheckDigit,
		reservation.IssuedAt,
	).Scan(&invoiceID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", common.ErrConflict
		}
		return "", err
	}
	return invoiceID, nil
}

func (r *FiscalRepo) GetLatestNFCeXMLContent(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID string,
) (string, []byte, error) {
	var fileName string
	var content []byte
	err := tx.QueryRow(ctx, `
		SELECT file_name, content
		FROM invoice_xml_files
		WHERE tenant_id=$1
		  AND invoice_id=$2
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, tenantID, invoiceID).Scan(&fileName, &content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, common.ErrNotFound
		}
		return "", nil, err
	}
	return fileName, content, nil
}

func (r *FiscalRepo) StoreSignedNFCeXML(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID, accessKey, fileName string,
	content []byte,
	sha256 string,
) (string, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE invoices
		SET status='signed', updated_at=now()
		WHERE tenant_id=$1
		  AND id=$2
		  AND access_key=$3
		  AND status='reserved'
	`, tenantID, invoiceID, accessKey)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", common.ErrConflict
	}

	var xmlID string
	err = tx.QueryRow(ctx, `
		INSERT INTO invoice_xml_files(tenant_id, invoice_id, file_name, content, sha256)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id::text
	`, tenantID, invoiceID, fileName, content, sha256).Scan(&xmlID)
	if err != nil {
		return "", err
	}
	return xmlID, nil
}

func (r *FiscalRepo) MarkNFCeSubmitted(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID, accessKey string,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE invoices
		SET status='submitted', updated_at=now()
		WHERE tenant_id=$1
		  AND id=$2
		  AND access_key=$3
		  AND status='signed'
	`, tenantID, invoiceID, accessKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrConflict
	}
	return nil
}

func (r *FiscalRepo) ApplyNFCeAuthorizationResult(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID string,
	result fisc.NFCeAuthorizationResult,
) error {
	var (
		tag pgconn.CommandTag
		err error
	)
	switch {
	case result.IsAuthorized():
		tag, err = tx.Exec(ctx, `
			UPDATE invoices
			SET status='authorized',
			    authorization_protocol=$4,
			    authorized_at=$5,
			    rejection_code=NULL,
			    rejection_message=NULL,
			    updated_at=now()
			WHERE tenant_id=$1
			  AND id=$2
			  AND access_key=$3
			  AND status='submitted'
		`, tenantID, invoiceID, result.AccessKey, result.Protocol, result.AuthorizedAt)
	case result.IsRejected():
		tag, err = tx.Exec(ctx, `
			UPDATE invoices
			SET status='rejected',
			    authorization_protocol=NULL,
			    authorized_at=NULL,
			    rejection_code=$4,
			    rejection_message=$5,
			    updated_at=now()
			WHERE tenant_id=$1
			  AND id=$2
			  AND access_key=$3
			  AND status='submitted'
		`, tenantID, invoiceID, result.AccessKey, result.RejectionCode, result.RejectionMessage)
	default:
		return common.ErrValidation
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrConflict
	}
	return nil
}

func (r *FiscalRepo) GetNFCeCancellationEventForUpdate(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID string,
) (fisc.NFCeCancellationEvent, []byte, error) {
	var out fisc.NFCeCancellationEvent
	var signedXML []byte
	err := tx.QueryRow(ctx, `
		SELECT
			id::text,
			tenant_id::text,
			invoice_id::text,
			event_type,
			sequence,
			event_id,
			environment,
			status,
			justification,
			signed_sha256,
			response_sha256,
			status_code,
			reason,
			protocol,
			registered_at,
			created_at,
			updated_at,
			signed_xml
		FROM invoice_fiscal_events
		WHERE tenant_id=$1
		  AND invoice_id=$2
		  AND event_type='110111'
		ORDER BY sequence DESC
		LIMIT 1
		FOR UPDATE
	`, tenantID, invoiceID).Scan(
		&out.ID,
		&out.TenantID,
		&out.InvoiceID,
		&out.EventType,
		&out.Sequence,
		&out.EventID,
		&out.Environment,
		&out.Status,
		&out.Justification,
		&out.SignedSHA256,
		&out.ResponseSHA256,
		&out.StatusCode,
		&out.Reason,
		&out.Protocol,
		&out.RegisteredAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&signedXML,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeCancellationEvent{}, nil, common.ErrNotFound
		}
		return fisc.NFCeCancellationEvent{}, nil, err
	}
	return out, signedXML, nil
}

func (r *FiscalRepo) InsertSignedNFCeCancellationEvent(
	ctx context.Context,
	tx db.DBTX,
	event fisc.NFCeCancellationEvent,
	actorUserID string,
	signedXML []byte,
	sha256 string,
) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO invoice_fiscal_events(
			tenant_id,
			invoice_id,
			event_type,
			sequence,
			event_id,
			environment,
			status,
			justification,
			signed_xml,
			signed_sha256,
			created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6,'signed',$7,$8,$9,$10)
		RETURNING id::text
	`,
		event.TenantID,
		event.InvoiceID,
		event.EventType,
		event.Sequence,
		event.EventID,
		event.Environment,
		event.Justification,
		signedXML,
		sha256,
		actorUserID,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", common.ErrConflict
		}
		return "", err
	}
	return id, nil
}

func (r *FiscalRepo) MarkNFCeCancellationSubmitted(
	ctx context.Context,
	tx db.DBTX,
	tenantID, eventID string,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE invoice_fiscal_events
		SET status='submitted', updated_at=now()
		WHERE tenant_id=$1
		  AND event_id=$2
		  AND status='signed'
	`, tenantID, eventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrConflict
	}
	return nil
}

func (r *FiscalRepo) ApplyNFCeCancellationResult(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID, eventID string,
	result fisc.NFCeCancellationRemoteResult,
	responseSHA256 string,
) error {
	if result.Pending() || len(result.ResponseXML) == 0 {
		return common.ErrValidation
	}

	if result.Registered() {
		tag, err := tx.Exec(ctx, `
			UPDATE invoice_fiscal_events
			SET status='registered',
			    response_xml=$4,
			    response_sha256=$5,
			    status_code=$6,
			    reason=$7,
			    protocol=$8,
			    registered_at=$9,
			    updated_at=now()
			WHERE tenant_id=$1
			  AND invoice_id=$2
			  AND event_id=$3
			  AND status='submitted'
		`,
			tenantID,
			invoiceID,
			eventID,
			result.ResponseXML,
			responseSHA256,
			result.StatusCode,
			result.Reason,
			result.Protocol,
			result.RegisteredAt,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return common.ErrConflict
		}

		tag, err = tx.Exec(ctx, `
			UPDATE invoices i
			SET status='cancelled',
			    cancellation_protocol=$4,
			    cancelled_at=$5,
			    cancellation_reason=e.justification,
			    updated_at=now()
			FROM invoice_fiscal_events e
			WHERE i.tenant_id=$1
			  AND i.id=$2
			  AND i.access_key=$3
			  AND i.status='authorized'
			  AND e.tenant_id=i.tenant_id
			  AND e.invoice_id=i.id
			  AND e.event_id=$6
			  AND e.status='registered'
		`,
			tenantID,
			invoiceID,
			result.AccessKey,
			result.Protocol,
			result.RegisteredAt,
			eventID,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return common.ErrConflict
		}
		return nil
	}

	if result.Rejected() {
		tag, err := tx.Exec(ctx, `
			UPDATE invoice_fiscal_events
			SET status='rejected',
			    response_xml=$4,
			    response_sha256=$5,
			    status_code=$6,
			    reason=$7,
			    protocol=NULL,
			    registered_at=NULL,
			    updated_at=now()
			WHERE tenant_id=$1
			  AND invoice_id=$2
			  AND event_id=$3
			  AND status='submitted'
		`,
			tenantID,
			invoiceID,
			eventID,
			result.ResponseXML,
			responseSHA256,
			result.StatusCode,
			result.Reason,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return common.ErrConflict
		}
		return nil
	}
	return common.ErrValidation
}

func (r *FiscalRepo) ReserveNextNFCeNumber(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	series int,
) (int64, error) {
	var number int64
	err := tx.QueryRow(ctx, `
		INSERT INTO fiscal_document_sequences(tenant_id, model, series, next_number)
		VALUES ($1, 65, $2, 2)
		ON CONFLICT (tenant_id, model, series) DO UPDATE
		SET next_number=fiscal_document_sequences.next_number + 1,
		    updated_at=now()
		WHERE fiscal_document_sequences.next_number <= 999999999
		RETURNING next_number - 1
	`, tenantID, series).Scan(&number)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, common.ErrFiscalSequenceExhausted
		}
		return 0, err
	}
	return number, nil
}

func (r *FiscalRepo) GetNFCeReadiness(ctx context.Context, tenantID string) (fisc.NFCeReadiness, error) {
	out := fisc.NFCeReadiness{
		TenantID: tenantID,
		Model:    65,
	}
	err := r.db.QueryRow(ctx, `
		SELECT
			NULLIF(btrim(c.legal_name), '') IS NOT NULL
			  AND COALESCE(
			    upper(regexp_replace(c.cnpj, '[^A-Za-z0-9]', '', 'g'))
			      ~ '^[A-Z0-9]{12}[0-9]{2}$',
			    false
			  )
			  AND NULLIF(btrim(c.ie), '') IS NOT NULL
			  AND btrim(c.crt) IN ('1','2','3','4') AS issuer_identity_configured,
			NULLIF(btrim(c.address_street), '') IS NOT NULL
			  AND NULLIF(btrim(c.address_number), '') IS NOT NULL
			  AND NULLIF(btrim(c.address_neighborhood), '') IS NOT NULL
			  AND NULLIF(btrim(c.address_city), '') IS NOT NULL
			  AND upper(btrim(c.address_state)) IN (
			    'RO','AC','AM','RR','PA','AP','TO','MA','PI','CE','RN','PB','PE',
			    'AL','SE','BA','MG','ES','RJ','SP','PR','SC','RS','MS','MT','GO','DF'
			  )
			  AND COALESCE(btrim(c.address_zip) ~ '^[0-9]{8}$', false)
			    AS issuer_address_configured,
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
			(SELECT count(*)::int FROM products p WHERE p.tenant_id=c.id AND p.active=true)
				AS active_products,
			(
				SELECT count(*)::int
				FROM products p
				WHERE p.tenant_id=c.id
				  AND p.active=true
				  AND NULLIF(btrim(p.ncm), '') IS NULL
			) AS products_missing_ncm,
			(
				SELECT count(*)::int
				FROM products p
				LEFT JOIN product_fiscal_profiles fp
				  ON fp.tenant_id=p.tenant_id
				 AND fp.product_id=p.id
				WHERE p.tenant_id=c.id
				  AND p.active=true
				  AND fp.product_id IS NULL
			) AS products_missing_fiscal_profile
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
		&out.ProductsMissingFiscalProfile,
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
	if out.ProductsMissingFiscalProfile > 0 {
		reasons = append(reasons, "product_fiscal_profile")
	}
	out.BlockingReasons = reasons
	out.ReadyForHomologationData = len(reasons) == 0
	return out, nil
}

func (r *FiscalRepo) GetProductFiscalProfiles(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	productIDs []string,
) (map[string]fisc.ProductFiscalProfile, error) {
	out := make(map[string]fisc.ProductFiscalProfile, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT
			product_id::text,
			cfop,
			icms_origin,
			icms_regime,
			icms_code,
			pis_cst,
			cofins_cst,
			ibs_cbs_cst,
			ibs_cbs_classification,
			is_cst,
			is_classification,
			reference_version
		FROM product_fiscal_profiles
		WHERE tenant_id=$1
		  AND product_id = ANY($2::uuid[])
	`, tenantID, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var profile fisc.ProductFiscalProfile
		profile.TenantID = tenantID
		if err := rows.Scan(
			&profile.ProductID,
			&profile.CFOP,
			&profile.ICMSOrigin,
			&profile.ICMSRegime,
			&profile.ICMSCode,
			&profile.PISCST,
			&profile.COFINSCST,
			&profile.IBSCBSCST,
			&profile.IBSCBSClassification,
			&profile.ISCST,
			&profile.ISClassification,
			&profile.ReferenceVersion,
		); err != nil {
			return nil, err
		}
		out[profile.ProductID] = profile
	}
	return out, rows.Err()
}

func (r *FiscalRepo) UpsertProductFiscalProfile(
	ctx context.Context,
	tx db.DBTX,
	tenantID, actorUserID string,
	profile fisc.ProductFiscalProfile,
) error {
	tag, err := tx.Exec(ctx, `
		INSERT INTO product_fiscal_profiles(
			tenant_id,
			product_id,
			cfop,
			icms_origin,
			icms_regime,
			icms_code,
			pis_cst,
			cofins_cst,
			ibs_cbs_cst,
			ibs_cbs_classification,
			is_cst,
			is_classification,
			reference_version,
			updated_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (tenant_id, product_id) DO UPDATE
		SET cfop=EXCLUDED.cfop,
		    icms_origin=EXCLUDED.icms_origin,
		    icms_regime=EXCLUDED.icms_regime,
		    icms_code=EXCLUDED.icms_code,
		    pis_cst=EXCLUDED.pis_cst,
		    cofins_cst=EXCLUDED.cofins_cst,
		    ibs_cbs_cst=EXCLUDED.ibs_cbs_cst,
		    ibs_cbs_classification=EXCLUDED.ibs_cbs_classification,
		    is_cst=EXCLUDED.is_cst,
		    is_classification=EXCLUDED.is_classification,
		    reference_version=EXCLUDED.reference_version,
		    updated_by_user_id=EXCLUDED.updated_by_user_id,
		    updated_at=now()
	`,
		tenantID,
		profile.ProductID,
		profile.CFOP,
		profile.ICMSOrigin,
		profile.ICMSRegime,
		profile.ICMSCode,
		profile.PISCST,
		profile.COFINSCST,
		profile.IBSCBSCST,
		profile.IBSCBSClassification,
		profile.ISCST,
		profile.ISClassification,
		profile.ReferenceVersion,
		actorUserID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return common.ErrNotFound
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return common.ErrValidation
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrConflict
	}
	return nil
}

func (r *FiscalRepo) CreateSaleItemFiscalSnapshots(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	snapshots []fisc.SaleItemFiscalSnapshot,
) error {
	for _, snapshot := range snapshots {
		_, err := tx.Exec(ctx, `
			INSERT INTO sale_item_fiscal_snapshots(
				tenant_id,
				sale_item_id,
				sale_id,
				product_id,
				product_code,
				product_description,
				unit,
				ncm,
				cest,
				cfop,
				icms_origin,
				icms_regime,
				icms_code,
				pis_cst,
				cofins_cst,
				ibs_cbs_cst,
				ibs_cbs_classification,
				is_cst,
				is_classification,
				reference_version,
				snapshot_sha256
			)
			VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21
			)
		`,
			tenantID,
			snapshot.SaleItemID,
			snapshot.SaleID,
			snapshot.ProductID,
			snapshot.ProductCode,
			snapshot.ProductDescription,
			snapshot.Unit,
			snapshot.NCM,
			snapshot.CEST,
			snapshot.CFOP,
			snapshot.ICMSOrigin,
			snapshot.ICMSRegime,
			snapshot.ICMSCode,
			snapshot.PISCST,
			snapshot.COFINSCST,
			snapshot.IBSCBSCST,
			snapshot.IBSCBSClassification,
			snapshot.ISCST,
			snapshot.ISClassification,
			snapshot.ReferenceVersion,
			snapshot.SnapshotSHA256,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return common.ErrConflict
			}
			if errors.As(err, &pgErr) && pgErr.Code == "23514" {
				return common.ErrValidation
			}
			return err
		}
	}
	return nil
}

func (r *FiscalRepo) GetSaleItemFiscalSnapshots(
	ctx context.Context,
	tx db.DBTX,
	tenantID, saleID string,
) ([]fisc.SaleItemFiscalSnapshot, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			sale_item_id::text,
			sale_id::text,
			product_id::text,
			product_code,
			product_description,
			unit,
			ncm,
			cest,
			cfop,
			icms_origin,
			icms_regime,
			icms_code,
			pis_cst,
			cofins_cst,
			ibs_cbs_cst,
			ibs_cbs_classification,
			is_cst,
			is_classification,
			reference_version,
			snapshot_sha256
		FROM sale_item_fiscal_snapshots
		WHERE tenant_id=$1 AND sale_id=$2
		ORDER BY sale_item_id
	`, tenantID, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]fisc.SaleItemFiscalSnapshot, 0)
	for rows.Next() {
		var snapshot fisc.SaleItemFiscalSnapshot
		snapshot.TenantID = tenantID
		if err := rows.Scan(
			&snapshot.SaleItemID,
			&snapshot.SaleID,
			&snapshot.ProductID,
			&snapshot.ProductCode,
			&snapshot.ProductDescription,
			&snapshot.Unit,
			&snapshot.NCM,
			&snapshot.CEST,
			&snapshot.CFOP,
			&snapshot.ICMSOrigin,
			&snapshot.ICMSRegime,
			&snapshot.ICMSCode,
			&snapshot.PISCST,
			&snapshot.COFINSCST,
			&snapshot.IBSCBSCST,
			&snapshot.IBSCBSClassification,
			&snapshot.ISCST,
			&snapshot.ISClassification,
			&snapshot.ReferenceVersion,
			&snapshot.SnapshotSHA256,
		); err != nil {
			return nil, err
		}
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

func (r *FiscalRepo) InsertInvoiceItemTaxCalculation(
	ctx context.Context,
	tx db.DBTX,
	calculation fisc.InvoiceItemTaxCalculation,
) error {
	legacyJSON, err := json.Marshal(calculation.LegacyTax)
	if err != nil {
		return err
	}
	rtcJSON, err := json.Marshal(calculation.RTCTax)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO invoice_item_tax_calculations(
			tenant_id,
			invoice_id,
			sale_id,
			sale_item_id,
			calculation_version,
			legacy_tax,
			rtc_tax,
			calculation_sha256
		)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8)
	`,
		calculation.TenantID,
		calculation.InvoiceID,
		calculation.SaleID,
		calculation.SaleItemID,
		calculation.CalculationVersion,
		string(legacyJSON),
		string(rtcJSON),
		calculation.CalculationSHA256,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return common.ErrConflict
			case "23503":
				return common.ErrNotFound
			case "23514":
				return common.ErrValidation
			}
		}
		return err
	}
	return nil
}

func (r *FiscalRepo) GetInvoiceItemTaxCalculations(
	ctx context.Context,
	tx db.DBTX,
	tenantID, invoiceID string,
) ([]fisc.InvoiceItemTaxCalculation, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			invoice_id::text,
			sale_id::text,
			sale_item_id::text,
			calculation_version,
			legacy_tax::text,
			rtc_tax::text,
			calculation_sha256
		FROM invoice_item_tax_calculations
		WHERE tenant_id=$1 AND invoice_id=$2
		ORDER BY sale_item_id
	`, tenantID, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]fisc.InvoiceItemTaxCalculation, 0)
	for rows.Next() {
		var calculation fisc.InvoiceItemTaxCalculation
		var legacyJSON, rtcJSON string
		calculation.TenantID = tenantID
		if err := rows.Scan(
			&calculation.InvoiceID,
			&calculation.SaleID,
			&calculation.SaleItemID,
			&calculation.CalculationVersion,
			&legacyJSON,
			&rtcJSON,
			&calculation.CalculationSHA256,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(legacyJSON), &calculation.LegacyTax); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(rtcJSON), &calculation.RTCTax); err != nil {
			return nil, err
		}
		out = append(out, calculation)
	}
	return out, rows.Err()
}

func (r *FiscalRepo) LockNFCeInutilizationRange(
	ctx context.Context,
	tx db.DBTX,
	tenantID string,
	year, series int,
) error {
	key := fmt.Sprintf("%s:nfce-inutilization:%d:%d", tenantID, year, series)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, key)
	return err
}

func (r *FiscalRepo) NFCeNumberRangeIsAvailable(
	ctx context.Context,
	tx db.DBTX,
	tenantID, environment string,
	year, series int,
	startNumber, endNumber int64,
) (bool, error) {
	var available bool
	err := tx.QueryRow(ctx, `
		SELECT
		  NOT EXISTS (
		    SELECT 1
		    FROM invoices i
		    WHERE i.tenant_id=$1
		      AND i.model=65
		      AND i.series=$4
		      AND EXTRACT(YEAR FROM i.issued_at)::int=$3
		      AND i.document_number BETWEEN $5 AND $6
		  )
		  AND NOT EXISTS (
		    SELECT 1
		    FROM nfce_number_inutilizations n
		    WHERE n.tenant_id=$1
		      AND n.environment=$2
		      AND n.year=$3
		      AND n.model=65
		      AND n.series=$4
		      AND n.status IN ('signed','submitted','registered')
		      AND int8range(n.start_number, n.end_number, '[]')
		          && int8range($5, $6, '[]')
		  )
	`, tenantID, environment, year, series, startNumber, endNumber).Scan(&available)
	return available, err
}

func (r *FiscalRepo) GetNFCeInutilizationByRequestForUpdate(
	ctx context.Context,
	tx db.DBTX,
	tenantID, requestID string,
) (fisc.NFCeInutilization, []byte, error) {
	var out fisc.NFCeInutilization
	var signedXML []byte
	err := tx.QueryRow(ctx, `
		SELECT
		  id::text, tenant_id::text, environment, year, model, series,
		  start_number, end_number, request_id, status, justification,
		  signed_sha256, response_sha256, status_code, reason, protocol,
		  registered_at, created_at, updated_at, signed_xml
		FROM nfce_number_inutilizations
		WHERE tenant_id=$1 AND request_id=$2
		FOR UPDATE
	`, tenantID, requestID).Scan(
		&out.ID, &out.TenantID, &out.Environment, &out.Year, &out.Model,
		&out.Series, &out.StartNumber, &out.EndNumber, &out.RequestID,
		&out.Status, &out.Justification, &out.SignedSHA256,
		&out.ResponseSHA256, &out.StatusCode, &out.Reason, &out.Protocol,
		&out.RegisteredAt, &out.CreatedAt, &out.UpdatedAt, &signedXML,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeInutilization{}, nil, common.ErrNotFound
		}
		return fisc.NFCeInutilization{}, nil, err
	}
	return out, signedXML, nil
}

func (r *FiscalRepo) InsertSignedNFCeInutilization(
	ctx context.Context,
	tx db.DBTX,
	record fisc.NFCeInutilization,
	actorUserID string,
	signedXML []byte,
	sha256 string,
) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO nfce_number_inutilizations(
		  tenant_id, environment, issuer_uf, issuer_cnpj, year, model, series,
		  start_number, end_number, request_id, status, justification,
		  signed_xml, signed_sha256, created_by_user_id
		)
		VALUES ($1,$2,$3,$4,$5,65,$6,$7,$8,$9,'signed',$10,$11,$12,$13)
		RETURNING id::text
	`,
		record.TenantID, record.Environment,
		strings.ToUpper(strings.TrimSpace(record.IssuerUF)),
		strings.ToUpper(strings.TrimSpace(record.IssuerCNPJ)),
		record.Year, record.Series, record.StartNumber, record.EndNumber,
		record.RequestID, record.Justification, signedXML, sha256, actorUserID,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return "", common.ErrConflict
		}
		return "", err
	}
	return id, nil
}

func (r *FiscalRepo) MarkNFCeInutilizationSubmitted(
	ctx context.Context,
	tx db.DBTX,
	tenantID, requestID string,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE nfce_number_inutilizations
		SET status='submitted', updated_at=now()
		WHERE tenant_id=$1 AND request_id=$2 AND status='signed'
	`, tenantID, requestID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return common.ErrConflict
	}
	return nil
}

func (r *FiscalRepo) ApplyNFCeInutilizationResult(
	ctx context.Context,
	tx db.DBTX,
	tenantID, requestID string,
	result fisc.NFCeInutilizationRemoteResult,
	responseSHA256 string,
) error {
	if result.Pending() || len(result.ResponseXML) == 0 {
		return common.ErrValidation
	}
	if result.Registered() {
		tag, err := tx.Exec(ctx, `
			UPDATE nfce_number_inutilizations
			SET status='registered', response_xml=$3, response_sha256=$4,
			    status_code=$5, reason=$6, protocol=$7, registered_at=$8,
			    updated_at=now()
			WHERE tenant_id=$1 AND request_id=$2 AND status='submitted'
		`,
			tenantID, requestID, result.ResponseXML, responseSHA256,
			result.StatusCode, result.Reason, result.Protocol, result.RegisteredAt,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return common.ErrConflict
		}
		return nil
	}
	if result.Rejected() {
		tag, err := tx.Exec(ctx, `
			UPDATE nfce_number_inutilizations
			SET status='rejected', response_xml=$3, response_sha256=$4,
			    status_code=$5, reason=$6, protocol=NULL, registered_at=NULL,
			    updated_at=now()
			WHERE tenant_id=$1 AND request_id=$2 AND status='submitted'
		`,
			tenantID, requestID, result.ResponseXML, responseSHA256,
			result.StatusCode, result.Reason,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return common.ErrConflict
		}
		return nil
	}
	return common.ErrValidation
}

