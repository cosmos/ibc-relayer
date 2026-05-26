package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cosmos/ibc-relayer/db"
	"github.com/cosmos/ibc-relayer/shared/database"
)

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sqldb := database.NewMigrationDB(pool)
	defer func() { _ = sqldb.Close() }()
	return db.Migrate(ctx, sqldb, []db.Migration{db.Migrations})
}
