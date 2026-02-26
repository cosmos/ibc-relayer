package ibcv2_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cosmos/platform-relayer/db/gen/db"
	mock_ibcv2 "github.com/cosmos/platform-relayer/mocks/relayer/ibcv2"
	mock_config "github.com/cosmos/platform-relayer/mocks/shared/config"
	"github.com/cosmos/platform-relayer/relayer/ibcv2"
	"github.com/cosmos/platform-relayer/shared/config"
	"github.com/cosmos/platform-relayer/shared/lmt"
)

const (
	evmChainID    = "evmChainID"
	cosmosChainID = "cosmosChainID"
	clientID      = "client-10"
	inputSize     = 100
)

type MockPipeline struct {
	Input  chan *ibcv2.IBCV2Transfer
	Output chan *ibcv2.IBCV2Transfer
}

// newDisconnectedIBCV2Pipeline creates a ibcv2 pipeline where the input is
// not sent to the output
func newDisconnectedIBCV2Pipeline() *MockPipeline {
	input := make(chan *ibcv2.IBCV2Transfer, inputSize)
	output := make(chan *ibcv2.IBCV2Transfer)

	return &MockPipeline{
		Input:  input,
		Output: output,
	}
}

func (m *MockPipeline) Push(ctx context.Context, transfer *ibcv2.IBCV2Transfer) bool {
	select {
	case <-ctx.Done():
		return false
	default:
		select {
		case <-ctx.Done():
			return false
		case m.Input <- transfer:
			return true
		}
	}
}

func (m *MockPipeline) Poll() (*ibcv2.IBCV2Transfer, error) {
	t, ok := <-m.Output
	if !ok {
		return t, fmt.Errorf("closed")
	}
	return t, nil
}

func (m *MockPipeline) Close() {
}

func TestRelayDispatcher_Run(t *testing.T) {
	t.Run("submits all waiting transfers until context is cancelled", func(t *testing.T) {
		// this test doenst assert much (besides checking that the error is
		// what we expect), but the real test here is making sure this doesnt
		// dead lock and properly drains the output, which if this returns we
		// did properly

		lmt.ConfigureLogger()
		ctx, cancel := context.WithCancel(lmt.LoggerContext(context.Background()))

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		mockStorage := mock_ibcv2.NewMockUnfinishedIBCV2TransferStorage(t)

		unfinishedTranfsers := []db.Ibcv2Transfer{
			newDBTransfer(evmChainID, cosmosChainID, clientID, 0),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 1),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 2),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 3),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 4),
		}
		mockStorage.EXPECT().GetUnfinishedIBCV2Transfers(ctx).Return(unfinishedTranfsers, nil)

		mockPipeline := newDisconnectedIBCV2Pipeline()
		mockManager := mock_ibcv2.NewMockPipelineManager(t)
		mockManager.EXPECT().Close()
		for _, transfer := range unfinishedTranfsers {
			t := ibcv2.NewIBCV2Transfer(ctx, transfer)
			mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
				return arg.PacketSequenceNumber == t.PacketSequenceNumber
			})).Return(mockPipeline, nil)
		}

		poll := time.Minute // only poll once per minute
		pusher := ibcv2.NewRelayDispatcher(mockStorage, poll, mockManager, true)

		// cancel context after waiting .5 seconds
		go func() {
			time.Sleep(500 * time.Millisecond)
			cancel()
		}()

		assert.ErrorIs(t, pusher.Run(ctx), context.Canceled)
	})
}

func TestRelayDispatcher_SubmitUnfinishedTransfers(t *testing.T) {
	t.Run("submits all unfinished transfers", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		mockStorage := mock_ibcv2.NewMockUnfinishedIBCV2TransferStorage(t)

		unfinishedTranfsers := []db.Ibcv2Transfer{
			newDBTransfer(evmChainID, cosmosChainID, clientID, 0),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 1),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 2),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 3),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 4),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 5),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 6),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 7),
		}
		mockStorage.EXPECT().GetUnfinishedIBCV2Transfers(ctx).Return(unfinishedTranfsers, nil)

		mockPipeline := newDisconnectedIBCV2Pipeline()
		mockManager := mock_ibcv2.NewMockPipelineManager(t)
		for _, transfer := range unfinishedTranfsers {
			t := ibcv2.NewIBCV2Transfer(ctx, transfer)
			mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
				return arg.PacketSequenceNumber == t.PacketSequenceNumber
			})).Return(mockPipeline, nil)
		}

		pusher := ibcv2.NewRelayDispatcher(mockStorage, 0, mockManager, true)

		assert.NoError(t, pusher.SubmitWaitingUnfinishedTransfers(ctx))
		assert.Len(t, mockPipeline.Input, len(unfinishedTranfsers))
	})

	t.Run("error getting unfinished transfers", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		mockStorage := mock_ibcv2.NewMockUnfinishedIBCV2TransferStorage(t)

		getTransfersErr := fmt.Errorf("error")
		mockStorage.EXPECT().GetUnfinishedIBCV2Transfers(ctx).Return(nil, getTransfersErr)

		pusher := ibcv2.NewRelayDispatcher(mockStorage, 0, nil, true)

		assert.ErrorIs(t, pusher.SubmitWaitingUnfinishedTransfers(ctx), getTransfersErr)
	})

	t.Run("submitting transfer fails, transfer is marked as failed", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		mockStorage := mock_ibcv2.NewMockUnfinishedIBCV2TransferStorage(t)
		mockStorage.EXPECT().UpdateTransferState(ctx, db.UpdateTransferStateParams{Status: db.Ibcv2RelayStatusFAILED, SourceChainID: evmChainID, PacketSourceClientID: clientID, PacketSequenceNumber: 0}).Return(nil)

		unfinishedTranfsers := []db.Ibcv2Transfer{newDBTransfer(evmChainID, cosmosChainID, clientID, 0)}
		mockStorage.EXPECT().GetUnfinishedIBCV2Transfers(ctx).Return(unfinishedTranfsers, nil)

		mockPipeline := newDisconnectedIBCV2Pipeline()
		mockManager := mock_ibcv2.NewMockPipelineManager(t)
		for _, transfer := range unfinishedTranfsers {
			t := ibcv2.NewIBCV2Transfer(ctx, transfer)
			mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
				return arg.PacketSequenceNumber == t.PacketSequenceNumber
			})).Return(nil, fmt.Errorf("unexpected error"))
		}

		pusher := ibcv2.NewRelayDispatcher(mockStorage, 0, mockManager, true)

		assert.NoError(t, pusher.SubmitWaitingUnfinishedTransfers(ctx))
		assert.Empty(t, mockPipeline.Input)
	})

	t.Run("submitting transfer fails, others in batch are still submitted", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		mockStorage := mock_ibcv2.NewMockUnfinishedIBCV2TransferStorage(t)
		mockStorage.EXPECT().UpdateTransferState(ctx, db.UpdateTransferStateParams{Status: db.Ibcv2RelayStatusFAILED, SourceChainID: evmChainID, PacketSourceClientID: clientID, PacketSequenceNumber: 0}).Return(nil)

		unfinishedTranfsers := []db.Ibcv2Transfer{
			newDBTransfer(evmChainID, cosmosChainID, clientID, 0),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 1),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 2),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 3),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 4),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 5),
			newDBTransfer(evmChainID, cosmosChainID, clientID, 6),
		}

		mockStorage.EXPECT().GetUnfinishedIBCV2Transfers(ctx).Return(unfinishedTranfsers, nil)

		mockManager := mock_ibcv2.NewMockPipelineManager(t)
		mockPipeline := newDisconnectedIBCV2Pipeline()

		transfer := ibcv2.NewIBCV2Transfer(ctx, unfinishedTranfsers[0])
		mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
			return arg.PacketSequenceNumber == transfer.PacketSequenceNumber
		})).Return(nil, fmt.Errorf("unexpected error"))

		for _, transfer := range unfinishedTranfsers[1:] {
			t := ibcv2.NewIBCV2Transfer(ctx, transfer)
			mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
				return arg.PacketSequenceNumber == t.PacketSequenceNumber
			})).Return(mockPipeline, nil)
		}

		pusher := ibcv2.NewRelayDispatcher(mockStorage, 0, mockManager, true)

		assert.NoError(t, pusher.SubmitWaitingUnfinishedTransfers(ctx))
		assert.Len(t, mockPipeline.Input, len(unfinishedTranfsers)-1)
	})
}

func TestRelayDispatcher_SubmitTransfer(t *testing.T) {
	t.Run("gets pipeline and pushes", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		t1 := newTransfer(evmChainID, cosmosChainID, clientID, 0)
		t2 := newTransfer(evmChainID, cosmosChainID, clientID, 1)

		mockPipeline := newDisconnectedIBCV2Pipeline()
		mockManager := mock_ibcv2.NewMockPipelineManager(t)
		mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
			return arg.PacketSequenceNumber == 0
		})).Return(mockPipeline, nil)
		mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
			return arg.PacketSequenceNumber == 1
		})).Return(mockPipeline, nil)

		pusher := ibcv2.NewRelayDispatcher(nil, 0, mockManager, true)

		assert.NoError(t, pusher.SubmitTransfer(ctx, t1))
		assert.NoError(t, pusher.SubmitTransfer(ctx, t2))
		assert.Len(t, mockPipeline.Input, 2)
	})

	t.Run("returns an error when transfer isnt pushed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{IBCV2: &config.IBCV2Config{}}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		t1 := newTransfer(evmChainID, cosmosChainID, clientID, 0)
		t2 := newTransfer(evmChainID, cosmosChainID, clientID, 1)

		mockPipeline := newDisconnectedIBCV2Pipeline()
		mockManager := mock_ibcv2.NewMockPipelineManager(t)
		mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
			return arg.PacketSequenceNumber == t1.PacketSequenceNumber
		})).Return(mockPipeline, nil)
		mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
			return arg.PacketSequenceNumber == t2.PacketSequenceNumber
		})).Return(mockPipeline, nil)

		pusher := ibcv2.NewRelayDispatcher(nil, 0, mockManager, true)
		assert.NoError(t, pusher.SubmitTransfer(ctx, t1))
		assert.NoError(t, pusher.SubmitTransfer(ctx, t2))
		assert.Len(t, mockPipeline.Input, 2)

		cancel()

		t3 := newTransfer(evmChainID, cosmosChainID, clientID, 3)
		mockManager.EXPECT().Pipeline(mock.Anything, mock.MatchedBy(func(arg *ibcv2.IBCV2Transfer) bool {
			return arg.PacketSequenceNumber == t3.PacketSequenceNumber
		})).Return(mockPipeline, nil)

		assert.Error(t, pusher.SubmitTransfer(ctx, t3))
		assert.Len(t, mockPipeline.Input, 2)
	})
}

func newTransfer(sourceChainID, destinationChainID, packetSourceClientID string, packetSequenceNumber uint32) *ibcv2.IBCV2Transfer {
	return &ibcv2.IBCV2Transfer{
		SourceChainID:        sourceChainID,
		PacketSequenceNumber: packetSequenceNumber,
		PacketSourceClientID: packetSourceClientID,
		DestinationChainID:   destinationChainID,
	}
}

func newDBTransfer(sourceChainID, destinationChainID, packetSourceClientID string, packetSequenceNumber int32) db.Ibcv2Transfer {
	return db.Ibcv2Transfer{
		SourceChainID:        sourceChainID,
		PacketSequenceNumber: packetSequenceNumber,
		PacketSourceClientID: packetSourceClientID,
		DestinationChainID:   destinationChainID,
	}
}
