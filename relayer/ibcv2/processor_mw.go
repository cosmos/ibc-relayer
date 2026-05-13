package ibcv2

import (
	"context"
	"errors"
	"fmt"

	"github.com/deliveryhero/pipeline/v2"
	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	"github.com/cosmos/ibc-relayer/shared/lmt"
)

type TransferStateStorage interface {
	UpdateTransferState(ctx context.Context, arg db.UpdateTransferStateParams) error
}

// IBCV2Processor defines the methods needed to be a processor in a ibcv2
// transfer pipeline and to be wrapped by the IBCV2ProcessorMW helper.
//
//nolint:revive // IBCV2Processor is the canonical name across the codebase
type IBCV2Processor interface {
	// ShouldProcess returns true if a processor should process some input,
	// false otherwise
	ShouldProcess(input *IBCV2Transfer) bool

	// State returns the state that a transfer is in, if it is being processed
	// by this processor
	State() db.Ibcv2RelayStatus

	// inherit Process and Cancel methods
	pipeline.Processor[*IBCV2Transfer, *IBCV2Transfer]
}

// IBCV2ProcessorMW is a wrapper around a pipeline processor that has ibcv2
// specific helper logic. All ibcv2 processors should be wrapped with this.
//
//nolint:revive // IBCV2ProcessorMW is the canonical name across the codebase
type IBCV2ProcessorMW[Input *IBCV2Transfer, Output *IBCV2Transfer] struct {
	storage  TransferStateStorage
	internal IBCV2Processor
}

func NewIBCV2ProcessorMW[Input *IBCV2Transfer, Output *IBCV2Transfer](
	storage TransferStateStorage,
	internal IBCV2Processor,
) IBCV2ProcessorMW[Input, Output] {
	return IBCV2ProcessorMW[Input, Output]{
		storage:  storage,
		internal: internal,
	}
}

// Process is a noop if the input transfer has errored, (i.e. another processor
// returned an error for this transfer). If the interal processor implements a
// ShouldProcess method, that will be invoked to check if the processor should
// be invoked or skipped. Finally, the internal processor is called. If the
// processor returns an error, the internal processor's cancel function will be
// called and the transfer will be marked as an error.
func (p IBCV2ProcessorMW[Input, Output]) Process(
	ctx context.Context,
	input *IBCV2Transfer,
) (Output, error) {
	if input.Error() != "" || input == nil {
		// if we get input that has errored, pass to the next processor
		// input may be nil when all the input and output channels are closing
		return input, nil
	}
	if input.GetState() == db.Ibcv2RelayStatusFAILED {
		// do not try and process transfers that have been marked as failed
		return input, nil
	}

	// check that the internal processor should process the input, if not,
	// return the input without processing
	if !p.internal.ShouldProcess(input) {
		return input, nil
	}

	// this internal processor should process this transfer, update its state
	// to match the processor that is about to process it
	stateUpdate := db.UpdateTransferStateParams{
		Status:               p.internal.State(),
		SourceChainID:        input.GetSourceChainID(),
		PacketSourceClientID: input.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(input.GetPacketSequenceNumber()), //nolint:gosec // G115 bounded conversion
	}
	if err := p.storage.UpdateTransferState(ctx, stateUpdate); err != nil {
		wrapped := fmt.Errorf(
			"updating transfer state to %s from state %s: %w",
			p.internal.State(),
			input.GetState(),
			err,
		)
		p.Cancel(input, wrapped)
		input.ProcessingError = wrapped
		return input, nil
	}

	// update the transfers state to the processor that is about to process
	// this transfer
	prevState := input.State
	input.State = p.internal.State()
	input.RecordTransferState(ctx, string(input.State))

	input.GetLogger().Debug(
		fmt.Sprintf("processing transfer at state %s", string(input.State)),
		zap.String("state", string(input.State)),
		zap.String("prev_state", string(prevState)),
	)

	output, err := p.internal.Process(ctx, input)
	if err != nil {
		// if the processor returns an error, call the cancel function and set
		// the transfers ProcessingError to the error.
		//
		// NOTE: we are not actually returning the error that the internal
		// processor returned via this Process call. We do this because if
		// simply let the cancel function run and do nothing else, there is no
		// way to deliver the transfer to the Pipelines output channel. So
		// instead, we set a field on the transfer that signals an error
		// occurred, but still continue to send it through the pipeline (but it
		// will just be ignored by every other step since because of the Error
		// check at the start of this function). The internal Processor cancel
		// functions can still handle the error though, typically this would
		// look like logging it, and updating the state in the db to 'FAILED'
		// if the error is fatal and the transfer should not be retried.

		// if the error is context cancelled, the pipeline is over and we dont
		// care about the oboe, return without calling cancel since the outer
		// processor should do it for us
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		p.Cancel(input, err)
		input.ProcessingError = err
		return input, nil
	}

	input.GetLogger().Debug(
		fmt.Sprintf("successfully processed transfer at state %s", string(input.State)),
		zap.String("state", string(input.State)),
		zap.String("prev_state", string(prevState)),
	)

	return output, nil
}

// Cancel calls the internal processors Cancel function
func (p IBCV2ProcessorMW[Input, Output]) Cancel(input *IBCV2Transfer, err error) {
	if input == nil {
		return
	}
	p.internal.Cancel(input, err)
}

// IBCV2Processor defines the methods needed to be a processor in a ibcv2
// transfer pipeline and to be wrapped by the IBCV2ProcessorMW helper.
//
//nolint:revive // IBCV2BatchProcessor is the canonical name across the codebase
type IBCV2BatchProcessor interface {
	// ShouldProcess returns true if a processor should process some input
	// within a batch, false otherwise
	ShouldProcess(input *IBCV2Transfer) bool

	// State returns the state that a transfer is in, if it is being processed
	// by this processor
	State() db.Ibcv2RelayStatus

	// inherit Process and Cancel methods
	pipeline.Processor[[]*IBCV2Transfer, []*IBCV2Transfer]
}

// IBCV2BatchProcessorMW is a wrapper around a pipeline processor that has ibcv2
// specific helper logic. All ibcv2 processors should be wrapped with this.
//
//nolint:revive // IBCV2BatchProcessorMW is the canonical name across the codebase
type IBCV2BatchProcessorMW[Input []*IBCV2Transfer, Output []*IBCV2Transfer] struct {
	storage  TransferStateStorage
	internal IBCV2BatchProcessor
}

func NewIBCV2BatchProcessorMW[Input []*IBCV2Transfer, Output []*IBCV2Transfer](
	storage TransferStateStorage,
	internal IBCV2BatchProcessor,
) IBCV2BatchProcessorMW[Input, Output] {
	return IBCV2BatchProcessorMW[Input, Output]{
		storage:  storage,
		internal: internal,
	}
}

func (p IBCV2BatchProcessorMW[Input, Output]) Process(
	ctx context.Context,
	batch []*IBCV2Transfer,
) ([]*IBCV2Transfer, error) {
	lmt.Logger(ctx).Debug(
		fmt.Sprintf("processing %d transfer batch", len(batch)),
		zap.String("state", string(p.internal.State())),
	)

	var notProcessing []*IBCV2Transfer
	toProcess := make([]*IBCV2Transfer, 0, len(batch))
	for _, input := range batch {
		if input.Error() != "" {
			// note that if we return an error here then processing the entire
			// batch will fail, we instead we just ignore this input when
			// processing
			notProcessing = append(notProcessing, input)
			continue
		}
		if input.GetState() == db.Ibcv2RelayStatusFAILED {
			// do not try and process transfers that have been marked as failed
			notProcessing = append(notProcessing, input)
			continue
		}

		if !p.internal.ShouldProcess(input) {
			// note that if we return an error here then processing the entire
			// batch will fail, we instead we just ignore this input when
			// processing
			notProcessing = append(notProcessing, input)
			continue
		}

		// this internal processor should process this input in a batch, update
		// its state to match the processor that is about to process it
		stateUpdate := db.UpdateTransferStateParams{
			Status:               p.internal.State(),
			SourceChainID:        input.GetSourceChainID(),
			PacketSourceClientID: input.GetPacketSourceClientID(),
			PacketSequenceNumber: int32(input.GetPacketSequenceNumber()), //nolint:gosec // G115 bounded conversion
		}
		if err := p.storage.UpdateTransferState(ctx, stateUpdate); err != nil {
			wrapped := fmt.Errorf(
				"updating transfer state to %s from state %s: %w",
				p.internal.State(),
				input.GetState(),
				err,
			)
			p.Cancel([]*IBCV2Transfer{input}, wrapped)
			input.ProcessingError = wrapped
			notProcessing = append(notProcessing, input)
			continue
		}

		// update the transfers state to the processor that is about to process
		// this transfer
		input.State = p.internal.State()
		input.RecordTransferState(ctx, string(input.State))
		toProcess = append(toProcess, input)
	}

	if len(toProcess) == 0 {
		return notProcessing, nil
	}

	output, err := p.internal.Process(ctx, toProcess)
	if err != nil {
		// if the processor returns an error, call the cancel function and set
		// the transfers ProcessingError to the error.
		//
		// NOTE: we are not actually returning the error that the internal
		// processor returned via this Process call. We do this because if
		// simply let the cancel function run and do nothing else, there is no
		// way to deliver the transfer to the Pipelines output channel. So
		// instead, we set a field on the transfer that signals an error
		// occurred, but still continue to send it through the pipeline (but it
		// will just be ignored by every other step since because of the Error
		// check at the start of this function). The internal Processor cancel
		// functions can still handle the error though, typically this would
		// look like logging it, and updating the state in the db to 'FAILED'
		// if the error is fatal and the transfer should not be retried.

		// if the error is context cancelled, the pipeline is over and we dont
		// care about the oboe, return without calling cancel since the outer
		// processor should do it for us
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		p.Cancel(toProcess, err)
		for _, input := range toProcess {
			input.ProcessingError = err
		}
		return append(toProcess, notProcessing...), nil
	}

	return append(output, notProcessing...), nil
}

// Cancel calls the internal processors Cancel function
func (p IBCV2BatchProcessorMW[Input, Output]) Cancel(input []*IBCV2Transfer, err error) {
	p.internal.Cancel(input, err)
}

func (p IBCV2BatchProcessorMW[Input, Output]) ShouldProcess(input *IBCV2Transfer) bool {
	return p.internal.ShouldProcess(input)
}

func (p IBCV2BatchProcessorMW[Input, Output]) State() db.Ibcv2RelayStatus {
	return p.internal.State()
}
