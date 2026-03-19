package ibcv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
)

type TransferSendTxFinalityStorage interface {
	UpdateTransferSourceTxFinalizedTime(ctx context.Context, arg db.UpdateTransferSourceTxFinalizedTimeParams) error
}

type CheckSendFinalityProcessor struct {
	bridgeClientManager  BridgeClientManager
	sourceChainID        string
	storage              TransferSendTxFinalityStorage
	sourceFinalityOffset *uint64
}

func NewCheckSendFinalityProcessor(
	bridgeClientmanager BridgeClientManager,
	sourceChainID string,
	storage TransferSendTxFinalityStorage,
	sourceFinalityOffset *uint64,
) CheckSendFinalityProcessor {
	return CheckSendFinalityProcessor{
		bridgeClientManager:  bridgeClientmanager,
		sourceChainID:        sourceChainID,
		storage:              storage,
		sourceFinalityOffset: sourceFinalityOffset,
	}
}

var ErrSendNotFinalized = errors.New("send tx not finalized")

func (processor CheckSendFinalityProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	sourceChainClient, err := processor.bridgeClientManager.GetClient(ctx, transfer.GetSourceChainID())
	if err != nil {
		return nil, fmt.Errorf("getting source bridge client for chain %s: %w", transfer.GetSourceChainID(), err)
	}

	finalized, err := sourceChainClient.IsTxFinalized(ctx, transfer.GetSourceTxHash(), processor.sourceFinalityOffset)
	if err != nil {
		return nil, fmt.Errorf("checking if tx %s is finalized on chain %s: %w", transfer.GetSourceTxHash(), transfer.GetSourceChainID(), err)
	}
	if !finalized {
		return nil, ErrSendNotFinalized
	}

	if _, sourceTxFinalizedTimeSet := transfer.GetSourceTxFinalizedTime(); !sourceTxFinalizedTimeSet {
		finalizedTime := time.Now()

		if err := processor.storage.UpdateTransferSourceTxFinalizedTime(ctx, db.UpdateTransferSourceTxFinalizedTimeParams{
			SourceChainID:         transfer.GetSourceChainID(),
			PacketSourceClientID:  transfer.GetPacketSourceClientID(),
			PacketSequenceNumber:  int32(transfer.GetPacketSequenceNumber()),
			SourceTxFinalizedTime: pgtype.Timestamp{Valid: true, Time: finalizedTime},
		}); err != nil {
			return nil, fmt.Errorf("updating source tx finalized time: %w", err)
		}

		transfer.SourceTxFinalizedTime = &finalizedTime
	}

	return transfer, nil
}

func (processor CheckSendFinalityProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	// log error, mark packet as failed if fatal error and we cannot retry, if
	// not fatal error, do nothing so it will be retried by this stage
	if errors.Is(err, ErrSendNotFinalized) {
		if time.Since(transfer.GetSourceTxTime()) > time.Minute*30 {
			transfer.GetLogger().Warn(
				"source transaction not finalized on source chain after 30 mins, is the node lagging behind?",
				zap.String("tx_hash", transfer.GetSourceTxHash()),
				zap.Time("tx_time", transfer.GetSourceTxTime()),
				zap.Error(err),
			)
		}
		return
	}
	transfer.GetLogger().Error("error checking finality of send tx", zap.Error(err))
}

// ShouldProcess determines when this processor should be run.
func (processor CheckSendFinalityProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasRecvTxHash := transfer.GetRecvTxHash()
	return !hasRecvTxHash
}

func (processor CheckSendFinalityProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusAWAITINGSENDFINALITY
}
