package repo

import "github.com/jackc/pgx/v5/pgxpool"

type Repositories struct {
	Users     *UsersRepo
	Products  *ProductsRepo
	Inventory *InventoryRepo
	Sales     *SalesRepo
	Finance   *FinanceRepo
	Fiscal    *FiscalRepo
	Cash      *CashRepo
	Audit     *AuditRepo
}

func New(db *pgxpool.Pool) *Repositories {
	return &Repositories{
		Users:     NewUsersRepo(db),
		Products:  NewProductsRepo(db),
		Inventory: NewInventoryRepo(db),
		Sales:     NewSalesRepo(db),
		Finance:   NewFinanceRepo(db),
		Fiscal:    NewFiscalRepo(db),
		Cash:      NewCashRepo(db),
		Audit:     NewAuditRepo(db),
	}
}
