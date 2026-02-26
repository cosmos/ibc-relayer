package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os/signal"
	"syscall"

	"github.com/cosmos/platform-relayer/proto/gen/relayerapi"
	"github.com/cosmos/platform-relayer/shared/lmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var sourceChainID = flag.String(
	"source-chain-id",
	"",
	"source chain id",
)

var txHash = flag.String(
	"tx-hash",
	"",
	"the tx hash to relay",
)

var relayerGRPCURL = flag.String(
	"relayer-grpc-url",
	"relayer-grpc.dev.skip-internal.money:443",
	"url of the grpc endpoint for the relayer to relay this transfer (must be on tailscale for access)",
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	lmt.ConfigureLogger()
	ctx = lmt.LoggerContext(ctx)

	if *sourceChainID == "" || *txHash == "" {
		lmt.Logger(ctx).Fatal("source-chain-id and tx-hash are required")
	}

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	})))
	conn, err := grpc.NewClient(*relayerGRPCURL, opts...)
	if err != nil {
		lmt.Logger(ctx).Fatal("creating grpc connection to relayer", zap.String("relayer_grpc_url", *relayerGRPCURL), zap.Error(err))
	}
	defer conn.Close()

	relayer := relayerapi.NewRelayerApiServiceClient(conn)

	req := &relayerapi.RelayRequest{
		TxHash:  *txHash,
		ChainId: *sourceChainID,
	}

	if _, err = relayer.Relay(ctx, req); err != nil {
		lmt.Logger(ctx).Fatal("submitting relay request",
			zap.String("chain_id", *sourceChainID),
			zap.String("tx_hash", *txHash),
			zap.Error(err),
		)
	}

	lmt.Logger(ctx).Info("successfully submitted relay request",
		zap.String("chain_id", *sourceChainID),
		zap.String("tx_hash", *txHash),
	)
}
