package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	genrelayservice "github.com/cosmos/ibc-relayer/proto/gen/relayerapi"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/relayerapi/services/health"
	"github.com/cosmos/ibc-relayer/relayerapi/services/relayerapi"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/cosmos/ibc-relayer/shared/metrics"
)

type RelayerGRPCServer struct {
	server            *grpc.Server
	healthcheckServer *http.Server
	address           string
}

func NewRelayerGRPCServer(ctx context.Context, pool *pgxpool.Pool, bridgeClientManager ibcv2.BridgeClientManager, address string) (*RelayerGRPCServer, error) {
	metricsInterceptor := metrics.UnaryServerInterceptor(ctx)
	configInterceptor := config.UnaryServerInterceptor(config.GetConfigReader(ctx))

	healthService := health.NewHealthService(pool)
	server := grpc.NewServer(grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(metricsInterceptor, configInterceptor)))
	genrelayservice.RegisterRelayerApiServiceServer(
		server,
		relayerapi.NewRelayerAPIService(
			ctx,
			db.New(pool),
			bridgeClientManager,
		),
	)
	grpc_health_v1.RegisterHealthServer(server, healthService)
	reflection.Register(server)

	healthcheckServer := newHealthcheckServer(ctx, healthService)

	return &RelayerGRPCServer{server: server, healthcheckServer: healthcheckServer, address: address}, nil
}

func (s *RelayerGRPCServer) Start(ctx context.Context) {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		lmt.Logger(ctx).Fatal("failed to create relayer gRPC server", zap.String("address", s.address), zap.Error(err))
	}

	go func() {
		<-ctx.Done()
		lmt.Logger(ctx).Info("shutting down relayer gRPC server")
		s.server.GracefulStop()
	}()

	mux := cmux.New(listener)

	grpcL := mux.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	httpL := mux.Match(cmux.HTTP1Fast())

	lmt.Logger(ctx).Info(
		"relayer gRPC server is listening",
		zap.String("address", fmt.Sprintf("http://%s", listener.Addr())),
	)

	go func() {
		if err := s.server.Serve(grpcL); err != nil {
			lmt.Logger(ctx).Error("error serving relayer gRPC server", zap.Error(err))
		}
	}()

	go func() {
		if err := s.healthcheckServer.Serve(httpL); err != nil {
			lmt.Logger(ctx).Error("error serving relayer healthcheck server", zap.Error(err))
		}
	}()

	if err := mux.Serve(); err != nil {
		lmt.Logger(ctx).Error("error serving grpc + healthcheck multiplexer", zap.Error(err))
	}

	lmt.Logger(ctx).Info("relayer gRPC server was terminated")
}

func newHealthcheckServer(ctx context.Context, healthService *health.HealthService) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		res, err := healthService.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if res.Status == grpc_health_v1.HealthCheckResponse_SERVING {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("OK"))
			if err != nil {
				lmt.Logger(ctx).Error("error writing healthcheck response", zap.Error(err))
			}
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, err := w.Write([]byte("Service Unavailable"))
			if err != nil {
				lmt.Logger(ctx).Error("error writing healthcheck response", zap.Error(err))
			}
		}
	})
	healthcheckServer := http.Server{Handler: mux}

	return &healthcheckServer
}
