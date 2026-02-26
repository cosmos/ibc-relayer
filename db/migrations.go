package db

import "embed"

//go:embed migrations/*.sql
var migrationsFS embed.FS

var Migrations = Migration{
	FS: migrationsFS,
}
