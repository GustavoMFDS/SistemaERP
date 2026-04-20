package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tx represents a database transaction.
// pgx.Tx satisfies this interface.
type Tx interface {
	DBTX
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// UnitOfWork formalizes transaction boundaries.
// It is the single entrypoint to start a transaction in the application layer.
type UnitOfWork interface {
	Begin(ctx context.Context) (Tx, error)
}

type PgxUnitOfWork struct {
	pool *pgxpool.Pool
}

func NewPgxUnitOfWork(pool *pgxpool.Pool) *PgxUnitOfWork {
	return &PgxUnitOfWork{pool: pool}
}

func (u *PgxUnitOfWork) Begin(ctx context.Context) (Tx, error) {
	return u.pool.BeginTx(ctx, pgx.TxOptions{})
}
