package tx

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cosmos/ibc-relayer/db/gen/db"
)

type Txer interface {
	ExecTx(ctx context.Context, fn func(q *db.Queries) error) error
	db.Queries
}

type Tx struct {
	*db.Queries
	pool *pgxpool.Pool
}

func New(queries *db.Queries, pool *pgxpool.Pool) Tx {
	return Tx{Queries: queries, pool: pool}
}

func (txer Tx) ExecTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := txer.pool.Begin(ctx)
	if err != nil {
		return err
	}

	q := db.New(tx)

	err = fn(q)
	if err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx err: %w, rb err: %w", err, rbErr)
		}
		return err
	}

	return tx.Commit(ctx)
}
