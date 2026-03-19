package ibcv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	"github.com/cosmos/ibc-relayer/shared/bridges/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/metrics"
)

const (
	RetryTimeoutExpiry = 2 * time.Minute
)

var ErrRetryingTimeoutPacket = errors.New("retrying timeout packet")

type TransferClearTimeoutTxStorage interface {
	ClearTimeoutTx(ctx context.Context, args db.ClearTimeoutTxParams) error
}

type RetryTimeoutPacketProcessor struct {
	bridgeClientManager BridgeClientManager
	storage             TransferClearTimeoutTxStorage
	sourceChainID       string
}

func NewRetryTimeoutPacketProcessor(
	bridgeClientManager BridgeClientManager,
	storage TransferClearTimeoutTxStorage,
	sourceChainID string,
) RetryTimeoutPacketProcessor {
	return RetryTimeoutPacketProcessor{
		bridgeClientManager: bridgeClientManager,
		storage:             storage,
		sourceChainID:       sourceChainID,
	}
}

func (processor RetryTimeoutPacketProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	sourceChainClient, err := processor.bridgeClientManager.GetClient(ctx, processor.sourceChainID)
	if err != nil {
		return nil, fmt.Errorf("getting client for transfer source chain %s: %w", processor.sourceChainID, err)
	}

	timeoutTxHash, ok := transfer.GetTimeoutTxHash()
	if !ok {
		return nil, fmt.Errorf("transfer does not have timeout tx hash, violates should process, this is a bug")
	}

	timeoutTxTime, ok := transfer.GetTimeoutTxTime()
	if !ok {
		return nil, fmt.Errorf("transfer does not have a timeout tx time, violates should process, this is a bug")
	}

	retry, err := sourceChainClient.ShouldRetryTx(ctx, timeoutTxHash, RetryTimeoutExpiry, timeoutTxTime)
	if err != nil {
		return nil, fmt.Errorf("checking if transfer with timeout tx hash %s, submitted at %s, should be retried: %w", timeoutTxHash, timeoutTxTime.UTC().Format(time.RFC3339), err)
	}
	if !retry {
		// if we don't need to retry, do nothing and return early
		return transfer, nil
	}

	transfer.GetLogger().Warn("clearing timeout tx for retry", zap.String("timeout_tx_hash", timeoutTxHash), zap.Time("timeout_tx_time", timeoutTxTime))

	update := db.ClearTimeoutTxParams{
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()),
	}
	if err := processor.storage.ClearTimeoutTx(ctx, update); err != nil {
		return nil, fmt.Errorf("clearing timeout tx hash %s and time %s: %w", timeoutTxHash, timeoutTxTime.UTC().Format(time.RFC3339), err)
	}

	transfer.RecordTransactionRetried(ctx, metrics.IBCV2SendToTimeoutRelayType)

	// return an error so that the packet is marked as errored and no longer
	// processed by the pipeline for this run, it will be picked up without the
	// timeout tx hash next run and retried then
	return nil, ErrRetryingTimeoutPacket
}

func (processor RetryTimeoutPacketProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	timeoutTxHash, _ := transfer.GetTimeoutTxHash()
	timeoutTxTime, _ := transfer.GetTimeoutTxTime()

	if errors.Is(err, ErrRetryingTimeoutPacket) {
		transfer.GetLogger().Warn(
			"retrying timeout packet tx",
			zap.String("timeout_tx_hash", timeoutTxHash),
			zap.Time("timeout_tx_time", timeoutTxTime),
		)
		return
	}
	if errors.Is(err, ibcv2.ErrTxNotFound) {
		transfer.GetLogger().Debug(
			"timeout packet tx not yet found on chain, waiting for it to be found",
			zap.String("timeout_tx_hash", timeoutTxHash),
			zap.Time("timeout_tx_time", timeoutTxTime),
		)
		return
	}

	transfer.GetLogger().Error(
		"error checking timeout packet retry",
		zap.Error(err),
		zap.String("timeout_tx_hash", timeoutTxHash),
		zap.Time("timeout_tx_time", timeoutTxTime),
	)
}

// ShouldProcess determines when this processor should be run.
func (processor RetryTimeoutPacketProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasTimeoutTxHash := transfer.GetTimeoutTxHash()
	_, hasTimeoutTxTime := transfer.GetTimeoutTxTime()

	// only run this processor if we have timeout tx details
	return hasTimeoutTxHash && hasTimeoutTxTime
}

func (processor RetryTimeoutPacketProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusDELIVERTIMEOUTPACKET
}
