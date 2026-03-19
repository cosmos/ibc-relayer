package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"math/big"
	"slices"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/cosmos/ibc-relayer/proto/gen/relayerapi"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var destChainID = flag.String(
	"dest-chain-id",
	"",
	"destination chain to send the transfer to",
)

var sourceClientID = flag.String(
	"source-client-id",
	"",
	"client id on the source chain to initiate the transfer from",
)

var denom = flag.String(
	"denom",
	"",
	"the denom to transfer",
)

var memo = flag.String(
	"memo",
	"",
	"an additional memo to send with the transfer",
)

var receiver = flag.String(
	"receiver",
	"",
	"receiver of the transfer",
)

var amount = flag.Uint64(
	"amount",
	1,
	"amount of denom to transfer",
)

var cfgPath = flag.String(
	"config",
	"./config/local/config.yml",
	"path to the config file to use",
)

var relayerGRPCURL = flag.String(
	"relayer-grpc-url",
	"relayer-grpc.dev.skip-internal.money:443",
	"url of the grpc endpoint for the relayer to relay this transfer (must be on tailscale for access)",
)

var privateKey = flag.String(
	"private-key",
	"",
	"private key of the wallet that will send the transfer",
)

var sourceChainID = flag.String(
	"source-chain-id",
	"",
	"the source chain id to make the transfer from",
)

func ibcv2Transfer(ctx context.Context) error {
	flag.Parse()

	lmt.Logger(ctx).Info("config path", zap.String("path", *cfgPath))
	lmt.Logger(ctx).Info("private key", zap.String("private key", *privateKey))
	lmt.Logger(ctx).Info("receiver", zap.String("receiver", *receiver))
	lmt.Logger(ctx).Info("denom", zap.String("denom", *denom))
	lmt.Logger(ctx).Info("source chain id", zap.String("source chain id", *sourceChainID))
	lmt.Logger(ctx).Info("dest chain id", zap.String("dest chain id", *destChainID))

	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	ctx = config.ConfigReaderContext(ctx, config.NewConfigReader(cfg))

	logger := lmt.Logger(ctx).With(zap.String("source_chain_id", *sourceChainID), zap.String("dest_chain_id", *destChainID))
	ctx = lmt.WithLogger(ctx, logger)

	// validate source chain
	sourceChain, err := config.GetConfigReader(ctx).GetChainConfig(*sourceChainID)
	if err != nil {
		return fmt.Errorf("loading source chain config: %w", err)
	}
	if !slices.Contains(sourceChain.SupportedBridges, config.BridgeType_IBCV2) {
		return fmt.Errorf("source chain does not support ibcv2")
	}
	if sourceChain.EVM == nil {
		return fmt.Errorf("only evm type source chains are supported")
	}
	// if sourceChain.ChainID != "1" {
	//	return fmt.Errorf("only ethereum mainnet is supported as a source chain")
	//}

	// validate dest chain
	destChain, err := config.GetConfigReader(ctx).GetChainConfig(*destChainID)
	if err != nil {
		return fmt.Errorf("loading dest chain config: %w", err)
	}
	if !slices.Contains(destChain.SupportedBridges, config.BridgeType_IBCV2) {
		return fmt.Errorf("dest chain does not support ibcv2")
	}
	if destChain.Cosmos == nil {
		return fmt.Errorf("only cosmos type dest chains are supported")
	}
	// if destChain.ChainID != "cosmoshub-4" {
	//	return fmt.Errorf("only cosmoshub is supported as a dest chain")
	//}

	// validate receiver is bech32
	if _, _, err = bech32.DecodeAndConvert(*receiver); err != nil {
		return fmt.Errorf("receiver %s is not a valid bech32 address: %w", *receiver, err)
	}

	// validate denom
	if !common.IsHexAddress(*denom) {
		return fmt.Errorf("denom %s is not a valid hex address", *denom)
	}

	// validate amount
	if *amount <= 0 {
		return fmt.Errorf("amount must be positive %d", *amount)
	}
	amt := new(big.Int).SetUint64(*amount)

	// validate clients and chain
	sourceChainIBCV2Config, err := config.GetConfigReader(ctx).GetIBCV2Config(*sourceChainID)
	if err != nil {
		return fmt.Errorf("loading source chain ibcv2 config: %w", err)
	}
	clientCounterpartyChainID, ok := sourceChainIBCV2Config.CounterpartyChains[*sourceClientID]
	if !ok {
		return fmt.Errorf("could not determine counterparty chain id for source client %s (mapping for counterparty client is likely missing for chains ibcv2 config)", *sourceClientID)
	}
	if clientCounterpartyChainID != *destChainID {
		return fmt.Errorf("source client (%s) counterparty chain id (%s) does not match the dest chain id (%s)", *sourceClientID, clientCounterpartyChainID, *destChainID)
	}

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13})))
	conn, err := grpc.NewClient(*relayerGRPCURL, opts...)
	if err != nil {
		return fmt.Errorf("creating grpc connection to relayer at %s: %w", *relayerGRPCURL, err)
	}
	defer conn.Close()
	relayer := relayerapi.NewRelayerApiServiceClient(conn)

	clientManager, err := ibcv2.NewClientManagerFromConfig(ctx, map[string]string{*sourceChainID: *privateKey}, nil, "", "", sourceChain)
	if err != nil {
		return fmt.Errorf("creating ibcv2 client manager: %w", err)
	}

	sourceClient, err := clientManager.GetClient(ctx, *sourceChainID)
	if err != nil {
		return fmt.Errorf("getting ibcv2 client for source chain id %s: %w", *sourceChainID, err)
	}

	lmt.Logger(ctx).Info(
		fmt.Sprintf("sending ibcv2 transfer from %s to %s", *sourceChainID, *destChainID),
		zap.String("denom", *denom),
		zap.String("receiver", *receiver),
		zap.String("amount", amt.String()),
		zap.String("memo", *memo),
	)

	sendTxHash, err := sourceClient.SendTransfer(ctx, *sourceClientID, *denom, *receiver, amt, *memo)
	if err != nil {
		return fmt.Errorf("sending ibcv2 transfer from %s to %s: %w", *sourceChainID, *destChainID, err)
	}

	lmt.Logger(ctx).Info(
		fmt.Sprintf("successfully sent ibcv2 transfer from %s to %s", *sourceChainID, *destChainID),
		zap.String("tx_hash", sendTxHash),
	)

	packets, err := sourceClient.SendPacketsFromTx(ctx, *sourceChainID, sendTxHash)
	if err != nil {
		return fmt.Errorf("getting send packets from tx %s on source chain %s: %w", sendTxHash, *sourceChainID, err)
	}
	if len(packets) != 1 {
		return fmt.Errorf("unexpected number of send packets in tx %s on chain %s: expected %d but got %d", sendTxHash, *sourceChainID, 1, len(packets))
	}
	packet := packets[0]

	relayRequest := relayerapi.RelayRequest{
		TxHash:  sendTxHash,
		ChainId: *sourceChainID,
	}

	ctx = lmt.WithLogger(ctx, lmt.Logger(ctx).With(
		zap.String("tx_hash", sendTxHash),
		zap.Time("tx_time", packet.Timestamp),
		zap.Uint64("packet_sequence_number", packet.Sequence),
		zap.String("packet_source_client_id", packet.SourceClient),
		zap.String("packet_destination_client_id", packet.DestinationClient),
		zap.Time("packet_timeout_timestamp", packet.TimeoutTimestamp),
	))
	lmt.Logger(ctx).Info(
		fmt.Sprintf("submitting packet to be relayed from %s to %s", *sourceChainID, *destChainID),
	)

	if _, err = relayer.Relay(ctx, &relayRequest); err != nil {
		return fmt.Errorf("relaying packet: %w", err)
	}

	lmt.Logger(ctx).Info("successfully submitted packet to be relayed")
	return nil
}
