package bootstrapadmin

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const placeholderCNPJ = "00000000000000"

// Input contains the business and account identity for the one-time install.
// Password is deliberately passed separately and is never included in SQL or logs.
type Input struct {
	LegalName  string
	TradeName  string
	CNPJ       string
	AdminName  string
	AdminEmail string
}

func Validate(input Input, password string) error {
	if input.LegalName != strings.TrimSpace(input.LegalName) || utf8.RuneCountInString(input.LegalName) < 2 ||
		utf8.RuneCountInString(input.LegalName) > 160 || strings.ContainsAny(input.LegalName, "\r\n\x00") {
		return errors.New("invalid company legal name")
	}
	if input.TradeName != "" && (utf8.RuneCountInString(input.TradeName) > 120 || strings.ContainsAny(input.TradeName, "\r\n\x00")) {
		return errors.New("invalid company trade name")
	}
	if !validCNPJ(input.CNPJ) {
		return errors.New("invalid CNPJ")
	}
	if input.AdminName != strings.TrimSpace(input.AdminName) || utf8.RuneCountInString(input.AdminName) < 2 ||
		utf8.RuneCountInString(input.AdminName) > 120 || strings.ContainsAny(input.AdminName, "\r\n\x00") {
		return errors.New("invalid administrator name")
	}
	if len(input.AdminEmail) < 5 || len(input.AdminEmail) > 254 || strings.ContainsAny(input.AdminEmail, " \t\r\n\x00") {
		return errors.New("invalid administrator email")
	}
	address, err := mail.ParseAddress(input.AdminEmail)
	if err != nil || address.Address != input.AdminEmail {
		return errors.New("invalid administrator email")
	}
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 || strings.TrimSpace(password) == "" {
		return errors.New("password must be 12-72 UTF-8 bytes and at least 12 characters")
	}
	return nil
}

func validCNPJ(value string) bool {
	if len(value) != 14 {
		return false
	}
	allEqual := true
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
		if value[i] != value[0] {
			allEqual = false
		}
	}
	if allEqual {
		return false
	}
	return cnpjDigit(value[:12], []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == int(value[12]-'0') &&
		cnpjDigit(value[:13], []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}) == int(value[13]-'0')
}

func cnpjDigit(value string, weights []int) int {
	sum := 0
	for i := range value {
		sum += int(value[i]-'0') * weights[i]
	}
	rem := sum % 11
	if rem < 2 {
		return 0
	}
	return 11 - rem
}

// Create performs a single-use first-admin bootstrap against the fresh schema
// produced by Esterium's migrations and production-safe role seed. It refuses
// databases with users, a non-placeholder tenant, or any business data.
func Create(ctx context.Context, pool *pgxpool.Pool, input Input, password string) error {
	if pool == nil {
		return errors.New("database pool is required")
	}
	if err := Validate(input, password); err != nil {
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("could not hash administrator password")
	}
	defer func() {
		for i := range passwordHash {
			passwordHash[i] = 0
		}
	}()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return errors.New("could not begin first-admin transaction")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize competing setup attempts, including on separate installer processes.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(5139254469240086861)`); err != nil {
		return errors.New("could not lock first-admin setup")
	}
	var databaseName string
	var superuser, canCreateRole, canCreateDatabase, replication, bypassRLS bool
	if err := tx.QueryRow(ctx, `
		SELECT current_database(), r.rolsuper, r.rolcreaterole, r.rolcreatedb,
		       r.rolreplication, r.rolbypassrls
		FROM pg_roles r WHERE r.rolname=current_user
	`).Scan(&databaseName, &superuser, &canCreateRole, &canCreateDatabase, &replication, &bypassRLS); err != nil ||
		databaseName == "postgres" || strings.HasPrefix(databaseName, "template") ||
		superuser || canCreateRole || canCreateDatabase || replication || bypassRLS {
		return errors.New("first-admin setup requires the restricted application database role")
	}

	var companyCount, placeholderCount int
	err = tx.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE cnpj=$1)
		FROM companies
	`, placeholderCNPJ).Scan(&companyCount, &placeholderCount)
	if err != nil || companyCount != 1 || placeholderCount != 1 {
		return errors.New("database is not a fresh single-tenant installation")
	}
	var companyID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM companies WHERE cnpj=$1 FOR UPDATE`, placeholderCNPJ).Scan(&companyID); err != nil || companyID == "" {
		return errors.New("database is not a fresh single-tenant installation")
	}

	var usersExist, membershipsExist bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&usersExist); err != nil || usersExist {
		return errors.New("first-admin setup is only allowed before any user exists")
	}
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM user_tenants) OR
		       EXISTS(SELECT 1 FROM user_roles) OR
		       EXISTS(SELECT 1 FROM user_tenant_roles)
	`).Scan(&membershipsExist); err != nil || membershipsExist {
		return errors.New("first-admin setup found pre-existing user membership data")
	}

	// Only migration metadata and reference data may be populated at this point.
	rows, err := tx.Query(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname='public'
		  AND tablename NOT IN ('schema_migrations','companies','users','user_tenants',
		      'user_roles','user_tenant_roles','roles','permissions','role_permissions')
		ORDER BY tablename
	`)
	if err != nil {
		return errors.New("could not inspect fresh database state")
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			return errors.New("could not inspect fresh database state")
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return errors.New("could not inspect fresh database state")
	}
	rows.Close()
	for _, table := range tables {
		var hasRows bool
		query := `SELECT EXISTS(SELECT 1 FROM ` + pgx.Identifier{table}.Sanitize() + `)`
		if err := tx.QueryRow(ctx, query).Scan(&hasRows); err != nil || hasRows {
			return errors.New("database contains business data; first-admin setup refused")
		}
	}

	var adminRoleID string
	var permissionCount int
	err = tx.QueryRow(ctx, `
		SELECT r.id::text, count(rp.permission_id)
		FROM roles r
		LEFT JOIN role_permissions rp ON rp.role_id=r.id
		WHERE r.name='admin'
		GROUP BY r.id
	`).Scan(&adminRoleID, &permissionCount)
	if err != nil || adminRoleID == "" || permissionCount == 0 {
		return errors.New("production-safe administrator role seed is missing")
	}

	var duplicateCNPJ bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM companies WHERE cnpj=$1 AND id<>$2)`, input.CNPJ, companyID).Scan(&duplicateCNPJ); err != nil || duplicateCNPJ {
		return errors.New("company CNPJ is already in use")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE companies SET legal_name=$1, trade_name=NULLIF($2,''), cnpj=$3, updated_at=now()
		WHERE id=$4 AND cnpj=$5
	`, input.LegalName, input.TradeName, input.CNPJ, companyID, placeholderCNPJ); err != nil {
		return errors.New("could not initialize company identity")
	}

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users(email,name,password_hash,active)
		VALUES($1,$2,$3,true)
		RETURNING id::text
	`, input.AdminEmail, input.AdminName, string(passwordHash)).Scan(&userID)
	if err != nil {
		return errors.New("could not create first administrator")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_tenants(user_id,tenant_id) VALUES($1,$2)`, userID, companyID); err != nil {
		return errors.New("could not link administrator to company")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, userID, adminRoleID); err != nil {
		return errors.New("could not assign administrator role")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_tenant_roles(user_id,tenant_id,role_id) VALUES($1,$2,$3)`, userID, companyID, adminRoleID); err != nil {
		return errors.New("could not assign tenant administrator role")
	}
	auditService := audit.New(pool, nil)
	if err := auditService.RecordTx(ctx, tx, audit.Event{
		TenantID: companyID, ActorUserID: userID,
		Action: "install.first_admin.create", ResourceType: "user", ResourceID: userID,
		Metadata: map[string]any{"bootstrap": true, "role": "admin"},
	}); err != nil {
		return errors.New("could not record first-admin audit event")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("first-admin transaction was not committed")
	}
	return nil
}
