package database

import (
	"context"
	"database/sql"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	rdsauth "github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

func NewDatabase(ctx context.Context, connString string, useIAMAuth bool, iamAuthRegion string) (*pgxpool.Pool, error) {
	pgxConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	if useIAMAuth {
		pgxConfig.BeforeConnect = func(ctx context.Context, connConfig *pgx.ConnConfig) error {
			cfg, err := awsconfig.LoadDefaultConfig(context.Background())
			if err != nil {
				return fmt.Errorf("failed to load AWS config: %w", err)
			}

			region := iamAuthRegion
			if region == "" {
				region = cfg.Region
			}
			if region == "" {
				return fmt.Errorf("no AWS region configured: set postgres.iam_auth_region or AWS_REGION")
			}

			authToken, err := rdsauth.BuildAuthToken(context.Background(), fmt.Sprintf("%s:%d", connConfig.Host, connConfig.Port), region, connConfig.User, cfg.Credentials)
			if err != nil {
				return fmt.Errorf("failed to build auth token: %w", err)
			}

			connConfig.Password = authToken

			return nil
		}
	}

	return pgxpool.NewWithConfig(ctx, pgxConfig)
}

func NewMigrationDB(pool *pgxpool.Pool) *sql.DB {
	return stdlib.OpenDBFromPool(pool)
}
