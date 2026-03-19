package ibcv2

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	"github.com/cosmos/ibc-relayer/proto/gen/ibcv2relayer"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
)

type TransferTimeoutTxWithTxStorage interface {
	TransferTimeoutTxStorage
	ExecTx(ctx context.Context, fn func(q *db.Queries) error) error
}

type BatchTimeoutPacketProcessor struct {
	bridgeClientManager            BridgeClientManager
	transferTimeoutTxWithTxStorage TransferTimeoutTxWithTxStorage
	relayService                   ibcv2relayer.RelayerServiceClient
	sourceChainID                  string
	sourceClientID                 string
	destinationChainID             string
	destinationClientID            string
}

func NewBatchTimeoutPacketProcessor(
	bridgeClientmanager BridgeClientManager,
	transferTimeoutTxWithTxStorage TransferTimeoutTxWithTxStorage,
	relayService ibcv2relayer.RelayerServiceClient,
	sourceChainID string,
	sourceClientID string,
	destinationChainID string,
	destinationClientID string,
) BatchTimeoutPacketProcessor {
	return BatchTimeoutPacketProcessor{
		bridgeClientManager:            bridgeClientmanager,
		transferTimeoutTxWithTxStorage: transferTimeoutTxWithTxStorage,
		relayService:                   relayService,
		sourceChainID:                  sourceChainID,
		sourceClientID:                 sourceClientID,
		destinationChainID:             destinationChainID,
		destinationClientID:            destinationClientID,
	}
}

// Process uses the ibc ibcv2 proof relayer to get the tx bytes that should be
// submitted on the source chain for the timeout packet
func (processor BatchTimeoutPacketProcessor) Process(ctx context.Context, transfers []*IBCV2Transfer) ([]*IBCV2Transfer, error) {
	// get tx bytes and packet sequence numbers for all transfers in batch
	var txIDs [][]byte
	var sequences []uint64
	txSet := make(map[string]struct{})
	for _, transfer := range transfers {
		if _, ok := txSet[transfer.GetSourceTxHash()]; ok {
			sequences = append(sequences, uint64(transfer.GetPacketSequenceNumber()))
			continue
		}

		txID, err := hex.DecodeString(strings.TrimPrefix(transfer.GetSourceTxHash(), "0x"))
		if err != nil {
			err = fmt.Errorf("decoding hex source tx hash %s to bytes: %w", transfer.GetSourceTxHash(), err)
			transfer.ProcessingError = err
			continue
		}

		txIDs = append(txIDs, txID)
		txSet[transfer.GetSourceChainID()] = struct{}{}
		sequences = append(sequences, uint64(transfer.GetPacketSequenceNumber()))
	}

	logger := lmt.Logger(ctx).With(zap.Int("num_transfers", len(sequences)), zap.Any("sequences", sequences))
	logger.Info("processing batch timeout submission")

	relayByTxReqStartTs := time.Now()
	req := &ibcv2relayer.RelayByTxRequest{
		SrcChain:           processor.destinationChainID,
		DstChain:           processor.sourceChainID,
		TimeoutTxIds:       txIDs,
		SrcClientId:        processor.destinationClientID,
		DstClientId:        processor.sourceClientID,
		DstPacketSequences: sequences,
	}
	resp, err := processor.relayService.RelayByTx(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("getting timeout tx bytes from ibcv2 relayer for batch of %d transfers: %w", len(sequences), err)
	}

	dur := time.Since(relayByTxReqStartTs)
	lmt.Logger(ctx).Info(
		fmt.Sprintf("ibc relayer returned %d timeout tx bytes in %s", len(resp.GetTx()), dur),
		zap.Duration("duration", dur),
		zap.Int("num_tx_bytes", len(resp.GetTx())),
	)

	timeoutTxBytes := resp.GetTx()
	to := resp.GetAddress()

	sourceChainClient, err := processor.bridgeClientManager.GetClient(ctx, processor.sourceChainID)
	if err != nil {
		return nil, fmt.Errorf("getting client for transfer source chain %s: %w", processor.sourceChainID, err)
	}

	if sourceChainClient.ChainType() == config.ChainType_EVM {
		// for evm chains, we must wait until the chain has caught up to the
		// current time before estimating the gas of the tx during delivery or
		// else it will revert
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err = sourceChainClient.WaitForChain(waitCtx); err != nil {
			return nil, fmt.Errorf("waiting for chain: %w", err)
		}
	}

	// submit the timeout on the source chain
	timeoutTx, err := sourceChainClient.DeliverTx(ctx, timeoutTxBytes, to)
	if err != nil {
		return nil, fmt.Errorf("submitting batch timeout tx: %w", err)
	}

	logger.Info("delivered batch timeout tx", zap.String("tx_hash", timeoutTx.Hash))

	err = processor.transferTimeoutTxWithTxStorage.ExecTx(ctx, func(q *db.Queries) error {
		for _, transfer := range transfers {
			if transfer.ProcessingError != nil {
				continue
			}

			update := db.UpdateTransferTimeoutTxParams{
				TimeoutTxHash:           pgtype.Text{Valid: true, String: timeoutTx.Hash},
				TimeoutTxTime:           pgtype.Timestamp{Valid: true, Time: timeoutTx.Timestamp},
				TimeoutTxRelayerAddress: pgtype.Text{Valid: true, String: timeoutTx.RelayerAddress},
				SourceChainID:           transfer.GetSourceChainID(),
				PacketSourceClientID:    transfer.GetPacketSourceClientID(),
				PacketSequenceNumber:    int32(transfer.GetPacketSequenceNumber()),
			}
			if err = q.UpdateTransferTimeoutTx(ctx, update); err != nil {
				return fmt.Errorf("updating transfer timeout tx with timeout tx %s: %w", timeoutTx, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("updating batch transfers timeout txs: %w", err)
	}

	for _, transfer := range transfers {
		if transfer.ProcessingError != nil {
			continue
		}

		transfer.TimeoutTxHash = &timeoutTx.Hash
		transfer.TimeoutTxTime = &timeoutTx.Timestamp
	}
	return transfers, nil
}

func (processor BatchTimeoutPacketProcessor) Cancel(transfers []*IBCV2Transfer, err error) {
	// log error, mark packet as failed if fatal error and we cannot retry, if
	// not fatal error, do nothing so it will be retried by this stage
	for _, transfer := range transfers {
		transfer.GetLogger().Error("error timing out packet", zap.Error(err))
	}
}

// ShouldProcess determines when this processor should be run.
func (processor BatchTimeoutPacketProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	// we only want to try and submit a timeout for a transfer if it is past
	// its timeout timestamp, and if it does not have a recv or ack submitted
	// for it. If it does have a recv submitted for it and is past its timeout
	// time, a timeout should not be submitted and we should continue with the
	// relaying pipeline as normal.
	_, hasRecvTxHash := transfer.GetRecvTxHash()
	_, hasAckTxHash := transfer.GetAckTxHash()
	shouldBeTimedOut := transfer.IsTimedOut() && !hasRecvTxHash && !hasAckTxHash

	// if we have already submitted a timeout tx, dont try again
	_, hasTimeoutTxHash := transfer.GetTimeoutTxHash()
	return shouldBeTimedOut && !hasTimeoutTxHash
}

func (processor BatchTimeoutPacketProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusDELIVERTIMEOUTPACKET
}
