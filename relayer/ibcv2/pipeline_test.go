package ibcv2_test

import (
	"context"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	mockproofapi "github.com/cosmos/ibc-relayer/mocks/proto/gen/proofapi"
	mockibcv2 "github.com/cosmos/ibc-relayer/mocks/relayer/ibcv2"
	mockibcv2bridge "github.com/cosmos/ibc-relayer/mocks/shared/bridges/ibcv2"
	mockconfig "github.com/cosmos/ibc-relayer/mocks/shared/config"
	"github.com/cosmos/ibc-relayer/proto/gen/proofapi"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	ibcv2bridge "github.com/cosmos/ibc-relayer/shared/bridges/ibcv2"
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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Twice()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := &MockAcceptingBridgeClient{}
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		// mock request to get ack tx bytes from relayer service
		recvTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		ackRequest := &proofapi.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxID},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &proofapi.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Once()

		priceClient := mockibcv2.NewMockPriceClient(t)

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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Twice()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mockibcv2bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// packet not yet received by dest
		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil).Once()

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil).Once()
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainTypeCOSMOS).Once()

		// return once will cause the pipeline to error, then we check again
		// and error the pipeline again, then we check a third time and the tx
		// has then been found
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, ibcv2bridge.ErrTxNotFound).Twice()
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil).Once()

		mockDestBridgeClient.EXPECT().IsTxFinalized(mock.Anything, mockTxHash, mock.Anything).Return(true, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusSUCCESS, nil)

		// mock request to get ack tx bytes from relayer service
		recvTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		ackRequest := &proofapi.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxID},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &proofapi.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Once()

		priceClient := mockibcv2.NewMockPriceClient(t)

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
		require.NoError(t, err)
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
		require.NoError(t, err)
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
		require.NoError(t, err)
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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Times(3)
		mockStorage.EXPECT().ClearRecvTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mockibcv2bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// packet not yet received by dest
		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil).Once()

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil).Once()
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainTypeCOSMOS).Once()

		// return once will cause the pipeline to error, then we check again
		// and error the pipeline again, then we check a third time and the tx
		// has timed out and should be retried
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, ibcv2bridge.ErrTxNotFound).Once()
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(true, nil).Once()

		// expect to do the beginning of the pipeline again

		// packet not yet received by dest
		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil).Once()

		// mock request to get receive tx bytes from relayer service
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil).Once()
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainTypeCOSMOS).Once()

		// this time, the tx is found immediately and does not need to be retried
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil).Once()

		mockDestBridgeClient.EXPECT().IsTxFinalized(mock.Anything, mockTxHash, mock.Anything).Return(true, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusSUCCESS, nil)

		// mock request to get ack tx bytes from relayer service
		recvTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		ackRequest := &proofapi.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxID},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &proofapi.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Once()

		priceClient := mockibcv2.NewMockPriceClient(t)

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
		require.NoError(t, err)
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
		require.NoError(t, err)
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
		require.NoError(t, err)
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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mockibcv2bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)
		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil)
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainTypeCOSMOS)
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusSUCCESS, nil)

		priceClient := mockibcv2.NewMockPriceClient(t)

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
		require.NoError(t, err)
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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := &MockAcceptingBridgeClient{}
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get timeout tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			TimeoutTxIds:       [][]byte{sourceTxID},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		timeoutTxBytes := []byte("123456")
		timeoutResponse := &proofapi.RelayByTxResponse{Tx: timeoutTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(timeoutResponse, nil).Once()

		priceClient := mockibcv2.NewMockPriceClient(t)

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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10
		packetTimeoutTs := time.Now()

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mockibcv2bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
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

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get timeout tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			TimeoutTxIds:       [][]byte{sourceTxID},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		timeoutTxBytes := []byte("123456")
		timeoutResponse := &proofapi.RelayByTxResponse{Tx: timeoutTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(timeoutResponse, nil).Once()

		priceClient := mockibcv2.NewMockPriceClient(t)

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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Times(4)
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := &MockAcceptingBridgeClient{}
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Twice()

		// mock request to get ack tx bytes from relayer service
		recvTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		ackRequest := &proofapi.RelayByTxRequest{
			SrcChain:           destChainID,
			DstChain:           sourceChainID,
			SourceTxIds:        [][]byte{recvTxID},
			SrcClientId:        destClientID,
			DstClientId:        sourceClientID,
			DstPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		ackTxBytes := []byte("123456")
		ackResponse := &proofapi.RelayByTxResponse{Tx: ackTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, ackRequest).Return(ackResponse, nil).Twice()

		priceClient := mockibcv2.NewMockPriceClient(t)

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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mockibcv2bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)
		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil)
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainTypeCOSMOS)
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusERROR, nil)

		priceClient := mockibcv2.NewMockPriceClient(t)

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
		require.NoError(t, err)
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

		configReader := mockconfig.NewMockConfigReader(t)
		configReader.EXPECT().GetChainConfig(mock.Anything).Return(config.ChainConfig{}, nil).Maybe()
		ctx = config.ConfigReaderContext(ctx, configReader)

		packetSequenceNumber := 10

		mockStorage := mockibcv2.NewMockStorage(t)
		// return a nil error anytime we try to update a transfer in storage
		mockStorage.EXPECT().UpdateTransferState(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().UpdateTransferWriteAckTx(mock.Anything, mock.Anything).Return(nil)
		mockStorage.EXPECT().ExecTx(mock.Anything, mock.Anything).Return(nil).Once()
		mockStorage.EXPECT().UpdateTransferSourceTxFinalizedTime(mock.Anything, mock.Anything).Return(nil)

		mockSourceBridgeClient := &MockAcceptingBridgeClient{}
		mockDestBridgeClient := mockibcv2bridge.NewMockBridgeClient(t)
		clients := map[string]ibcv2bridge.BridgeClient{sourceChainID: mockSourceBridgeClient, destChainID: mockDestBridgeClient}
		bridgeClientManager := ibcv2.NewClientManager(clients)

		mockRelayService := mockproofapi.NewMockProofApiServiceClient(t)

		// mock request to get receive tx bytes from relayer service
		sourceTxID, err := hex.DecodeString(mockTxHash)
		require.NoError(t, err)
		recvRequest := &proofapi.RelayByTxRequest{
			SrcChain:           sourceChainID,
			DstChain:           destChainID,
			SourceTxIds:        [][]byte{sourceTxID},
			SrcClientId:        sourceClientID,
			DstClientId:        destClientID,
			SrcPacketSequences: []uint64{uint64(packetSequenceNumber)},
		}
		recvTxBytes := []byte("123456")
		recvResponse := &proofapi.RelayByTxResponse{Tx: recvTxBytes}
		mockRelayService.EXPECT().RelayByTx(mock.Anything, recvRequest).Return(recvResponse, nil).Once()

		mockDestBridgeClient.EXPECT().IsPacketReceived(mock.Anything, destClientID, uint64(packetSequenceNumber)).Return(false, nil)
		mockDestBridgeClient.EXPECT().DeliverTx(mock.Anything, recvResponse.GetTx(), recvResponse.GetAddress()).Return(&ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil)
		mockDestBridgeClient.EXPECT().ChainType().Return(config.ChainTypeCOSMOS)
		mockDestBridgeClient.EXPECT().ShouldRetryTx(mock.Anything, mockTxHash, ibcv2.RetryRecvExpiry, mockTxTime).Return(false, nil)
		mockDestBridgeClient.EXPECT().PacketWriteAckStatus(mock.Anything, mockTxHash, uint64(packetSequenceNumber), sourceClientID, destClientID).Return(db.Ibcv2WriteAckStatusUNKNOWN, nil)

		priceClient := mockibcv2.NewMockPriceClient(t)

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
		require.NoError(t, err)
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

func (*MockAcceptingBridgeClient) IsPacketReceived(ctx context.Context, clientID string, sequence uint64) (bool, error) {
	return false, nil
}

func (*MockAcceptingBridgeClient) IsPacketCommitted(ctx context.Context, clientID string, sequence uint64) (bool, error) {
	return true, nil
}

func (*MockAcceptingBridgeClient) FindRecvTx(ctx context.Context, sourceClientID string, destClientID string, sequence uint64, timeoutTimestamp time.Time) (*ibcv2bridge.BridgeTx, error) {
	panic("unimplemented")
}

func (*MockAcceptingBridgeClient) FindAckTx(ctx context.Context, sourceClientID string, destClientID string, sequence uint64) (*ibcv2bridge.BridgeTx, error) {
	panic("unimplemented")
}

func (*MockAcceptingBridgeClient) FindTimeoutTx(ctx context.Context, sourceClientID string, destClientID string, sequence uint64) (*ibcv2bridge.BridgeTx, error) {
	panic("unimplemented")
}

func (*MockAcceptingBridgeClient) DeliverTx(ctx context.Context, tx []byte, address string) (*ibcv2bridge.BridgeTx, error) {
	return &ibcv2bridge.BridgeTx{Hash: mockTxHash, Timestamp: mockTxTime}, nil
}

func (*MockAcceptingBridgeClient) PacketWriteAckStatus(ctx context.Context, hash string, sequence uint64, sourceClientID string, destClientID string) (db.Ibcv2WriteAckStatus, error) {
	return db.Ibcv2WriteAckStatusSUCCESS, nil
}

func (*MockAcceptingBridgeClient) SendPacketsFromTx(ctx context.Context, sourceChainID string, txHash string) ([]*ibcv2bridge.PacketInfo, error) {
	return nil, nil
}

func (*MockAcceptingBridgeClient) ShouldRetryTx(ctx context.Context, txHash string, expiry time.Duration, sentAtTx time.Time) (bool, error) {
	return false, nil
}

func (*MockAcceptingBridgeClient) IsTxFinalized(ctx context.Context, txHash string, offset *uint64) (bool, error) {
	return true, nil
}

func (*MockAcceptingBridgeClient) IsTimestampFinalized(ctx context.Context, timestamp time.Time, offset *uint64) (bool, error) {
	return true, nil
}

func (*MockAcceptingBridgeClient) WaitForChain(ctx context.Context) error {
	return nil
}

func (*MockAcceptingBridgeClient) LatestOnChainTimestamp(ctx context.Context) (time.Time, error) {
	return time.Time{}, nil
}

func (*MockAcceptingBridgeClient) ChainType() config.ChainType {
	return config.ChainTypeCOSMOS
}

//nolint:nilnil // mock returns zero values to satisfy interface
func (*MockAcceptingBridgeClient) SignerGasTokenBalance(ctx context.Context) (*big.Int, error) {
	return nil, nil
}

//nolint:nilnil // mock returns zero values to satisfy interface
func (*MockAcceptingBridgeClient) TxFee(ctx context.Context, txHash string) (*big.Int, error) {
	return nil, nil
}

func (*MockAcceptingBridgeClient) TxExecutionStatus(ctx context.Context, txHash string) (ibcv2bridge.TxExecutionStatus, error) {
	return ibcv2bridge.TxExecutionStatus{Status: "SUCCESS"}, nil
}

func (*MockAcceptingBridgeClient) SendTransfer(ctx context.Context, ics20Address string, clientID string, denom string, receiver string, amount *big.Int, memo string, timeout time.Duration) (string, error) {
	return "", nil
}

func (*MockAcceptingBridgeClient) IFTTransfer(ctx context.Context, iftAddress string, clientID string, receiver string, amount *big.Int, timeout time.Duration) (string, error) {
	return "", nil
}

func (*MockAcceptingBridgeClient) ClientState(ctx context.Context, clientID string) (ibcv2bridge.ClientState, error) {
	return ibcv2bridge.ClientState{}, nil
}

func (*MockAcceptingBridgeClient) TimestampAtHeight(ctx context.Context, height uint64) (time.Time, error) {
	return time.Now(), nil
}

func (*MockAcceptingBridgeClient) WaitForTx(ctx context.Context, hash string) error {
	return nil
}

func (*MockAcceptingBridgeClient) GetTransactionSender(ctx context.Context, hash string) (string, error) {
	// Return a non-blacklisted address by default
	return "0x0000000000000000000000000000000000000000", nil
}
