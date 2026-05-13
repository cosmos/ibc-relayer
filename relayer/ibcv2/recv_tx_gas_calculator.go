package ibcv2

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
)

type TransferRecvTxGasCostStorage interface {
	UpdateTransferRecvTxGasCostUSD(ctx context.Context, arg db.UpdateTransferRecvTxGasCostUSDParams) error
}

type RecvTxGasCalculatorProcessor struct {
	bridgeClientManager                 BridgeClientManager
	storage                             TransferRecvTxGasCostStorage
	priceClient                         PriceClient
	destinationChainGasTokenCoingeckoID string
	destinationChainGasTokenDecimals    uint8
}

func NewRecvTxGasCalculatorProcessor(
	bridgeClientManager BridgeClientManager,
	storage TransferRecvTxGasCostStorage,
	priceClient PriceClient,
	destinationChainGasTokenCoingeckoID string,
	destinationChainGasTokenDecimals uint8,
) RecvTxGasCalculatorProcessor {
	return RecvTxGasCalculatorProcessor{
		bridgeClientManager:                 bridgeClientManager,
		storage:                             storage,
		priceClient:                         priceClient,
		destinationChainGasTokenCoingeckoID: destinationChainGasTokenCoingeckoID,
		destinationChainGasTokenDecimals:    destinationChainGasTokenDecimals,
	}
}

func (p RecvTxGasCalculatorProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	destinationBridgeClient, err := p.bridgeClientManager.GetClient(ctx, transfer.GetDestinationChainID())
	if err != nil {
		return nil, fmt.Errorf("getting ibcv2 bridge client for transfer destination chain %s: %w", transfer.GetDestinationChainID(), err)
	}

	recvTxHash, hasRecvTxHash := transfer.GetRecvTxHash()
	if !hasRecvTxHash {
		return nil, errors.New("this is a bug! transfer does not have a recv tx, violating should process")
	}

	gasCostInGasToken, err := destinationBridgeClient.TxFee(ctx, recvTxHash)
	if err != nil {
		return nil, fmt.Errorf("getting gas cost of recv tx %s on %s: %w", recvTxHash, transfer.GetDestinationChainID(), err)
	}

	gasCostUSD, err := p.priceClient.GetCoinUsdValue(ctx, p.destinationChainGasTokenCoingeckoID, p.destinationChainGasTokenDecimals, gasCostInGasToken)
	if err != nil {
		return nil, fmt.Errorf("getting usd value of recv tx gas cost on chain %s: %w", transfer.GetDestinationChainID(), err)
	}

	update := db.UpdateTransferRecvTxGasCostUSDParams{
		RecvTxGasCostUsd:     gasCostUSD,
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()), //nolint:gosec // G115 bounded conversion
	}
	if err := p.storage.UpdateTransferRecvTxGasCostUSD(ctx, update); err != nil {
		return nil, fmt.Errorf("updating transfer recv tx gas cost usd value: %w", err)
	}

	return transfer, nil
}

func (RecvTxGasCalculatorProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	recvTxHash, _ := transfer.GetRecvTxHash()
	recvTxTime, _ := transfer.GetRecvTxTime()
	transfer.GetLogger().Error("calculating recv packet gas cost", zap.Error(err), zap.String("recv_tx_hash", recvTxHash), zap.Time("recv_tx_time", recvTxTime))
}

func (RecvTxGasCalculatorProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasRecvGasCostUSD := transfer.GetRecvGasCostUSD()
	if hasRecvGasCostUSD {
		return false
	}

	_, hasRecvTxHash := transfer.GetRecvTxHash()
	return hasRecvTxHash
}

func (RecvTxGasCalculatorProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusCALCULATINGRECVTXGASCOST
}
