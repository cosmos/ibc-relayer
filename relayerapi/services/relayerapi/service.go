package relayerapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	protorelayerapi "github.com/cosmos/ibc-relayer/proto/gen/relayerapi"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/cosmos/ibc-relayer/shared/metrics"
	"github.com/cosmos/ibc-relayer/shared/utils"
)

const (
	ErrCode_DuplicateKey = "23505"
)

type RelayerAPIService struct {
	protorelayerapi.UnsafeRelayerApiServiceServer
	config              config.ConfigReader
	db                  RelayerAPIQueries
	bridgeClientManager ibcv2.BridgeClientManager
}

type RelayerAPIQueries interface {
	InsertIBCV2Transfer(ctx context.Context, arg db.InsertIBCV2TransferParams) error
	GetTransfersBySourceTx(ctx context.Context, arg db.GetTransfersBySourceTxParams) ([]db.Ibcv2Transfer, error)
	InsertRelaySubmission(ctx context.Context, arg db.InsertRelaySubmissionParams) error
	GetRelaySubmission(ctx context.Context, arg db.GetRelaySubmissionParams) (db.Ibcv2RelaySubmission, error)
}

func NewRelayerAPIService(
	ctx context.Context,
	db RelayerAPIQueries,
	bridgeClientManager ibcv2.BridgeClientManager,
) *RelayerAPIService {
	return &RelayerAPIService{
		db:                  db,
		config:              config.GetConfigReader(ctx),
		bridgeClientManager: bridgeClientManager,
	}
}

func (s *RelayerAPIService) Status(
	ctx context.Context,
	request *protorelayerapi.StatusRequest,
) (*protorelayerapi.StatusResponse, error) {
	if request.GetTxHash() == "" || request.GetChainId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "tx_hash and chain_id are required")
	}

	// First check if this tx was ever submitted to the relay endpoint
	_, err := s.db.GetRelaySubmission(ctx, db.GetRelaySubmissionParams{
		SourceChainID: request.GetChainId(),
		SourceTxHash:  request.GetTxHash(),
	})
	if err != nil {
		// If no relay submission found, return NOT_FOUND
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "transaction not submitted to relayer")
		}
		return nil, status.Errorf(codes.Internal, "failed to query relay submission")
	}

	transfers, err := s.db.GetTransfersBySourceTx(ctx, db.GetTransfersBySourceTxParams{
		SourceChainID: request.GetChainId(),
		SourceTxHash:  request.GetTxHash(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query transfers")
	}

	var statuses []*protorelayerapi.PacketStatus
	for _, t := range transfers {
		statuses = append(statuses, &protorelayerapi.PacketStatus{
			State:          mapDBStatusToProto(t.Status),
			SequenceNumber: uint64(t.PacketSequenceNumber),
			SourceClientId: t.PacketSourceClientID,
			SendTx:         &protorelayerapi.TransactionInfo{TxHash: t.SourceTxHash, ChainId: t.SourceChainID},
			RecvTx:         toTxInfo(t.RecvTxHash, t.DestinationChainID),
			AckTx:          toTxInfo(t.AckTxHash, t.SourceChainID),
			TimeoutTx:      toTxInfo(t.TimeoutTxHash, t.SourceChainID),
		})
	}
	return &protorelayerapi.StatusResponse{PacketStatuses: statuses}, nil
}

func (s *RelayerAPIService) Relay(
	ctx context.Context,
	request *protorelayerapi.RelayRequest,
) (*protorelayerapi.RelayResponse, error) {
	if request.GetTxHash() == "" || request.GetChainId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "tx_hash and chain_id are required")
	}

	sourceChainID := request.GetChainId()

	lmt.Logger(ctx).Info("received relay request",
		zap.String("tx_hash", request.GetTxHash()),
		zap.String("chain_id", sourceChainID),
	)

	if err := s.db.InsertRelaySubmission(ctx, db.InsertRelaySubmissionParams{
		SourceChainID: sourceChainID,
		SourceTxHash:  request.GetTxHash(),
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == ErrCode_DuplicateKey {
			metrics.FromContext(ctx).AddRelayRequest(metrics.IBCV2BridgeType, uint32(codes.OK))
			return &protorelayerapi.RelayResponse{}, nil
		}
		metrics.FromContext(ctx).AddRelayRequest(metrics.IBCV2BridgeType, uint32(codes.Internal))
		return nil, status.Errorf(codes.Internal, "failed to insert relay submission")
	}

	client, err := s.bridgeClientManager.GetClient(ctx, sourceChainID)
	if err != nil {
		metrics.FromContext(ctx).AddRelayRequest(metrics.IBCV2BridgeType, uint32(codes.InvalidArgument))
		return nil, status.Errorf(codes.InvalidArgument, "unsupported chain: %s", sourceChainID)
	}

	// Check for blacklisted OFAC accounts
	if client.ChainType() == config.ChainType_EVM {
		transactionSender, err := client.GetTransactionSender(ctx, request.GetTxHash())
		if err != nil {
			return nil, fmt.Errorf("checking OFAC Blacklist - error getting transaction sender: %w", err)
		}
		if _, isBlacklisted := utils.OFACAddressMap[strings.ToLower(transactionSender)]; isBlacklisted {
			return nil, status.Error(codes.InvalidArgument, "Transaction sender is blacklisted - cannot relay")
		}
	}

	packets, err := client.SendPacketsFromTx(ctx, sourceChainID, request.GetTxHash())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get packets from tx: %v", err)
	}

	lmt.Logger(ctx).Info("found packets in transaction",
		zap.String("tx_hash", request.GetTxHash()),
		zap.Int("packet_count", len(packets)),
	)

	for _, packet := range packets {
		destChainID, err := s.config.GetClientCounterpartyChainID(sourceChainID, packet.SourceClient)
		if err != nil {
			lmt.Logger(ctx).Warn("skipping packet with unknown destination",
				zap.String("source_client", packet.SourceClient),
				zap.Error(err),
			)
			continue
		}

		insert := db.InsertIBCV2TransferParams{
			SourceChainID:             sourceChainID,
			DestinationChainID:        destChainID,
			SourceTxHash:              request.GetTxHash(),
			SourceTxTime:              pgtype.Timestamp{Valid: true, Time: packet.Timestamp},
			PacketSequenceNumber:      int32(packet.Sequence),
			PacketSourceClientID:      packet.SourceClient,
			PacketDestinationClientID: packet.DestinationClient,
			PacketTimeoutTimestamp:    pgtype.Timestamp{Valid: true, Time: packet.TimeoutTimestamp},
		}

		if err := s.db.InsertIBCV2Transfer(ctx, insert); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == ErrCode_DuplicateKey {
				lmt.Logger(ctx).Debug("packet already exists",
					zap.Uint64("sequence", packet.Sequence),
				)
				continue
			}
			metrics.FromContext(ctx).AddRelayRequest(metrics.IBCV2BridgeType, uint32(codes.Internal))
			return nil, status.Errorf(codes.Internal, "failed to insert packet")
		}

		lmt.Logger(ctx).Info("submitted packet to be relayed",
			zap.Uint64("sequence", packet.Sequence),
			zap.String("source_client", packet.SourceClient),
		)
	}

	metrics.FromContext(ctx).AddRelayRequest(metrics.IBCV2BridgeType, uint32(codes.OK))
	return &protorelayerapi.RelayResponse{}, nil
}

func mapDBStatusToProto(s db.Ibcv2RelayStatus) protorelayerapi.TransferState {
	switch s {
	case db.Ibcv2RelayStatusCOMPLETEWITHACK,
		db.Ibcv2RelayStatusCOMPLETEWITHTIMEOUT,
		db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKSUCCESS,
		db.Ibcv2RelayStatusCOMPLETEWITHWRITEACKERROR:
		return protorelayerapi.TransferState_TRANSFER_STATE_COMPLETE
	case db.Ibcv2RelayStatusFAILED:
		return protorelayerapi.TransferState_TRANSFER_STATE_FAILED
	default:
		return protorelayerapi.TransferState_TRANSFER_STATE_PENDING
	}
}

// Converts  nullable DB field to proto TransactionInfo
func toTxInfo(hash pgtype.Text, chainID string) *protorelayerapi.TransactionInfo {
	if !hash.Valid {
		return nil
	}
	return &protorelayerapi.TransactionInfo{TxHash: hash.String, ChainId: chainID}
}
