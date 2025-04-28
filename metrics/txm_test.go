package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestTxmMetrics(t *testing.T) GenericTXMMetrics {
	m, err := NewGenericTxmMetrics("1")
	require.NoError(t, err)
	return m
}

func TestTxmMetrics_IncrementNumBroadcastedTxs(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.IncrementNumBroadcastedTxs(ctx)

	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promNumBroadcasted.WithLabelValues("1")),
		0.001,
	)
}

func TestTxmMetrics_RecordTimeUntilTxBroadcast(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.RecordTimeUntilTxBroadcast(ctx, 1.5) // 1.5 seconds

	require.Positive(t,
		testutil.CollectAndCount(promTimeUntilBroadcast),
	)
}

func TestTxmMetrics_IncrementNumGasBumps(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.IncrementNumGasBumps(ctx)

	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promNumGasBumps.WithLabelValues("1")),
		0.001,
	)
}

func TestTxmMetrics_IncrementGasBumpExceedsLimit(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.IncrementGasBumpExceedsLimit(ctx)

	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promGasBumpExceedsLimit.WithLabelValues("1")),
		0.001,
	)
}

func TestTxmMetrics_IncrementNumConfirmedTxs(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.IncrementNumConfirmedTxs(ctx, 2)

	require.InEpsilon(t,
		2.0,
		testutil.ToFloat64(promNumConfirmedTxs.WithLabelValues("1")),
		0.001,
	)
}

func TestTxmMetrics_RecordTimeUntilTxConfirmed(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.RecordTimeUntilTxConfirmed(ctx, 2.5) // 2.5 seconds

	require.Positive(t,
		testutil.CollectAndCount(promTimeUntilTxConfirmed),
	)
}

func TestTxmMetrics_RecordBlocksUntilTxConfirmed(t *testing.T) {
	m := setupTestTxmMetrics(t)
	ctx := context.Background()

	m.RecordBlocksUntilTxConfirmed(ctx, 3)

	require.Positive(t,
		testutil.CollectAndCount(promBlocksUntilTxConfirmed),
	)
}
