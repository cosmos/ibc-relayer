package ibcv2_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	mock_ibcv2 "github.com/cosmos/ibc-relayer/mocks/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/lmt"
)

func TestConditionalBatchProcessor(t *testing.T) {
	t.Run("transfers are processed in a single batch after timeout", func(t *testing.T) {
		ctx := context.Background()
		t1 := newTransfer(sourceChainID, destChainID, sourceClientID, 0)
		t2 := newTransfer(sourceChainID, destChainID, sourceClientID, 0)
		t3 := newTransfer(sourceChainID, destChainID, sourceClientID, 0)

		processor := mock_ibcv2.NewMockIBCV2BatchProcessor(t)
		processor.EXPECT().ShouldProcess(mock.Anything).Return(true)
		processor.EXPECT().State().Return(db.Ibcv2RelayStatusPENDING)

		expectedBatchInput := []*ibcv2.IBCV2Transfer{t1, t2, t3}
		hash := "0xdeadbeef"
		t1.AckTxHash = &hash
		t2.AckTxHash = &hash
		t3.AckTxHash = &hash
		expectedBatchOutput := []*ibcv2.IBCV2Transfer{t1, t2, t3}
		processor.EXPECT().Process(ctx, expectedBatchInput).Return(expectedBatchOutput, nil)

		input := make(chan *ibcv2.IBCV2Transfer)
		output := ibcv2.ConditionallyBatchProcess(ctx, 1, 10, 3*time.Second, input, processor)

		for _, transfer := range expectedBatchInput {
			input <- transfer
		}
		for _, expected := range expectedBatchOutput {
			out := <-output
			assert.Equal(t, expected, out)
		}
		assert.Empty(t, output)
	})

	t.Run("transfers are processed in a single batch when batch size is reached", func(t *testing.T) {
		ctx := context.Background()

		// create 100 transfers as input
		var expectedBatchInput []*ibcv2.IBCV2Transfer
		for i := range 100 {
			transfer := newTransfer(sourceChainID, destChainID, sourceClientID, uint32(i))
			expectedBatchInput = append(expectedBatchInput, transfer)
		}

		processor := mock_ibcv2.NewMockIBCV2BatchProcessor(t)
		processor.EXPECT().ShouldProcess(mock.Anything).Return(true)
		processor.EXPECT().State().Return(db.Ibcv2RelayStatusPENDING)

		// construct the batches that we expect to be passed to the processor
		// and have it modify its output with a tx hash
		inputs := splitToBatches(expectedBatchInput, 10)
		hash := "0xdeadbeef"
		for _, batch := range inputs {
			var singleBatchOutput []*ibcv2.IBCV2Transfer
			for _, input := range batch {
				output := *input
				output.AckTxHash = &hash
				singleBatchOutput = append(singleBatchOutput, &output)
			}
			processor.EXPECT().Process(ctx, batch).Return(singleBatchOutput, nil)
		}

		input := make(chan *ibcv2.IBCV2Transfer)

		// batch size is only 10 but we are sending 100 transfers onto the
		// input, we should expect the batches to process before the timeout
		output := ibcv2.ConditionallyBatchProcess(ctx, 1, 10, 100*time.Second, input, processor)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			count := 0
			// iterate over the output, make sure we get back the modified
			// input we expect
			for out := range output {
				expected := expectedBatchInput[count]
				expected.AckTxHash = &hash
				assert.Equal(t, expected, out)
				count++
			}
			assert.Empty(t, output)
			assert.Equal(t, 100, count)
		}()

		for _, transfer := range expectedBatchInput {
			input <- transfer
		}
		time.Sleep(500 * time.Millisecond)
		close(input)
		wg.Wait()
	})

	t.Run("shouldnt include in batch", func(t *testing.T) {
		ctx := context.Background()
		transfer := newTransfer(sourceChainID, destChainID, sourceClientID, 0)

		processor := mock_ibcv2.NewMockIBCV2BatchProcessor(t)
		processor.EXPECT().ShouldProcess(transfer).Return(false)

		input := make(chan *ibcv2.IBCV2Transfer)
		output := ibcv2.ConditionallyBatchProcess(ctx, 1, 10, 10*time.Second, input, processor)
		input <- transfer
		out := <-output
		assert.Equal(t, transfer, out)
	})

	t.Run("errored transfers should not be included", func(t *testing.T) {
		ctx := context.Background()
		transfer := newTransfer(sourceChainID, destChainID, sourceClientID, 0)
		transfer.ProcessingError = fmt.Errorf("i've errored")

		processor := mock_ibcv2.NewMockIBCV2BatchProcessor(t)
		processor.EXPECT().ShouldProcess(transfer).Return(true).Maybe()

		input := make(chan *ibcv2.IBCV2Transfer)
		output := ibcv2.ConditionallyBatchProcess(ctx, 1, 10, 10*time.Second, input, processor)
		input <- transfer
		out := <-output
		assert.Equal(t, transfer, out)
	})

	t.Run("batch processing is slow, new transfers can still get pushed onto pipeline while processing is happening", func(t *testing.T) {
		ctx := context.Background()
		t1 := newTransfer(sourceChainID, destChainID, sourceClientID, 0)
		t2 := newTransfer(sourceChainID, destChainID, sourceClientID, 0)
		t3 := newTransfer(sourceChainID, destChainID, sourceClientID, 0)

		processor := mock_ibcv2.NewMockIBCV2BatchProcessor(t)
		processor.EXPECT().ShouldProcess(mock.Anything).Return(true)
		processor.EXPECT().State().Return(db.Ibcv2RelayStatusPENDING)

		expectedBatchInput := []*ibcv2.IBCV2Transfer{t1, t2, t3}
		hash := "0xdeadbeef"
		t1.AckTxHash = &hash
		t2.AckTxHash = &hash
		t3.AckTxHash = &hash
		expectedBatchOutput := []*ibcv2.IBCV2Transfer{t1, t2, t3}
		processor.EXPECT().Process(ctx, expectedBatchInput).Return(expectedBatchOutput, nil).After(10 * time.Second).Once()

		input := make(chan *ibcv2.IBCV2Transfer)
		concurrency := 10
		output := ibcv2.ConditionallyBatchProcess(ctx, concurrency, 3, 3*time.Second, input, processor)

		t4 := newTransfer(sourceChainID, destChainID, "client-1", 1)
		t5 := newTransfer(sourceChainID, destChainID, "client-1", 1)
		t6 := newTransfer(sourceChainID, destChainID, "client-1", 1)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			// we want to get 4,5,6 back before 1,2,3 since they process faster
			assert.Equal(t, t4, <-output)
			assert.Equal(t, t5, <-output)
			assert.Equal(t, t6, <-output)

			assert.Equal(t, t1, <-output)
			assert.Equal(t, t2, <-output)
			assert.Equal(t, t3, <-output)
		}()

		// push 3 transfers onto the batch, this will reach the batch size and
		// block for 10 seconds to process. we want to ensure that the
		for _, transfer := range expectedBatchInput {
			input <- transfer
		}

		time.Sleep(1 * time.Second)

		// want to ensure first call to Process has been called (i.e. this
		// batch has started its 'slow' processing round)
		if !processor.AssertExpectations(t) {
			t.Fatal("failed expectations")
		}

		expectedSecondBatchInput := []*ibcv2.IBCV2Transfer{t4, t5, t6}
		expectedSecondBatchOutput := []*ibcv2.IBCV2Transfer{t4, t5, t6}

		// process this second batch quickly, we should expect to get these back first
		processor.EXPECT().Process(ctx, expectedSecondBatchInput).Return(expectedSecondBatchOutput, nil).Once()
		for _, transfer := range expectedSecondBatchInput {
			input <- transfer
		}

		wg.Wait()
		assert.Empty(t, output)
	})

	t.Run("single concurrency processing, pushing more than two batches at a time with slow processing should not block", func(t *testing.T) {
		lmt.ConfigureLogger()
		ctx := context.Background()

		t1 := newTransfer(sourceChainID, destChainID, sourceClientID, 1)
		t2 := newTransfer(sourceChainID, destChainID, sourceClientID, 2)

		t3 := newTransfer(sourceChainID, destChainID, sourceClientID, 3)
		t4 := newTransfer(sourceChainID, destChainID, sourceClientID, 4)

		t5 := newTransfer(sourceChainID, destChainID, sourceClientID, 5)
		t6 := newTransfer(sourceChainID, destChainID, sourceClientID, 6)

		t7 := newTransfer(sourceChainID, destChainID, sourceClientID, 7)
		t8 := newTransfer(sourceChainID, destChainID, sourceClientID, 8)

		t9 := newTransfer(sourceChainID, destChainID, sourceClientID, 9)
		t10 := newTransfer(sourceChainID, destChainID, sourceClientID, 10)
		allTransfers := []*ibcv2.IBCV2Transfer{t1, t2, t3, t4, t5, t6, t7, t8, t9, t10}

		processor := mock_ibcv2.NewMockIBCV2BatchProcessor(t)
		processor.EXPECT().ShouldProcess(mock.Anything).Return(true)
		processor.EXPECT().State().Return(db.Ibcv2RelayStatusPENDING)

		expectedBatch1Input := []*ibcv2.IBCV2Transfer{t1, t2}
		expectedBatch1Output := []*ibcv2.IBCV2Transfer{t1, t2}
		processor.EXPECT().Process(ctx, expectedBatch1Input).Return(expectedBatch1Output, nil).After(1 * time.Second).Once().Run(func(args mock.Arguments) { fmt.Println("processing first batch") })

		expectedBatch2Input := []*ibcv2.IBCV2Transfer{t3, t4}
		expectedBatch2Output := []*ibcv2.IBCV2Transfer{t3, t4}
		processor.EXPECT().Process(ctx, expectedBatch2Input).Return(expectedBatch2Output, nil).After(1 * time.Second).Once().Run(func(args mock.Arguments) { fmt.Println("processing second batch") })

		expectedBatch3Input := []*ibcv2.IBCV2Transfer{t5, t6}
		expectedBatch3Output := []*ibcv2.IBCV2Transfer{t5, t6}
		processor.EXPECT().Process(ctx, expectedBatch3Input).Return(expectedBatch3Output, nil).After(1 * time.Second).Once().Run(func(args mock.Arguments) { fmt.Println("processing third batch") })

		expectedBatch4Input := []*ibcv2.IBCV2Transfer{t7, t8}
		expectedBatch4Output := []*ibcv2.IBCV2Transfer{t7, t8}
		processor.EXPECT().Process(ctx, expectedBatch4Input).Return(expectedBatch4Output, nil).After(1 * time.Second).Once().Run(func(args mock.Arguments) { fmt.Println("processing fourth batch") })

		expectedBatch5Input := []*ibcv2.IBCV2Transfer{t9, t10}
		expectedBatch5Output := []*ibcv2.IBCV2Transfer{t9, t10}
		processor.EXPECT().Process(ctx, expectedBatch5Input).Return(expectedBatch5Output, nil).After(1 * time.Second).Once().Run(func(args mock.Arguments) { fmt.Println("processing fifth batch") })
		input := make(chan *ibcv2.IBCV2Transfer)

		concurrency := 1
		batchSize := 2
		output := ibcv2.ConditionallyBatchProcess(ctx, concurrency, batchSize, 3*time.Second, input, processor)

		for _, transfer := range allTransfers {
			// if we fail to immediately push a transfer onto the input, we
			// will fail the test
			time.Sleep(5 * time.Millisecond) // need to sleep to ensure the consuming goroutine has time to pull the input off the channel
			select {
			case input <- transfer:
			default:
				t.Fatal("could not immediately push transfer onto input channel")
			}
		}

		// dont actually care about the output, that is tested elsewhere, just
		// want to make sure the input doesnt block
		for range allTransfers {
			<-output
		}
	})
}

func splitToBatches[T any](ts []T, batchSize int) [][]T {
	batches := make([][]T, 0)
	for i := 0; i < len(ts); i += batchSize {
		end := i + batchSize
		if end > len(ts) {
			end = len(ts)
		}
		batches = append(batches, ts[i:end])
	}
	return batches
}
