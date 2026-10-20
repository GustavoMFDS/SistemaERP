package customers

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Customer struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

type CustomerInput struct {
	Name  string  `json:"name"`
	Email *string `json:"email"`
	Phone *string `json:"phone"`
}

type ListPage struct {
	Items  []Customer `json:"items"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
	Total  int        `json:"total"`
}

type Service struct {
	pool  *pgxpool.Pool
	audit *audit.Service
}

func New(pool *pgxpool.Pool, a *audit.Service) *Service {
	return &Service{pool: pool, audit: a}
}

func normalize(input CustomerInput) (CustomerInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if utf8.RuneCountInString(input.Name) < 2 ||
		utf8.RuneCountInString(input.Name) > 120 ||
		strings.ContainsAny(input.Name, "\r\n\x00") {
		return CustomerInput{}, common.ErrValidation
	}
	if input.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*input.Email))
		if len(email) > 254 || strings.ContainsAny(email, "\r\n\x00") {
			return CustomerInput{}, common.ErrValidation
		}
		if email == "" {
			input.Email = nil
		} else {
			addr, err := mail.ParseAddress(email)
			if err != nil || addr.Address != email {
				return CustomerInput{}, common.ErrValidation
			}
			input.Email = &email
		}
	}
	if input.Phone != nil {
		phone := strings.TrimSpace(*input.Phone)
		if len(phone) > 30 || strings.ContainsAny(phone, "\r\n\x00") {
			return CustomerInput{}, common.ErrValidation
		}
		if phone == "" {
			input.Phone = nil
		} else {
			input.Phone = &phone
		}
	}
	return input, nil
}

// requireCustomerWrite repeats the RBAC check inside the write transaction.
// It prevents accidental bypass by non-HTTP/internal callers and rejects
// suspended memberships even if the request began before suspension.
func requireCustomerWrite(ctx context.Context, tx pgx.Tx, tenantID, actorID string) error {
	var allowed bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM user_tenants ut
			JOIN users u ON u.id=ut.user_id AND u.active=true
			JOIN user_tenant_roles ur ON ur.tenant_id=ut.tenant_id AND ur.user_id=ut.user_id
			JOIN role_permissions rp ON rp.role_id=ur.role_id
			JOIN permissions p ON p.id=rp.permission_id AND p.code='customer:write'
			WHERE ut.tenant_id=$1 AND ut.user_id=$2 AND ut.active=true
		)
	`, tenantID, actorID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return common.ErrForbidden
	}
	return nil
}

func (s *Service) List(ctx context.Context, tenantID, query string, limit, offset int) (ListPage, error) {
	query = strings.TrimSpace(query)
	if limit < 1 || limit > 50 || offset < 0 || offset > 5000 || len(query) > 100 {
		return ListPage{}, common.ErrValidation
	}
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM customers
		WHERE tenant_id=$1 AND ($2='' OR strpos(lower(name), lower($2)) > 0)
	`, tenantID, query).Scan(&total); err != nil {
		return ListPage{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, email::text, phone
		FROM customers
		WHERE tenant_id=$1 AND ($2='' OR strpos(lower(name), lower($2)) > 0)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`, tenantID, query, limit, offset)
	if err != nil {
		return ListPage{}, err
	}
	defer rows.Close()
	items := make([]Customer, 0)
	for rows.Next() {
		var c Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone); err != nil {
			return ListPage{}, err
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, err
	}
	return ListPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Create(ctx context.Context, tenantID, actorID string, input CustomerInput,
	requestID, ip, userAgent string,
) (Customer, error) {
	input, err := normalize(input)
	if err != nil {
		return Customer{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Customer{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireCustomerWrite(ctx, tx, tenantID, actorID); err != nil {
		return Customer{}, err
	}
	var item Customer
	err = tx.QueryRow(ctx, `
		INSERT INTO customers(tenant_id,name,email,phone)
		VALUES($1,$2,$3,$4)
		RETURNING id::text,name,email::text,phone
	`, tenantID, input.Name, input.Email, input.Phone).
		Scan(&item.ID, &item.Name, &item.Email, &item.Phone)
	if err != nil {
		return Customer{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID,
		Action: "customer.create", ResourceType: "customer", ResourceID: item.ID,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
	}); err != nil {
		return Customer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Customer{}, err
	}
	return item, nil
}

func (s *Service) Update(ctx context.Context, tenantID, actorID, customerID string,
	input CustomerInput, requestID, ip, userAgent string,
) (Customer, error) {
	if _, err := uuid.Parse(customerID); err != nil {
		return Customer{}, common.ErrValidation
	}
	input, err := normalize(input)
	if err != nil {
		return Customer{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Customer{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireCustomerWrite(ctx, tx, tenantID, actorID); err != nil {
		return Customer{}, err
	}
	var item Customer
	err = tx.QueryRow(ctx, `
		UPDATE customers SET name=$3,email=$4,phone=$5,updated_at=now()
		WHERE tenant_id=$1 AND id=$2
		RETURNING id::text,name,email::text,phone
	`, tenantID, customerID, input.Name, input.Email, input.Phone).
		Scan(&item.ID, &item.Name, &item.Email, &item.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, common.ErrNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Customer{}, common.ErrConflict
		}
		return Customer{}, err
	}
	if err := s.audit.RecordTx(ctx, tx, audit.Event{
		TenantID: tenantID, ActorUserID: actorID,
		Action: "customer.update", ResourceType: "customer", ResourceID: item.ID,
		RequestID: requestID, IP: ip, UserAgent: userAgent,
	}); err != nil {
		return Customer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Customer{}, err
	}
	return item, nil
}
