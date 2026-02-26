package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	migrateiofs "github.com/golang-migrate/migrate/v4/source/iofs"

	// Register the pgx driver with database/sql; required by sql.Open
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Migration struct {
	// FS is the file system that contains all the SQL migration files
	FS fs.FS
}

func Migrate(ctx context.Context, dsn string, migrations []Migration) error {
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to create sql db: %w", err)
	}
	defer func() { _ = sqldb.Close() }()

	for _, migration := range migrations {
		if err := runMigration(ctx, sqldb, migration); err != nil {
			return err
		}
	}
	return nil
}

func runMigration(ctx context.Context, sqldb *sql.DB, migration Migration) error {
	// Use a dedicated connection for each module's migrations
	conn, err := sqldb.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	driver, err := migratepg.WithConnection(ctx, conn, &migratepg.Config{
		MigrationsTable: migratepg.DefaultMigrationsTable,
	})
	if err != nil {
		return fmt.Errorf("failed to create db migrate driver: %w", err)
	}

	src, err := migrateiofs.New(migration.FS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		_, _ = m.Close()
		return fmt.Errorf("failed to run migration: %w", err)
	}

	if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
		if srcErr != nil {
			return fmt.Errorf("failed to close migration source: %w", srcErr)
		}
		if dbErr != nil {
			return fmt.Errorf("failed to close migration database: %w", dbErr)
		}
	}
	return nil
}
