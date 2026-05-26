package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/cosmos/ibc-relayer/db"
)

// defaultTestDSN matches the Postgres instance brought up by docker-compose.yml.
const defaultTestDSN = "postgres://relayer:relayer@localhost:42500/relayer?sslmode=disable"

// TestMigrate exercises the embedded migration runner against a real Postgres.
// `make test` brings the database up before invoking `go test`, so this test
// runs in CI. When Postgres is not reachable (e.g. a developer running
// `go test ./...` without the compose stack), the test skips rather than fail.
func TestMigrate(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		dsn = defaultTestDSN
	}

	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("sql.Open failed; skipping integration test: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sqldb.PingContext(pingCtx); err != nil {
		t.Skipf("Postgres at %q not reachable; skipping integration test: %v", dsn, err)
	}

	ctx := context.Background()
	if err := db.Migrate(ctx, sqldb, []db.Migration{db.Migrations}); err != nil {
		t.Fatalf("first Migrate call failed: %v", err)
	}

	// Idempotency: a second invocation against an up-to-date schema must succeed
	// (golang-migrate's ErrNoChange is handled inside db.Migrate). This is the
	// load-bearing property for relayer startup in multi-replica deployments.
	if err := db.Migrate(ctx, sqldb, []db.Migration{db.Migrations}); err != nil {
		t.Fatalf("second Migrate call (idempotency) failed: %v", err)
	}

	// Verify both that the version-tracking table exists and that at least one
	// table the migrations actually create is present — guards against a future
	// refactor where db.Migrate returns nil without having applied anything
	// (e.g. wrong embed.FS path silently empty).
	var version int64
	var dirty bool
	if err := sqldb.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("querying schema_migrations: %v", err)
	}
	if dirty {
		t.Fatalf("schema_migrations reports dirty=true at version %d", version)
	}

	var rawTxsExists bool
	if err := sqldb.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'raw_txs')",
	).Scan(&rawTxsExists); err != nil {
		t.Fatalf("checking raw_txs existence: %v", err)
	}
	if !rawTxsExists {
		t.Fatal("expected raw_txs table to exist after Migrate (first migration creates it)")
	}
}
