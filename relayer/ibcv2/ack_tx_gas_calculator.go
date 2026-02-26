package ibcv2

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/cosmos/platform-relayer/db/gen/db"
)

type TransferAckTxGasCostStorage interface {
	UpdateTransferAckTxGasCostUSD(ctx context.Context, arg db.UpdateTransferAckTxGasCostUSDParams) error
}

type AckTxGasCalculatorProcessor struct {
	bridgeClientManager            BridgeClientManager
	storage                        TransferAckTxGasCostStorage
	priceClient                    PriceClient
	sourceChainGasTokenCoingeckoID string
	sourceChainGasTokenDecimals    uint8
}

func NewAckTxGasCalculatorProcessor(
	bridgeClientManager BridgeClientManager,
	storage TransferAckTxGasCostStorage,
	priceClient PriceClient,
	sourceChainGasTokenCoingeckoID string,
	sourceChainGasTokenDecimals uint8,
) AckTxGasCalculatorProcessor {
	return AckTxGasCalculatorProcessor{
		bridgeClientManager:            bridgeClientManager,
		storage:                        storage,
		priceClient:                    priceClient,
		sourceChainGasTokenCoingeckoID: sourceChainGasTokenCoingeckoID,
		sourceChainGasTokenDecimals:    sourceChainGasTokenDecimals,
	}
}

func (processor AckTxGasCalculatorProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	sourceBridgeClient, err := processor.bridgeClientManager.GetClient(ctx, transfer.GetSourceChainID())
	if err != nil {
		return nil, fmt.Errorf("getting ibcv2 bridge client for transfer source chain %s: %w", transfer.GetSourceChainID(), err)
	}

	ackTxHash, hasAckTxHash := transfer.GetAckTxHash()
	if !hasAckTxHash {
		return nil, fmt.Errorf("this is a bug! transfer does not have a ack tx, violating should process")
	}

	gasCostInGasToken, err := sourceBridgeClient.TxFee(ctx, ackTxHash)
	if err != nil {
		return nil, fmt.Errorf("getting gas cost of ack tx %s on %s: %w", ackTxHash, transfer.GetSourceChainID(), err)
	}

	gasCostUSD, err := processor.priceClient.GetCoinUsdValue(ctx, processor.sourceChainGasTokenCoingeckoID, processor.sourceChainGasTokenDecimals, gasCostInGasToken)
	if err != nil {
		return nil, fmt.Errorf("getting usd value of ack tx gas cost on chain %s: %w", transfer.GetSourceChainID(), err)
	}

	update := db.UpdateTransferAckTxGasCostUSDParams{
		AckTxGasCostUsd:      gasCostUSD,
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()),
	}
	if err := processor.storage.UpdateTransferAckTxGasCostUSD(ctx, update); err != nil {
		return nil, fmt.Errorf("updating transfer ack tx gas cost usd value: %w", err)
	}

	return transfer, nil
}

func (processor AckTxGasCalculatorProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	ackTxHash, _ := transfer.GetAckTxHash()
	ackTxTime, _ := transfer.GetAckTxTime()
	transfer.GetLogger().Error("calculating ack packet gas cost", zap.Error(err), zap.String("ack_tx_hash", ackTxHash), zap.Time("ack_tx_time", ackTxTime))
}

func (processor AckTxGasCalculatorProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasAckGasCostUSD := transfer.GetAckGasCostUSD()
	if hasAckGasCostUSD {
		return false
	}

	_, hasAckTxHash := transfer.GetAckTxHash()
	return hasAckTxHash
}

func (processor AckTxGasCalculatorProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusCALCULATINGACKTXGASCOST
}
