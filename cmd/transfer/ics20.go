package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"slices"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/cosmos/ibc-relayer/proto/gen/relayerapi"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
)

var denom = flag.String(
	"denom",
	"",
	"ERC20 contract address to transfer (ics20 only)",
)

var memo = flag.String(
	"memo",
	"",
	"an additional memo to send with the transfer (ics20 only)",
)

var ics20Address = flag.String(
	"ics20-address",
	"",
	"address of the ICS20Transfer contract on the source chain (ics20 only)",
)

func ics20Transfer(ctx context.Context) error {
	lmt.Logger(ctx).Info("config path", zap.String("path", *cfgPath))
	lmt.Logger(ctx).Info("ics20 address", zap.String("ics20_address", *ics20Address))
	lmt.Logger(ctx).Info("denom", zap.String("denom", *denom))
	lmt.Logger(ctx).Info("receiver", zap.String("receiver", *receiver))
	lmt.Logger(ctx).Info("source chain id", zap.String("source_chain_id", *sourceChainID))
	lmt.Logger(ctx).Info("dest chain id", zap.String("dest_chain_id", *destChainID))

	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	ctx = config.ConfigReaderContext(ctx, config.NewConfigReader(cfg))

	logger := lmt.Logger(ctx).With(zap.String("source_chain_id", *sourceChainID), zap.String("dest_chain_id", *destChainID))
	ctx = lmt.WithLogger(ctx, logger)

	sourceChain, err := config.GetConfigReader(ctx).GetChainConfig(*sourceChainID)
	if err != nil {
		return fmt.Errorf("loading source chain config: %w", err)
	}
	if !slices.Contains(sourceChain.SupportedBridges, config.BridgeTypeIBCV2) {
		return errors.New("source chain does not support ibcv2")
	}
	if sourceChain.EVM == nil {
		return errors.New("ics20 transfers require an evm source chain")
	}

	destChain, err := config.GetConfigReader(ctx).GetChainConfig(*destChainID)
	if err != nil {
		return fmt.Errorf("loading dest chain config: %w", err)
	}
	if !slices.Contains(destChain.SupportedBridges, config.BridgeTypeIBCV2) {
		return errors.New("dest chain does not support ibcv2")
	}
	switch {
	case destChain.Cosmos != nil:
		if _, _, err = bech32.DecodeAndConvert(*receiver); err != nil {
			return fmt.Errorf("receiver %s is not a valid bech32 address: %w", *receiver, err)
		}
	case destChain.EVM != nil:
		if !common.IsHexAddress(*receiver) {
			return fmt.Errorf("receiver %s is not a valid hex address (EVM destination)", *receiver)
		}
	default:
		return errors.New("dest chain must be evm or cosmos type")
	}

	if !common.IsHexAddress(*ics20Address) {
		return fmt.Errorf("ics20-address %s is not a valid hex address", *ics20Address)
	}
	if !common.IsHexAddress(*denom) {
		return fmt.Errorf("denom %s is not a valid hex address", *denom)
	}

	if *amount == 0 {
		return errors.New("amount must be positive")
	}
	amt := new(big.Int).SetUint64(*amount)

	sourceChainIBCV2Config, err := config.GetConfigReader(ctx).GetIBCV2Config(*sourceChainID)
	if err != nil {
		return fmt.Errorf("loading source chain ibcv2 config: %w", err)
	}
	clientCounterpartyChainID, ok := sourceChainIBCV2Config.CounterpartyChains[*sourceClientID]
	if !ok {
		return fmt.Errorf("could not determine counterparty chain id for source client %s", *sourceClientID)
	}
	if clientCounterpartyChainID != *destChainID {
		return fmt.Errorf("source client (%s) counterparty chain id (%s) does not match the dest chain id (%s)", *sourceClientID, clientCounterpartyChainID, *destChainID)
	}

	if *relayerGRPCURL == "" {
		return errors.New("--relayer-grpc-url is required")
	}

	var creds credentials.TransportCredentials
	if *insecureDial {
		creds = insecure.NewCredentials()
	} else {
		creds = credentials.NewTLS(&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}) //nolint:gosec // CLI tool; user-supplied endpoint
	}
	conn, err := grpc.NewClient(*relayerGRPCURL, grpc.WithTransportCredentials(creds))
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
		fmt.Sprintf("sending ics20 transfer from %s to %s", *sourceChainID, *destChainID),
		zap.String("ics20_address", *ics20Address),
		zap.String("denom", *denom),
		zap.String("receiver", *receiver),
		zap.String("amount", amt.String()),
		zap.String("memo", *memo),
	)

	sendTxHash, err := sourceClient.SendTransfer(ctx, *ics20Address, *sourceClientID, *denom, *receiver, amt, *memo, *timeout)
	if err != nil {
		return fmt.Errorf("sending ics20 transfer from %s to %s: %w", *sourceChainID, *destChainID, err)
	}

	lmt.Logger(ctx).Info(
		fmt.Sprintf("successfully sent ics20 transfer from %s to %s", *sourceChainID, *destChainID),
		zap.String("tx_hash", sendTxHash),
	)

	if _, err = relayer.Relay(ctx, &relayerapi.RelayRequest{TxHash: sendTxHash, ChainId: *sourceChainID}); err != nil {
		return fmt.Errorf("submitting relay request: %w", err)
	}

	lmt.Logger(ctx).Info("successfully submitted packet to be relayed", zap.String("tx_hash", sendTxHash))
	return nil
}
