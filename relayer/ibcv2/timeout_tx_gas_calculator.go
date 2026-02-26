package ibcv2

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/cosmos/platform-relayer/db/gen/db"
)

type TransferTimeoutTxGasCostStorage interface {
	UpdateTransferTimeoutTxGasCostUSD(ctx context.Context, arg db.UpdateTransferTimeoutTxGasCostUSDParams) error
}

type TimeoutTxGasCalculatorProcessor struct {
	bridgeClientManager            BridgeClientManager
	storage                        TransferTimeoutTxGasCostStorage
	priceClient                    PriceClient
	sourceChainGasTokenCoingeckoID string
	sourceChainGasTokenDecimals    uint8
}

func NewTimeoutTxGasCalculatorProcessor(
	bridgeClientManager BridgeClientManager,
	storage TransferTimeoutTxGasCostStorage,
	priceClient PriceClient,
	sourceChainGasTokenCoingeckoID string,
	sourceChainGasTokenDecimals uint8,
) TimeoutTxGasCalculatorProcessor {
	return TimeoutTxGasCalculatorProcessor{
		bridgeClientManager:            bridgeClientManager,
		storage:                        storage,
		priceClient:                    priceClient,
		sourceChainGasTokenCoingeckoID: sourceChainGasTokenCoingeckoID,
		sourceChainGasTokenDecimals:    sourceChainGasTokenDecimals,
	}
}

func (processor TimeoutTxGasCalculatorProcessor) Process(ctx context.Context, transfer *IBCV2Transfer) (*IBCV2Transfer, error) {
	sourceBridgeClient, err := processor.bridgeClientManager.GetClient(ctx, transfer.GetSourceChainID())
	if err != nil {
		return nil, fmt.Errorf("getting ibcv2 bridge client for transfer source chain %s: %w", transfer.GetSourceChainID(), err)
	}

	recvTxHash, hasTimeoutTxHash := transfer.GetTimeoutTxHash()
	if !hasTimeoutTxHash {
		return nil, fmt.Errorf("this is a bug! transfer does not have a recv tx, violating should process")
	}

	gasCostInGasToken, err := sourceBridgeClient.TxFee(ctx, recvTxHash)
	if err != nil {
		return nil, fmt.Errorf("getting gas cost of timeout tx %s on %s: %w", recvTxHash, transfer.GetSourceChainID(), err)
	}

	gasCostUSD, err := processor.priceClient.GetCoinUsdValue(ctx, processor.sourceChainGasTokenCoingeckoID, processor.sourceChainGasTokenDecimals, gasCostInGasToken)
	if err != nil {
		return nil, fmt.Errorf("getting usd value of timeout tx gas cost on chain %s: %w", transfer.GetSourceChainID(), err)
	}

	update := db.UpdateTransferTimeoutTxGasCostUSDParams{
		TimeoutTxGasCostUsd:  gasCostUSD,
		SourceChainID:        transfer.GetSourceChainID(),
		PacketSourceClientID: transfer.GetPacketSourceClientID(),
		PacketSequenceNumber: int32(transfer.GetPacketSequenceNumber()),
	}
	if err := processor.storage.UpdateTransferTimeoutTxGasCostUSD(ctx, update); err != nil {
		return nil, fmt.Errorf("updating transfer timeout tx gas cost usd value: %w", err)
	}

	return transfer, nil
}

func (processor TimeoutTxGasCalculatorProcessor) Cancel(transfer *IBCV2Transfer, err error) {
	timeoutTxHash, _ := transfer.GetTimeoutTxHash()
	timeoutTxTime, _ := transfer.GetTimeoutTxTime()
	transfer.GetLogger().Error("calculating timeout packet gas cost", zap.Error(err), zap.String("timeout_tx_hash", timeoutTxHash), zap.Time("timeout_tx_time", timeoutTxTime))
}

func (processor TimeoutTxGasCalculatorProcessor) ShouldProcess(transfer *IBCV2Transfer) bool {
	_, hasTimeoutGasCostUSD := transfer.GetTimeoutGasCostUSD()
	if hasTimeoutGasCostUSD {
		return false
	}

	_, hasTimeoutTxHash := transfer.GetTimeoutTxHash()
	return hasTimeoutTxHash
}

func (processor TimeoutTxGasCalculatorProcessor) State() db.Ibcv2RelayStatus {
	return db.Ibcv2RelayStatusCALCULATINGTIMEOUTTXGASCOST
}
