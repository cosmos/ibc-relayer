package health

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type HealthService struct {
	*health.Server

	pool *pgxpool.Pool
}

func NewHealthService(pool *pgxpool.Pool) *HealthService {
	return &HealthService{
		Server: health.NewServer(),
		pool:   pool,
	}
}

func (s *HealthService) Check(ctx context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	response, err := s.Server.Check(ctx, request)
	if err != nil || response.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return response, err
	}

	if err := s.pool.Ping(ctx); err != nil {
		return nil, err
	}

	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}
