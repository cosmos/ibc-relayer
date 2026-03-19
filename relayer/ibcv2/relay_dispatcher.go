package ibcv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	"github.com/cosmos/ibc-relayer/shared/lmt"
)

type UnfinishedIBCV2TransferStorage interface {
	// GetUnfinishedIBCV2Transfers gets all ibcv2 transfers that are not
	// marked as completed or failed in the db. Note that any of these
	// transfers may be in the current pipeline.
	GetUnfinishedIBCV2Transfers(ctx context.Context) ([]db.Ibcv2Transfer, error)

	UpdateTransferState(ctx context.Context, arg db.UpdateTransferStateParams) error
}

type PipelineManager interface {
	Pipeline(ctx context.Context, transfer *IBCV2Transfer) (IBCV2Pipeline, error)
	Close()
}

type RelayDispatcher struct {
	storage         UnfinishedIBCV2TransferStorage
	pipelineManager PipelineManager
	pollInterval    time.Duration
	defaultEnabled  bool
}

func NewRelayDispatcher(
	storage UnfinishedIBCV2TransferStorage,
	pollInterval time.Duration,
	manager PipelineManager,
	defaultEnabled bool,
) *RelayDispatcher {
	return &RelayDispatcher{
		storage:         storage,
		pollInterval:    pollInterval,
		pipelineManager: manager,
		defaultEnabled:  defaultEnabled,
	}
}

func (r *RelayDispatcher) Run(ctx context.Context) error {
	// tick immediately, then after the first tick use the user supplied poll
	// interval
	const (
		initialPoll = time.Millisecond
	)

	ticker := time.NewTicker(initialPoll)
	defer ticker.Stop()

	// check if ibcv2 relaying is enabled or not
	if !r.defaultEnabled {
		lmt.Logger(ctx).Info("ibcv2 relaying disabled", zap.Bool("default_enabled", r.defaultEnabled))
		ticker.Stop()
	} else {
		lmt.Logger(ctx).Info("ibcv2 relaying enabled", zap.Bool("default_enabled", r.defaultEnabled))
	}

	for {
		select {
		case <-ctx.Done():
			lmt.Logger(ctx).Info("relay dispatcher context cancelled, closing pipelines")
			r.pipelineManager.Close()
			return ctx.Err()
		case <-ticker.C:
			ticker.Stop()

			if err := r.SubmitWaitingUnfinishedTransfers(ctx); err != nil {
				lmt.Logger(ctx).Error("error submitting unfinished transfers to pipeline", zap.Error(err))
			}

			ticker.Reset(r.pollInterval)
		}
	}
}

// SubmitWaitingUnfinishedTransfers gets all unfinished transfers from storage
// that are not currently in a running pipeline, and submits them to their
// respective pipeline. If the transfer is submitted successfully, it is marked
// as in flight so it will no longer be submitted again. This does not monitor
// any pipelines output channel, so no in flight transfers will be marked as
// not in flight via this function.
func (r *RelayDispatcher) SubmitWaitingUnfinishedTransfers(ctx context.Context) error {
	transfers, err := r.storage.GetUnfinishedIBCV2Transfers(ctx)
	if err != nil {
		return fmt.Errorf("getting unfinished ibcv2 transfers from storage: %w", err)
	}

	for _, transfer := range transfers {
		t := NewIBCV2Transfer(ctx, transfer)
		t.AlertOnExcessiveRelayLatency(ctx)

		// submit transfers and ignore error if we are being notified that the
		// transfer is already in the pipeline
		if err = r.SubmitTransfer(ctx, t); err != nil && !errors.Is(err, ErrTransferAlreadyInPipeline) {
			// if we fail to submit a transfer that is not already in the
			// pipeline and it fails, it must mean this is a configuration
			// issue and we cannot continue, mark the transfer as failed so it
			// is not retired since it will never recover
			lmt.Logger(ctx).Error(
				"unable to submit transfer to pipeline, marking as FAILED and not retrying",
				zap.Error(err),
				zap.String("source_chain_id", t.GetSourceChainID()),
				zap.String("packet_source_client_id", t.GetPacketSourceClientID()),
				zap.Uint32("packet_sequence_number", t.GetPacketSequenceNumber()),
			)

			update := db.UpdateTransferStateParams{
				Status:               db.Ibcv2RelayStatusFAILED,
				SourceChainID:        t.GetSourceChainID(),
				PacketSourceClientID: t.GetPacketSourceClientID(),
				PacketSequenceNumber: int32(t.GetPacketSequenceNumber()),
			}
			if err = r.storage.UpdateTransferState(ctx, update); err != nil {
				lmt.Logger(ctx).Error(
					"error updating transfer state to FAILED",
					zap.Error(err),
					zap.String("source_chain_id", t.GetSourceChainID()),
					zap.String("packet_source_client_id", t.GetPacketSourceClientID()),
					zap.Uint32("packet_sequence_number", t.GetPacketSequenceNumber()),
				)
			}
		}
	}

	return nil
}

var ErrTransferAlreadyInPipeline = errors.New("transfer already in pipeline")

// SubmitTransfer submits a transfer to be added to the ibcv2 packet
// pipeline. This call may block when submitting the transfer if it not yet
// ready to be received. If the context is cancelled while blocked, an error
// will be returned and the transfer will not be submitted. Returns a nil error
// if the transfer has been pushed onto the pipeline.
func (r *RelayDispatcher) SubmitTransfer(ctx context.Context, transfer *IBCV2Transfer) error {
	// get the pipeline to push this transfer onto
	pipeline, err := r.pipelineManager.Pipeline(ctx, transfer)
	if err != nil {
		return fmt.Errorf("getting pipeline to push transfer onto from manager: %w", err)
	}

	if pushed := pipeline.Push(ctx, transfer); !pushed {
		return ErrTransferAlreadyInPipeline
	}

	return nil
}
