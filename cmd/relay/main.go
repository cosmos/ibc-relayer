package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/cosmos/ibc-relayer/proto/gen/relayerapi"
	"github.com/cosmos/ibc-relayer/shared/lmt"
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
	"",
	"url of the grpc endpoint for the relayer to relay this transfer",
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	lmt.ConfigureLogger()
	ctx = lmt.LoggerContext(ctx)

	if *sourceChainID == "" || *txHash == "" || *relayerGRPCURL == "" {
		lmt.Logger(ctx).Fatal("source-chain-id, tx-hash, and relayer-grpc-url are required")
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // CLI tool; user-supplied endpoint
			MinVersion:         tls.VersionTLS13,
		})),
	}
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
