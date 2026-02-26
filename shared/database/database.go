package database

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	rdsauth "github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDatabase(ctx context.Context, connString string, useIAMAuth bool) (*pgxpool.Pool, error) {
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

			authToken, err := rdsauth.BuildAuthToken(context.Background(), fmt.Sprintf("%s:%d", connConfig.Host, connConfig.Port), "us-east-2", connConfig.User, cfg.Credentials)
			if err != nil {
				return fmt.Errorf("failed to build auth token: %w", err)
			}

			connConfig.Password = authToken

			return nil
		}
	}

	return pgxpool.NewWithConfig(ctx, pgxConfig)
}
