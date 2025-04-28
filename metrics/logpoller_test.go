package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestLogPollerMetrics(t *testing.T) GenericLogPollerMetrics {
	m, err := NewGenericLogPollerMetrics("1", "test-network")
	require.NoError(t, err)
	return m
}

func TestLogPollerMetrics_RecordQueryDuration(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.RecordQueryDuration(ctx, "PollLogs", Read, 0.005)
	require.Positive(t, testutil.CollectAndCount(PromLpQueryDuration))
}

func TestLogPollerMetrics_RecordQueryDatasetSize(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.RecordQueryDatasetSize(ctx, "PollLogs", Read, 10)

	require.InEpsilon(t,
		10.0,
		testutil.ToFloat64(PromLpQueryDataSets.WithLabelValues("test-network", "1", "PollLogs", "read")),
		0.001,
	)
}

func TestLogPollerMetrics_IncrementLogsInserted(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.IncrementLogsInserted(ctx, 5)

	require.InEpsilon(t,
		5.0,
		testutil.ToFloat64(PromLpLogsInserted.WithLabelValues("test-network", "1")),
		0.001,
	)
}

func TestLogPollerMetrics_IncrementBlocksInserted(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.IncrementBlocksInserted(ctx, 3)

	require.InEpsilon(t,
		3.0,
		testutil.ToFloat64(PromLpBlocksInserted.WithLabelValues("test-network", "1")),
		0.001,
	)
}
