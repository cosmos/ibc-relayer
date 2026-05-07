package ibcv2_test

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	mock_ibcv2 "github.com/cosmos/ibc-relayer/mocks/relayer/ibcv2"
	mock_bridge "github.com/cosmos/ibc-relayer/mocks/shared/bridges/ibcv2"
	"github.com/cosmos/ibc-relayer/relayer/ibcv2"
	bridge "github.com/cosmos/ibc-relayer/shared/bridges/ibcv2"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/metrics"
)

type submittedTxCostStorageStub struct {
	submissions []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow
	updated     []db.UpdateIBCV2RelayerTxSubmissionTrackingParams
	expired     []int32
}

type metricsStub struct {
	metrics.NoOpMetrics
	confirmed []confirmedCall
	gasCosts  []gasCostCall
}

type confirmedCall struct {
	success            bool
	sourceChainID      string
	destinationChainID string
}

type gasCostCall struct {
	chainID  string
	gasCost  string
	decimals uint8
}

func (s *submittedTxCostStorageStub) GetUnresolvedIBCV2RelayerTxSubmissions(_ context.Context, _ int32) ([]db.GetUnresolvedIBCV2RelayerTxSubmissionsRow, error) {
	return s.submissions, nil
}

func (s *submittedTxCostStorageStub) UpdateIBCV2RelayerTxSubmissionTracking(_ context.Context, arg db.UpdateIBCV2RelayerTxSubmissionTrackingParams) error {
	s.updated = append(s.updated, arg)
	return nil
}

func (s *submittedTxCostStorageStub) ExpirePendingIBCV2RelayerTxSubmission(_ context.Context, id int32) error {
	s.expired = append(s.expired, id)
	return nil
}

func (m *metricsStub) AddTransactionConfirmed(success bool, sourceChainID, destinationChainID, sourceChainName, destinationChainName, chainEnvironment string) {
	m.confirmed = append(m.confirmed, confirmedCall{
		success:            success,
		sourceChainID:      sourceChainID,
		destinationChainID: destinationChainID,
	})
}

func (m *metricsStub) AddTransactionGasCost(chainID, chainName, chainEnvironment string, gasCost big.Int, gasTokenDecimals uint8) {
	m.gasCosts = append(m.gasCosts, gasCostCall{
		chainID:  chainID,
		gasCost:  gasCost.String(),
		decimals: gasTokenDecimals,
	})
}

func unresolvedSubmission(id int32, txHash, chainID, destinationChainID string, txType db.Ibcv2RelayerTxSubmissionType, submittedAt time.Time) db.GetUnresolvedIBCV2RelayerTxSubmissionsRow {
	return db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
		ID:                 id,
		TxHash:             txHash,
		ChainID:            chainID,
		DestinationChainID: destinationChainID,
		TxType:             txType,
		Status:             db.Ibcv2RelayerTxSubmissionStatusPENDING,
		SubmittedAt:        pgtype.Timestamptz{Valid: true, Time: submittedAt},
	}
}

func TestSubmittedTxCostTrackerTrackResolvesNativeAndUSDGasCosts(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"source-chain": {
				ChainID:   "source-chain",
				ChainName: "Source",
			},
			"chain-a": {
				ChainID:             "chain-a",
				ChainName:           "Destination",
				GasTokenCoingeckoID: strPtr("ethereum"),
				GasTokenDecimals:    18,
			},
		},
	}))
	metricsRecorder := &metricsStub{}
	ctx = metrics.ContextWithMetrics(ctx, metricsRecorder)

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			unresolvedSubmission(1, "0xabc", "source-chain", "chain-a", db.Ibcv2RelayerTxSubmissionTypeRECVPACKET, time.Now()),
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)
	client := mock_bridge.NewMockBridgeClient(t)
	priceClient := mock_ibcv2.NewMockPriceClient(t)

	manager.EXPECT().GetClient(ctx, "chain-a").Return(client, nil).Once()
	client.EXPECT().TxExecutionStatus(ctx, "0xabc").Return(mock_bridge_status("SUCCESS", ""), nil).Once()
	client.EXPECT().TxFee(ctx, "0xabc").Return(big.NewInt(42), nil).Once()

	usd := numericValue(t, "12.34")
	priceClient.EXPECT().GetCoinUsdValue(ctx, "ethereum", uint8(18), big.NewInt(42)).Return(usd, nil).Once()

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, priceClient, 0, 10, time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Len(t, storage.updated, 1)
	require.Equal(t, int32(1), storage.updated[0].ID)
	require.Equal(t, "12.34", numericString(t, storage.updated[0].GasCostUsd))
	require.Equal(t, "42", numericString(t, storage.updated[0].GasCostAmount))
	require.Equal(t, db.Ibcv2RelayerTxSubmissionStatusSUCCESS, storage.updated[0].Status)
	require.False(t, storage.updated[0].ExecutionError.Valid)
	require.Equal(t, []confirmedCall{{success: true, sourceChainID: "source-chain", destinationChainID: "chain-a"}}, metricsRecorder.confirmed)
	require.Equal(t, []gasCostCall{{chainID: "chain-a", gasCost: "42", decimals: 18}}, metricsRecorder.gasCosts)
}

func TestSubmittedTxCostTrackerTrackResolvesNativeCostWithoutUSD(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"source-chain": {
				ChainID:   "source-chain",
				ChainName: "Source",
			},
			"chain-a": {
				ChainID:   "chain-a",
				ChainName: "Destination",
			},
		},
	}))
	metricsRecorder := &metricsStub{}
	ctx = metrics.ContextWithMetrics(ctx, metricsRecorder)

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			unresolvedSubmission(1, "0xabc", "source-chain", "chain-a", db.Ibcv2RelayerTxSubmissionTypeACKPACKET, time.Now()),
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)
	client := mock_bridge.NewMockBridgeClient(t)

	manager.EXPECT().GetClient(ctx, "source-chain").Return(client, nil).Once()
	client.EXPECT().TxExecutionStatus(ctx, "0xabc").Return(mock_bridge_status("FAILED", "out of gas"), nil).Once()
	client.EXPECT().TxFee(ctx, "0xabc").Return(big.NewInt(42), nil).Once()

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, nil, 0, 10, time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Len(t, storage.updated, 1)
	require.Equal(t, "42", numericString(t, storage.updated[0].GasCostAmount))
	require.False(t, storage.updated[0].GasCostUsd.Valid)
	require.Equal(t, db.Ibcv2RelayerTxSubmissionStatusFAILED, storage.updated[0].Status)
	require.Equal(t, "out of gas", storage.updated[0].ExecutionError.String)
	require.Equal(t, []confirmedCall{{success: false, sourceChainID: "source-chain", destinationChainID: "chain-a"}}, metricsRecorder.confirmed)
	require.Equal(t, []gasCostCall{{chainID: "source-chain", gasCost: "42", decimals: 0}}, metricsRecorder.gasCosts)
}

func TestSubmittedTxCostTrackerTrackResolvesStatusWhenUSDEnrichmentFails(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"source-chain": {
				ChainID:   "source-chain",
				ChainName: "Source",
			},
			"chain-a": {
				ChainID:             "chain-a",
				ChainName:           "Destination",
				GasTokenCoingeckoID: strPtr("osmosis"),
				GasTokenDecimals:    6,
			},
		},
	}))
	metricsRecorder := &metricsStub{}
	ctx = metrics.ContextWithMetrics(ctx, metricsRecorder)

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			unresolvedSubmission(1, "0xabc", "source-chain", "chain-a", db.Ibcv2RelayerTxSubmissionTypeRECVPACKET, time.Now()),
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)
	client := mock_bridge.NewMockBridgeClient(t)
	priceClient := mock_ibcv2.NewMockPriceClient(t)

	manager.EXPECT().GetClient(ctx, "chain-a").Return(client, nil).Once()
	client.EXPECT().TxExecutionStatus(ctx, "0xabc").Return(mock_bridge_status("SUCCESS", ""), nil).Once()
	client.EXPECT().TxFee(ctx, "0xabc").Return(big.NewInt(42), nil).Once()
	priceClient.EXPECT().GetCoinUsdValue(ctx, "osmosis", uint8(6), big.NewInt(42)).Return(pgtype.Numeric{}, assertiveErr("price unavailable")).Once()

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, priceClient, 0, 10, time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Len(t, storage.updated, 1)
	require.Equal(t, "42", numericString(t, storage.updated[0].GasCostAmount))
	require.False(t, storage.updated[0].GasCostUsd.Valid)
	require.Equal(t, db.Ibcv2RelayerTxSubmissionStatusSUCCESS, storage.updated[0].Status)
	require.False(t, storage.updated[0].ExecutionError.Valid)
	require.Equal(t, []confirmedCall{{success: true, sourceChainID: "source-chain", destinationChainID: "chain-a"}}, metricsRecorder.confirmed)
	require.Equal(t, []gasCostCall{{chainID: "chain-a", gasCost: "42", decimals: 6}}, metricsRecorder.gasCosts)
}

func TestSubmittedTxCostTrackerTrackContinuesWhenStatusNotReady(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"chain-a": {
				ChainID: "chain-a",
			},
		},
	}))

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			unresolvedSubmission(1, "0xabc", "chain-a", "dest-chain", db.Ibcv2RelayerTxSubmissionTypeACKPACKET, time.Now()),
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)
	client := mock_bridge.NewMockBridgeClient(t)

	manager.EXPECT().GetClient(ctx, "chain-a").Return(client, nil).Once()
	client.EXPECT().TxExecutionStatus(ctx, "0xabc").Return(bridge.TxExecutionStatus{}, assertiveErr("not found")).Once()

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, nil, 0, 10, time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Empty(t, storage.updated)
}

func TestSubmittedTxCostTrackerTrackContinuesWhenFeeLookupFails(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"source-chain": {
				ChainID:   "source-chain",
				ChainName: "Source",
			},
			"chain-a": {
				ChainID:   "chain-a",
				ChainName: "Destination",
			},
		},
	}))
	metricsRecorder := &metricsStub{}
	ctx = metrics.ContextWithMetrics(ctx, metricsRecorder)

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			unresolvedSubmission(1, "0xabc", "source-chain", "chain-a", db.Ibcv2RelayerTxSubmissionTypeRECVPACKET, time.Now()),
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)
	client := mock_bridge.NewMockBridgeClient(t)

	manager.EXPECT().GetClient(ctx, "chain-a").Return(client, nil).Once()
	client.EXPECT().TxExecutionStatus(ctx, "0xabc").Return(mock_bridge_status("SUCCESS", ""), nil).Once()
	client.EXPECT().TxFee(ctx, "0xabc").Return(nil, assertiveErr("decode failed")).Once()

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, nil, 0, 10, time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Empty(t, storage.updated)
	require.Empty(t, metricsRecorder.confirmed)
	require.Empty(t, metricsRecorder.gasCosts)
}

func TestSubmittedTxCostTrackerTrackCoalescesNilFeeToZero(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"source-chain": {
				ChainID:   "source-chain",
				ChainName: "Source",
			},
			"chain-a": {
				ChainID:   "chain-a",
				ChainName: "Destination",
			},
		},
	}))
	metricsRecorder := &metricsStub{}
	ctx = metrics.ContextWithMetrics(ctx, metricsRecorder)

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			unresolvedSubmission(1, "0xabc", "source-chain", "chain-a", db.Ibcv2RelayerTxSubmissionTypeRECVPACKET, time.Now()),
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)
	client := mock_bridge.NewMockBridgeClient(t)

	manager.EXPECT().GetClient(ctx, "chain-a").Return(client, nil).Once()
	client.EXPECT().TxExecutionStatus(ctx, "0xabc").Return(mock_bridge_status("SUCCESS", ""), nil).Once()
	client.EXPECT().TxFee(ctx, "0xabc").Return(nil, nil).Once()

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, nil, 0, 10, time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Len(t, storage.updated, 1)
	require.Equal(t, "0", numericString(t, storage.updated[0].GasCostAmount))
	require.Equal(t, "0", numericString(t, storage.updated[0].GasCostUsd))
	require.Equal(t, db.Ibcv2RelayerTxSubmissionStatusSUCCESS, storage.updated[0].Status)
}

func TestSubmittedTxCostTrackerTrackExpiresOldAttemptedSubmission(t *testing.T) {
	t.Parallel()

	ctx := config.ConfigReaderContext(context.Background(), config.NewConfigReader(config.Config{
		Chains: map[string]config.ChainConfig{
			"chain-a": {
				ChainID: "chain-a",
			},
		},
	}))

	storage := &submittedTxCostStorageStub{
		submissions: []db.GetUnresolvedIBCV2RelayerTxSubmissionsRow{
			{
				ID:                 1,
				TxHash:             "0xabc",
				ChainID:            "chain-a",
				DestinationChainID: "dest-chain",
				TxType:             db.Ibcv2RelayerTxSubmissionTypeACKPACKET,
				Status:             db.Ibcv2RelayerTxSubmissionStatusPENDING,
				SubmittedAt:        pgtype.Timestamptz{Valid: true, Time: time.Now().Add(-7 * time.Hour)},
			},
		},
	}

	manager := mock_ibcv2.NewMockBridgeClientManager(t)

	tracker := ibcv2.NewSubmittedTxCostTracker(storage, manager, nil, 0, 10, 6*time.Hour)
	require.NoError(t, tracker.Track(ctx))
	require.Empty(t, storage.updated)
	require.Equal(t, []int32{1}, storage.expired)
}

type assertiveErr string

func (e assertiveErr) Error() string {
	return string(e)
}

func numericValue(t *testing.T, value string) pgtype.Numeric {
	t.Helper()

	n := pgtype.Numeric{Valid: true}
	require.NoError(t, n.Scan(value))
	return n
}

func numericString(t *testing.T, value pgtype.Numeric) string {
	t.Helper()

	require.True(t, value.Valid)

	text, err := value.MarshalJSON()
	require.NoError(t, err)
	return strings.Trim(string(text), "\"")
}

func strPtr(v string) *string {
	return &v
}

func mock_bridge_status(status string, errorMessage string) bridge.TxExecutionStatus {
	return bridge.TxExecutionStatus{
		Status:       status,
		ErrorMessage: errorMessage,
	}
}
