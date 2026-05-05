package transferstats

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	mockconfig "github.com/cosmos/ibc-relayer/mocks/shared/config"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/metrics"
)

type fakeStorage struct {
	statuses    []string
	statusesErr error
	rows        []db.CountTransfersByChainPairAndStatusRow
	rowsErr     error
}

func (f *fakeStorage) ListRelayStatuses(ctx context.Context) ([]string, error) {
	return f.statuses, f.statusesErr
}

func (f *fakeStorage) CountTransfersByChainPairAndStatus(ctx context.Context) ([]db.CountTransfersByChainPairAndStatusRow, error) {
	return f.rows, f.rowsErr
}

type recordedSet struct {
	src, dst, srcName, dstName, env, state string
	count                                  float64
}

type recordingMetrics struct {
	metrics.Metrics
	mu      sync.Mutex
	records []recordedSet
}

func newRecordingMetrics() *recordingMetrics {
	return &recordingMetrics{Metrics: metrics.NewNoOpMetrics()}
}

func (r *recordingMetrics) SetTransferCount(src, dst, srcName, dstName, env, state string, count float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, recordedSet{src, dst, srcName, dstName, env, state, count})
}

func (r *recordingMetrics) find(t *testing.T, src, dst, state string) recordedSet {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.records {
		if rec.src == src && rec.dst == dst && rec.state == state {
			return rec
		}
	}
	t.Fatalf("no record for src=%s dst=%s state=%s (records=%v)", src, dst, state, r.records)
	return recordedSet{}
}

func testContext(t *testing.T, rec *recordingMetrics) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = metrics.ContextWithMetrics(ctx, rec)
	reader := mockconfig.NewMockConfigReader(t)
	reader.EXPECT().GetChainConfig("chainA").Return(config.ChainConfig{ChainID: "chainA", ChainName: "ChainA", Environment: "dev"}, nil).Maybe()
	reader.EXPECT().GetChainConfig("chainB").Return(config.ChainConfig{ChainID: "chainB", ChainName: "ChainB", Environment: "dev"}, nil).Maybe()
	ctx = config.ConfigReaderContext(ctx, reader)
	return ctx
}

func TestTick_EmitsZeroForAbsentStatuses(t *testing.T) {
	storage := &fakeStorage{
		statuses: []string{"PENDING", "DELIVER_RECV_PACKET", "COMPLETE_WITH_ACK"},
		rows: []db.CountTransfersByChainPairAndStatusRow{
			{SourceChainID: "chainA", DestinationChainID: "chainB", Status: db.Ibcv2RelayStatus("DELIVER_RECV_PACKET"), Count: 3},
			{SourceChainID: "chainA", DestinationChainID: "chainB", Status: db.Ibcv2RelayStatus("COMPLETE_WITH_ACK"), Count: 5},
		},
	}
	rec := newRecordingMetrics()
	ctx := testContext(t, rec)

	New(storage).tick(ctx)

	require.Len(t, rec.records, 3)
	require.Equal(t, float64(0), rec.find(t, "chainA", "chainB", "PENDING").count)
	require.Equal(t, float64(3), rec.find(t, "chainA", "chainB", "DELIVER_RECV_PACKET").count)
	require.Equal(t, float64(5), rec.find(t, "chainA", "chainB", "COMPLETE_WITH_ACK").count)
}

func TestTick_DrainedBacklogOnFreshMonitor(t *testing.T) {
	storage := &fakeStorage{
		statuses: []string{"PENDING", "DELIVER_RECV_PACKET", "COMPLETE_WITH_ACK"},
		rows: []db.CountTransfersByChainPairAndStatusRow{
			{SourceChainID: "chainA", DestinationChainID: "chainB", Status: db.Ibcv2RelayStatus("COMPLETE_WITH_ACK"), Count: 8},
		},
	}
	rec := newRecordingMetrics()
	ctx := testContext(t, rec)

	New(storage).tick(ctx)

	require.Len(t, rec.records, 3)
	require.Equal(t, float64(0), rec.find(t, "chainA", "chainB", "PENDING").count)
	require.Equal(t, float64(0), rec.find(t, "chainA", "chainB", "DELIVER_RECV_PACKET").count)
	require.Equal(t, float64(8), rec.find(t, "chainA", "chainB", "COMPLETE_WITH_ACK").count)
}

func TestTick_ListStatusesErrorSkipsTick(t *testing.T) {
	storage := &fakeStorage{
		statusesErr: errors.New("db down"),
		rows: []db.CountTransfersByChainPairAndStatusRow{
			{SourceChainID: "chainA", DestinationChainID: "chainB", Status: db.Ibcv2RelayStatus("COMPLETE_WITH_ACK"), Count: 1},
		},
	}
	rec := newRecordingMetrics()
	ctx := testContext(t, rec)

	New(storage).tick(ctx)

	require.Empty(t, rec.records)
}

func TestTick_CountsErrorSkipsTick(t *testing.T) {
	storage := &fakeStorage{
		statuses: []string{"PENDING"},
		rowsErr:  errors.New("db down"),
	}
	rec := newRecordingMetrics()
	ctx := testContext(t, rec)

	New(storage).tick(ctx)

	require.Empty(t, rec.records)
}
