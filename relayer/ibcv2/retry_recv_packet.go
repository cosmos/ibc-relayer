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
	RetryRecvExpiry = 2 * time.Minute
)

var ErrRetryingRecvPacket = errors.New("retrying recv packet")

type TransferClearRecvTxStorage interface {
	ClearRecvTx(ctx context.Context, args db.ClearRecvTxParams) error
}

type RetryRecvPacketProcessor struct {
	bridgeClientManager BridgeClientManager
	storage             TransferClearRecvTxStorage
	destinationChainID  string
}

func NewRetryRecvPacketProcessor(
	bridgeClientManager BridgeClientManager,
	storage TransferClearRecvTxStorage,
	destinationChainID string,
) RetryRecvPacketProcessor {
	return RetryRecvPacketProcessor{
		bridgeClientManager: bridgeClientManager,
		storage:             storage,
		destinationChainID:  destinationChainID,
	}
}

func (processor RetryRecvPacketProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	destinationChainClient, err := processor.bridgeClientManager.GetClient(ctx, processor.destinationChainID)
	if err != nil {
		return nil, fmt.Errorf("getting client for transfer destination chain %s: %w", processor.destinationChainID, err)
	}

	recvTxHash, ok := transfer.GetRecvTxHash()
	if !ok {
		return nil, fmt.Errorf("transfer does not have recv tx hash, violates should process, this is a bug")
	}

	recvTxTime, ok := transfer.GetRecvTxTime()
	if !ok {
		return nil, fmt.Errorf("transfer does not have a recv tx time, violates should process, this is a bug")
	}

	retry, err := destinationChainClient.ShouldRetryTx(ctx, recvTxHash, RetryRecvExpiry, recvTxTime)
	if err != nil {
		return nil, fmt.Errorf("checking if transfer with recv tx hash %s, submitted at %s, should be retried: %w", recvTxHash, recvTxTime.UTC().Format(time.RFC3339), err)
	}
	if !retry {
		// if we don't need to retry, do nothing and return early
		return transfer, nil
	}

	update := db.ClearRecvTxParams{
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()),
	}
	if err := processor.storage.ClearRecvTx(ctx, update); err != nil {
		return nil, fmt.Errorf("clearing recv tx hash %s and time %s: %w", recvTxHash, recvTxTime.UTC().Format(time.RFC3339), err)
	}

	transfer.RecordTransactionRetried(ctx, metrics.IBCV2SendToRecvRelayType)

	// return an error so that the packet is marked as errored and no longer
	// processed by the pipeline for this run, it will be picked up without the
	// recv tx hash next run and retried then
	return nil, ErrRetryingRecvPacket
}

func (processor RetryRecvPacketProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	recvTxHash, _ := transfer.GetRecvTxHash()
	recvTxTime, _ := transfer.GetRecvTxTime()

	if errors.Is(err, ErrRetryingRecvPacket) {
		transfer.GetLogger().Warn(
			"retrying recv packet tx",
			zap.String("recv_tx_hash", recvTxHash),
			zap.Time("recv_tx_time", recvTxTime),
		)
		return
	}
	if errors.Is(err, ibcv2.ErrTxNotFound) {
		transfer.GetLogger().Debug(
			"recv packet tx not yet found on chain, waiting for it to be found",
			zap.String("recv_tx_hash", recvTxHash),
			zap.Time("recv_tx_time", recvTxTime),
		)
		return
	}

	transfer.GetLogger().Error(
		"error checking recv packet retry",
		zap.Error(err),
		zap.String("recv_tx_hash", recvTxHash),
		zap.Time("recv_tx_time", recvTxTime),
	)
}

// ShouldProcess determines when this processor should be run.
func (processor RetryRecvPacketProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasRecvTxHash := transfer.GetRecvTxHash()
	_, hasRecvTxTime := transfer.GetRecvTxTime()
	_, hasWriteAckTxHash := transfer.GetWriteAckTxHash()

	// only run this processor if we have recv tx details, but we have not yet
	// found the write ack for this recv
	return hasRecvTxHash && hasRecvTxTime && !hasWriteAckTxHash
}

func (processor RetryRecvPacketProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusDELIVERRECVPACKET
}
