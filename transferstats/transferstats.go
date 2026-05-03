package transferstats

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/cosmos/ibc-relayer/db/gen/db"
	"github.com/cosmos/ibc-relayer/shared/config"
	"github.com/cosmos/ibc-relayer/shared/lmt"
	"github.com/cosmos/ibc-relayer/shared/metrics"
)

const tickInterval = 15 * time.Second

type Storage interface {
	ListRelayStatuses(ctx context.Context) ([]string, error)
	CountTransfersByChainPairAndStatus(ctx context.Context) ([]db.CountTransfersByChainPairAndStatusRow, error)
}

type Monitor struct {
	storage Storage
}

func New(storage Storage) *Monitor {
	return &Monitor{storage: storage}
}

func (m *Monitor) Start(ctx context.Context) error {
	lmt.Logger(ctx).Info("Starting transfer stats monitor")
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.tick(ctx)
		}
	}
}

func (m *Monitor) tick(ctx context.Context) {
	statuses, err := m.storage.ListRelayStatuses(ctx)
	if err != nil {
		lmt.Logger(ctx).Error("listing relay statuses", zap.Error(err))
		return
	}
	rows, err := m.storage.CountTransfersByChainPairAndStatus(ctx)
	if err != nil {
		lmt.Logger(ctx).Error("counting transfers by chain pair and status", zap.Error(err))
		return
	}

	type pair struct{ src, dst string }
	counts := make(map[pair]map[string]int64, len(rows))
	for _, r := range rows {
		p := pair{r.SourceChainID, r.DestinationChainID}
		if counts[p] == nil {
			counts[p] = make(map[string]int64, len(statuses))
		}
		counts[p][string(r.Status)] = r.Count
	}

	for p, byStatus := range counts {
		srcCfg, srcFound := chainConfigOrFallback(ctx, p.src)
		dstCfg, dstFound := chainConfigOrFallback(ctx, p.dst)
		if !srcFound {
			lmt.Logger(ctx).Warn("chain config lookup failed, falling back to chain id as name", zap.String("chain_id", p.src))
		}
		if !dstFound {
			lmt.Logger(ctx).Warn("chain config lookup failed, falling back to chain id as name", zap.String("chain_id", p.dst))
		}
		for _, s := range statuses {
			metrics.FromContext(ctx).SetTransferCount(
				p.src, p.dst,
				srcCfg.ChainName, dstCfg.ChainName,
				string(srcCfg.Environment), s,
				float64(byStatus[s]),
			)
		}
	}
}

func chainConfigOrFallback(ctx context.Context, chainID string) (config.ChainConfig, bool) {
	chainConfig, err := config.GetConfigReader(ctx).GetChainConfig(chainID)
	if err != nil {
		return config.ChainConfig{
			ChainID:   chainID,
			ChainName: chainID,
		}, false
	}
	return chainConfig, true
}
