package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	rdsauth "github.com/aws/aws-sdk-go-v2/feature/rds/auth"

	"github.com/cosmos/ibc-relayer/db"
	"github.com/cosmos/ibc-relayer/shared/config"
)

// consumeMigrateSubcommand returns true and strips the first argument when the
// binary is invoked as `relayer migrate ...`, letting the remaining args flow
// through the normal flag parser.
func consumeMigrateSubcommand() bool {
	if len(os.Args) < 2 || os.Args[1] != "migrate" {
		return false
	}
	os.Args = append(os.Args[:1], os.Args[2:]...)
	return true
}

func runMigrations(ctx context.Context, cfg config.Config) error {
	dsn, err := migrationDSN(ctx, cfg)
	if err != nil {
		return fmt.Errorf("building migration dsn: %w", err)
	}
	return db.Migrate(ctx, dsn, []db.Migration{db.Migrations})
}

func migrationDSN(ctx context.Context, cfg config.Config) (string, error) {
	user, ok := os.LookupEnv("POSTGRES_USER")
	if !ok {
		user = "relayer"
	}

	password, sslmode, err := migrationCredentials(ctx, cfg, user)
	if err != nil {
		return "", err
	}

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", cfg.Postgres.Hostname, cfg.Postgres.Port),
		Path:   cfg.Postgres.Database,
	}
	if sslmode != "" {
		u.RawQuery = "sslmode=" + sslmode
	}
	return u.String(), nil
}

func migrationCredentials(ctx context.Context, cfg config.Config, user string) (password, sslmode string, err error) {
	if !cfg.Postgres.IAMAuthEnabled {
		pwd, ok := os.LookupEnv("POSTGRES_PASSWORD")
		if !ok {
			pwd = "relayer"
		}
		// Leave sslmode unset so the migration connection follows the pgx default,
		// matching the runtime pool (avoids silently disabling SSL where the DB
		// enforces it).
		return pwd, "", nil
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", "", fmt.Errorf("loading aws config: %w", err)
	}
	region := cfg.Postgres.IAMAuthRegion
	if region == "" {
		region = awsCfg.Region
	}
	if region == "" {
		return "", "", fmt.Errorf("no AWS region configured: set postgres.iam_auth_region or AWS_REGION")
	}
	endpoint := fmt.Sprintf("%s:%s", cfg.Postgres.Hostname, cfg.Postgres.Port)
	token, err := rdsauth.BuildAuthToken(ctx, endpoint, region, user, awsCfg.Credentials)
	if err != nil {
		return "", "", fmt.Errorf("building rds auth token: %w", err)
	}
	return token, "require", nil
}
