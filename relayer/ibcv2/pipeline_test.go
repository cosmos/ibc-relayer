package ibcv2_test

import (
	"context"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	mock_relay_service "github.com/cosmos/ibc-relayer/mocks/proto/gen/ibcv2relayer"
	mock_ibcv2 "github.com/cosmos/ibc-relayer/mocks/relayer/ibcv2"
	mock_ibcv2_bridge "github.com/cosmos/ibc-relayer/mocks/shared/bridges/ibcv2"
	mock_config "github.com/cosmos/ibc-relayer/mocks/shared/config"
	"github.com/cosmos/ibc-relayer/proto/gen/ibcv2relayer"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	ibcv2_bridge "github.com/cosmos/ibc-relayer/shared/bridges/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
)

const (
	sourceChainID  = "1"
	sourceClientID = "client-10"

	destChainID  = "cosmoshub-4"
	destClientID = "client-256"

	mockTxHash = "8C97C1AF5AFAC31BB88CCFE4900C8D2D"
)

var mockTxTime = time.Now()

func TestPipeline(t *testing.T) {
	t.Run("single transfer through pipeline, no errors", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Twice()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := &MockAcceptingBridgeClient{}
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		// mock request to get ack tx bytes from relayer service
		recvTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		ackRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxId},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &ibcv2relayer.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Once()

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = true
		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		pipeline.Input <- &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}

		output := <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHACK, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, mockTxHash, *output.AckTxHash)
		assert.Equal(t, mockTxTime, *output.AckTxTime)
	})

	t.Run("recv tx not found for two iterations, then is found", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Twice()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mock_ibcv2_bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// packet not yet received by dest
		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil).Once()

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil).Once()
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainType_COSMOS).Once()

		// return once will cause the pipeline to error, then we check again
		// and error the pipeline again, then we check a third time and the tx
		// has then been found
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, ibcv2_bridge.ErrTxNotFound).Twice()
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil).Once()

		mockDestBridgeClient.EXPECT().IsTxFinalized(mock.Anything, mockTxHash, mock.Anything).Return(true, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusSUCCESS, nil)

		// mock request to get ack tx bytes from relayer service
		recvTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		ackRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxId},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &ibcv2relayer.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Once()

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = true

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		assert.True(t, pipeline.Push(ctx, &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}))
		output, err := pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusDELIVERRECVPACKET, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Nil(t, output.WriteAckTxHash)
		assert.Nil(t, output.WriteAckTxTime)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)

		output.ProcessingError = nil
		assert.True(t, pipeline.Push(ctx, output))
		output, err = pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusDELIVERRECVPACKET, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Nil(t, output.WriteAckTxHash)
		assert.Nil(t, output.WriteAckTxTime)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)

		output.ProcessingError = nil
		assert.True(t, pipeline.Push(ctx, output))
		output, err = pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHACK, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, mockTxHash, *output.AckTxHash)
		assert.Equal(t, mockTxTime, *output.AckTxTime)
	})

	t.Run("recv tx not found, then times out and is retried", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Times(3)
		mockStorage.EXPECT().ClearRecvTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mock_ibcv2_bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// packet not yet received by dest
		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil).Once()

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil).Once()
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainType_COSMOS).Once()

		// return once will cause the pipeline to error, then we check again
		// and error the pipeline again, then we check a third time and the tx
		// has timed out and should be retried
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, ibcv2_bridge.ErrTxNotFound).Once()
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(true, nil).Once()

		// expect to do the beginning of the pipeline again

		// packet not yet received by dest
		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil).Once()

		// mock request to get receive tx bytes from relayer service
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil).Once()
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainType_COSMOS).Once()

		// this time, the tx is found immediately and does not need to be retried
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil).Once()

		mockDestBridgeClient.EXPECT().IsTxFinalized(mock.Anything, mockTxHash, mock.Anything).Return(true, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusSUCCESS, nil)

		// mock request to get ack tx bytes from relayer service
		recvTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		ackRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxId},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &ibcv2relayer.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Once()

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = true

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		assert.True(t, pipeline.Push(ctx, &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}))
		output, err := pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusDELIVERRECVPACKET, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Nil(t, output.WriteAckTxHash)
		assert.Nil(t, output.WriteAckTxTime)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)

		output.ProcessingError = nil
		assert.True(t, pipeline.Push(ctx, output))
		output, err = pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusDELIVERRECVPACKET, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Nil(t, output.WriteAckTxHash)
		assert.Nil(t, output.WriteAckTxTime)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)

		output.RecvTxHash = nil
		output.RecvTxTime = nil
		output.ProcessingError = nil
		assert.True(t, pipeline.Push(ctx, output))
		output, err = pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHACK, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, mockTxHash, *output.AckTxHash)
		assert.Equal(t, mockTxTime, *output.AckTxTime)
	})

	t.Run("success ack transfer, pipeline does not relay success acks", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mock_ibcv2_bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)
		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil)
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainType_COSMOS)
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusSUCCESS, nil)

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = false
		opts.ShouldRelayErrorAcks = true

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		pipeline.Push(ctx, &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		})

		output, err := pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKSUCCESS, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, db.Ibcv2WriteAckStatusSUCCESS, *output.WriteAckStatus)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)
	})

	t.Run("timed out transfer cosmos destination chain", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := &MockAcceptingBridgeClient{}
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get timeout tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			TimeoutTxIds:       [][]byte{sourceTxId},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		timeoutTxBytes := []byte("123456")
		timeoutResponse := &ibcv2relayer.RelayByTxResponse{Tx: timeoutTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(timeoutResponse, nil).Once()

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			ibcv2.NewSmallPipelineOpts(),
		)

		packetTimeoutTs := time.Now() // timed out!

		pipeline.Input <- &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}

		output := <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHTIMEOUT, output.GetState())
		assert.Equal(t, mockTxHash, *output.TimeoutTxHash)
		assert.Equal(t, mockTxTime, *output.TimeoutTxTime)
		assert.Nil(t, output.RecvTxHash)
		assert.Nil(t, output.RecvTxTime)
		assert.Nil(t, output.WriteAckTxHash)
		assert.Nil(t, output.WriteAckTxTime)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)
	})

	t.Run("timed out transfer evm destination chain", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10
		packetTimeoutTs := time.Now()

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mock_ibcv2_bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)

		// simulate the chain advancing into the future and finality progressing
		// First 4 calls: timeout not yet finalized, then on 5th call it's finalized
		mockDestBridgeClient.EXPECT().IsTimestampFinalized(mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()
		mockDestBridgeClient.EXPECT().IsTimestampFinalized(mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()
		mockDestBridgeClient.EXPECT().IsTimestampFinalized(mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()
		mockDestBridgeClient.EXPECT().IsTimestampFinalized(mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()
		mockDestBridgeClient.EXPECT().IsTimestampFinalized(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
		// now the timeout is considered finalized

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get timeout tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			TimeoutTxIds:       [][]byte{sourceTxId},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		timeoutTxBytes := []byte("123456")
		timeoutResponse := &ibcv2relayer.RelayByTxResponse{Tx: timeoutTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(timeoutResponse, nil).Once()

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			ibcv2.NewSmallPipelineOpts(),
		)

		pipeline.Input <- &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}

		output := <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusAWAITINGTIMEOUTFINALITY, output.GetState())
		assert.Nil(t, output.TimeoutTxHash)
		assert.Nil(t, output.TimeoutTxTime)
		output.ProcessingError = nil
		pipeline.Push(ctx, output)

		output = <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusAWAITINGTIMEOUTFINALITY, output.GetState())
		assert.Nil(t, output.TimeoutTxHash)
		assert.Nil(t, output.TimeoutTxTime)
		output.ProcessingError = nil
		pipeline.Push(ctx, output)

		output = <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusAWAITINGTIMEOUTFINALITY, output.GetState())
		assert.Nil(t, output.TimeoutTxHash)
		assert.Nil(t, output.TimeoutTxTime)
		output.ProcessingError = nil
		pipeline.Push(ctx, output)

		output = <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusAWAITINGTIMEOUTFINALITY, output.GetState())
		assert.Nil(t, output.TimeoutTxHash)
		assert.Nil(t, output.TimeoutTxTime)
		output.ProcessingError = nil
		pipeline.Push(ctx, output)

		output = <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHTIMEOUT, output.GetState())
		assert.Equal(t, mockTxHash, *output.TimeoutTxHash)
		assert.Equal(t, mockTxTime, *output.TimeoutTxTime)
	})

	t.Run("two sequential transfers through pipeline, no errors", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Times(4)
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := &MockAcceptingBridgeClient{}
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Twice()

		// mock request to get ack tx bytes from relayer service
		recvTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		ackRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxId},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &ibcv2relayer.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Twice()

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = true
		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		pipeline.Input <- &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}

		output := <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHACK, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, mockTxHash, *output.AckTxHash)
		assert.Equal(t, mockTxTime, *output.AckTxTime)

		pipeline.Input <- &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		}

		output = <-pipeline.Output
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHACK, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, mockTxHash, *output.AckTxHash)
		assert.Equal(t, mockTxTime, *output.AckTxTime)
	})

	t.Run("error ack transfer, pipeline does not relay error acks", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mock_ibcv2_bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)
		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil)
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainType_COSMOS)
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusERROR, nil)

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = true
		opts.ShouldRelayErrorAcks = false

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		pipeline.Push(ctx, &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		})

		output, err := pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKERROR, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, db.Ibcv2WriteAckStatusERROR, *output.WriteAckStatus)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)
	})

	t.Run("unknown ack status transfer, pipeline does not relay error acks", func(t *testing.T) {
		ctx := context.Background()

		configReader := mock_config.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mock_ibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mock_ibcv2_bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2_bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mock_relay_service.NewMockRelayerServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxId, err := hex.DecodeString(mockTxHash)
		assert.NoError(t, err)
		recvRequest := &ibcv2relayer.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxId},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &ibcv2relayer.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)
		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil)
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainType_COSMOS)
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusUNKNOWN, nil)

		priceClient := mock_ibcv2.NewMockPriceClient(t)

		opts := ibcv2.NewSmallPipelineOpts()
		opts.ShouldRelaySuccessAcks = true
		opts.ShouldRelayErrorAcks = false

		pipeline := ibcv2.NewPipeline(
			ctx,
			mockStorage,
			bridgeClientManager,
			mockRelayService,
			priceClient,
			sourceChainID,
			sourceClientID,
			destChainID,
			destClientID,
			opts,
		)

		packetTimeoutTs := time.Now().Add(time.Hour)

		pipeline.Push(ctx, &ibcv2.IBCV2Transfer{
			State:                     db.Ibcv2RelayStatusPENDING,
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              mockTxHash,
			SourceTxTime:              mockTxTime,
			PacketSequenceNumber:      uint32(packetSequenceNumber),
			PacketSourceClientID:      sourceClientID,
			PacketDestinationClientID: destClientID,
			PacketTimeoutTimestamp:    packetTimeoutTs,
		})

		output, err := pipeline.Poll()
		assert.NoError(t, err)
		assert.Equal(t, db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKERROR, output.GetState())
		assert.Equal(t, mockTxHash, *output.RecvTxHash)
		assert.Equal(t, mockTxTime, *output.RecvTxTime)
		assert.Equal(t, mockTxHash, *output.WriteAckTxHash)
		assert.Equal(t, mockTxTime, *output.WriteAckTxTime)
		assert.Equal(t, db.Ibcv2WriteAckStatusUNKNOWN, *output.WriteAckStatus)
		assert.Nil(t, output.AckTxHash)
		assert.Nil(t, output.AckTxTime)
	})
}

func TestPipelineDeduper(t *testing.T) {
	t.Run("cant push duplicated transfers onto the pipeline", func(t *testing.T) {
		ctx := context.Background()

		mockPipeline := newDisconnectedIBCV2Pipeline()
		deduper := ibcv2.NewPipelineDeduper(mockPipeline)

		transfer := newTransfer(sourceChainID, destChainID, sourceClientID, 0)
		assert.True(t, deduper.Push(ctx, transfer))

		// cant push the same transfer twice until it has been pushed to the output channel
		assert.False(t, deduper.Push(ctx, transfer))

		// push input onto output channel
		in := <-mockPipeline.Input
		mockPipeline.Output <- in

		// wait for .5 seconds to make sure the deduper sees the output
		time.Sleep(500 * time.Millisecond)

		// ensure we can now push the transfer again
		assert.True(t, deduper.Push(ctx, transfer))
	})

	t.Run("concurrent duplicate pushes results in only one on pipeline", func(t *testing.T) {
		ctx := context.Background()

		mockPipeline := newDisconnectedIBCV2Pipeline()
		deduper := ibcv2.NewPipelineDeduper(mockPipeline)

		transfer := newTransfer(sourceChainID, destChainID, sourceClientID, 0)

		go deduper.Push(ctx, transfer)
		go deduper.Push(ctx, transfer)

		// cant use a wait group for this ddddd since that introduces too much
		// latency between spawning the goroutines and the push call being
		// executed where even if the locking is incorrect, still only one will
		// be pushed onto the pipeline. to get around this, we simply sleep for
		// 100ms
		time.Sleep(100 * time.Millisecond)

		assert.Len(t, mockPipeline.Input, 1)
	})
}

type MockAcceptingBridgeClient struct{}

func (mock *MockAcceptingBridgeClient) IsPacketReceived(ctx context.Context, clientID string, sequence uint64) (bool, error) {
	return false, nil
}

func (mock *MockAcceptingBridgeClient) IsPacketCommitted(ctx context.Context, clientID string, sequence uint64) (bool, error) {
	return true, nil
}

func (mock *MockAcceptingBridgeClient) FindRecvTx(ctx context.Context, sourceClientID string, destClientID string, sequence uint64, timeoutTimestamp time.Time) (*ibcv2_bridge.BridgeTx, error) {
	panic("unimplemented")
}

func (mock *MockAcceptingBridgeClient) FindAckTx(ctx context.Context, sourceClientID string, destClientID string, sequence uint64) (*ibcv2_bridge.BridgeTx, error) {
	panic("unimplemented")
}

func (mock *MockAcceptingBridgeClient) FindTimeoutTx(ctx context.Context, sourceClientID string, destClientID string, sequence uint64) (*ibcv2_bridge.BridgeTx, error) {
	panic("unimplemented")
}

func (mock *MockAcceptingBridgeClient) DeliverTx(ctx context.Context, tx []byte, address string) (*ibcv2_bridge.BridgeTx, error) {
	return &ibcv2_bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil
}

func (mock *MockAcceptingBridgeClient) PacketWriteAckStatus(ctx context.Context, hash string, sequence uint64, sourceClientID string, destClientID string) (db.Ibcv2WriteAckStatus, error) {
	return db.Ibcv2WriteAckStatusSUCCESS, nil
}

func (mock *MockAcceptingBridgeClient) SendPacketsFromTx(ctx context.Context, sourceChainID string, txHash string) ([]*ibcv2_bridge.PacketInfo, error) {
	return nil, nil
}

func (mock *MockAcceptingBridgeClient) ShouldRetryTx(ctx context.Context, txHash string, expiry time.Duration, sentAtTx time.Time) (bool, error) {
	return false, nil
}

func (mock *MockAcceptingBridgeClient) IsTxFinalized(ctx context.Context, txHash string, offset *uint64) (bool, error) {
	return true, nil
}

func (mock *MockAcceptingBridgeClient) IsTimestampFinalized(ctx context.Context, timestamp time.Time, offset *uint64) (bool, error) {
	return true, nil
}

func (mock *MockAcceptingBridgeClient) WaitForChain(ctx context.Context) error {
	return nil
}

func (mock *MockAcceptingBridgeClient) LatestOnChainTimestamp(ctx context.Context) (time.Time, error) {
	return time.Time{}, nil
}

func (mock *MockAcceptingBridgeClient) ChainType() config.ChainType {
	return config.ChainType_COSMOS
}

func (mock *MockAcceptingBridgeClient) SignerGasTokenBalance(ctx context.Context) (*big.Int, error) {
	return nil, nil
}

func (mock *MockAcceptingBridgeClient) TxFee(ctx context.Context, txHash string) (*big.Int, error) {
	return nil, nil
}

func (mock *MockAcceptingBridgeClient) SendTransfer(ctx context.Context, clientID string, denom string, receiver string, amount *big.Int, memo string) (string, error) {
	return "", nil
}

func (mock *MockAcceptingBridgeClient) ClientState(ctx context.Context, clientID string) (ibcv2_bridge.ClientState, error) {
	return ibcv2_bridge.ClientState{}, nil
}

func (mock *MockAcceptingBridgeClient) TimestampAtHeight(ctx context.Context, height uint64) (time.Time, error) {
	return time.Now(), nil
}

func (mock *MockAcceptingBridgeClient) WaitForTx(ctx context.Context, hash string) error {
	return nil
}

func (mock *MockAcceptingBridgeClient) GetTransactionSender(ctx context.Context, hash string) (string, error) {
	// Return a non-blacklisted address by default
	return "0x0000000000000000000000000000000000000000", nil
}
