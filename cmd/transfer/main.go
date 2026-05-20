package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/shared/lmt"
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	lmt.ConfigureLogger()
	ctx = lmt.LoggerContext(ctx)

	if len(flag.Args()) < 1 {
		lmt.Logger(ctx).Fatal("too few arguments, expected 'transfer transfer_type' (ics20 or ift)")
	}
	transferType := flag.Arg(0)
	switch transferType {
	case "ics20":
		if err := ics20Transfer(ctx); err != nil {
			lmt.Logger(ctx).Fatal("error sending ics20 transfer", zap.Error(err))
		}
	case "ift":
		if err := iftTransfer(ctx); err != nil {
			lmt.Logger(ctx).Fatal("error sending ift transfer", zap.Error(err))
		}
	default:
		lmt.Logger(ctx).Fatal("unexpected transfer type", zap.String("got", transferType), zap.Any("expected", []string{"ics20", "ift"}))
	}
}
