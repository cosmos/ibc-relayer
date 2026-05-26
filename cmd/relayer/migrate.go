package main

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cosmos/ibc-relayer/db"
	"github.com/cosmos/ibc-relayer/shared/database"
)

// isMigrateSubcommand reports whether the binary was invoked as `relayer migrate ...`.
func isMigrateSubcommand() bool {
	return len(os.Args) > 1 && os.Args[1] == "migrate"
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sqldb := database.NewMigrationDB(pool)
	defer func() { _ = sqldb.Close() }()
	return db.Migrate(ctx, sqldb, []db.Migration{db.Migrations})
}
