package ibcv2_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	mockibcv2 "github.com/cosmos/ibc-relayer/mocks/relayer/ibcv2"
	mockconfig "github.com/cosmos/ibc-relayer/mocks/shared/config"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
)

func TestIBCV2ProcessorMW_Process(t *testing.T) {
	sourceChainID := "sourceChainID"
	packetSequenceNumber := 10
	packetSourceClientID := "client-10"

	t.Run("happy path, state updated, processor output returned", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)
		input := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusPENDING,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: uint32(packetSequenceNumber),
			PacketSourceClientID: packetSourceClientID,
		}

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)

		// expect that a storage update will happen, updating the transfer to
		// the state that is about to be processed
		mockUpdate := db.UpdateTransferStateParams{
			Status:               db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        input.GetSourceChainID(),
			PacketSequenceNumber: int32(input.GetPacketSequenceNumber()),
			PacketSourceClientID: input.GetPacketSourceClientID(),
		}
		mockStorage.EXPECT().UpdateTransferState(ctx, mockUpdate).Return(nil)

		mockProcessor := mockibcv2.NewMockIBCV2Processor(t)

		// expect that the input to the mock processor will have its state
		// updated to be the state of the mock processor
		processorInput := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: uint32(packetSequenceNumber),
			PacketSourceClientID: packetSourceClientID,
		}
		recvTxHash := "0xdeadbeef"
		processorOutput := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        input.GetSourceChainID(),
			PacketSequenceNumber: input.GetPacketSequenceNumber(),
			PacketSourceClientID: input.GetPacketSourceClientID(),
			RecvTxHash:           &recvTxHash,
		}
		mockProcessor.EXPECT().Process(ctx, processorInput).Return(processorOutput, nil)

		// this mock processor should process this input
		mockProcessor.EXPECT().ShouldProcess(input).Return(true)
		// the state that the mock processor should return
		mockProcessor.EXPECT().State().Return(db.Ibcv2RelayStatusGETACKPACKET)

		output, err := ibcv2.NewIBCV2ProcessorMW(mockStorage, mockProcessor).Process(ctx, input)
		require.NoError(t, err)

		// expect that the output from the processor is returned unchanged
		assert.Equal(t, *processorOutput, *output)
	})

	t.Run("transfer in error state is not processed", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		input := &ibcv2.IBCV2Transfer{ProcessingError: errors.New("error")}

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)

		mockProcessor := mockibcv2.NewMockIBCV2Processor(t)

		output, err := ibcv2.NewIBCV2ProcessorMW(mockStorage, mockProcessor).Process(ctx, input)
		require.NoError(t, err)

		// the mockProcessor should not be called when the input has errored,
		// so the input should be returned as the output
		assert.Equal(t, *input, *output)
	})

	t.Run("transfer is not processed and state not updated if processor.ShouldProcess returns false", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)
		input := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusPENDING,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: uint32(packetSequenceNumber),
			PacketSourceClientID: packetSourceClientID,
		}

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)

		mockProcessor := mockibcv2.NewMockIBCV2Processor(t)

		// this mock processor should *NOT* process this input
		mockProcessor.EXPECT().ShouldProcess(input).Return(false)

		output, err := ibcv2.NewIBCV2ProcessorMW(mockStorage, mockProcessor).Process(ctx, input)
		require.NoError(t, err)

		// the mockProcessor should not be called when ShouldProcess has
		// returned false so the input should be returned as output
		assert.Equal(t, *input, *output)
	})

	t.Run("updating transfer state fails, processor is not called, cancel function is called", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)
		input := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusPENDING,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: uint32(packetSequenceNumber),
			PacketSourceClientID: packetSourceClientID,
		}
		expected := *input

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)

		// expect that a storage update will be attempted and then fail
		stateUpdateErr := errors.New("update state error")
		mockUpdate := db.UpdateTransferStateParams{
			Status:               db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        input.GetSourceChainID(),
			PacketSequenceNumber: int32(input.GetPacketSequenceNumber()),
			PacketSourceClientID: input.GetPacketSourceClientID(),
		}
		mockStorage.EXPECT().UpdateTransferState(ctx, mockUpdate).Return(stateUpdateErr)

		mockProcessor := mockibcv2.NewMockIBCV2Processor(t)

		// this mock processor should process this input
		mockProcessor.EXPECT().ShouldProcess(input).Return(true)

		// expect that the processors cancel function is called with an
		// unmodified input (i.e. no state update yet) and the error that
		// occurred.
		// NOTE: using a custom matcher here since the processor will wrap the
		// error before calling cancel, so we have to check if the wrapped
		// error contains our target error, if we simply require 'err' as the
		// arg to Cancel, this will fail
		mockProcessor.EXPECT().Cancel(input, mock.MatchedBy(func(arg error) bool { return errors.Is(arg, stateUpdateErr) }))

		// the state that the mock processor should return
		mockProcessor.EXPECT().State().Return(db.Ibcv2RelayStatusGETACKPACKET)

		output, err := ibcv2.NewIBCV2ProcessorMW(mockStorage, mockProcessor).Process(ctx, input)
		require.NoError(t, err)

		// expected that the output ProcessingError contains the stateUpdateErr
		require.ErrorIs(t, output.ProcessingError, stateUpdateErr)

		// have to clear the output ProcessingError since it is wrapped and
		// wont actually match the stateUpdateError when we compare the structs
		output.ProcessingError = nil

		// expect the output is equal to the original input, i.e no state
		// changes happen to the output
		assert.Equal(t, expected, *output)
	})

	t.Run("processing input fails, cancel function is called", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)
		input := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusPENDING,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: uint32(packetSequenceNumber),
			PacketSourceClientID: packetSourceClientID,
		}
		expected := *input

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)

		// expect that a storage update will happen, updating the transfer to
		// the state that is about to be processed
		mockUpdate := db.UpdateTransferStateParams{
			Status:               db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        input.GetSourceChainID(),
			PacketSequenceNumber: int32(input.GetPacketSequenceNumber()),
			PacketSourceClientID: input.GetPacketSourceClientID(),
		}
		mockStorage.EXPECT().UpdateTransferState(ctx, mockUpdate).Return(nil)

		mockProcessor := mockibcv2.NewMockIBCV2Processor(t)

		// expect that the input to the mock processor will have its state
		// updated to be the state of the mock processor
		processorInput := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: uint32(packetSequenceNumber),
			PacketSourceClientID: packetSourceClientID,
		}
		processingErr := errors.New("processing error")
		mockProcessor.EXPECT().Process(ctx, processorInput).Return(nil, processingErr)

		// this mock processor should process this input
		mockProcessor.EXPECT().ShouldProcess(input).Return(true)
		// the state that the mock processor should return
		mockProcessor.EXPECT().State().Return(db.Ibcv2RelayStatusGETACKPACKET)
		// expect that the processors cancel function is called with an
		// unmodified input (i.e. no state update yet) and the error that
		// occurred.
		// NOTE: using a custom matcher here since the processor will wrap the
		// error before calling cancel, so we have to check if the wrapped
		// error contains our target error, if we simply require 'err' as the
		// arg to Cancel, this will fail
		mockProcessor.EXPECT().Cancel(input, mock.MatchedBy(func(arg error) bool { return errors.Is(arg, processingErr) }))

		output, err := ibcv2.NewIBCV2ProcessorMW(mockStorage, mockProcessor).Process(ctx, input)
		require.NoError(t, err)

		// expected that the output ProcessingError contains the processingErr
		require.ErrorIs(t, output.ProcessingError, processingErr)

		// have to clear the output ProcessingError since it is wrapped and
		// wont actually match the processingErr when we compare the structs
		output.ProcessingError = nil

		// expect that the input is returned as output *WITH* its state updated
		expected.State = db.Ibcv2RelayStatusGETACKPACKET
		assert.Equal(t, expected, *output)
	})
}

func TestIBCV2BatchProcessorMW_Process(t *testing.T) {
	sourceChainID := "sourceChainID"
	packetSourceClientID := "client-10"

	t.Run("empty toProcess batch is a no-op", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)
		input := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusPENDING,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: 10,
			PacketSourceClientID: packetSourceClientID,
			ProcessingError:      errors.New("error"),
		}

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)
		mockProcessor := mockibcv2.NewMockIBCV2BatchProcessor(t)
		mockProcessor.EXPECT().State().Return(db.Ibcv2RelayStatusGETACKPACKET)

		output, err := ibcv2.NewIBCV2BatchProcessorMW(mockStorage, mockProcessor).Process(ctx, []*ibcv2.IBCV2Transfer{input})
		require.NoError(t, err)
		assert.Equal(t, []*ibcv2.IBCV2Transfer{input}, output)
	})

	t.Run("state update failure skips processing for that transfer", func(t *testing.T) {
		ctx := context.Background()
		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)
		input := &ibcv2.IBCV2Transfer{
			State:                db.Ibcv2RelayStatusPENDING,
			SourceChainID:        sourceChainID,
			PacketSequenceNumber: 11,
			PacketSourceClientID: packetSourceClientID,
		}
		expected := *input

		mockStorage := mockibcv2.NewMockTransferStateStorage(t)
		stateUpdateErr := errors.New("update state error")
		mockStorage.EXPECT().UpdateTransferState(ctx, db.UpdateTransferStateParams{
			Status:               db.Ibcv2RelayStatusGETACKPACKET,
			SourceChainID:        input.GetSourceChainID(),
			PacketSequenceNumber: int32(input.GetPacketSequenceNumber()),
			PacketSourceClientID: input.GetPacketSourceClientID(),
		}).Return(stateUpdateErr)

		mockProcessor := mockibcv2.NewMockIBCV2BatchProcessor(t)
		mockProcessor.EXPECT().State().Return(db.Ibcv2RelayStatusGETACKPACKET)
		mockProcessor.EXPECT().ShouldProcess(input).Return(true)
		mockProcessor.EXPECT().Cancel([]*ibcv2.IBCV2Transfer{input}, mock.MatchedBy(func(arg error) bool {
			return errors.Is(arg, stateUpdateErr)
		}))

		output, err := ibcv2.NewIBCV2BatchProcessorMW(mockStorage, mockProcessor).Process(ctx, []*ibcv2.IBCV2Transfer{input})
		require.NoError(t, err)
		require.Len(t, output, 1)
		require.ErrorIs(t, output[0].ProcessingError, stateUpdateErr)

		output[0].ProcessingError = nil
		assert.Equal(t, expected, *output[0])
	})
}
