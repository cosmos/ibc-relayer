package ibcv2

import (
	"context"
	"fmt"
	"time"

	"github.com/deliveryhero/pipeline/v2"
	"go.uber.org/zap"

	"github.com/cosmos/platform-relayer/shared/lmt"
	"github.com/cosmos/platform-relayer/shared/metrics"
)

func ConditionallyBatchProcess(
	ctx context.Context,
	concurrency int,
	maxSize int,
	maxDuration time.Duration,
	in <-chan *IBCV2Transfer,
	processor IBCV2BatchProcessor,
) <-chan *IBCV2Transfer {
	out := make(chan *IBCV2Transfer)

	toBatch := make(chan *IBCV2Transfer)

	go func() {
		for i := range in {
			if i.Error() != "" {
				// if the input has errored, do not place it in a batch,
				// send it to the output channel immediately
				out <- i
				continue
			}
			if !processor.ShouldProcess(i) {
				// if the processor does not want to process the input, do
				// not place it in a batch, and send it to the output channel
				out <- i
				continue
			}
			// push input onto a batch
			i.GetLogger().Debug(fmt.Sprintf("pushing transfer into %s batch", processor.State()))
			toBatch <- i
		}
		// close the input to ProcessBatch, that will cause its output to
		// eventually close, which will close the batchOutput, which will close
		// the out
		close(toBatch)
	}()

	// read from the toBatch chan and collect transfers onto a buffer until the
	// buffer reaches max size, or it has waited for maxDuration
	batches := pipeline.Collect(ctx, maxSize, maxDuration, toBatch)

	// the following batch processing can be very slow, especially if
	// concurrency is set very low. if this processing is taking a long time
	// and there are many IBCV2Transfers coming into the input, new batches
	// that the collector collects will not get read off its channel, causing
	// the collector to not collect anymore transfers into a batch, causing the
	// input channel to not be read from, causing the whole pipeline to come to
	// a halt.
	//
	// to avoid this we make the collectors output channel (batches) a buffered
	// channel. this means that we can accumulate bufferSize batches that have
	// been collected in this channel, without blocking the collection of new
	// buffers which unblocks the input channel and keeps the whole pipeline
	// flowing.
	const bufferSize = 1000
	bufferedBatches := make(chan []*IBCV2Transfer, bufferSize)
	go func() {
		for batch := range batches {
			bufferedBatches <- batch
			if len(bufferedBatches) >= 3 {
				lmt.Logger(ctx).Warn(
					fmt.Sprintf("batches accumulating in buffer... %d batches currently waiting to be processed", len(bufferedBatches)),
					zap.String("relay_type", string(metrics.RelayTypeFromContext(ctx))),
					zap.Int("current_buffer_size", len(bufferedBatches)),
					zap.Int("max_buffer_size", bufferSize),
				)
			}
		}
		close(bufferedBatches)
	}()
	batchsResults := pipeline.ProcessConcurrently(ctx, concurrency, processor, bufferedBatches)

	go func() {
		// waiting on the output must be in a separate goroutine to handle the
		// case where we have pushed > batchSize to the batch processors input
		// channel (toBatch). In that case we will block on pushing to the
		// input channel. If there is nothing to pull items off of the output
		// channel, then the processor will block on pushing to the output and
		// there will be a deadlock. So to avoid this, pulling off of the
		// output channel is done in a different goroutine than pushing the
		// input
		for batchResults := range batchsResults {
			for _, output := range batchResults {
				// an item has been received from the batch processor, push it
				// to the output channel
				out <- output
			}
		}
		close(out)
	}()

	return out
}
