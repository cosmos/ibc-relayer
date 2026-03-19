package ibcv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
)

type CheckTimeoutFinalityProcessor struct {
	bridgeClientManager       BridgeClientManager
	destinationFinalityOffset *uint64
}

func NewCheckTimeoutFinalityProcessor(
	bridgeClientmanager BridgeClientManager,
	destinationFinalityOffset *uint64,
) CheckTimeoutFinalityProcessor {
	return CheckTimeoutFinalityProcessor{
		bridgeClientManager:       bridgeClientmanager,
		destinationFinalityOffset: destinationFinalityOffset,
	}
}

var ErrTimeoutNotFinalized = errors.New("timeout tx not finalized")

func (processor CheckTimeoutFinalityProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	destinationChainClient, err := processor.bridgeClientManager.GetClient(ctx, transfer.GetDestinationChainID())
	if err != nil {
		return nil, fmt.Errorf("getting destination bridge client for chain %s: %w", transfer.GetDestinationChainID(), err)
	}

	// Use the same offset-based finality as recv/ack processors.
	// This checks if there is a finalized block (per the offset) with timestamp >= the packet's timeout timestamp.
	finalized, err := destinationChainClient.IsTimestampFinalized(
		ctx,
		transfer.GetPacketTimeoutTimestamp(),
		processor.destinationFinalityOffset,
	)
	if err != nil {
		return nil, fmt.Errorf("checking timeout finality on chain %s: %w", transfer.GetDestinationChainID(), err)
	}
	if !finalized {
		return nil, ErrTimeoutNotFinalized
	}

	return transfer, nil
}

func (processor CheckTimeoutFinalityProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	// log error, mark packet as failed if fatal error and we cannot retry, if
	// not fatal error, do nothing so it will be retried by this stage
	if errors.Is(err, ErrTimeoutNotFinalized) {
		if time.Since(transfer.GetPacketTimeoutTimestamp()) > time.Minute*30 {
			transfer.GetLogger().Warn(
				"packet timeout timestamp not finalized on transfer destination chain 30 minutes after packet timeout timestamp, is the node lagging behind?",
				zap.Error(err),
			)
		}
		return
	}
	transfer.GetLogger().Error("error checking finality of packet timeout timeout on transfer destination chain", zap.Error(err))
}

// ShouldProcess determines when this processor should be run.
func (processor CheckTimeoutFinalityProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	// we only want to try and submit a timeout for a transfer if it is past
	// its timeout timestamp, and if it does not have a recv or ack submitted
	// for it. If it does have a recv submitted for it and is past its timeout
	// time, a timeout should not be submitted and we should continue with the
	// relaying pipeline as normal.
	_, hasRecvTxHash := transfer.GetRecvTxHash()
	_, hasAckTxHash := transfer.GetAckTxHash()
	shouldBeTimedOut := transfer.IsTimedOut() && !hasRecvTxHash && !hasAckTxHash

	// if we have already submitted a timeout tx, dont try again
	_, hasTimeoutTxHash := transfer.GetTimeoutTxHash()
	return shouldBeTimedOut && !hasTimeoutTxHash
}

func (processor CheckTimeoutFinalityProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusAWAITINGTIMEOUTFINALITY
}
