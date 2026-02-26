package ibcv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/cosmos/platform-relayer/db/gen/db"
	"github.com/cosmos/platform-relayer/shared/bridges/ibcv2"
	"github.com/cosmos/platform-relayer/shared/metrics"
)

const (
	RetryAckExpiry = 2 * time.Minute
)

var ErrRetryingAckPacket = errors.New("retrying ack packet")

type TransferClearAckTxStorage interface {
	ClearAckTx(ctx context.Context, args db.ClearAckTxParams) error
}

type RetryAckPacketProcessor struct {
	bridgeClientManager BridgeClientManager
	storage             TransferClearAckTxStorage
	sourceChainID       string
}

func NewRetryAckPacketProcessor(
	bridgeClientManager BridgeClientManager,
	storage TransferClearAckTxStorage,
	sourceChainID string,
) RetryAckPacketProcessor {
	return RetryAckPacketProcessor{
		bridgeClientManager: bridgeClientManager,
		storage:             storage,
		sourceChainID:       sourceChainID,
	}
}

func (processor RetryAckPacketProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	sourceChainClient, err := processor.bridgeClientManager.GetClient(ctx, processor.sourceChainID)
	if err != nil {
		return nil, fmt.Errorf("getting client for transfer source chain %s: %w", processor.sourceChainID, err)
	}

	ackTxHash, ok := transfer.GetAckTxHash()
	if !ok {
		return nil, fmt.Errorf("transfer does not have ack tx hash, violates should process, this is a bug")
	}

	ackTxTime, ok := transfer.GetAckTxTime()
	if !ok {
		return nil, fmt.Errorf("transfer does not have a ack tx time, violates should process, this is a bug")
	}

	retry, err := sourceChainClient.ShouldRetryTx(ctx, ackTxHash, RetryAckExpiry, ackTxTime)
	if err != nil {
		return nil, fmt.Errorf("checking if transfer with ack tx hash %s, submitted at %s, should be retried: %w", ackTxHash, ackTxTime.UTC().Format(time.RFC3339), err)
	}
	if !retry {
		// if we don't need to retry, do nothing and return early
		return transfer, nil
	}

	transfer.GetLogger().Warn("clearing ack tx for retry", zap.String("ack_tx_hash", ackTxHash), zap.Time("ack_tx_time", ackTxTime))

	update := db.ClearAckTxParams{
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()),
	}
	if err := processor.storage.ClearAckTx(ctx, update); err != nil {
		return nil, fmt.Errorf("clearing ack tx hash %s and time %s: %w", ackTxHash, ackTxTime.UTC().Format(time.RFC3339), err)
	}

	transfer.RecordTransactionRetried(ctx, metrics.IBCV2RecvToAckRelayType)

	// return an error so that the packet is marked as errored and no longer
	// processed by the pipeline for this run, it will be picked up without the
	// ack tx hash next run and retried then
	return nil, ErrRetryingAckPacket
}

func (processor RetryAckPacketProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	ackTxHash, _ := transfer.GetAckTxHash()
	ackTxTime, _ := transfer.GetAckTxTime()

	if errors.Is(err, ErrRetryingAckPacket) {
		transfer.GetLogger().Warn(
			"retrying ack packet tx",
			zap.String("ack_tx_hash", ackTxHash),
			zap.Time("ack_tx_time", ackTxTime),
		)
		return
	}
	if errors.Is(err, ibcv2.ErrTxNotFound) {
		transfer.GetLogger().Debug(
			"ack packet tx not yet found on chain, waiting for it to be found",
			zap.String("ack_tx_hash", ackTxHash),
			zap.Time("ack_tx_time", ackTxTime),
		)
		return
	}

	transfer.GetLogger().Error(
		"error checking ack packet retry",
		zap.Error(err),
		zap.String("ack_tx_hash", ackTxHash),
		zap.Time("ack_tx_time", ackTxTime),
	)
}

// ShouldProcess determines when this processor should be run.
func (processor RetryAckPacketProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasAckTxHash := transfer.GetAckTxHash()
	_, hasAckTxTime := transfer.GetAckTxTime()

	// only run this processor if we have ack tx details
	return hasAckTxHash && hasAckTxTime
}

func (processor RetryAckPacketProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusDELIVERACKPACKET
}
