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
		ReadHeaderTimeout: 10 * time.Second,
	}

	server.RegisterOnShutdown(func() {
		lmt.Logger(ctx).Info(
			"Shutting down Prometheus server",
			zap.String("addr", fmt.Sprintf("http://%s", addr)), //nolint:revive // local prometheus listener for logging
		)
	})

	go func() {
		<-ctx.Done()

		shutdownTimeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownTimeoutCtx); err != nil {
			lmt.Logger(ctx).Error(
				"Failed to shutdown the Prometheus server",
				zap.String("addr", fmt.Sprintf("http://%s", addr)), //nolint:revive // local prometheus listener for logging
				zap.Error(err),
			)
		}
	}()

	lmt.Logger(ctx).Info("Starting Prometheus server", zap.String("addr", fmt.Sprintf("http://%s", addr))) //nolint:revive // local prometheus listener for logging
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		lmt.Logger(ctx).Error(
			"Prometheus server error",
			zap.String("addr", fmt.Sprintf("http://%s", addr)), //nolint:revive // local prometheus listener for logging
			zap.Error(err),
		)
		return err
	}

	return nil
}
