package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/shared/lmt"
)

func StartPrometheus(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr: addr,
		Handler: promhttp.InstrumentMetricHandler(
			prom.DefaultRegisterer, promhttp.HandlerFor(
				prom.DefaultGatherer,
				promhttp.HandlerOpts{},
			)),
	}

	server.RegisterOnShutdown(func() {
		lmt.Logger(ctx).Info(
			"Shutting down Prometheus server",
			zap.String("addr", fmt.Sprintf("http://%s", addr)),
		)
	})

	go func() {
		<-ctx.Done()

		shutdownTimeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownTimeoutCtx); err != nil {
			lmt.Logger(ctx).Error(
				"Failed to shutdown the Prometheus server",
				zap.String("addr", fmt.Sprintf("http://%s", addr)),
				zap.Error(err),
			)
		}
	}()

	lmt.Logger(ctx).Info("Starting Prometheus server", zap.String("addr", fmt.Sprintf("http://%s", addr)))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		lmt.Logger(ctx).Error(
			"Prometheus server error",
			zap.String("addr", fmt.Sprintf("http://%s", addr)),
			zap.Error(err),
		)
		return err
	}

	return nil
}
