package ibcv2 //nolint:testpackage // tests reference unexported helpers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cosmos/ibc-relayer/shared/metrics"
)

type countingMetrics struct {
	metrics.NoOpMetrics
	nonceReplaced map[string]int
}

func newCountingMetrics() *countingMetrics {
	return &countingMetrics{nonceReplaced: map[string]int{}}
}

func (m *countingMetrics) AddTransactionNonceReplaced(chainID string) {
	m.nonceReplaced[chainID]++
}

func TestSelectNonceWithFallback(t *testing.T) {
	const chainID = "test-chain"

	cases := []struct {
		name             string
		pendingNonce     uint64
		confirmedNonce   uint64
		expectedNonce    uint64
		expectedReplaced int
	}{
		{
			name:             "no gap returns pending",
			pendingNonce:     100,
			confirmedNonce:   100,
			expectedNonce:    100,
			expectedReplaced: 0,
		},
		{
			name:             "gap equal to threshold returns pending",
			pendingNonce:     105,
			confirmedNonce:   100,
			expectedNonce:    105,
			expectedReplaced: 0,
		},
		{
			name:             "gap above threshold falls back to confirmed",
			pendingNonce:     110,
			confirmedNonce:   100,
			expectedNonce:    100,
			expectedReplaced: 1,
		},
		{
			name:             "pending less than confirmed returns pending without fallback",
			pendingNonce:     99,
			confirmedNonce:   100,
			expectedNonce:    99,
			expectedReplaced: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newCountingMetrics()
			ctx := metrics.ContextWithMetrics(context.Background(), m)

			got := selectNonceWithFallback(ctx, chainID, tc.pendingNonce, tc.confirmedNonce)

			assert.Equal(t, tc.expectedNonce, got)
			assert.Equal(t, tc.expectedReplaced, m.nonceReplaced[chainID])
		})
	}
}
