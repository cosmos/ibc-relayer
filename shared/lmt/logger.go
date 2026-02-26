package lmt

import (
	"context"
	"flag"

	"go.uber.org/zap"
)

var (
	logDev   = flag.Bool("log-dev", false, "use development logger (stacktraces and pretty-printing)")
	logLevel = zap.LevelFlag("log-level", zap.InfoLevel, "minimum enabled logging level (debug, info, warn, error, dpanic, panic, fatal)")
)

type loggerContextKey struct{}

func ConfigureLogger() {
	var config zap.Config
	if *logDev {
		config = zap.NewDevelopmentConfig()
	} else {
		config = zap.NewProductionConfig()
	}
	config.Level.SetLevel(*logLevel)
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}

	zap.ReplaceGlobals(logger)
}

func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func LoggerContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, zap.L())
}

func Logger(ctx context.Context) *zap.Logger {
	logger, ok := ctx.Value(loggerContextKey{}).(*zap.Logger)
	if !ok || logger == nil {
		return zap.L()
	}
	return logger
}
