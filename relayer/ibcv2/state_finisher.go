package ibcv2

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/cosmos/platform-relayer/db/gen/db"
)

// NOTE: this processor is not a IBCV2Processor (it does not have
// ShouldProcess or State methods). This is because this processor doesnt have
// a state that should be represented in the db.

type StateFinisher struct {
	transferStateStorage   TransferStateStorage
	shouldRelaySuccessAcks bool
	shouldRelayErrorAcks   bool
}

func NewStateFinisherProcessor(transferStateStorage TransferStateStorage, shouldRelaySuccessAcks bool, shouldRelayErrorAcks bool) StateFinisher {
	return StateFinisher{
		transferStateStorage:   transferStateStorage,
		shouldRelaySuccessAcks: shouldRelaySuccessAcks,
		shouldRelayErrorAcks:   shouldRelayErrorAcks,
	}
}

// Process updates the state in the db to a finalized state if the transfer has
// not errored
func (processor StateFinisher) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	if transfer.Error() != "" {
		// do nothing for transfers that have errored
		return transfer, nil
	}

	if !transfer.IsComplete(processor.shouldRelaySuccessAcks, processor.shouldRelayErrorAcks) {
		// transfer is not done, so dont update its state
		recv, _ := transfer.GetRecvTxHash()
		writeAck, _ := transfer.GetWriteAckTxHash()
		ack, _ := transfer.GetAckTxHash()
		writeAckStatus, _ := transfer.GetWriteAckStatus()
		timeout, _ := transfer.GetTimeoutTxHash()
		transfer.GetLogger().Warn(
			"state finisher received transfer that is not errored and not complete",
			zap.String("recv_tx_hash", recv),
			zap.String("write_ack_tx_hash", writeAck),
			zap.String("write_ack_status", string(writeAckStatus)),
			zap.String("ack_tx_hash", ack),
			zap.String("timeout_tx_hash", timeout),
		)
		return transfer, nil
	}

	if _, timedOut := transfer.GetTimeoutTxHash(); timedOut {
		transfer.State = db.Ibcv2RelayStatusCOMPLETEWITHTIMEOUT
		transfer.RecordSendTimeoutLatency(ctx)
		transfer.RecordTimeoutRelayed(ctx)
	} else {
		writeAckStatus, hasWriteAckStatus := transfer.GetWriteAckStatus()
		if !hasWriteAckStatus {
			transfer.GetLogger().Warn("this is a bug! non timeout transfer is trying to be marked as complete without any write ack status, not marking as complete!")
			return transfer, nil
		}

		if _, ackd := transfer.GetAckTxHash(); ackd {
			transfer.State = db.Ibcv2RelayStatusCOMPLETEWITHACK
			transfer.RecordAckRecvLatency(ctx)
			transfer.RecordAckRelayed(ctx)
		} else {
			// no ack tx hash but the transfer is complete
			isErrorAck := writeAckStatus == db.Ibcv2WriteAckStatusERROR || writeAckStatus == db.Ibcv2WriteAckStatusUNKNOWN
			isSuccessAck := writeAckStatus == db.Ibcv2WriteAckStatusSUCCESS
			if isErrorAck {
				transfer.State = db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKERROR
			}
			if isSuccessAck {
				transfer.State = db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKSUCCESS
			}
		}
	}

	update := db.UpdateTransferStateParams{
		Status:               transfer.GetState(),
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()),
	}
	if err := processor.transferStateStorage.UpdateTransferState(ctx, update); err != nil {
		transfer.GetLogger().Error(
			"error updating transfer to finalized state",
			zap.Error(err),
			zap.String("state", string(transfer.GetState())),
		)
		transfer.ProcessingError = fmt.Errorf("updating transfer state to %s: %w", transfer.GetState(), err)
	}

	transfer.GetLogger().Info("transfer complete", zap.String("state", string(transfer.GetState())))

	return transfer, nil
}

func (processor StateFinisher) Cancel(transfer *IBCV2Transfer, err error) {
	// log error, mark packet as failed if fatal error and we cannot retry, if
	// not fatal error, do nothing so it will be retried by this stage
	transfer.GetLogger().Error("error updating transfer to finalized state", zap.Error(err))
}
