package relayerapi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/cosmos/platform-relayer/db/gen/db"
	ibcv2mocks "github.com/cosmos/platform-relayer/mocks/relayer/ibcv2"
	relayerapimocks "github.com/cosmos/platform-relayer/mocks/relayerapi/services/relayerapi"
	bridgemocks "github.com/cosmos/platform-relayer/mocks/shared/bridges/ibcv2"
	protorelayerapi "github.com/cosmos/platform-relayer/proto/gen/relayerapi"
	"github.com/cosmos/platform-relayer/relayerapi/services/relayerapi"
	"github.com/cosmos/platform-relayer/shared/bridges/ibcv2"
	"github.com/cosmos/platform-relayer/shared/config"
)

type RelayerAPIServiceSuite struct {
	suite.Suite

	Ctx context.Context
}

func (s *RelayerAPIServiceSuite) SetupTest() {
	chainConfigMap := map[string]config.ChainConfig{
		"cosmosChainID": {
			Type:    config.ChainType_COSMOS,
			ChainID: "cosmosChainID",
			Cosmos: &config.CosmosConfig{
				AddressPrefix: "noble",
			},
			IBCV2: &config.IBCV2Config{
				CounterpartyChains: map[string]string{
					"08-wasm-1": "evmChainID",
				},
			},
		},
		"evmChainID": {
			Type:    config.ChainType_EVM,
			ChainID: "evmChainID",
			EVM:     &config.EVMConfig{},
			IBCV2: &config.IBCV2Config{
				CounterpartyChains: map[string]string{
					"08-wasm-1": "cosmosChainID",
				},
			},
		},
	}
	cfg := config.Config{
		Chains: chainConfigMap,
		RelayerAPI: config.RelayerAPIConfig{
			Address: "0.0.0.0:9000",
		},
	}
	configReader := config.NewConfigReader(cfg)
	s.Ctx = config.ConfigReaderContext(context.Background(), configReader)
}

func (s *RelayerAPIServiceSuite) TestRelay_RejectsBlacklistedOFACAddress() {
	blacklistedAddress := "0x04dba1194ee10112fe6c3207c0687def0e78bacf" // From OFACAddressMap
	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockBridgeClientManager := ibcv2mocks.NewMockBridgeClientManager(s.T())
	mockBridgeClient := bridgemocks.NewMockBridgeClient(s.T())

	mockDB.EXPECT().InsertRelaySubmission(s.Ctx, mock.Anything).Return(nil)
	mockBridgeClientManager.EXPECT().GetClient(s.Ctx, "evmChainID").Return(mockBridgeClient, nil)
	mockBridgeClient.EXPECT().ChainType().Return(config.ChainType_EVM)
	mockBridgeClient.EXPECT().GetTransactionSender(s.Ctx, txHash).Return(blacklistedAddress, nil)

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, mockBridgeClientManager)

	request := &protorelayerapi.RelayRequest{
		TxHash:  txHash,
		ChainId: "evmChainID",
	}

	_, err := service.Relay(s.Ctx, request)
	s.Require().Error(err)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
	s.Require().Contains(err.Error(), "blacklisted")
}

func (s *RelayerAPIServiceSuite) TestRelay_SucceedsOnValidTxHash() {
	request := &protorelayerapi.RelayRequest{
		TxHash:  "0xabc123",
		ChainId: "evmChainID",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockClientManager := ibcv2mocks.NewMockBridgeClientManager(s.T())
	mockBridgeClient := bridgemocks.NewMockBridgeClient(s.T())

	mockDB.EXPECT().InsertRelaySubmission(s.Ctx, mock.Anything).Return(nil)
	mockClientManager.EXPECT().GetClient(s.Ctx, "evmChainID").Return(mockBridgeClient, nil)
	mockBridgeClient.EXPECT().ChainType().Return(config.ChainType_EVM)
	mockBridgeClient.EXPECT().GetTransactionSender(s.Ctx, "0xabc123").Return("0x0000000000000000000000000000000000000000", nil)

	packetTime := time.Now()
	timeoutTime := packetTime.Add(time.Hour)
	mockBridgeClient.EXPECT().SendPacketsFromTx(s.Ctx, "evmChainID", "0xabc123").Return([]*ibcv2.PacketInfo{
		{
			Sequence:          1,
			SourceClient:      "08-wasm-1",
			DestinationClient: "dest-client",
			TimeoutTimestamp:  timeoutTime,
			Timestamp:         packetTime,
		},
	}, nil)

	mockDB.EXPECT().InsertIBCV2Transfer(s.Ctx, mock.MatchedBy(func(arg db.InsertIBCV2TransferParams) bool {
		return arg.SourceChainID == "evmChainID" &&
			arg.DestinationChainID == "cosmosChainID" &&
			arg.SourceTxHash == "0xabc123" &&
			arg.PacketSequenceNumber == 1 &&
			arg.PacketSourceClientID == "08-wasm-1"
	})).Return(nil)

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, mockClientManager)

	resp, err := service.Relay(s.Ctx, request)
	s.Require().NoError(err)
	s.Require().True(proto.Equal(&protorelayerapi.RelayResponse{}, resp), "expected empty response")
}

func (s *RelayerAPIServiceSuite) TestRelay_FailsOnInvalidTxHash() {
	request := &protorelayerapi.RelayRequest{
		TxHash:  "",
		ChainId: "evmChainID",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockClientManager := ibcv2mocks.NewMockBridgeClientManager(s.T())

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, mockClientManager)

	_, err := service.Relay(s.Ctx, request)
	s.Require().Error(err)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
}

func (s *RelayerAPIServiceSuite) TestRelay_FailsOnMissingChainId() {
	request := &protorelayerapi.RelayRequest{
		TxHash:  "0xabc123",
		ChainId: "",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockClientManager := ibcv2mocks.NewMockBridgeClientManager(s.T())

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, mockClientManager)

	_, err := service.Relay(s.Ctx, request)
	s.Require().Error(err)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
}

func (s *RelayerAPIServiceSuite) TestRelay_FailsOnDatabaseFailure() {
	request := &protorelayerapi.RelayRequest{
		TxHash:  "0xabc123",
		ChainId: "evmChainID",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockClientManager := ibcv2mocks.NewMockBridgeClientManager(s.T())
	mockBridgeClient := bridgemocks.NewMockBridgeClient(s.T())

	mockDB.EXPECT().InsertRelaySubmission(s.Ctx, mock.Anything).Return(nil)
	mockClientManager.EXPECT().GetClient(s.Ctx, "evmChainID").Return(mockBridgeClient, nil)
	mockBridgeClient.EXPECT().ChainType().Return(config.ChainType_EVM)
	mockBridgeClient.EXPECT().GetTransactionSender(s.Ctx, "0xabc123").Return("0x0000000000000000000000000000000000000000", nil)

	packetTime := time.Now()
	timeoutTime := packetTime.Add(time.Hour)
	mockBridgeClient.EXPECT().SendPacketsFromTx(s.Ctx, "evmChainID", "0xabc123").Return([]*ibcv2.PacketInfo{
		{
			Sequence:          1,
			SourceClient:      "08-wasm-1",
			DestinationClient: "dest-client",
			TimeoutTimestamp:  timeoutTime,
			Timestamp:         packetTime,
		},
	}, nil)

	mockDB.EXPECT().InsertIBCV2Transfer(s.Ctx, mock.Anything).Return(errors.New("database error"))

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, mockClientManager)

	_, err := service.Relay(s.Ctx, request)
	s.Require().Error(err)
	s.Require().Equal(codes.Internal, status.Code(err))
}

func (s *RelayerAPIServiceSuite) TestStatus_FailsOnMissingTxHash() {
	request := &protorelayerapi.StatusRequest{
		TxHash:  "",
		ChainId: "11155111",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, nil)

	_, err := service.Status(s.Ctx, request)
	s.Require().Error(err)
	s.Require().Equal(codes.InvalidArgument, status.Code(err))
}

func (s *RelayerAPIServiceSuite) TestStatus_ReturnsNotFoundForUnsubmittedTx() {
	request := &protorelayerapi.StatusRequest{
		TxHash:  "0xabc123",
		ChainId: "11155111",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockDB.On("GetRelaySubmission", mock.Anything, db.GetRelaySubmissionParams{
		SourceChainID: "11155111",
		SourceTxHash:  "0xabc123",
	}).Return(db.Ibcv2RelaySubmission{}, pgx.ErrNoRows)

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, nil)

	_, err := service.Status(s.Ctx, request)
	s.Require().Error(err)
	s.Require().Equal(codes.NotFound, status.Code(err))
}

func (s *RelayerAPIServiceSuite) TestStatus_ReturnsEmptyForNoTransfers() {
	request := &protorelayerapi.StatusRequest{
		TxHash:  "0xabc123",
		ChainId: "11155111",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockDB.On("GetRelaySubmission", mock.Anything, db.GetRelaySubmissionParams{
		SourceChainID: "11155111",
		SourceTxHash:  "0xabc123",
	}).Return(db.Ibcv2RelaySubmission{}, nil)
	mockDB.On("GetTransfersBySourceTx", mock.Anything, db.GetTransfersBySourceTxParams{
		SourceChainID: "11155111",
		SourceTxHash:  "0xabc123",
	}).Return([]db.Ibcv2Transfer{}, nil)

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, nil)

	resp, err := service.Status(s.Ctx, request)
	s.Require().NoError(err)
	s.Require().Empty(resp.PacketStatuses)
}

func (s *RelayerAPIServiceSuite) TestStatus_ReturnsPacketStatuses() {
	request := &protorelayerapi.StatusRequest{
		TxHash:  "0xabc123",
		ChainId: "11155111",
	}

	mockDB := relayerapimocks.NewMockRelayerAPIQueries(s.T())
	mockDB.On("GetRelaySubmission", mock.Anything, db.GetRelaySubmissionParams{
		SourceChainID: "11155111",
		SourceTxHash:  "0xabc123",
	}).Return(db.Ibcv2RelaySubmission{}, nil)
	mockDB.On("GetTransfersBySourceTx", mock.Anything, db.GetTransfersBySourceTxParams{
		SourceChainID: "11155111",
		SourceTxHash:  "0xabc123",
	}).Return([]db.Ibcv2Transfer{
		{
			SourceChainID:        "11155111",
			DestinationChainID:   "cosmoshub-4",
			SourceTxHash:         "0xabc123",
			PacketSequenceNumber: 100,
			PacketSourceClientID: "08-wasm-1",
			Status:               db.Ibcv2RelayStatusPENDING,
		},
		{
			SourceChainID:        "11155111",
			DestinationChainID:   "cosmoshub-4",
			SourceTxHash:         "0xabc123",
			PacketSequenceNumber: 101,
			PacketSourceClientID: "08-wasm-1",
			Status:               db.Ibcv2RelayStatusCOMPLETEWITHACK,
			RecvTxHash:           pgtype.Text{String: "0xdef456", Valid: true},
			AckTxHash:            pgtype.Text{String: "0xghi789", Valid: true},
		},
	}, nil)

	service := relayerapi.NewRelayerAPIService(s.Ctx, mockDB, nil)

	resp, err := service.Status(s.Ctx, request)
	s.Require().NoError(err)
	s.Require().Len(resp.PacketStatuses, 2)

	s.Require().Equal(protorelayerapi.TransferState_TRANSFER_STATE_PENDING,
		resp.PacketStatuses[0].State)
	s.Require().Equal(uint64(100), resp.PacketStatuses[0].SequenceNumber)
	s.Require().Equal("08-wasm-1", resp.PacketStatuses[0].SourceClientId)
	s.Require().Equal("0xabc123", resp.PacketStatuses[0].SendTx.TxHash)
	s.Require().Nil(resp.PacketStatuses[0].RecvTx)
	s.Require().Nil(resp.PacketStatuses[0].AckTx)

	s.Require().Equal(protorelayerapi.TransferState_TRANSFER_STATE_COMPLETE,
		resp.PacketStatuses[1].State)
	s.Require().Equal(uint64(101), resp.PacketStatuses[1].SequenceNumber)
	s.Require().Equal("0xdef456", resp.PacketStatuses[1].RecvTx.TxHash)
	s.Require().Equal("cosmoshub-4", resp.PacketStatuses[1].RecvTx.ChainId)
	s.Require().Equal("0xghi789", resp.PacketStatuses[1].AckTx.TxHash)
	s.Require().Equal("11155111", resp.PacketStatuses[1].AckTx.ChainId)
}

func TestRelayerAPIServiceSuite(t *testing.T) {
	suite.Run(t, new(RelayerAPIServiceSuite))
}
